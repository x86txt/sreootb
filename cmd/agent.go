package cmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/x86txt/sreootb/internal/agent"
	"github.com/x86txt/sreootb/internal/config"
)

// agentCmd represents the agent command
var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Run SREootb in agent mode",
	Long: `Start the SREootb agent to monitor sites and report back to the server.
Agents can be deployed on different networks to monitor sites from various locations.`,
	RunE: runAgent,
}

func init() {
	rootCmd.AddCommand(agentCmd)

	// Agent-specific flags
	agentCmd.Flags().String("server-url", "", "URL of the SREootb server")
	agentCmd.Flags().String("api-key", "", "API key for agent authentication")
	agentCmd.Flags().String("agent-id", "", "unique identifier for this agent")
	agentCmd.Flags().Duration("check-interval", 0, "interval between server checks")
	agentCmd.Flags().String("bind", "127.0.0.1:8081", "address to bind the agent health endpoint")

	// Config generation flags
	agentCmd.Flags().Bool("gen-config", false, "generate sample agent configuration file")
	agentCmd.Flags().Bool("gen-systemd", false, "generate systemd service file for agent")

	// Bind flags to viper
	viper.BindPFlag("agent.server_url", agentCmd.Flags().Lookup("server-url"))
	viper.BindPFlag("agent.api_key", agentCmd.Flags().Lookup("api-key"))
	viper.BindPFlag("agent.agent_id", agentCmd.Flags().Lookup("agent-id"))
	viper.BindPFlag("agent.check_interval", agentCmd.Flags().Lookup("check-interval"))
	viper.BindPFlag("agent.bind", agentCmd.Flags().Lookup("bind"))
}

func runAgent(cmd *cobra.Command, args []string) error {
	// Check for config generation flags first
	genConfig, _ := cmd.Flags().GetBool("gen-config")
	genSystemd, _ := cmd.Flags().GetBool("gen-systemd")

	if genConfig {
		return generateAgentConfig()
	}

	if genSystemd {
		return generateAgentSystemd()
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	// Create agent instance
	agentInstance, err := agent.New(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create agent")
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

	// Start agent
	log.Info().Str("server_url", cfg.Agent.ServerURL).Msg("Starting SREootb agent")
	return agentInstance.Start(ctx)
}

func generateAgentConfig() error {
	// Generate a random API key
	apiKeyBytes := make([]byte, 32)
	if _, err := rand.Read(apiKeyBytes); err != nil {
		return fmt.Errorf("failed to generate API key: %w", err)
	}
	apiKey := hex.EncodeToString(apiKeyBytes)

	// Get hostname for agent ID
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "agent-unknown"
	}

	configContent := fmt.Sprintf(`# SRE: Out of the Box (SREootb) Agent Configuration

# Logging configuration
log:
  level: "info"          # trace, debug, info, warn, error
  format: "console"      # console, json

# Agent configuration
agent:
  server_url: "https://your-server.com:8081"   # SREootb server agent API URL
  api_key: "%s"          # Agent API key (generated - register this with the server)
  agent_id: "%s"                               # Unique agent identifier
  check_interval: "30s"                        # How often to check in with server
  bind: "127.0.0.1:8082"                       # Health endpoint bind address
  user_agent: "SREootb-Agent/2.0"               # User agent for HTTP requests

# Server configuration is not needed for agent mode
# Use 'sreootb server --gen-config' to generate server configuration
`, apiKey, hostname)

	filename := "sreootb-agent.yaml"
	if err := os.WriteFile(filename, []byte(configContent), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("✅ Agent configuration file generated: %s\n", filename)
	fmt.Println("🔑 Important: Register this agent on your SREootb server using:")
	fmt.Printf("   API Key: %s\n", apiKey)
	fmt.Printf("   Agent ID: %s\n", hostname)
	fmt.Println("📝 Edit the server_url in the configuration file and then start the agent with:")
	fmt.Printf("   sreootb agent --config %s\n", filename)
	return nil
}

func generateAgentSystemd() error {
	execPath, err := os.Executable()
	if err != nil {
		execPath = "/usr/local/bin/sreootb"
	} else {
		execPath, _ = filepath.Abs(execPath)
	}

	serviceContent := fmt.Sprintf(`[Unit]
Description=SRE Out of the Box Agent
Documentation=https://github.com/x86txt/sreootb
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=sreootb
Group=sreootb
ExecStart=%s agent --config /etc/sreootb/sreootb-agent.yaml
ExecReload=/bin/kill -HUP $MAINPID
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=sreootb-agent

# Security settings
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/sreootb /var/log/sreootb

# Resource limits
LimitNOFILE=65536
MemoryLimit=256M

# Working directory
WorkingDirectory=/var/lib/sreootb

# Environment
Environment=SREOOB_MODE=agent

[Install]
WantedBy=multi-user.target
`, execPath)

	filename := "sreootb-agent.service"
	if err := os.WriteFile(filename, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("failed to write systemd service file: %w", err)
	}

	fmt.Printf("✅ Systemd service file generated: %s\n", filename)
	fmt.Println("📝 To install and start the service:")
	fmt.Println("   sudo cp sreootb-agent.service /etc/systemd/system/")
	fmt.Println("   sudo mkdir -p /etc/sreootb /var/lib/sreootb /var/log/sreootb")
	fmt.Println("   sudo cp sreootb-agent.yaml /etc/sreootb/")
	fmt.Println("   sudo useradd -r -s /bin/false sreootb")
	fmt.Println("   sudo chown -R sreootb:sreootb /var/lib/sreootb /var/log/sreootb")
	fmt.Println("   sudo systemctl daemon-reload")
	fmt.Println("   sudo systemctl enable --now sreootb-agent")
	return nil
}
