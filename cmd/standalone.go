package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
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

	// Standalone-specific flags (combines common server and agent flags)
	standaloneCmd.Flags().String("bind", "0.0.0.0:8080", "address to bind the web server to")
	standaloneCmd.Flags().String("agent-bind", "127.0.0.1:8082", "address to bind the agent health endpoint")
	standaloneCmd.Flags().String("db-path", "./db/sreootb.db", "path to SQLite database file")
	standaloneCmd.Flags().Bool("auto-tls", true, "enable automatic TLS certificate generation")
	standaloneCmd.Flags().Duration("check-interval", 30*time.Second, "agent check interval")
	standaloneCmd.Flags().String("min-scan-interval", "10s", "minimum allowed scan interval")
	standaloneCmd.Flags().String("max-scan-interval", "24h", "maximum allowed scan interval")

	// Config generation flags
	standaloneCmd.Flags().Bool("gen-config", false, "generate sample standalone configuration file")

	// Bind flags to viper with standalone prefix to avoid conflicts
	viper.BindPFlag("standalone.server_bind", standaloneCmd.Flags().Lookup("bind"))
	viper.BindPFlag("standalone.agent_bind", standaloneCmd.Flags().Lookup("agent-bind"))
	viper.BindPFlag("standalone.db_path", standaloneCmd.Flags().Lookup("db-path"))
	viper.BindPFlag("standalone.auto_tls", standaloneCmd.Flags().Lookup("auto-tls"))
	viper.BindPFlag("standalone.check_interval", standaloneCmd.Flags().Lookup("check-interval"))
	viper.BindPFlag("standalone.min_scan_interval", standaloneCmd.Flags().Lookup("min-scan-interval"))
	viper.BindPFlag("standalone.max_scan_interval", standaloneCmd.Flags().Lookup("max-scan-interval"))
}

func runStandalone(cmd *cobra.Command, args []string) error {
	// Check for config generation flag first
	genConfig, _ := cmd.Flags().GetBool("gen-config")
	if genConfig {
		return generateStandaloneConfig()
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

	// Get flag values
	serverBind, _ := cmd.Flags().GetString("bind")
	agentBind, _ := cmd.Flags().GetString("agent-bind")
	dbPath, _ := cmd.Flags().GetString("db-path")
	autoTLS, _ := cmd.Flags().GetBool("auto-tls")
	checkInterval, _ := cmd.Flags().GetDuration("check-interval")
	minScanIntervalStr, _ := cmd.Flags().GetString("min-scan-interval")
	maxScanIntervalStr, _ := cmd.Flags().GetString("max-scan-interval")

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
	serverURL := "http://localhost:8080"
	if autoTLS {
		serverURL = "https://localhost:8080"
	}

	// Create combined configuration
	cfg := &config.Config{
		Log: config.LogConfig{
			Level:  viper.GetString("log.level"),
			Format: viper.GetString("log.format"),
		},
		Server: config.ServerConfig{
			Bind:    serverBind,
			AutoTLS: autoTLS,
			Database: config.DatabaseConfig{
				Type:       "sqlite",
				SQLitePath: dbPath,
			},
			AdminAPIKey:     adminAPIKey,
			MinScanInterval: minScanInterval,
			MaxScanInterval: maxScanInterval,
			DevMode:         false,
		},
		Agent: config.AgentConfig{
			ServerURL:     serverURL,
			APIKey:        adminAPIKey, // Use same key for simplicity
			AgentID:       hostname + "-local",
			CheckInterval: checkInterval,
			Bind:          agentBind,
			InsecureTLS:   autoTLS, // If using auto-TLS, skip verification for localhost
		},
	}

	// Ensure database directory exists
	if err := os.MkdirAll("./db", 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	log.Info().
		Str("server_bind", serverBind).
		Str("agent_bind", agentBind).
		Str("db_path", dbPath).
		Bool("auto_tls", autoTLS).
		Dur("check_interval", checkInterval).
		Msg("Standalone configuration created")

	return cfg, nil
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
  bind: "0.0.0.0:8080"                     # Web server bind address
  auto_tls: true                           # Enable automatic TLS certificate generation
  
  # Database Configuration (SQLite for standalone mode)
  database:
    type: "sqlite"                         # Use SQLite for simplicity
    sqlite_path: "./db/sreootb.db"         # SQLite database file path
  
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

# Standalone mode automatically:
# 1. Creates SQLite database in ./db/ directory
# 2. Starts web server with monitoring dashboard
# 3. Starts local agent that connects to the server
# 4. Uses the same API key for both components
# 5. Provides complete monitoring solution in single process
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
