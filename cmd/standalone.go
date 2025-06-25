package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/x86txt/sreootb/internal/agent"
	"github.com/x86txt/sreootb/internal/config"
	"github.com/x86txt/sreootb/internal/server"
)

// standaloneCmd represents the standalone command
var standaloneCmd = &cobra.Command{
	Use:   "standalone",
	Short: "Run SREootb in standalone mode (server + local agent)",
	Long: `Start SREootb in standalone mode - runs both server and a local agent in a single process.
This is perfect for simple deployments where you want both monitoring server and a local monitoring agent.

The server will use SQLite database and the local agent will automatically connect to the server
using an internally generated API key. This provides an "out of the box" monitoring solution.`,
	RunE: runStandalone,
}

func init() {
	rootCmd.AddCommand(standaloneCmd)

	// Server flags (same as server command)
	standaloneCmd.Flags().String("bind", "0.0.0.0:8080", "address to bind the web server to")
	standaloneCmd.Flags().String("tls-cert", "", "path to TLS certificate file")
	standaloneCmd.Flags().String("tls-key", "", "path to TLS private key file")
	standaloneCmd.Flags().Bool("auto-tls", true, "enable automatic TLS certificate generation")
	standaloneCmd.Flags().Bool("http3", false, "enable HTTP/3 support (requires TLS)")

	// Database configuration flags
	standaloneCmd.Flags().String("db-type", "sqlite", "database type: sqlite or cockroachdb")
	standaloneCmd.Flags().String("db-sqlite-path", "./db/sreootb.db", "path to SQLite database file (when using sqlite)")
	standaloneCmd.Flags().String("db-host", "", "database host (when using cockroachdb)")
	standaloneCmd.Flags().Int("db-port", 26257, "database port (when using cockroachdb)")
	standaloneCmd.Flags().String("db-database", "", "database name (when using cockroachdb)")
	standaloneCmd.Flags().String("db-user", "", "database user (when using cockroachdb)")
	standaloneCmd.Flags().String("db-password", "", "database password (when using cockroachdb)")
	standaloneCmd.Flags().String("db-ssl-mode", "require", "SSL mode for CockroachDB connection")
	standaloneCmd.Flags().String("db-ssl-root-cert", "", "path to SSL root certificate")
	standaloneCmd.Flags().String("db-ssl-cert", "", "path to SSL client certificate")
	standaloneCmd.Flags().String("db-ssl-key", "", "path to SSL client key")
	standaloneCmd.Flags().Int("db-max-open-conns", 25, "maximum number of open database connections")
	standaloneCmd.Flags().Int("db-max-idle-conns", 5, "maximum number of idle database connections")

	// Server-specific settings
	standaloneCmd.Flags().String("min-scan-interval", "10s", "minimum allowed scan interval")
	standaloneCmd.Flags().String("max-scan-interval", "24h", "maximum allowed scan interval")
	standaloneCmd.Flags().Bool("dev-mode", false, "enable development mode")

	// Agent-specific flags
	standaloneCmd.Flags().String("agent-bind", "127.0.0.1:8082", "address to bind the agent health endpoint")
	standaloneCmd.Flags().Duration("check-interval", 30*time.Second, "agent check interval")
	standaloneCmd.Flags().Bool("insecure-tls", false, "skip TLS certificate verification for agent (insecure)")

	// Config generation flags
	standaloneCmd.Flags().Bool("gen-config", false, "generate sample standalone configuration file")
	standaloneCmd.Flags().Bool("gen-systemd", false, "generate systemd service file for standalone")

	// Bind server flags to viper with standalone prefix
	viper.BindPFlag("standalone.server.bind", standaloneCmd.Flags().Lookup("bind"))
	viper.BindPFlag("standalone.server.tls_cert", standaloneCmd.Flags().Lookup("tls-cert"))
	viper.BindPFlag("standalone.server.tls_key", standaloneCmd.Flags().Lookup("tls-key"))
	viper.BindPFlag("standalone.server.auto_tls", standaloneCmd.Flags().Lookup("auto-tls"))
	viper.BindPFlag("standalone.server.http3", standaloneCmd.Flags().Lookup("http3"))

	// Bind database flags to viper
	viper.BindPFlag("standalone.server.database.type", standaloneCmd.Flags().Lookup("db-type"))
	viper.BindPFlag("standalone.server.database.sqlite_path", standaloneCmd.Flags().Lookup("db-sqlite-path"))
	viper.BindPFlag("standalone.server.database.host", standaloneCmd.Flags().Lookup("db-host"))
	viper.BindPFlag("standalone.server.database.port", standaloneCmd.Flags().Lookup("db-port"))
	viper.BindPFlag("standalone.server.database.database", standaloneCmd.Flags().Lookup("db-database"))
	viper.BindPFlag("standalone.server.database.user", standaloneCmd.Flags().Lookup("db-user"))
	viper.BindPFlag("standalone.server.database.password", standaloneCmd.Flags().Lookup("db-password"))
	viper.BindPFlag("standalone.server.database.ssl_mode", standaloneCmd.Flags().Lookup("db-ssl-mode"))
	viper.BindPFlag("standalone.server.database.ssl_root_cert", standaloneCmd.Flags().Lookup("db-ssl-root-cert"))
	viper.BindPFlag("standalone.server.database.ssl_cert", standaloneCmd.Flags().Lookup("db-ssl-cert"))
	viper.BindPFlag("standalone.server.database.ssl_key", standaloneCmd.Flags().Lookup("db-ssl-key"))
	viper.BindPFlag("standalone.server.database.max_open_conns", standaloneCmd.Flags().Lookup("db-max-open-conns"))
	viper.BindPFlag("standalone.server.database.max_idle_conns", standaloneCmd.Flags().Lookup("db-max-idle-conns"))

	// Bind server settings
	viper.BindPFlag("standalone.server.min_scan_interval", standaloneCmd.Flags().Lookup("min-scan-interval"))
	viper.BindPFlag("standalone.server.max_scan_interval", standaloneCmd.Flags().Lookup("max-scan-interval"))
	viper.BindPFlag("standalone.server.dev_mode", standaloneCmd.Flags().Lookup("dev-mode"))

	// Bind agent flags
	viper.BindPFlag("standalone.agent.bind", standaloneCmd.Flags().Lookup("agent-bind"))
	viper.BindPFlag("standalone.agent.check_interval", standaloneCmd.Flags().Lookup("check-interval"))
	viper.BindPFlag("standalone.agent.insecure_tls", standaloneCmd.Flags().Lookup("insecure-tls"))
}

func runStandalone(cmd *cobra.Command, args []string) error {
	// Check for config generation flags first
	genConfig, _ := cmd.Flags().GetBool("gen-config")
	genSystemd, _ := cmd.Flags().GetBool("gen-systemd")

	if genConfig {
		return generateStandaloneConfig()
	}

	if genSystemd {
		return generateStandaloneSystemd()
	}

	log.Info().Msg("Starting SREootb in standalone mode (server + local agent)")

	// Load or create standalone configuration
	cfg, err := createStandaloneConfig(cmd)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create standalone configuration")
	}

	// Setup graceful shutdown context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// WaitGroup to coordinate server and agent shutdown
	var wg sync.WaitGroup
	errChan := make(chan error, 2)

	// Start server
	wg.Add(1)
	go func() {
		defer wg.Done()

		srv, err := server.New(cfg, staticFS, appFS)
		if err != nil {
			errChan <- fmt.Errorf("failed to create server: %w", err)
			return
		}

		log.Info().Str("bind", cfg.Server.Bind).Msg("Starting server component")
		if err := srv.Start(ctx); err != nil && err != context.Canceled {
			errChan <- fmt.Errorf("server error: %w", err)
		}
	}()

	// Wait a moment for server to start before starting agent
	time.Sleep(2 * time.Second)

	// Start local agent
	wg.Add(1)
	go func() {
		defer wg.Done()

		agentInstance, err := agent.New(cfg)
		if err != nil {
			errChan <- fmt.Errorf("failed to create agent: %w", err)
			return
		}

		log.Info().Str("server_url", cfg.Agent.ServerURL).Msg("Starting local agent component")
		if err := agentInstance.Start(); err != nil && err != context.Canceled {
			errChan <- fmt.Errorf("agent error: %w", err)
		}
	}()

	// Wait for shutdown signal or error
	select {
	case sig := <-sigChan:
		log.Info().Str("signal", sig.String()).Msg("Shutdown signal received")
		cancel()
	case err := <-errChan:
		log.Error().Err(err).Msg("Component error, shutting down")
		cancel()
		return err
	}

	// Wait for both components to shut down
	log.Info().Msg("Waiting for components to shut down...")
	wg.Wait()
	log.Info().Msg("Standalone mode shutdown complete")

	return nil
}

func createStandaloneConfig(cmd *cobra.Command) (*config.Config, error) {
	// Generate internal API key for agent-server communication
	adminAPIKey, err := generateSecureAPIKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate admin API key: %w", err)
	}

	// Get server flag values
	serverBind, _ := cmd.Flags().GetString("bind")
	tlsCert, _ := cmd.Flags().GetString("tls-cert")
	tlsKey, _ := cmd.Flags().GetString("tls-key")
	autoTLS, _ := cmd.Flags().GetBool("auto-tls")
	http3, _ := cmd.Flags().GetBool("http3")

	// Database flags
	dbType, _ := cmd.Flags().GetString("db-type")
	dbSQLitePath, _ := cmd.Flags().GetString("db-sqlite-path")
	dbHost, _ := cmd.Flags().GetString("db-host")
	dbPort, _ := cmd.Flags().GetInt("db-port")
	dbDatabase, _ := cmd.Flags().GetString("db-database")
	dbUser, _ := cmd.Flags().GetString("db-user")
	dbPassword, _ := cmd.Flags().GetString("db-password")
	dbSSLMode, _ := cmd.Flags().GetString("db-ssl-mode")
	dbSSLRootCert, _ := cmd.Flags().GetString("db-ssl-root-cert")
	dbSSLCert, _ := cmd.Flags().GetString("db-ssl-cert")
	dbSSLKey, _ := cmd.Flags().GetString("db-ssl-key")
	dbMaxOpenConns, _ := cmd.Flags().GetInt("db-max-open-conns")
	dbMaxIdleConns, _ := cmd.Flags().GetInt("db-max-idle-conns")

	// Server settings
	minScanIntervalStr, _ := cmd.Flags().GetString("min-scan-interval")
	maxScanIntervalStr, _ := cmd.Flags().GetString("max-scan-interval")
	devMode, _ := cmd.Flags().GetBool("dev-mode")

	// Agent flags
	agentBind, _ := cmd.Flags().GetString("agent-bind")
	checkInterval, _ := cmd.Flags().GetDuration("check-interval")
	insecureTLS, _ := cmd.Flags().GetBool("insecure-tls")

	// Parse scan intervals to Duration
	minScanInterval, err := time.ParseDuration(minScanIntervalStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse min-scan-interval: %w", err)
	}

	maxScanInterval, err := time.ParseDuration(maxScanIntervalStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse max-scan-interval: %w", err)
	}

	// Get hostname for agent ID
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "standalone-agent"
	}

	// Build server URL for agent (use localhost since they're in same process)
	// Agent should connect to the agent API server (port 8081), not web GUI server
	agentServerURL := "http://localhost:8081"
	if autoTLS || (tlsCert != "" && tlsKey != "") {
		agentServerURL = "https://localhost:8081"
	}

	// Create combined configuration
	cfg := &config.Config{
		Log: config.LogConfig{
			Level:  viper.GetString("log.level"),
			Format: viper.GetString("log.format"),
		},
		Server: config.ServerConfig{
			Bind:      serverBind,
			AgentBind: "0.0.0.0:8081", // Default agent API bind
			TLSCert:   tlsCert,
			TLSKey:    tlsKey,
			AutoTLS:   autoTLS,
			Database: config.DatabaseConfig{
				Type:            dbType,
				SQLitePath:      dbSQLitePath,
				Host:            dbHost,
				Port:            dbPort,
				Database:        dbDatabase,
				User:            dbUser,
				Password:        dbPassword,
				SSLMode:         dbSSLMode,
				SSLRootCert:     dbSSLRootCert,
				SSLCert:         dbSSLCert,
				SSLKey:          dbSSLKey,
				MaxOpenConns:    dbMaxOpenConns,
				MaxIdleConns:    dbMaxIdleConns,
				ConnMaxLifetime: 300 * time.Second, // Default values
				ConnMaxIdleTime: 60 * time.Second,
			},
			AdminAPIKey:     adminAPIKey,
			MinScanInterval: minScanInterval,
			MaxScanInterval: maxScanInterval,
			DevMode:         devMode,
		},
		Agent: config.AgentConfig{
			ServerURL:     agentServerURL, // Connect to agent API server, not web GUI
			APIKey:        "",             // Will be set to server's agent API key after ensureAgentAPIKey
			AgentID:       hostname + "-local",
			CheckInterval: checkInterval,
			Bind:          agentBind,
			InsecureTLS:   insecureTLS,
		},
	}

	// Ensure agent API key is generated/loaded by the server
	if err := ensureAgentAPIKeyForStandalone(cfg); err != nil {
		return nil, fmt.Errorf("failed to ensure agent API key: %w", err)
	}

	// Now set the agent to use the server's agent API key
	cfg.Agent.APIKey = cfg.Server.AgentAPIKey

	// Ensure database directory exists for SQLite
	if dbType == "sqlite" {
		dbDir := filepath.Dir(dbSQLitePath)
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create database directory: %w", err)
		}
	}

	log.Info().
		Str("server_bind", serverBind).
		Str("agent_bind", agentBind).
		Str("db_type", dbType).
		Str("db_path", dbSQLitePath).
		Bool("auto_tls", autoTLS).
		Bool("http3", http3).
		Dur("check_interval", checkInterval).
		Msg("Standalone configuration created")

	return cfg, nil
}

// ensureAgentAPIKeyForStandalone ensures the agent API key exists for standalone mode
func ensureAgentAPIKeyForStandalone(cfg *config.Config) error {
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

func generateStandaloneConfig() error {
	// Generate a secure API key
	apiKey, err := generateSecureAPIKey()
	if err != nil {
		return fmt.Errorf("failed to generate API key: %w", err)
	}

	// Get hostname
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "standalone-host"
	}

	configContent := fmt.Sprintf(`# SRE: Out of the Box (SREootb) Standalone Configuration
# This configuration runs both server and local agent in a single process

# Logging configuration
log:
  level: "info"          # trace, debug, info, warn, error
  format: "console"      # console, json

# Server configuration
server:
  # Web server settings
  bind: "0.0.0.0:8080"                     # Web server bind address
  
  # TLS Configuration
  auto_tls: true                           # Enable automatic ed25519 TLS certificate generation
  # tls_cert: "/path/to/cert.pem"          # Custom TLS certificate (overrides auto_tls)
  # tls_key: "/path/to/key.pem"            # Custom TLS private key
  # http3: false                           # Enable HTTP/3 support (requires TLS)
  
  # Database Configuration
  database:
    # SQLite configuration (default for standalone mode)
    type: "sqlite"                         # Database type: sqlite or cockroachdb
    sqlite_path: "./db/sreootb.db"         # SQLite database file path
    
    # CockroachDB configuration (uncomment to use CockroachDB cluster)
    # type: "cockroachdb"
    # host: "localhost"                    # CockroachDB cluster host
    # port: 26257                          # CockroachDB port
    # database: "sreootb"                  # Database name
    # user: "sreootb_user"                 # Database user
    # password: "secure_password"          # Database password
    # ssl_mode: "require"                  # SSL mode: disable, allow, prefer, require
    # ssl_root_cert: "/path/to/ca.crt"     # SSL root certificate
    # ssl_cert: "/path/to/client.crt"      # SSL client certificate
    # ssl_key: "/path/to/client.key"       # SSL client key
    
    # Connection pool settings (for CockroachDB)
    max_open_conns: 25                     # Maximum open connections
    max_idle_conns: 5                      # Maximum idle connections
    conn_max_lifetime: "300s"              # Connection maximum lifetime
    conn_max_idle_time: "60s"              # Connection maximum idle time
  
  # Authentication
  admin_api_key: "%s"                      # Admin API key (auto-generated)
  
  # Monitoring settings
  min_scan_interval: "10s"                 # Minimum allowed scan interval
  max_scan_interval: "24h"                 # Maximum allowed scan interval
  dev_mode: false                          # Development mode

# Local agent configuration (connects to local server)
agent:
  server_url: "https://localhost:8080"     # Connect to local server (auto-TLS enabled)
  api_key: "%s"                            # Same key as admin (for simplicity)
  agent_id: "%s-local"                     # Local agent identifier
  check_interval: "30s"                    # How often to check in with server
  bind: "127.0.0.1:8082"                   # Agent health endpoint
  insecure_tls: true                       # Skip TLS verification for localhost

# Standalone mode features:
# 1. Single process runs both server and agent
# 2. Auto-generates secure API keys
# 3. Supports all server and agent configuration options
# 4. Can use SQLite (simple) or CockroachDB (HA)
# 5. Supports custom TLS certificates or auto-TLS
# 6. Perfect for simple deployments and development

# Usage examples:
# Basic:           sreootb standalone
# Custom config:   sreootb standalone --config sreootb-standalone.yaml
# Custom TLS:      sreootb standalone --tls-cert cert.pem --tls-key key.pem
# CockroachDB:     sreootb standalone --db-type cockroachdb --db-host localhost
# Custom ports:    sreootb standalone --bind 0.0.0.0:9090 --agent-bind 127.0.0.1:9091
`, apiKey, apiKey, hostname)

	filename := "sreootb-standalone.yaml"
	if err := os.WriteFile(filename, []byte(configContent), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("✅ Standalone configuration file generated: %s\n", filename)
	fmt.Printf("🔑 Admin API Key: %s\n", apiKey)
	fmt.Println("📝 This configuration runs both server and agent in one process:")
	fmt.Println("   - Web dashboard available at: https://localhost:8080")
	fmt.Println("   - SQLite database stored in: ./db/sreootb.db")
	fmt.Println("   - Local agent automatically connects to server")
	fmt.Printf("   sreootb standalone --config %s\n", filename)
	fmt.Println("\n🚀 Or run with default settings:")
	fmt.Println("   sreootb standalone")
	return nil
}

func generateStandaloneSystemd() error {
	execPath, err := os.Executable()
	if err != nil {
		execPath = "/usr/local/bin/sreootb"
	} else {
		execPath, _ = filepath.Abs(execPath)
	}

	serviceContent := fmt.Sprintf(`[Unit]
Description=SRE Out of the Box Standalone
Documentation=https://github.com/x86txt/sreootb
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=sreootb
Group=sreootb
ExecStart=%s standalone --config /etc/sreootb/sreootb-standalone.yaml
ExecReload=/bin/kill -HUP $MAINPID
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=sreootb-standalone

# Security settings
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/sreootb /var/log/sreootb
CapabilityBoundingSet=CAP_NET_BIND_SERVICE

# Resource limits
LimitNOFILE=65536
MemoryLimit=512M

# Working directory
WorkingDirectory=/var/lib/sreootb

# Environment
Environment=SREOOB_MODE=standalone

[Install]
WantedBy=multi-user.target
`, execPath)

	filename := "sreootb-standalone.service"
	if err := os.WriteFile(filename, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("failed to write systemd service file: %w", err)
	}

	fmt.Printf("✅ Systemd service file generated: %s\n", filename)
	fmt.Println("📝 To install and start the service:")
	fmt.Println("   sudo cp sreootb-standalone.service /etc/systemd/system/")
	fmt.Println("   sudo mkdir -p /etc/sreootb /var/lib/sreootb /var/log/sreootb")
	fmt.Println("   sudo cp sreootb-standalone.yaml /etc/sreootb/")
	fmt.Println("   sudo useradd -r -s /bin/false sreootb")
	fmt.Println("   sudo chown -R sreootb:sreootb /var/lib/sreootb /var/log/sreootb")
	fmt.Println("   sudo systemctl daemon-reload")
	fmt.Println("   sudo systemctl enable --now sreootb-standalone")
	return nil
}
