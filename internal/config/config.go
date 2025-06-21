package config

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/viper"
)

// Config holds the complete application configuration
type Config struct {
	Log    LogConfig    `mapstructure:"log"`
	Server ServerConfig `mapstructure:"server"`
	Agent  AgentConfig  `mapstructure:"agent"`
}

// LogConfig holds logging configuration
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// ServerConfig holds server-specific configuration
type ServerConfig struct {
	Bind            string        `mapstructure:"bind"`       // Web GUI bind address
	AgentBind       string        `mapstructure:"agent_bind"` // Agent API bind address (WebSocket/HTTP)
	TLSCert         string        `mapstructure:"tls_cert"`
	TLSKey          string        `mapstructure:"tls_key"`
	AgentTLSCert    string        `mapstructure:"agent_tls_cert"` // Separate TLS cert for agent port
	AgentTLSKey     string        `mapstructure:"agent_tls_key"`  // Separate TLS key for agent port
	DBPath          string        `mapstructure:"db_path"`
	AutoTLS         bool          `mapstructure:"auto_tls"`
	MinScanInterval time.Duration `mapstructure:"min_scan_interval"`
	MaxScanInterval time.Duration `mapstructure:"max_scan_interval"`
	DevMode         bool          `mapstructure:"dev_mode"`
}

// AgentConfig holds agent-specific configuration
type AgentConfig struct {
	ServerURL     string        `mapstructure:"server_url"`
	APIKey        string        `mapstructure:"api_key"`
	AgentID       string        `mapstructure:"agent_id"`
	CheckInterval time.Duration `mapstructure:"check_interval"`
	Bind          string        `mapstructure:"bind"`
	UserAgent     string        `mapstructure:"user_agent"`
}

// Load loads configuration from various sources
func Load() (*Config, error) {
	// Set defaults
	setDefaults()

	// Create config struct
	var cfg Config

	// Unmarshal configuration
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// setDefaults sets default configuration values
func setDefaults() {
	// Log defaults
	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.format", "console")

	// Server defaults
	viper.SetDefault("server.bind", "0.0.0.0:8080")
	viper.SetDefault("server.agent_bind", "0.0.0.0:8081") // Agent API on different port
	viper.SetDefault("server.db_path", "./sreootb.db")
	viper.SetDefault("server.auto_tls", true)
	viper.SetDefault("server.min_scan_interval", 10*time.Second)
	viper.SetDefault("server.max_scan_interval", 24*time.Hour)
	viper.SetDefault("server.dev_mode", false)

	// Agent defaults
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	viper.SetDefault("agent.agent_id", hostname)
	viper.SetDefault("agent.check_interval", 30*time.Second)
	viper.SetDefault("agent.bind", "127.0.0.1:8082")
	viper.SetDefault("agent.user_agent", "SREootb-Agent/2.0")
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Server validation
	if c.Server.Bind == "" {
		return fmt.Errorf("server bind address is required")
	}

	if c.Server.DBPath == "" {
		return fmt.Errorf("database path is required")
	}

	// Agent validation
	if c.Agent.ServerURL != "" {
		if c.Agent.APIKey == "" {
			return fmt.Errorf("agent API key is required when server URL is specified")
		}
		if len(c.Agent.APIKey) < 32 {
			return fmt.Errorf("agent API key must be at least 32 characters long")
		}
		if c.Agent.AgentID == "" {
			return fmt.Errorf("agent ID is required")
		}
	}

	return nil
}
