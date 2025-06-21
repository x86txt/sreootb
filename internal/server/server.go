package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/rs/zerolog/log"

	"github.com/x86txt/sreootb/internal/autotls"
	"github.com/x86txt/sreootb/internal/config"
	"github.com/x86txt/sreootb/internal/database"
	"github.com/x86txt/sreootb/internal/models"
	"github.com/x86txt/sreootb/internal/monitor"
	"github.com/x86txt/sreootb/internal/utils"
)

// AgentConn represents an active agent WebSocket connection
type AgentConn struct {
	AgentID   string
	Connected time.Time
	LastSeen  time.Time
	// TODO: Add websocket connection field when implementing
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
}

// New creates a new server instance
func New(cfg *config.Config) (*Server, error) {
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
	}

	// Setup routers
	srv.setupWebRouter()
	srv.setupAgentRouter()

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
	for agentID := range s.agentConns {
		// TODO: Close websocket connections when implemented
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
		})

		// Agent management
		r.Route("/agents", func(r chi.Router) {
			r.Get("/", s.handleGetAgents)
			r.Post("/", s.handleCreateAgent)
			r.Delete("/{id}", s.handleDeleteAgent)
		})

		// Monitoring
		r.Post("/check/manual", s.handleManualCheck)
		r.Get("/stats", s.handleGetStats)
		r.Get("/config", s.handleGetConfig)
		r.Get("/cert", s.handleGetCertInfo)

		// Health check
		r.Get("/health", s.handleHealth)
	})

	// Serve embedded frontend
	r.Get("/*", s.handleIndex)

	s.webRouter = r
}

// setupAgentRouter configures the agent API router with WebSocket support
func (s *Server) setupAgentRouter() {
	r := chi.NewRouter()

	// Minimal middleware for agents
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(s.agentAuthMiddleware)

	// WebSocket endpoint for agents
	r.HandleFunc("/ws", s.handleWebSocket)

	// Agent API routes (HTTP fallback)
	r.Route("/api/agent", func(r chi.Router) {
		r.Post("/register", s.handleAgentRegister)
		r.Post("/heartbeat", s.handleAgentHeartbeat)
		r.Post("/results", s.handleAgentResults)
		r.Get("/tasks", s.handleAgentTasks)
	})

	s.agentRouter = r
}

// agentAuthMiddleware validates agent API keys
func (s *Server) agentAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for WebSocket upgrade
		if r.URL.Path == "/ws" {
			next.ServeHTTP(w, r)
			return
		}

		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			http.Error(w, "Missing API key", http.StatusUnauthorized)
			return
		}

		// Validate API key
		keyHash := utils.HashAPIKey(apiKey)
		valid, err := s.db.ValidateAgentAPIKey(keyHash)
		if err != nil {
			log.Error().Err(err).Msg("Failed to validate API key")
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if !valid {
			http.Error(w, "Invalid API key", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// handleWebSocket handles WebSocket connections from agents
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement WebSocket upgrade and connection handling
	// Extract agent credentials from request
	agentID := r.Header.Get("X-Agent-ID")
	apiKey := r.Header.Get("X-API-Key")

	if agentID == "" || apiKey == "" {
		http.Error(w, "Missing agent credentials", http.StatusBadRequest)
		return
	}

	// Validate API key
	keyHash := utils.HashAPIKey(apiKey)
	valid, err := s.db.ValidateAgentAPIKey(keyHash)
	if err != nil {
		log.Error().Err(err).Msg("Failed to validate API key for WebSocket")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if !valid {
		http.Error(w, "Invalid API key", http.StatusUnauthorized)
		return
	}

	// TODO: Upgrade to WebSocket and handle connection
	log.Info().Str("agent_id", agentID).Msg("WebSocket connection requested (not yet implemented)")
	http.Error(w, "WebSocket not yet implemented", http.StatusNotImplemented)
}

// API Handlers for Web GUI
func (s *Server) handleGetSites(w http.ResponseWriter, r *http.Request) {
	sites, err := s.db.GetSites()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
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

	// Add connection status
	s.connMutex.RLock()
	for i := range agents {
		if conn, exists := s.agentConns[strconv.Itoa(agents[i].ID)]; exists {
			agents[i].Connected = true
			agents[i].LastSeen = &conn.LastSeen
		}
	}
	s.connMutex.RUnlock()

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
	features := []string{"embedded_frontend", "http1.1"}

	// Check TLS status
	tlsEnabled := false
	autoTLSEnabled := s.config.Server.AutoTLS && s.autoTLS != nil
	manualTLSEnabled := s.config.Server.TLSCert != "" && s.config.Server.TLSKey != ""

	if autoTLSEnabled || manualTLSEnabled {
		features = append(features, "http2", "tls")
		tlsEnabled = true
	}

	if autoTLSEnabled {
		features = append(features, "auto_tls")
	}

	config := map[string]interface{}{
		"scan_interval": map[string]interface{}{
			"min_seconds": int(s.config.Server.MinScanInterval.Seconds()),
			"max_seconds": int(s.config.Server.MaxScanInterval.Seconds()),
		},
		"version":        "2.0",
		"features":       features,
		"web_gui_port":   s.config.Server.Bind,
		"agent_api_port": s.config.Server.AgentBind,
		"tls_enabled":    tlsEnabled,
		"auto_tls":       autoTLSEnabled,
		"manual_tls":     manualTLSEnabled,
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

// Agent WebSocket message handlers
func (s *Server) handleAgentHeartbeatWS(conn *AgentConn, msg map[string]interface{}) {
	log.Debug().Str("agent_id", conn.AgentID).Msg("Received heartbeat")
	// Heartbeat is handled by updating LastSeen in handleWebSocket
}

func (s *Server) handleAgentResultsWS(conn *AgentConn, msg map[string]interface{}) {
	log.Debug().Str("agent_id", conn.AgentID).Msg("Received monitoring results")
	// Process monitoring results async
	go func() {
		// TODO: Parse and store monitoring results
		log.Info().Str("agent_id", conn.AgentID).Msg("Processing monitoring results")
	}()
}

func (s *Server) handleAgentRegisterWS(conn *AgentConn, msg map[string]interface{}) {
	log.Info().Str("agent_id", conn.AgentID).Msg("Agent registered via WebSocket")
	// TODO: Update agent registration in database
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

// Web GUI frontend
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
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
    </style>
</head>
<body>
    <div class="container">
        <h1>🚀 SRE: Out of the Box v2</h1>
        
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

// writeJSON writes a JSON response
func (s *Server) writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(data)
}
