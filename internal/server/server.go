package server

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"

	"github.com/x86txt/sreootb/internal/autotls"
	"github.com/x86txt/sreootb/internal/config"
	"github.com/x86txt/sreootb/internal/database"
	"github.com/x86txt/sreootb/internal/models"
	"github.com/x86txt/sreootb/internal/monitor"
	"github.com/x86txt/sreootb/internal/utils"
)

// Note: Web assets will be embedded from main package
// var webFS embed.FS will be passed from main

// AgentConn represents an active agent WebSocket connection
type AgentConn struct {
	AgentID   string
	Connected time.Time
	LastSeen  time.Time
	Conn      *websocket.Conn // WebSocket connection
	KeyHash   string          // API key hash for database lookups
}

// Server represents the main server instance
type Server struct {
	config      *config.Config
	db          *database.DB
	monitor     *monitor.Monitor
	webRouter   chi.Router            // Web GUI router
	agentRouter chi.Router            // Agent API router
	webSrv      *http.Server          // Web GUI server
	agentSrv    *http.Server          // Agent API server
	agentConns  map[string]*AgentConn // Active agent connections
	connMutex   sync.RWMutex          // Protect agent connections
	autoTLS     *autotls.Manager      // Auto-TLS manager
	staticFS    embed.FS              // Next.js static files
	appFS       embed.FS              // Next.js application files
	upgrader    websocket.Upgrader    // WebSocket upgrader

	// External hostname/IP cache (5-minute TTL)
	externalHostname   string
	externalIP         string
	hostnameExpiry     time.Time
	ipExpiry           time.Time
	externalCacheMutex sync.RWMutex // Protect external cache
}

// generateSecureAPIKey generates a cryptographically secure API key
func generateSecureAPIKey() (string, error) {
	bytes := make([]byte, 32) // 32 bytes = 64 hex characters
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// ensureAgentAPIKey ensures the agent API key exists, generating one if needed
func ensureAgentAPIKey(cfg *config.Config) error {
	keyFile := "agent-api.key"

	// Try to read existing key from file
	if data, err := os.ReadFile(keyFile); err == nil {
		cfg.Server.AgentAPIKey = strings.TrimSpace(string(data))
		return nil
	}

	// Generate new key if file doesn't exist or is empty
	if cfg.Server.AgentAPIKey == "" {
		key, err := generateSecureAPIKey()
		if err != nil {
			return fmt.Errorf("failed to generate agent API key: %w", err)
		}
		cfg.Server.AgentAPIKey = key
	}

	// Save key to file for persistence
	if err := os.WriteFile(keyFile, []byte(cfg.Server.AgentAPIKey), 0600); err != nil {
		return fmt.Errorf("failed to save agent API key: %w", err)
	}

	return nil
}

// New creates a new server instance
func New(cfg *config.Config, staticFS, appFS embed.FS) (*Server, error) {
	// Ensure agent API key exists
	if err := ensureAgentAPIKey(cfg); err != nil {
		return nil, fmt.Errorf("failed to ensure agent API key: %w", err)
	}

	// Display agent API key for administrators
	log.Info().
		Str("agent_api_key", cfg.Server.AgentAPIKey).
		Msg("Agent API Key - use this key when registering agents")

	// Initialize database
	db, err := database.New(cfg.Server.DBPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// Initialize monitor
	mon := monitor.New(db, cfg)

	// Initialize auto-TLS if enabled
	var autoTLSManager *autotls.Manager
	if cfg.Server.AutoTLS {
		autoTLSConfig := autotls.GetAutoTLSConfig(cfg.Server.Bind, cfg.Server.AgentBind)
		autoTLSManager = autotls.New(autoTLSConfig)
		log.Info().
			Bool("auto_tls", true).
			Str("cert_dir", autoTLSConfig.CertDir).
			Strs("dns_names", autoTLSConfig.DNSNames).
			Msg("Auto-TLS enabled")
	}

	// Create server
	srv := &Server{
		config:     cfg,
		db:         db,
		monitor:    mon,
		agentConns: make(map[string]*AgentConn),
		autoTLS:    autoTLSManager,
		staticFS:   staticFS,
		appFS:      appFS,
		upgrader:   websocket.Upgrader{},
	}

	// Setup routers
	srv.setupWebRouter()
	srv.setupAgentRouter()

	// Start external hostname/IP cache refresh in background
	go srv.startExternalCacheRefresh()

	return srv, nil
}

// Start starts both web and agent servers
func (s *Server) Start(ctx context.Context) error {
	// Start monitoring
	if err := s.monitor.Start(); err != nil {
		return fmt.Errorf("failed to start monitor: %w", err)
	}

	// Channel for server errors
	errChan := make(chan error, 2)

	// Start web GUI server
	go func() {
		if err := s.startWebServer(); err != nil {
			errChan <- fmt.Errorf("web server error: %w", err)
		}
	}()

	// Start agent API server
	go func() {
		if err := s.startAgentServer(); err != nil {
			errChan <- fmt.Errorf("agent server error: %w", err)
		}
	}()

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		log.Info().Msg("Shutting down servers")
		return s.shutdown()
	case err := <-errChan:
		s.shutdown()
		return err
	}
}

// startWebServer starts the web GUI server
func (s *Server) startWebServer() error {
	// Setup web server
	s.webSrv = &http.Server{
		Addr:         s.config.Server.Bind,
		Handler:      s.webRouter,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Setup TLS - auto-TLS takes precedence over manual certificates
	if s.config.Server.AutoTLS && s.autoTLS != nil {
		// Use auto-generated TLS certificate
		cert, err := s.autoTLS.GetCertificate()
		if err != nil {
			return fmt.Errorf("failed to get auto-TLS certificate: %w", err)
		}

		s.webSrv.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			NextProtos:   []string{"h2", "http/1.1"},
		}

		// Start HTTPS server with auto-TLS
		log.Info().
			Str("addr", s.config.Server.Bind).
			Bool("auto_tls", true).
			Msg("Starting web GUI HTTPS server with auto-TLS")
		return s.webSrv.ListenAndServeTLS("", "")
	} else if s.config.Server.TLSCert != "" && s.config.Server.TLSKey != "" {
		// Use manual TLS certificates
		cert, err := tls.LoadX509KeyPair(s.config.Server.TLSCert, s.config.Server.TLSKey)
		if err != nil {
			return fmt.Errorf("failed to load web server TLS certificate: %w", err)
		}

		s.webSrv.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			NextProtos:   []string{"h2", "http/1.1"},
		}

		// Start HTTPS server
		log.Info().
			Str("addr", s.config.Server.Bind).
			Bool("manual_tls", true).
			Msg("Starting web GUI HTTPS server")
		return s.webSrv.ListenAndServeTLS("", "")
	} else {
		// Start HTTP server
		log.Info().Str("addr", s.config.Server.Bind).Msg("Starting web GUI HTTP server")
		return s.webSrv.ListenAndServe()
	}
}

// startAgentServer starts the agent API server
func (s *Server) startAgentServer() error {
	// Setup agent server
	s.agentSrv = &http.Server{
		Addr:         s.config.Server.AgentBind,
		Handler:      s.agentRouter,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Setup TLS - auto-TLS takes precedence over manual certificates
	if s.config.Server.AutoTLS && s.autoTLS != nil {
		// Use auto-generated TLS certificate
		cert, err := s.autoTLS.GetCertificate()
		if err != nil {
			return fmt.Errorf("failed to get auto-TLS certificate for agent server: %w", err)
		}

		s.agentSrv.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			NextProtos:   []string{"h2", "http/1.1"},
		}

		// Start HTTPS server with auto-TLS
		log.Info().
			Str("addr", s.config.Server.AgentBind).
			Bool("auto_tls", true).
			Msg("Starting agent API HTTPS server with auto-TLS")
		return s.agentSrv.ListenAndServeTLS("", "")
	} else {
		// Check for manual TLS certificates
		agentCert := s.config.Server.AgentTLSCert
		agentKey := s.config.Server.AgentTLSKey
		if agentCert == "" {
			agentCert = s.config.Server.TLSCert
		}
		if agentKey == "" {
			agentKey = s.config.Server.TLSKey
		}

		if agentCert != "" && agentKey != "" {
			// Use manual TLS certificates
			cert, err := tls.LoadX509KeyPair(agentCert, agentKey)
			if err != nil {
				return fmt.Errorf("failed to load agent server TLS certificate: %w", err)
			}

			s.agentSrv.TLSConfig = &tls.Config{
				Certificates: []tls.Certificate{cert},
				NextProtos:   []string{"h2", "http/1.1"},
			}

			// Start HTTPS server
			log.Info().
				Str("addr", s.config.Server.AgentBind).
				Bool("manual_tls", true).
				Msg("Starting agent API HTTPS server")
			return s.agentSrv.ListenAndServeTLS("", "")
		} else {
			// Start HTTP server
			log.Info().Str("addr", s.config.Server.AgentBind).Msg("Starting agent API HTTP server")
			return s.agentSrv.ListenAndServe()
		}
	}
}

// shutdown gracefully shuts down all servers
func (s *Server) shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Close all agent connections
	s.connMutex.Lock()
	for agentID, conn := range s.agentConns {
		if conn.Conn != nil {
			// Send offline message before closing WebSocket
			conn.Conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"server_shutdown","message":"Server shutting down"}`))
			conn.Conn.Close()
		}
		delete(s.agentConns, agentID)
	}
	s.connMutex.Unlock()

	// Shutdown web server
	if s.webSrv != nil {
		if err := s.webSrv.Shutdown(ctx); err != nil {
			log.Error().Err(err).Msg("Error shutting down web server")
		}
	}

	// Shutdown agent server
	if s.agentSrv != nil {
		if err := s.agentSrv.Shutdown(ctx); err != nil {
			log.Error().Err(err).Msg("Error shutting down agent server")
		}
	}

	// Stop monitoring
	s.monitor.Stop()

	// Close database
	if err := s.db.Close(); err != nil {
		log.Error().Err(err).Msg("Error closing database")
	}

	return nil
}

// Get the frontend filesystem, stripping the "web" prefix
func getFrontendFS(appFS embed.FS) http.FileSystem {
	fsys, err := fs.Sub(appFS, "web")
	if err != nil {
		panic(err)
	}
	return http.FS(fsys)
}

// Get the static assets filesystem
func getStaticFS(staticFS embed.FS) http.FileSystem {
	fsys, err := fs.Sub(staticFS, "web/_next/static")
	if err != nil {
		panic(err)
	}
	return http.FS(fsys)
}

// spaHandler serves the Single Page Application, falling back to index.html
// for client-side routing.
func (s *Server) spaHandler(fileserver http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fileserver.ServeHTTP(w, r)
	}
}

// setupWebRouter configures the web GUI router
func (s *Server) setupWebRouter() {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Timeout(60 * time.Second))

	// CORS for web GUI
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Web GUI API routes
	r.Route("/api", func(r chi.Router) {
		// Sites management
		r.Route("/sites", func(r chi.Router) {
			r.Get("/", s.handleGetSites)
			r.Post("/", s.handleCreateSite)
			r.Get("/status", s.handleGetSitesStatus)
			r.Get("/{id}/history", s.handleGetSiteHistory)
			r.Delete("/{id}", s.handleDeleteSite)
			r.Get("/analytics", s.handleGetSitesAnalytics)
		})

		// Agent management
		r.Route("/agents", func(r chi.Router) {
			r.Get("/", s.handleGetAgents)
			r.Post("/", s.handleCreateAgent)
			r.Delete("/{id}", s.handleDeleteAgent)
			r.Get("/api-key", s.handleGetAgentAPIKey)
		})

		// Monitoring
		r.Post("/check/manual", s.handleManualCheck)
		r.Get("/stats", s.handleGetStats)
		r.Get("/config", s.handleGetConfig)
		r.Get("/cert", s.handleGetCertInfo)

		// Health check
		r.Get("/health", s.handleHealth)

		// Authentication
		r.Post("/auth/login", s.handleAuthLogin)
	})

	// Serve Next.js static files
	r.Handle("/_next/*", s.handleStaticFiles())
	r.Handle("/static/*", s.handleStaticFiles())

	// Serve Next.js application
	r.Get("/*", s.handleNextJSApp)

	s.webRouter = r
}

// handleNextJSApp serves the Next.js application pages
func (s *Server) handleNextJSApp(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Map paths to HTML files in the app directory
	var htmlFile string
	switch path {
	case "/":
		htmlFile = "index.html"
	case "/agents":
		htmlFile = "agents/index.html"
	case "/resources/http":
		htmlFile = "resources/http/index.html"
	case "/resources/ping":
		htmlFile = "resources/ping/index.html"
	default:
		// For any other route, try to find a corresponding HTML file
		// or fall back to index.html for client-side routing
		htmlFile = "index.html"
	}

	// Try to serve the specific HTML file (files are embedded with "web/" prefix)
	file, err := s.appFS.Open("web/" + htmlFile)
	if err != nil {
		// If the specific file doesn't exist, serve index.html for client-side routing
		file, err = s.appFS.Open("web/index.html")
		if err != nil {
			// If index.html doesn't exist, use fallback
			log.Warn().Str("path", path).Err(err).Msg("Next.js app files not found, using fallback")
			s.handleFallbackHTML(w, r)
			return
		}
	}
	defer file.Close()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	http.ServeContent(w, r, htmlFile, time.Time{}, file.(io.ReadSeeker))
}

// handleStaticFiles serves static assets from the embedded Next.js build
func (s *Server) handleStaticFiles() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Remove the /_next prefix and serve from staticFS
		path := strings.TrimPrefix(r.URL.Path, "/_next")
		if path == "" || path == "/" {
			http.NotFound(w, r)
			return
		}

		// Try to open the file from the static filesystem (files are embedded with "web/_next/static/" prefix)
		filePath := "web/_next/static" + strings.TrimPrefix(path, "/static")
		file, err := s.staticFS.Open(filePath)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer file.Close()

		// Set appropriate content type based on file extension
		ext := filepath.Ext(path)
		switch ext {
		case ".js":
			w.Header().Set("Content-Type", "application/javascript")
		case ".css":
			w.Header().Set("Content-Type", "text/css")
		case ".json":
			w.Header().Set("Content-Type", "application/json")
		case ".woff2":
			w.Header().Set("Content-Type", "font/woff2")
		case ".woff":
			w.Header().Set("Content-Type", "font/woff")
		case ".ttf":
			w.Header().Set("Content-Type", "font/ttf")
		case ".png":
			w.Header().Set("Content-Type", "image/png")
		case ".jpg", ".jpeg":
			w.Header().Set("Content-Type", "image/jpeg")
		case ".svg":
			w.Header().Set("Content-Type", "image/svg+xml")
		case ".ico":
			w.Header().Set("Content-Type", "image/x-icon")
		}

		// Set cache headers for static assets
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")

		http.ServeContent(w, r, path, time.Time{}, file.(io.ReadSeeker))
	})
}

// handleFallbackHTML serves basic HTML when Next.js files are not available
func (s *Server) handleFallbackHTML(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html>
<head>
    <title>SRE: Out of the Box</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 0; padding: 20px; 
               background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
               min-height: 100vh; color: white; }
        .container { max-width: 1200px; margin: 0 auto; 
                    background: rgba(255,255,255,0.1); border-radius: 10px; 
                    padding: 30px; backdrop-filter: blur(10px); }
        h1 { text-align: center; margin-bottom: 30px; }
        .stats { display: flex; justify-content: space-around; margin: 20px 0; }
        .stat { text-align: center; }
        .stat-value { font-size: 2em; font-weight: bold; display: block; }
        .status-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); 
                      gap: 20px; margin: 30px 0; }
        .status-card { background: rgba(255,255,255,0.15); border-radius: 8px; padding: 20px; }
        .info { background: rgba(255,255,255,0.1); border-radius: 8px; padding: 20px; margin: 20px 0; }
        button { background: rgba(255,255,255,0.2); color: white; border: 1px solid rgba(255,255,255,0.3);
                padding: 10px 20px; border-radius: 5px; cursor: pointer; margin: 5px; }
        button:hover { background: rgba(255,255,255,0.3); }
        .warning { background: rgba(255,193,7,0.2); border: 1px solid rgba(255,193,7,0.5); 
                  border-radius: 8px; padding: 15px; margin: 20px 0; color: #fff; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🚀 SRE: Out of the Box v2</h1>
        
        <div class="warning">
            <strong>⚠️ Fallback Mode:</strong> Next.js frontend not available. Using basic interface.
        </div>
        
        <div class="stats">
            <div class="stat">
                <span class="stat-value" id="total-sites">-</span>
                <span>Total Sites</span>
            </div>
            <div class="stat">
                <span class="stat-value" id="sites-up">-</span>
                <span>Sites Up</span>
            </div>
            <div class="stat">
                <span class="stat-value" id="sites-down">-</span>
                <span>Sites Down</span>
            </div>
            <div class="stat">
                <span class="stat-value" id="connected-agents">-</span>
                <span>Connected Agents</span>
            </div>
        </div>

        <div style="text-align: center; margin: 20px 0;">
            <button onclick="refreshData()">🔄 Refresh</button>
            <button onclick="manualCheck()">⚡ Manual Check</button>
        </div>
        
        <div id="sites-container" class="status-grid">
            <div class="status-card">Loading...</div>
        </div>

        <div class="info">
            <h3>🔧 Server Configuration</h3>
            <div id="config-info">Loading configuration...</div>
        </div>

        <div class="info">
            <h3>📡 API Endpoints</h3>
            <div>Web GUI API: <strong>` + s.config.Server.Bind + `</strong></div>
            <div>Agent API: <strong>` + s.config.Server.AgentBind + `</strong> (WebSocket)</div>
        </div>
    </div>

    <script>
        async function fetchData(url) {
            const response = await fetch(url);
            return response.json();
        }

        async function refreshData() {
            try {
                const stats = await fetchData('/api/stats');
                document.getElementById('total-sites').textContent = stats.total_sites || 0;
                document.getElementById('sites-up').textContent = stats.sites_up || 0;
                document.getElementById('sites-down').textContent = stats.sites_down || 0;
                document.getElementById('connected-agents').textContent = stats.connected_agents || 0;

                const sites = await fetchData('/api/sites/status');
                const container = document.getElementById('sites-container');
                
                if (sites.length === 0) {
                    container.innerHTML = '<div class="status-card">No sites configured</div>';
                } else {
                    container.innerHTML = sites.map(site => 
                        '<div class="status-card">' +
                        '<h4>' + site.name + '</h4>' +
                        '<div>Status: ' + (site.status || 'Unknown') + '</div>' +
                        '<div>URL: ' + site.url + '</div>' +
                        '</div>'
                    ).join('');
                }

                const config = await fetchData('/api/config');
                document.getElementById('config-info').innerHTML = 
                    'Features: ' + config.features.join(', ') + '<br>' +
                    'Version: ' + config.version + '<br>' +
                    'WebSocket: ' + (config.agent_webtransport_enabled ? 'Enabled' : 'Disabled');
            } catch (error) {
                console.error('Failed to refresh data:', error);
            }
        }

        async function manualCheck() {
            try {
                await fetch('/api/check/manual', { method: 'POST' });
                setTimeout(refreshData, 2000);
            } catch (error) {
                console.error('Failed to trigger manual check:', error);
            }
        }

        refreshData();
        setInterval(refreshData, 30000);
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(html))
}

// API Handlers for Web GUI
func (s *Server) handleGetSites(w http.ResponseWriter, r *http.Request) {
	sites, err := s.db.GetSites()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Ensure we return an empty array instead of null
	if sites == nil {
		sites = []*models.Site{}
	}
	s.writeJSON(w, sites)
}

func (s *Server) handleCreateSite(w http.ResponseWriter, r *http.Request) {
	var req models.SiteCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	site, err := s.db.AddSite(&req)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			http.Error(w, "Site with this URL already exists", http.StatusConflict)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Async refresh monitoring
	go func() {
		if err := s.monitor.RefreshMonitoring(); err != nil {
			log.Error().Err(err).Msg("Failed to refresh monitoring after adding site")
		}
	}()

	s.writeJSON(w, map[string]interface{}{
		"id":      site.ID,
		"message": "Site added successfully",
		"site":    site,
	})
}

func (s *Server) handleGetSitesStatus(w http.ResponseWriter, r *http.Request) {
	statuses, err := s.db.GetSiteStatus()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Ensure we return an empty array instead of null
	if statuses == nil {
		statuses = []*models.SiteStatus{}
	}
	s.writeJSON(w, statuses)
}

func (s *Server) handleGetSiteHistory(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid site ID", http.StatusBadRequest)
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
			limit = l
		}
	}

	history, err := s.db.GetSiteHistory(id, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.writeJSON(w, history)
}

func (s *Server) handleDeleteSite(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid site ID", http.StatusBadRequest)
		return
	}

	if err := s.db.DeleteSite(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "Site not found", http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Async refresh monitoring
	go func() {
		if err := s.monitor.RefreshMonitoring(); err != nil {
			log.Error().Err(err).Msg("Failed to refresh monitoring after deleting site")
		}
	}()

	s.writeJSON(w, map[string]string{"message": "Site deleted successfully"})
}

func (s *Server) handleGetAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := s.db.GetAgents()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Add connection status and server key identification
	serverKeyHash := utils.HashAPIKey(s.config.Server.AgentAPIKey)
	s.connMutex.RLock()
	for i := range agents {
		if conn, exists := s.agentConns[strconv.Itoa(agents[i].ID)]; exists {
			agents[i].Connected = true
			agents[i].LastSeen = &conn.LastSeen
		}

		// Check if this agent is using the server's API key
		agents[i].UsingServerKey = agents[i].APIKeyHash == serverKeyHash
	}
	s.connMutex.RUnlock()

	// Ensure we return an empty array instead of null
	if agents == nil {
		agents = []*models.Agent{}
	}

	s.writeJSON(w, agents)
}

func (s *Server) handleCreateAgent(w http.ResponseWriter, r *http.Request) {
	var req models.AgentCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	keyHash := utils.HashAPIKey(req.APIKey)
	agent, err := s.db.AddAgent(&req, keyHash)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			http.Error(w, "Agent with this API key already exists", http.StatusConflict)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	s.writeJSON(w, map[string]interface{}{
		"id":      agent.ID,
		"message": "Agent created successfully",
		"agent":   agent,
	})
}

func (s *Server) handleDeleteAgent(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid agent ID", http.StatusBadRequest)
		return
	}

	if err := s.db.DeleteAgent(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "Agent not found", http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	s.writeJSON(w, map[string]string{"message": "Agent deleted successfully"})
}

func (s *Server) handleManualCheck(w http.ResponseWriter, r *http.Request) {
	var req models.ManualCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.SiteIDs = nil
	}

	// Async check
	go func() {
		results, err := s.monitor.CheckSitesByID(req.SiteIDs)
		if err != nil {
			log.Error().Err(err).Msg("Manual check failed")
			return
		}
		log.Info().Int("count", len(results)).Msg("Manual check completed")
	}()

	s.writeJSON(w, map[string]string{"message": "Manual check initiated"})
}

func (s *Server) handleGetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.monitor.GetStats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Add agent connection stats
	s.connMutex.RLock()
	stats.ConnectedAgents = len(s.agentConns)
	s.connMutex.RUnlock()

	s.writeJSON(w, stats)
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	features := []string{"embedded_frontend", "http1.1", "websocket"}

	// Check TLS status
	tlsEnabled := false
	autoTLSEnabled := s.config.Server.AutoTLS && s.autoTLS != nil
	manualTLSEnabled := s.config.Server.TLSCert != "" && s.config.Server.TLSKey != ""

	if autoTLSEnabled || manualTLSEnabled {
		features = append(features, "http2", "tls", "wss")
		tlsEnabled = true
	}

	if autoTLSEnabled {
		features = append(features, "auto_tls")
	}

	// Determine WebSocket URL
	wsProtocol := "ws"
	if tlsEnabled {
		wsProtocol = "wss"
	}
	agentHost := strings.Replace(s.config.Server.AgentBind, ":", "", 1)
	if agentHost == "" {
		agentHost = "localhost:8081"
	}
	wsURL := fmt.Sprintf("%s://%s/ws", wsProtocol, agentHost)

	config := map[string]interface{}{
		"scan_interval": map[string]interface{}{
			"min_seconds": int(s.config.Server.MinScanInterval.Seconds()),
			"max_seconds": int(s.config.Server.MaxScanInterval.Seconds()),
		},
		"version":           "2.0",
		"features":          features,
		"web_gui_port":      s.config.Server.Bind,
		"agent_api_port":    s.config.Server.AgentBind,
		"websocket_url":     wsURL,
		"websocket_enabled": true,
		"tls_enabled":       tlsEnabled,
		"auto_tls":          autoTLSEnabled,
		"manual_tls":        manualTLSEnabled,
	}

	// Add auto-TLS certificate info if available
	if autoTLSEnabled {
		if certInfo, err := s.autoTLS.GetCertificateInfo(); err == nil {
			config["auto_tls_cert"] = certInfo
		}
	}

	s.writeJSON(w, config)
}

func (s *Server) handleGetCertInfo(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"auto_tls_enabled":   s.config.Server.AutoTLS,
		"manual_tls_enabled": s.config.Server.TLSCert != "" && s.config.Server.TLSKey != "",
	}

	if s.config.Server.AutoTLS && s.autoTLS != nil {
		certInfo, err := s.autoTLS.GetCertificateInfo()
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get certificate info: %v", err), http.StatusInternalServerError)
			return
		}
		response["auto_tls"] = certInfo
	}

	if s.config.Server.TLSCert != "" && s.config.Server.TLSKey != "" {
		response["manual_tls"] = map[string]interface{}{
			"cert_file": s.config.Server.TLSCert,
			"key_file":  s.config.Server.TLSKey,
		}

		if s.config.Server.AgentTLSCert != "" && s.config.Server.AgentTLSKey != "" {
			response["manual_agent_tls"] = map[string]interface{}{
				"cert_file": s.config.Server.AgentTLSCert,
				"key_file":  s.config.Server.AgentTLSKey,
			}
		}
	}

	s.writeJSON(w, response)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := map[string]interface{}{
		"status":       "healthy",
		"timestamp":    time.Now().Unix(),
		"version":      "2.0",
		"database":     "connected",
		"monitor":      "running",
		"web_server":   s.webSrv != nil,
		"agent_server": s.agentSrv != nil,
	}

	s.connMutex.RLock()
	health["connected_agents"] = len(s.agentConns)
	s.connMutex.RUnlock()

	s.writeJSON(w, health)
}

// handleGetSitesAnalytics returns analytics data for sites
func (s *Server) handleGetSitesAnalytics(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	hoursStr := r.URL.Query().Get("hours")
	hours := 1
	if hoursStr != "" {
		if h, err := strconv.Atoi(hoursStr); err == nil && h > 0 && h <= 168 {
			hours = h
		}
	}

	intervalStr := r.URL.Query().Get("interval_minutes")
	intervalMinutes := 5
	if intervalStr != "" {
		if i, err := strconv.Atoi(intervalStr); err == nil && i > 0 && i <= 60 {
			intervalMinutes = i
		}
	}

	siteIDsStr := r.URL.Query().Get("site_ids")
	var siteIDs []int
	if siteIDsStr != "" && siteIDsStr != "all" {
		for _, idStr := range strings.Split(siteIDsStr, ",") {
			if id, err := strconv.Atoi(strings.TrimSpace(idStr)); err == nil {
				siteIDs = append(siteIDs, id)
			}
		}
	}

	// Get sites
	sites, err := s.db.GetSiteStatus()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Filter sites if specific IDs requested
	var filteredSites []*models.SiteStatus
	if len(siteIDs) > 0 {
		siteIDMap := make(map[int]bool)
		for _, id := range siteIDs {
			siteIDMap[id] = true
		}
		for _, site := range sites {
			if siteIDMap[site.ID] {
				filteredSites = append(filteredSites, site)
			}
		}
	} else {
		filteredSites = sites
	}

	// Build analytics response
	now := time.Now()
	startTime := now.Add(-time.Duration(hours) * time.Hour)

	// Generate time series data points
	dataPoints := []map[string]interface{}{}
	for t := startTime; t.Before(now); t = t.Add(time.Duration(intervalMinutes) * time.Minute) {
		point := map[string]interface{}{
			"timestamp":      t.Format("15:04"),
			"full_timestamp": t.Format(time.RFC3339),
		}

		// Add placeholder data for each site
		for _, site := range filteredSites {
			key := fmt.Sprintf("site_%d", site.ID)
			if site.ResponseTime != nil {
				point[key] = *site.ResponseTime
			} else {
				point[key] = nil
			}
		}

		dataPoints = append(dataPoints, point)
	}

	// Build site info
	siteInfo := []map[string]interface{}{}
	for _, site := range filteredSites {
		info := map[string]interface{}{
			"id":                 site.ID,
			"name":               site.Name,
			"url":                site.URL,
			"last_status":        site.Status,
			"last_response_time": site.ResponseTime,
			"last_status_code":   site.StatusCode,
			"last_checked_at":    site.CheckedAt,
			"scan_interval":      site.ScanInterval,
		}
		siteInfo = append(siteInfo, info)
	}

	response := map[string]interface{}{
		"data":  dataPoints,
		"sites": siteInfo,
		"time_range": map[string]interface{}{
			"start": startTime.Format(time.RFC3339),
			"end":   now.Format(time.RFC3339),
			"hours": hours,
		},
	}

	s.writeJSON(w, response)
}

// handleAuthLogin handles admin authentication
func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		APIKey string `json:"apiKey"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate admin API key
	if req.APIKey == "" {
		http.Error(w, "API key is required", http.StatusBadRequest)
		return
	}

	if req.APIKey != s.config.Server.AdminAPIKey {
		http.Error(w, "Invalid API key", http.StatusUnauthorized)
		return
	}

	// Return success response
	s.writeJSON(w, map[string]interface{}{
		"success": true,
		"message": "Authentication successful",
	})
}

// Agent WebSocket message handlers
func (s *Server) handleAgentHeartbeatWS(conn *AgentConn, msg map[string]interface{}) {
	log.Debug().Str("agent_id", conn.AgentID).Msg("Received WebSocket heartbeat")

	// Extract and update OS information if present
	osInfo, hasOSInfo := msg["os_info"].(map[string]interface{})

	// Get remote IP from the WebSocket connection
	remoteIP := ""
	if conn.Conn != nil {
		if addr := conn.Conn.RemoteAddr(); addr != nil {
			if host, _, err := net.SplitHostPort(addr.String()); err == nil {
				remoteIP = host
			} else {
				remoteIP = addr.String()
			}
		}
	}

	if hasOSInfo && osInfo != nil && remoteIP != "" {
		if err := s.db.UpdateAgentWithRemoteIP(conn.KeyHash, "online", osInfo, remoteIP); err != nil {
			log.Error().Err(err).Str("key_hash", conn.KeyHash).Msg("Failed to update agent OS info with remote IP from heartbeat")
		}
	} else if hasOSInfo && osInfo != nil {
		if err := s.db.UpdateAgentOSInfo(conn.KeyHash, "online", osInfo); err != nil {
			log.Error().Err(err).Str("key_hash", conn.KeyHash).Msg("Failed to update agent OS info from heartbeat")
		}
	}

	// Send heartbeat acknowledgment
	response := map[string]interface{}{
		"type":      "heartbeat_ack",
		"status":    "ok",
		"timestamp": time.Now().Unix(),
	}
	s.sendWebSocketMessage(conn, response)
}

func (s *Server) handleAgentResultsWS(conn *AgentConn, msg map[string]interface{}) {
	log.Debug().Str("agent_id", conn.AgentID).Msg("Received WebSocket monitoring results")

	// Process monitoring results async
	go func() {
		// TODO: Parse and store monitoring results
		results, ok := msg["results"]
		if ok {
			log.Info().
				Str("agent_id", conn.AgentID).
				Interface("results", results).
				Msg("Processing monitoring results")
		}
	}()

	// Send acknowledgment
	response := map[string]interface{}{
		"type":      "results_ack",
		"status":    "received",
		"timestamp": time.Now().Unix(),
	}
	s.sendWebSocketMessage(conn, response)
}

func (s *Server) handleAgentRegisterWS(conn *AgentConn, msg map[string]interface{}) {
	log.Info().Str("agent_id", conn.AgentID).Msg("Agent registered via WebSocket")

	// Send registration acknowledgment
	response := map[string]interface{}{
		"type":      "register_ack",
		"status":    "registered",
		"timestamp": time.Now().Unix(),
	}
	s.sendWebSocketMessage(conn, response)
}

// HTTP fallback handlers for agents
func (s *Server) handleAgentRegister(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, map[string]string{"message": "Agent registered", "prefer": "WebSocket"})
}

func (s *Server) handleAgentHeartbeat(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, map[string]string{"message": "Heartbeat received", "prefer": "WebSocket"})
}

func (s *Server) handleAgentResults(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, map[string]string{"message": "Results received", "prefer": "WebSocket"})
}

func (s *Server) handleAgentTasks(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, map[string]interface{}{
		"tasks":  []string{},
		"prefer": "WebSocket",
	})
}

// writeJSON writes a JSON response
func (s *Server) writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(data)
}

// agentAuthMiddleware validates agent API keys
func (s *Server) agentAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get API key from header
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			http.Error(w, "Missing API key", http.StatusUnauthorized)
			return
		}

		// Validate against server's agent API key
		if apiKey != s.config.Server.AgentAPIKey {
			http.Error(w, "Invalid API key", http.StatusUnauthorized)
			return
		}

		// Continue to next handler
		next.ServeHTTP(w, r)
	})
}

// handleWebSocket handles WebSocket connections from agents
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Extract remote IP address
	remoteIP := extractRemoteIP(r)

	// Authenticate the WebSocket connection
	apiKey := r.Header.Get("X-API-Key")
	if apiKey == "" {
		// Try URL parameter as fallback
		apiKey = r.URL.Query().Get("api_key")
	}

	if apiKey == "" {
		log.Warn().Str("remote_ip", remoteIP).Msg("WebSocket connection attempt without API key")
		http.Error(w, "Missing API key", http.StatusUnauthorized)
		return
	}

	// Validate API key
	if apiKey != s.config.Server.AgentAPIKey {
		log.Warn().Str("api_key", apiKey[:8]+"...").Str("remote_ip", remoteIP).Msg("WebSocket connection attempt with invalid API key")
		http.Error(w, "Invalid API key", http.StatusUnauthorized)
		return
	}

	agentID := r.Header.Get("X-Agent-ID")
	if agentID == "" {
		agentID = r.URL.Query().Get("agent_id")
	}

	if agentID == "" {
		http.Error(w, "Missing agent ID", http.StatusBadRequest)
		return
	}

	// Configure WebSocket upgrader
	s.upgrader.CheckOrigin = func(r *http.Request) bool {
		// Allow connections from agents (could be more restrictive)
		return true
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Str("agent_id", agentID).Str("remote_ip", remoteIP).Msg("Failed to upgrade WebSocket connection")
		return
	}

	// Hash the API key for database operations
	keyHash := utils.HashAPIKey(apiKey)

	// Auto-register agent if it doesn't exist
	agent, err := s.db.GetAgentByKeyHash(keyHash)
	if err != nil {
		log.Error().Err(err).Str("key_hash", keyHash).Str("remote_ip", remoteIP).Msg("Failed to get agent by key hash")
		conn.Close()
		return
	}

	if agent == nil {
		// Auto-register new agent
		agentReq := &models.AgentCreateRequest{
			Name:        fmt.Sprintf("Agent-%s", agentID),
			APIKey:      apiKey,
			Description: stringPtr("Auto-registered agent (WebSocket)"),
		}

		agent, err = s.db.AddAgent(agentReq, keyHash)
		if err != nil {
			log.Error().Err(err).Str("agent_id", agentID).Str("remote_ip", remoteIP).Msg("Failed to auto-register WebSocket agent")
			conn.Close()
			return
		}

		log.Info().
			Str("agent_id", agentID).
			Int("db_id", agent.ID).
			Str("name", agent.Name).
			Str("remote_ip", remoteIP).
			Msg("Auto-registered new WebSocket agent")
	}

	// Create agent connection
	agentConn := &AgentConn{
		AgentID:   agentID,
		Connected: time.Now(),
		LastSeen:  time.Now(),
		Conn:      conn,
		KeyHash:   keyHash,
	}

	// Register the connection
	s.connMutex.Lock()
	s.agentConns[agentID] = agentConn
	s.connMutex.Unlock()

	log.Info().
		Str("agent_id", agentID).
		Int("db_id", agent.ID).
		Str("remote_addr", r.RemoteAddr).
		Str("remote_ip", remoteIP).
		Msg("Agent connected via WebSocket")

	// Update agent status to online with remote IP
	if err := s.db.UpdateAgentWithRemoteIP(keyHash, "online", nil, remoteIP); err != nil {
		log.Error().Err(err).Str("key_hash", keyHash).Str("remote_ip", remoteIP).Msg("Failed to update agent status to online")
	}

	// Handle the WebSocket connection
	s.handleAgentWebSocket(agentConn)
}

// handleAgentWebSocket handles messages from an agent WebSocket connection
func (s *Server) handleAgentWebSocket(agentConn *AgentConn) {
	defer func() {
		// Clean up connection
		s.connMutex.Lock()
		delete(s.agentConns, agentConn.AgentID)
		s.connMutex.Unlock()

		// Update agent status to offline
		if err := s.db.UpdateAgentStatus(agentConn.KeyHash, "offline"); err != nil {
			log.Error().Err(err).Str("key_hash", agentConn.KeyHash).Msg("Failed to update agent status to offline")
		}

		// Close WebSocket connection
		agentConn.Conn.Close()

		log.Info().Str("agent_id", agentConn.AgentID).Msg("Agent WebSocket connection closed")
	}()

	// Set connection timeouts
	agentConn.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	agentConn.Conn.SetPongHandler(func(string) error {
		agentConn.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Start ping ticker
	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()

	// Handle ping/pong
	go func() {
		for {
			select {
			case <-pingTicker.C:
				if err := agentConn.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					log.Debug().Err(err).Str("agent_id", agentConn.AgentID).Msg("Failed to send ping")
					return
				}
			}
		}
	}()

	// Read messages from agent
	for {
		messageType, data, err := agentConn.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Error().Err(err).Str("agent_id", agentConn.AgentID).Msg("WebSocket error")
			}
			break
		}

		// Update last seen
		agentConn.LastSeen = time.Now()

		// Handle different message types
		switch messageType {
		case websocket.TextMessage:
			s.handleWebSocketMessage(agentConn, data)
		case websocket.BinaryMessage:
			log.Debug().Str("agent_id", agentConn.AgentID).Msg("Received binary message (ignored)")
		}
	}
}

// handleWebSocketMessage processes text messages from agents
func (s *Server) handleWebSocketMessage(agentConn *AgentConn, data []byte) {
	var msg map[string]interface{}
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Error().Err(err).Str("agent_id", agentConn.AgentID).Msg("Failed to parse WebSocket message")
		return
	}

	msgType, ok := msg["type"].(string)
	if !ok {
		log.Error().Str("agent_id", agentConn.AgentID).Msg("WebSocket message missing type field")
		return
	}

	log.Debug().
		Str("agent_id", agentConn.AgentID).
		Str("message_type", msgType).
		Msg("Received WebSocket message")

	// Handle different message types
	switch msgType {
	case "heartbeat":
		s.handleAgentHeartbeatWS(agentConn, msg)
	case "monitoring_results":
		s.handleAgentResultsWS(agentConn, msg)
	case "monitoring_result":
		s.handleMonitoringResultWS(agentConn, msg)
	case "request_tasks":
		s.handleRequestTasksWS(agentConn, msg)
	case "status_update":
		s.handleAgentStatusUpdateWS(agentConn, msg)
	default:
		log.Warn().
			Str("agent_id", agentConn.AgentID).
			Str("message_type", msgType).
			Msg("Unknown WebSocket message type")
	}
}

// handleAgentStatusUpdateWS handles status update messages
func (s *Server) handleAgentStatusUpdateWS(agentConn *AgentConn, msg map[string]interface{}) {
	status, ok := msg["status"].(string)
	if !ok {
		status = "online"
	}

	log.Debug().
		Str("agent_id", agentConn.AgentID).
		Str("status", status).
		Msg("Agent status update")

	// Extract OS information if present
	osInfo, hasOSInfo := msg["os_info"].(map[string]interface{})

	// Get remote IP from the WebSocket connection
	remoteIP := ""
	if agentConn.Conn != nil {
		if addr := agentConn.Conn.RemoteAddr(); addr != nil {
			if host, _, err := net.SplitHostPort(addr.String()); err == nil {
				remoteIP = host
			} else {
				remoteIP = addr.String()
			}
		}
	}

	// Update status and OS info in database with remote IP
	if hasOSInfo && osInfo != nil && remoteIP != "" {
		if err := s.db.UpdateAgentWithRemoteIP(agentConn.KeyHash, status, osInfo, remoteIP); err != nil {
			log.Error().Err(err).Str("key_hash", agentConn.KeyHash).Msg("Failed to update agent OS info with remote IP")
		}
	} else if hasOSInfo && osInfo != nil {
		if err := s.db.UpdateAgentOSInfo(agentConn.KeyHash, status, osInfo); err != nil {
			log.Error().Err(err).Str("key_hash", agentConn.KeyHash).Msg("Failed to update agent OS info")
		}
	} else {
		if err := s.db.UpdateAgentStatus(agentConn.KeyHash, status); err != nil {
			log.Error().Err(err).Str("key_hash", agentConn.KeyHash).Msg("Failed to update agent status")
		}
	}

	// Send acknowledgment
	response := map[string]interface{}{
		"type":      "status_ack",
		"status":    "ok",
		"timestamp": time.Now().Unix(),
	}
	s.sendWebSocketMessage(agentConn, response)
}

func (s *Server) handleGetAgentAPIKey(w http.ResponseWriter, r *http.Request) {
	// Get server URL information
	serverURL := s.getServerURL(r)
	agentPort := s.getAgentPort()

	s.writeJSON(w, map[string]interface{}{
		"api_key":    s.config.Server.AgentAPIKey,
		"server_url": serverURL,
		"agent_port": agentPort,
	})
}

// getServerURL attempts to determine the server's URL from the request or external services
func (s *Server) getServerURL(r *http.Request) string {
	// First, try to get from request headers (for reverse proxy scenarios)
	if host := r.Header.Get("X-Forwarded-Host"); host != "" {
		scheme := "https"
		if r.Header.Get("X-Forwarded-Proto") == "http" {
			scheme = "http"
		}
		return fmt.Sprintf("%s://%s", scheme, host)
	}

	// Try the Host header
	if r.Host != "" {
		scheme := "https"
		if r.TLS == nil {
			scheme = "http"
		}
		return fmt.Sprintf("%s://%s", scheme, r.Host)
	}

	// Try to get external hostname/IP from ifconfig.io
	if hostname := s.getExternalHostname(); hostname != "" {
		scheme := "https"
		if !s.hasTLS() {
			scheme = "http"
		}
		webPort := s.getWebPort()
		if (scheme == "https" && webPort != "443") || (scheme == "http" && webPort != "80") {
			return fmt.Sprintf("%s://%s:%s", scheme, hostname, webPort)
		}
		return fmt.Sprintf("%s://%s", scheme, hostname)
	}

	// Fallback to localhost
	scheme := "https"
	if !s.hasTLS() {
		scheme = "http"
	}
	webPort := s.getWebPort()
	return fmt.Sprintf("%s://localhost:%s", scheme, webPort)
}

// getAgentPort extracts the port from the agent bind address
func (s *Server) getAgentPort() string {
	_, port, err := net.SplitHostPort(s.config.Server.AgentBind)
	if err != nil {
		return "8081" // default
	}
	return port
}

// getWebPort extracts the port from the web bind address
func (s *Server) getWebPort() string {
	_, port, err := net.SplitHostPort(s.config.Server.Bind)
	if err != nil {
		return "8080" // default
	}
	return port
}

// hasTLS checks if the server has TLS configured
func (s *Server) hasTLS() bool {
	return s.config.Server.AutoTLS ||
		(s.config.Server.TLSCert != "" && s.config.Server.TLSKey != "") ||
		(s.config.Server.AgentTLSCert != "" && s.config.Server.AgentTLSKey != "")
}

// getExternalHostname tries to get the external hostname or IP from cache first
func (s *Server) getExternalHostname() string {
	s.externalCacheMutex.RLock()
	defer s.externalCacheMutex.RUnlock()

	now := time.Now()

	// Try cached hostname first
	if s.externalHostname != "" && now.Before(s.hostnameExpiry) {
		return s.externalHostname
	}

	// Fallback to cached IP
	if s.externalIP != "" && now.Before(s.ipExpiry) {
		return s.externalIP
	}

	// No valid cache, return empty (background refresh will update it)
	return ""
}

// handleMonitoringResultWS handles individual monitoring result from agent
func (s *Server) handleMonitoringResultWS(agentConn *AgentConn, msg map[string]interface{}) {
	log.Debug().Str("agent_id", agentConn.AgentID).Msg("Received monitoring result via WebSocket")

	// Extract result data
	taskIDFloat, ok := msg["task_id"].(float64)
	if !ok {
		log.Error().Str("agent_id", agentConn.AgentID).Msg("Monitoring result missing task_id")
		return
	}
	taskID := int(taskIDFloat)

	status, _ := msg["status"].(string)
	responseTimeFloat, hasResponseTime := msg["response_time"].(float64)
	statusCodeFloat, hasStatusCode := msg["status_code"].(float64)
	errorMessage, hasErrorMessage := msg["error_message"].(string)
	metadata, _ := msg["metadata"].(map[string]interface{})
	checkedAtUnix, _ := msg["checked_at"].(float64)

	// Convert Unix timestamp to time.Time
	checkedAt := time.Unix(int64(checkedAtUnix), 0)
	if checkedAtUnix == 0 {
		checkedAt = time.Now()
	}

	// Create monitoring result with proper pointer handling
	result := models.MonitorResultRequest{
		TaskID:    taskID,
		Status:    status,
		Metadata:  metadata,
		CheckedAt: checkedAt,
	}

	// Only set pointer fields if values exist
	if hasResponseTime {
		result.ResponseTime = &responseTimeFloat
	}
	if hasStatusCode {
		statusCodeInt := int(statusCodeFloat)
		result.StatusCode = &statusCodeInt
	}
	if hasErrorMessage {
		result.ErrorMessage = &errorMessage
	}

	// Validate result
	if err := result.Validate(); err != nil {
		log.Warn().Err(err).Str("agent_id", agentConn.AgentID).Int("task_id", taskID).Msg("Invalid monitoring result via WebSocket")
		return
	}

	// Get agent from database
	agent, err := s.db.GetAgentByKeyHash(agentConn.KeyHash)
	if err != nil {
		log.Error().Err(err).Str("agent_id", agentConn.AgentID).Msg("Failed to get agent for result storage")
		return
	}

	if agent == nil {
		log.Error().Str("agent_id", agentConn.AgentID).Msg("Agent not found for result storage")
		return
	}

	// Store result in database
	if err := s.db.RecordMonitorResult(&result, agent.ID); err != nil {
		log.Error().Err(err).Str("agent_id", agentConn.AgentID).Int("task_id", taskID).Msg("Failed to store monitoring result")
		return
	}

	logEvent := log.Debug().
		Str("agent_id", agentConn.AgentID).
		Int("task_id", taskID).
		Str("status", status)

	if result.StatusCode != nil {
		logEvent = logEvent.Int("status_code", *result.StatusCode)
	}
	if result.ResponseTime != nil {
		logEvent = logEvent.Float64("response_time_ms", *result.ResponseTime)
	}

	logEvent.Msg("Stored monitoring result from WebSocket")

	// Send acknowledgment
	response := map[string]interface{}{
		"type":      "result_ack",
		"task_id":   taskID,
		"status":    "ok",
		"timestamp": time.Now().Unix(),
	}
	s.sendWebSocketMessage(agentConn, response)
}

// handleRequestTasksWS handles task request from agent
func (s *Server) handleRequestTasksWS(agentConn *AgentConn, msg map[string]interface{}) {
	log.Debug().Str("agent_id", agentConn.AgentID).Msg("Agent requested tasks via WebSocket")

	// Get agent from database
	agent, err := s.db.GetAgentByKeyHash(agentConn.KeyHash)
	if err != nil {
		log.Error().Err(err).Str("agent_id", agentConn.AgentID).Msg("Failed to get agent for task request")
		return
	}

	if agent == nil {
		log.Error().Str("agent_id", agentConn.AgentID).Msg("Agent not found for task request")
		return
	}

	// Get monitoring tasks for this agent
	tasks, err := s.db.GetTasksForAgent(agent.ID)
	if err != nil {
		log.Error().Err(err).Str("agent_id", agentConn.AgentID).Msg("Failed to get monitoring tasks for agent")
		return
	}

	// Send tasks to agent
	response := map[string]interface{}{
		"type":      "task_assignment",
		"tasks":     tasks,
		"timestamp": time.Now().Unix(),
	}
	s.sendWebSocketMessage(agentConn, response)

	log.Debug().
		Str("agent_id", agentConn.AgentID).
		Int("task_count", len(tasks)).
		Msg("Sent tasks to agent via WebSocket")
}

// broadcastTaskToAgents sends new/updated task to all connected agents
func (s *Server) broadcastTaskToAgents(task *models.MonitorTask) {
	log.Debug().Int("task_id", task.ID).Msg("Broadcasting task to all agents")

	s.connMutex.RLock()
	defer s.connMutex.RUnlock()

	for _, agentConn := range s.agentConns {
		if agentConn.Conn != nil {
			// Get agent to check if task applies to them
			agent, err := s.db.GetAgentByKeyHash(agentConn.KeyHash)
			if err != nil || agent == nil {
				continue
			}

			// Get all tasks for this agent to send updated list
			tasks, err := s.db.GetTasksForAgent(agent.ID)
			if err != nil {
				log.Error().Err(err).Str("agent_id", agentConn.AgentID).Msg("Failed to get tasks for broadcast")
				continue
			}

			// Send updated task list
			message := map[string]interface{}{
				"type":      "task_assignment",
				"tasks":     tasks,
				"timestamp": time.Now().Unix(),
			}
			s.sendWebSocketMessage(agentConn, message)
		}
	}
}

// removeTaskFromAgents notifies agents to remove a task
func (s *Server) removeTaskFromAgents(taskID int) {
	log.Debug().Int("task_id", taskID).Msg("Removing task from all agents")

	s.connMutex.RLock()
	defer s.connMutex.RUnlock()

	for _, agentConn := range s.agentConns {
		if agentConn.Conn != nil {
			message := map[string]interface{}{
				"type":      "task_removal",
				"task_ids":  []int{taskID},
				"timestamp": time.Now().Unix(),
			}
			s.sendWebSocketMessage(agentConn, message)
		}
	}
}

// startExternalCacheRefresh starts a background goroutine to refresh external hostname/IP cache
func (s *Server) startExternalCacheRefresh() {
	// Do an immediate refresh on startup
	s.refreshExternalCache()

	// Set up periodic refresh every 5 minutes
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.refreshExternalCache()
		}
	}
}

// refreshExternalCache fetches external hostname and IP in background
func (s *Server) refreshExternalCache() {
	now := time.Now()
	cacheExpiry := now.Add(5 * time.Minute)

	// Fetch hostname and IP concurrently
	var wg sync.WaitGroup
	var hostname, ip string

	// Fetch hostname
	wg.Add(1)
	go func() {
		defer wg.Done()
		hostname = s.fetchExternalInfo("https://ifconfig.io/host")
	}()

	// Fetch IP
	wg.Add(1)
	go func() {
		defer wg.Done()
		ip = s.fetchExternalInfo("https://ifconfig.io/ip")
	}()

	// Wait for both to complete
	wg.Wait()

	// Update cache with results
	s.externalCacheMutex.Lock()
	defer s.externalCacheMutex.Unlock()

	if hostname != "" {
		s.externalHostname = hostname
		s.hostnameExpiry = cacheExpiry
		log.Debug().Str("hostname", hostname).Msg("Updated external hostname cache")
	}

	if ip != "" {
		s.externalIP = ip
		s.ipExpiry = cacheExpiry
		log.Debug().Str("ip", ip).Msg("Updated external IP cache")
	}
}

// fetchExternalInfo fetches information from external service with timeout
// Used only for cache refreshing in background goroutine
func (s *Server) fetchExternalInfo(url string) string {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		log.Debug().Err(err).Str("url", url).Msg("Failed to fetch external info for cache")
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Debug().Int("status", resp.StatusCode).Str("url", url).Msg("External info API returned non-200 status")
		return ""
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Debug().Err(err).Str("url", url).Msg("Failed to read external info response")
		return ""
	}

	result := strings.TrimSpace(string(body))
	if result == "" {
		log.Debug().Str("url", url).Msg("External info API returned empty response")
	}

	return result
}

func (s *Server) handleGetMonitoringTasks(w http.ResponseWriter, r *http.Request) {
	// Get the agent ID from the API key
	apiKey := r.Header.Get("X-API-Key")
	if apiKey == "" {
		http.Error(w, "Missing API key", http.StatusUnauthorized)
		return
	}

	// Get agent by API key hash
	keyHash := utils.HashAPIKey(apiKey)
	agent, err := s.db.GetAgentByKeyHash(keyHash)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	if agent == nil {
		http.Error(w, "Agent not found", http.StatusUnauthorized)
		return
	}

	// Get monitoring tasks for this agent
	tasks, err := s.db.GetTasksForAgent(agent.ID)
	if err != nil {
		log.Error().Err(err).Int("agent_id", agent.ID).Msg("Failed to get monitoring tasks for agent")
		http.Error(w, "Failed to get monitoring tasks", http.StatusInternalServerError)
		return
	}

	// Return tasks
	response := models.AgentTasksResponse{
		Tasks:   make([]models.MonitorTask, len(tasks)),
		AgentID: agent.ID,
	}

	for i, task := range tasks {
		response.Tasks[i] = *task
	}

	log.Debug().Int("agent_id", agent.ID).Int("task_count", len(tasks)).Msg("Returned monitoring tasks to agent")

	s.writeJSON(w, response)
}

func (s *Server) handleSubmitMonitoringResults(w http.ResponseWriter, r *http.Request) {
	// Get the agent ID from the API key
	apiKey := r.Header.Get("X-API-Key")
	if apiKey == "" {
		http.Error(w, "Missing API key", http.StatusUnauthorized)
		return
	}

	// Get agent by API key hash
	keyHash := utils.HashAPIKey(apiKey)
	agent, err := s.db.GetAgentByKeyHash(keyHash)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	if agent == nil {
		http.Error(w, "Agent not found", http.StatusUnauthorized)
		return
	}

	// Parse the monitoring results
	var results []models.MonitorResultRequest
	if err := json.NewDecoder(r.Body).Decode(&results); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate and store each result
	var storedCount int
	for _, result := range results {
		if err := result.Validate(); err != nil {
			log.Warn().Err(err).Int("agent_id", agent.ID).Int("task_id", result.TaskID).Msg("Invalid monitoring result")
			continue
		}

		// Set checked_at if not provided
		if result.CheckedAt.IsZero() {
			result.CheckedAt = time.Now()
		}

		if err := s.db.RecordMonitorResult(&result, agent.ID); err != nil {
			log.Error().Err(err).Int("agent_id", agent.ID).Int("task_id", result.TaskID).Msg("Failed to store monitoring result")
			continue
		}

		storedCount++
	}

	log.Debug().
		Int("agent_id", agent.ID).
		Str("agent_name", agent.Name).
		Int("total_results", len(results)).
		Int("stored_results", storedCount).
		Msg("Processed monitoring results from agent")

	s.writeJSON(w, map[string]interface{}{
		"message":        "Results processed",
		"total_received": len(results),
		"stored":         storedCount,
		"timestamp":      time.Now().Unix(),
	})
}

func (s *Server) handleAgentHealth(w http.ResponseWriter, r *http.Request) {
	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Unix(),
		"version":   "2.0",
		"server":    "agent-api",
	}
	s.writeJSON(w, health)
}

func (s *Server) handleAgentCheckin(w http.ResponseWriter, r *http.Request) {
	// Extract remote IP address
	remoteIP := extractRemoteIP(r)

	var checkinData struct {
		AgentID   string                 `json:"agent_id"`
		Timestamp int64                  `json:"timestamp"`
		Status    string                 `json:"status"`
		OSInfo    map[string]interface{} `json:"os_info"`
		AgentInfo map[string]interface{} `json:"agent_info"`
	}

	if err := json.NewDecoder(r.Body).Decode(&checkinData); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	agentID := r.Header.Get("X-Agent-ID")
	if agentID == "" {
		agentID = checkinData.AgentID
	}

	if agentID == "" {
		http.Error(w, "Agent ID required", http.StatusBadRequest)
		return
	}

	// Get API key from the middleware (already validated)
	apiKey := r.Header.Get("X-API-Key")
	if apiKey == "" {
		http.Error(w, "API key missing", http.StatusUnauthorized)
		return
	}

	// Hash the API key to find/create agent in database
	keyHash := utils.HashAPIKey(apiKey)

	// Check if agent exists in database, if not create it
	agent, err := s.db.GetAgentByKeyHash(keyHash)
	if err != nil {
		log.Error().Err(err).Str("key_hash", keyHash).Str("remote_ip", remoteIP).Msg("Failed to get agent by key hash")
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	if agent == nil {
		// Auto-register new agent
		agentReq := &models.AgentCreateRequest{
			Name:        fmt.Sprintf("Agent-%s", agentID),
			APIKey:      apiKey,
			Description: stringPtr("Auto-registered agent"),
		}

		agent, err = s.db.AddAgent(agentReq, keyHash)
		if err != nil {
			log.Error().Err(err).Str("agent_id", agentID).Str("remote_ip", remoteIP).Msg("Failed to auto-register agent")
			http.Error(w, "Failed to register agent", http.StatusInternalServerError)
			return
		}

		log.Info().
			Str("agent_id", agentID).
			Int("db_id", agent.ID).
			Str("name", agent.Name).
			Str("remote_ip", remoteIP).
			Msg("Auto-registered new agent")
	}

	// Update agent status and OS information in database
	status := "online"
	if checkinData.Status != "" {
		status = checkinData.Status
	}

	if checkinData.OSInfo != nil {
		if err := s.db.UpdateAgentWithRemoteIP(keyHash, status, checkinData.OSInfo, remoteIP); err != nil {
			log.Error().Err(err).Str("key_hash", keyHash).Str("remote_ip", remoteIP).Msg("Failed to update agent OS info with remote IP from checkin")
		}
	} else {
		if err := s.db.UpdateAgentWithRemoteIP(keyHash, status, nil, remoteIP); err != nil {
			log.Error().Err(err).Str("key_hash", keyHash).Str("remote_ip", remoteIP).Msg("Failed to update agent status with remote IP")
		}
	}

	// Update agent connection tracking (in-memory)
	s.connMutex.Lock()
	if conn, exists := s.agentConns[agentID]; exists {
		conn.LastSeen = time.Now()
	} else {
		s.agentConns[agentID] = &AgentConn{
			AgentID:   agentID,
			Connected: time.Now(),
			LastSeen:  time.Now(),
		}
	}
	s.connMutex.Unlock()

	log.Debug().
		Str("agent_id", agentID).
		Int("db_id", agent.ID).
		Str("status", status).
		Str("remote_ip", remoteIP).
		Msg("Agent checked in")

	s.writeJSON(w, map[string]interface{}{
		"message":   "Checkin received",
		"timestamp": time.Now().Unix(),
		"status":    "ok",
		"agent_id":  agent.ID,
	})
}

// Helper function to create string pointer
func stringPtr(s string) *string {
	return &s
}

// sendWebSocketMessage sends a JSON message to an agent via WebSocket
func (s *Server) sendWebSocketMessage(agentConn *AgentConn, message map[string]interface{}) {
	data, err := json.Marshal(message)
	if err != nil {
		log.Error().Err(err).Str("agent_id", agentConn.AgentID).Msg("Failed to marshal WebSocket message")
		return
	}

	if err := agentConn.Conn.WriteMessage(websocket.TextMessage, data); err != nil {
		log.Error().Err(err).Str("agent_id", agentConn.AgentID).Msg("Failed to send WebSocket message")
	}
}

func (s *Server) setupAgentRouter() {
	r := chi.NewRouter()

	// Minimal middleware for agents
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	// Health endpoint for agent connectivity testing (no auth required)
	r.Get("/api/health", s.handleAgentHealth)

	// WebSocket endpoint for agents (auth handled in WebSocket handler)
	r.HandleFunc("/ws", s.handleWebSocket)

	// HTTP endpoints with authentication
	r.Group(func(r chi.Router) {
		r.Use(s.agentAuthMiddleware)

		// Agent checkin endpoint (HTTP fallback)
		r.Post("/api/agents/checkin", s.handleAgentCheckin)

		// Agent API routes (HTTP fallback)
		r.Route("/api/agent", func(r chi.Router) {
			r.Post("/register", s.handleAgentRegister)
			r.Post("/heartbeat", s.handleAgentHeartbeat)
			r.Post("/results", s.handleAgentResults)
			r.Get("/tasks", s.handleAgentTasks)
		})

		// Modern agent monitoring API
		r.Route("/api/monitoring", func(r chi.Router) {
			r.Get("/tasks", s.handleGetMonitoringTasks)
			r.Post("/results", s.handleSubmitMonitoringResults)
		})
	})

	s.agentRouter = r
}

// extractRemoteIP extracts the real remote IP address from the request
// Handles X-Forwarded-For, X-Real-IP headers for proxy scenarios
func extractRemoteIP(r *http.Request) string {
	// Check X-Forwarded-For header (for proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For can contain multiple IPs, take the first one
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			ip := strings.TrimSpace(ips[0])
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}

	// Check X-Real-IP header (for proxies)
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		if net.ParseIP(xri) != nil {
			return xri
		}
	}

	// Fall back to RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr // Return as-is if we can't split
	}
	return host
}
