package cmd

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/x86txt/sreootb/internal/config"
	"github.com/x86txt/sreootb/internal/server"
)

// Global variables to hold the embedded web filesystems
var staticFS embed.FS
var appFS embed.FS

// SetWebFS sets the embedded web filesystems from main package
func SetWebFS(static, app embed.FS) {
	staticFS = static
	appFS = app
}

// serverCmd represents the server command
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Run SREootb in server mode",
	Long: `Start the SREootb server with web interface and API endpoints.
This includes the monitoring dashboard, agent management, and site monitoring functionality.`,
	RunE: runServer,
}

func init() {
	rootCmd.AddCommand(serverCmd)

	// Server-specific flags
	serverCmd.Flags().String("bind", "0.0.0.0:8080", "address to bind the server to")
	serverCmd.Flags().String("tls-cert", "", "path to TLS certificate file")
	serverCmd.Flags().String("tls-key", "", "path to TLS private key file")
	serverCmd.Flags().Bool("auto-tls", false, "enable automatic TLS certificate generation")
	serverCmd.Flags().Bool("http3", false, "enable HTTP/3 support (requires TLS)")
	serverCmd.Flags().String("accent-color", "#E11D48", "custom accent color (hex code, e.g., #E11D48)")

	// Database configuration flags
	serverCmd.Flags().String("db-type", "sqlite", "database type: sqlite or cockroachdb")
	serverCmd.Flags().String("db-sqlite-path", "./db/sreootb.db", "path to SQLite database file (when using sqlite)")
	serverCmd.Flags().String("db-host", "", "database host (when using cockroachdb)")
	serverCmd.Flags().Int("db-port", 26257, "database port (when using cockroachdb)")
	serverCmd.Flags().String("db-database", "", "database name (when using cockroachdb)")
	serverCmd.Flags().String("db-user", "", "database user (when using cockroachdb)")
	serverCmd.Flags().String("db-password", "", "database password (when using cockroachdb)")
	serverCmd.Flags().String("db-ssl-mode", "require", "SSL mode for CockroachDB connection")
	serverCmd.Flags().String("db-ssl-root-cert", "", "path to SSL root certificate")
	serverCmd.Flags().String("db-ssl-cert", "", "path to SSL client certificate")
	serverCmd.Flags().String("db-ssl-key", "", "path to SSL client key")
	serverCmd.Flags().Int("db-max-open-conns", 25, "maximum number of open database connections")
	serverCmd.Flags().Int("db-max-idle-conns", 5, "maximum number of idle database connections")

	// Config generation flags
	serverCmd.Flags().Bool("gen-config", false, "generate sample server configuration file")
	serverCmd.Flags().Bool("gen-systemd", false, "generate systemd service file for server")

	// Bind flags to viper
	viper.BindPFlag("server.bind", serverCmd.Flags().Lookup("bind"))
	viper.BindPFlag("server.tls_cert", serverCmd.Flags().Lookup("tls-cert"))
	viper.BindPFlag("server.tls_key", serverCmd.Flags().Lookup("tls-key"))
	viper.BindPFlag("server.auto_tls", serverCmd.Flags().Lookup("auto-tls"))
	viper.BindPFlag("server.http3", serverCmd.Flags().Lookup("http3"))
	viper.BindPFlag("server.accent_color", serverCmd.Flags().Lookup("accent-color"))

	// Bind database flags to viper
	viper.BindPFlag("server.database.type", serverCmd.Flags().Lookup("db-type"))
	viper.BindPFlag("server.database.sqlite_path", serverCmd.Flags().Lookup("db-sqlite-path"))
	viper.BindPFlag("server.database.host", serverCmd.Flags().Lookup("db-host"))
	viper.BindPFlag("server.database.port", serverCmd.Flags().Lookup("db-port"))
	viper.BindPFlag("server.database.database", serverCmd.Flags().Lookup("db-database"))
	viper.BindPFlag("server.database.user", serverCmd.Flags().Lookup("db-user"))
	viper.BindPFlag("server.database.password", serverCmd.Flags().Lookup("db-password"))
	viper.BindPFlag("server.database.ssl_mode", serverCmd.Flags().Lookup("db-ssl-mode"))
	viper.BindPFlag("server.database.ssl_root_cert", serverCmd.Flags().Lookup("db-ssl-root-cert"))
	viper.BindPFlag("server.database.ssl_cert", serverCmd.Flags().Lookup("db-ssl-cert"))
	viper.BindPFlag("server.database.ssl_key", serverCmd.Flags().Lookup("db-ssl-key"))
	viper.BindPFlag("server.database.max_open_conns", serverCmd.Flags().Lookup("db-max-open-conns"))
	viper.BindPFlag("server.database.max_idle_conns", serverCmd.Flags().Lookup("db-max-idle-conns"))
}

func runServer(cmd *cobra.Command, args []string) error {
	// Check for config generation flags first
	genConfig, _ := cmd.Flags().GetBool("gen-config")
	genSystemd, _ := cmd.Flags().GetBool("gen-systemd")

	if genConfig {
		return generateServerConfig()
	}

	if genSystemd {
		return generateServerSystemd()
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	// Create server instance
	srv, err := server.New(cfg, staticFS, appFS)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create server")
	}

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Info().Msg("Shutdown signal received")
		cancel()
	}()

	// Start server
	log.Info().Str("bind", cfg.Server.Bind).Msg("Starting SREootb server")
	return srv.Start(ctx)
}

// generateSecureAPIKey generates a cryptographically secure API key
func generateSecureAPIKey() (string, error) {
	bytes := make([]byte, 32) // 32 bytes = 64 hex characters
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func generateServerConfig() error {
	// Generate a secure admin API key
	adminAPIKey, err := generateSecureAPIKey()
	if err != nil {
		return fmt.Errorf("failed to generate admin API key: %w", err)
	}

	configContent := fmt.Sprintf(`# SRE: Out of the Box (SREootb) Server Configuration

# Logging configuration
log:
  level: "info"          # trace, debug, info, warn, error
  format: "console"      # console, json

# Server configuration
server:
  # Web GUI server configuration
  bind: "0.0.0.0:8080"              # Web GUI bind address
  tls_cert: ""                      # Path to TLS certificate for web GUI (leave empty for auto-TLS)
  tls_key: ""                       # Path to TLS private key for web GUI (leave empty for auto-TLS)
  
  # Agent API server configuration
  agent_bind: "0.0.0.0:8081"        # Agent API bind address (WebSocket/HTTP)
  agent_tls_cert: ""                # Separate TLS cert for agent API (falls back to tls_cert)
  agent_tls_key: ""                 # Separate TLS key for agent API (falls back to tls_key)
  
  # TLS Configuration
  auto_tls: true                    # Enable automatic ed25519 TLS certificate generation
                                    # Certificates will be stored in ./certs/ directory
                                    # Includes proper DNS Alt Names for browser compatibility
  
  # Database Configuration
  database:
    # Database type: "sqlite" or "cockroachdb"
    type: "sqlite"
    
    # SQLite configuration (used when type = "sqlite")
    sqlite_path: "./db/sreootb.db"   # SQLite database file path
    
    # CockroachDB configuration (used when type = "cockroachdb")
    # Uncomment and configure for CockroachDB cluster deployment
    # host: "localhost"              # CockroachDB cluster host
    # port: 26257                    # CockroachDB port (default: 26257)
    # database: "sreootb"            # Database name
    # user: "sreootb_user"           # Database user
    # password: "secure_password"    # Database password
    # ssl_mode: "require"            # SSL mode: disable, allow, prefer, require, verify-ca, verify-full
    # ssl_root_cert: ""              # Path to SSL root certificate file
    # ssl_cert: ""                   # Path to SSL client certificate file  
    # ssl_key: ""                    # Path to SSL client key file
    
    # Connection pool settings (for CockroachDB)
    max_open_conns: 25               # Maximum open connections
    max_idle_conns: 5                # Maximum idle connections
    conn_max_lifetime: "300s"        # Connection maximum lifetime
    conn_max_idle_time: "60s"        # Connection maximum idle time
  
  # Authentication
  admin_api_key: "%s"               # Admin API key for web GUI access (generated)
  
  # General server settings
  min_scan_interval: "10s"          # Minimum allowed scan interval
  max_scan_interval: "24h"          # Maximum allowed scan interval
  dev_mode: false                   # Development mode

# Agent configuration is not needed for server mode
# Use 'sreootb agent --gen-config' to generate agent configuration

# Example CockroachDB HA deployment configuration:
# 1. Set up a 3-node CockroachDB cluster
# 2. Create the database and user:
#    CREATE DATABASE sreootb;
#    CREATE USER sreootb_user WITH PASSWORD 'secure_password';
#    GRANT ALL ON DATABASE sreootb TO sreootb_user;
# 3. Deploy this application to 3 servers with identical configuration
# 4. Each server can connect to the local CockroachDB node for optimal performance
`, adminAPIKey)

	filename := "sreootb-server.yaml"
	if err := os.WriteFile(filename, []byte(configContent), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("✅ Server configuration file generated: %s\n", filename)
	fmt.Printf("🔑 Admin API Key (for web GUI access): %s\n", adminAPIKey)
	fmt.Println("📝 Edit the configuration file to:")
	fmt.Println("   - Choose database type (sqlite or cockroachdb)")
	fmt.Println("   - Configure database connection parameters")
	fmt.Println("   - Set up TLS certificates if needed")
	fmt.Printf("   sreootb server --config %s\n", filename)
	fmt.Println("\n🏗️  For CockroachDB HA deployment:")
	fmt.Println("   1. Set up a 3-node CockroachDB cluster")
	fmt.Println("   2. Create database and user as shown in config comments")
	fmt.Println("   3. Deploy app to 3 servers with database.type: cockroachdb")
	fmt.Println("   4. Point each server to local CockroachDB node for best performance")
	return nil
}

func generateServerSystemd() error {
	execPath, err := os.Executable()
	if err != nil {
		execPath = "/usr/local/bin/sreootb"
	} else {
		execPath, _ = filepath.Abs(execPath)
	}

	serviceContent := fmt.Sprintf(`[Unit]
Description=SRE Out of the Box Server
Documentation=https://github.com/x86txt/sreootb
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=sreootb
Group=sreootb
ExecStart=%s server --config /etc/sreootb/sreootb-server.yaml
ExecReload=/bin/kill -HUP $MAINPID
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=sreootb-server

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
Environment=SREOOB_MODE=server

[Install]
WantedBy=multi-user.target
`, execPath)

	filename := "sreootb-server.service"
	if err := os.WriteFile(filename, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("failed to write systemd service file: %w", err)
	}

	fmt.Printf("✅ Systemd service file generated: %s\n", filename)
	fmt.Println("📝 To install and start the service:")
	fmt.Println("   sudo cp sreootb-server.service /etc/systemd/system/")
	fmt.Println("   sudo mkdir -p /etc/sreootb /var/lib/sreootb /var/log/sreootb")
	fmt.Println("   sudo cp sreootb-server.yaml /etc/sreootb/")
	fmt.Println("   sudo useradd -r -s /bin/false sreootb")
	fmt.Println("   sudo chown -R sreootb:sreootb /var/lib/sreootb /var/log/sreootb")
	fmt.Println("   sudo systemctl daemon-reload")
	fmt.Println("   sudo systemctl enable --now sreootb-server")
	return nil
}
