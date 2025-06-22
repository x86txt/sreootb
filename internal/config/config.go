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
	Bind            string         `mapstructure:"bind"`       // Web GUI bind address
	AgentBind       string         `mapstructure:"agent_bind"` // Agent API bind address (WebSocket/HTTP)
	TLSCert         string         `mapstructure:"tls_cert"`
	TLSKey          string         `mapstructure:"tls_key"`
	AgentTLSCert    string         `mapstructure:"agent_tls_cert"` // Separate TLS cert for agent port
	AgentTLSKey     string         `mapstructure:"agent_tls_key"`  // Separate TLS key for agent port
	Database        DatabaseConfig `mapstructure:"database"`
	AutoTLS         bool           `mapstructure:"auto_tls"`
	AdminAPIKey     string         `mapstructure:"admin_api_key"` // Admin API key for web GUI authentication
	AgentAPIKey     string         `mapstructure:"agent_api_key"` // Agent API key for agent authentication
	MinScanInterval time.Duration  `mapstructure:"min_scan_interval"`
	MaxScanInterval time.Duration  `mapstructure:"max_scan_interval"`
	DevMode         bool           `mapstructure:"dev_mode"`
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Type            string        `mapstructure:"type"`               // "sqlite" or "cockroachdb"
	SQLitePath      string        `mapstructure:"sqlite_path"`        // SQLite database file path
	Host            string        `mapstructure:"host"`               // CockroachDB host
	Port            int           `mapstructure:"port"`               // CockroachDB port
	Database        string        `mapstructure:"database"`           // CockroachDB database name
	User            string        `mapstructure:"user"`               // CockroachDB user
	Password        string        `mapstructure:"password"`           // CockroachDB password
	SSLMode         string        `mapstructure:"ssl_mode"`           // CockroachDB SSL mode
	SSLRootCert     string        `mapstructure:"ssl_root_cert"`      // CockroachDB SSL root certificate
	SSLCert         string        `mapstructure:"ssl_cert"`           // CockroachDB SSL client certificate
	SSLKey          string        `mapstructure:"ssl_key"`            // CockroachDB SSL client key
	MaxOpenConns    int           `mapstructure:"max_open_conns"`     // Maximum open connections
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`     // Maximum idle connections
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`  // Connection maximum lifetime
	ConnMaxIdleTime time.Duration `mapstructure:"conn_max_idle_time"` // Connection maximum idle time
}

// AgentConfig holds agent-specific configuration
type AgentConfig struct {
	ServerURL     string        `mapstructure:"server_url"`
	APIKey        string        `mapstructure:"api_key"`
	AgentID       string        `mapstructure:"agent_id"`
	CheckInterval time.Duration `mapstructure:"check_interval"`
	Bind          string        `mapstructure:"bind"`
	UserAgent     string        `mapstructure:"user_agent"`
	InsecureTLS   bool          `mapstructure:"insecure_tls"` // Skip TLS certificate verification
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
	viper.SetDefault("server.auto_tls", true)
	viper.SetDefault("server.admin_api_key", "sreootb-admin-2024-default-change-me-in-production") // Default admin API key
	viper.SetDefault("server.min_scan_interval", 10*time.Second)
	viper.SetDefault("server.max_scan_interval", 24*time.Hour)
	viper.SetDefault("server.dev_mode", false)

	// Database defaults
	viper.SetDefault("server.database.type", "sqlite")
	viper.SetDefault("server.database.sqlite_path", "./db/sreootb.db")
	viper.SetDefault("server.database.port", 26257) // CockroachDB default port
	viper.SetDefault("server.database.ssl_mode", "require")
	viper.SetDefault("server.database.max_open_conns", 25)
	viper.SetDefault("server.database.max_idle_conns", 5)
	viper.SetDefault("server.database.conn_max_lifetime", 300*time.Second)
	viper.SetDefault("server.database.conn_max_idle_time", 60*time.Second)

	// Agent defaults
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	viper.SetDefault("agent.agent_id", hostname)
	viper.SetDefault("agent.check_interval", 30*time.Second)
	viper.SetDefault("agent.bind", "127.0.0.1:8082")
	viper.SetDefault("agent.user_agent", "SREootb-Agent/2.0")
	viper.SetDefault("agent.insecure_tls", false)
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Server validation
	if c.Server.Bind == "" {
		return fmt.Errorf("server bind address is required")
	}

	// Database validation
	if err := c.validateDatabase(); err != nil {
		return fmt.Errorf("database configuration invalid: %w", err)
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

// validateDatabase validates database configuration
func (c *Config) validateDatabase() error {
	switch c.Server.Database.Type {
	case "sqlite":
		if c.Server.Database.SQLitePath == "" {
			return fmt.Errorf("sqlite_path is required when using SQLite database")
		}
	case "cockroachdb":
		if c.Server.Database.Host == "" {
			return fmt.Errorf("host is required when using CockroachDB")
		}
		if c.Server.Database.Database == "" {
			return fmt.Errorf("database name is required when using CockroachDB")
		}
		if c.Server.Database.User == "" {
			return fmt.Errorf("user is required when using CockroachDB")
		}
		if c.Server.Database.Port <= 0 || c.Server.Database.Port > 65535 {
			return fmt.Errorf("port must be between 1 and 65535")
		}
	default:
		return fmt.Errorf("database type must be either 'sqlite' or 'cockroachdb', got '%s'", c.Server.Database.Type)
	}

	return nil
}
