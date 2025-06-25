package models

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// User represents a user account
type User struct {
	ID               int        `json:"id" db:"id"`
	Email            string     `json:"email" db:"email"`
	PasswordHash     string     `json:"-" db:"password_hash"`
	FirstName        string     `json:"first_name" db:"first_name"`
	LastName         string     `json:"last_name" db:"last_name"`
	Role             string     `json:"role" db:"role"` // "admin", "user"
	EmailVerified    bool       `json:"email_verified" db:"email_verified"`
	TwoFactorEnabled bool       `json:"two_factor_enabled" db:"two_factor_enabled"`
	TwoFactorSecret  *string    `json:"-" db:"two_factor_secret"` // TOTP secret, encrypted
	LastLoginAt      *time.Time `json:"last_login_at" db:"last_login_at"`
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at" db:"updated_at"`
}

// UserSession represents an active user session
type UserSession struct {
	ID        string    `json:"id" db:"id"` // UUID
	UserID    int       `json:"user_id" db:"user_id"`
	Token     string    `json:"-" db:"token_hash"` // Hashed session token
	ExpiresAt time.Time `json:"expires_at" db:"expires_at"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UserAgent *string   `json:"user_agent" db:"user_agent"`
	IPAddress *string   `json:"ip_address" db:"ip_address"`
}

// EmailVerification represents an email verification token
type EmailVerification struct {
	ID        int       `json:"id" db:"id"`
	UserID    int       `json:"user_id" db:"user_id"`
	Token     string    `json:"-" db:"token"` // Verification token
	ExpiresAt time.Time `json:"expires_at" db:"expires_at"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	Used      bool      `json:"used" db:"used"`
}

// TwoFactorAuth represents 2FA backup codes
type TwoFactorAuth struct {
	ID        int        `json:"id" db:"id"`
	UserID    int        `json:"user_id" db:"user_id"`
	Code      string     `json:"-" db:"code_hash"` // Hashed backup code
	Used      bool       `json:"used" db:"used"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
	UsedAt    *time.Time `json:"used_at" db:"used_at"`
}

// UserRegistrationRequest represents a user registration request
type UserRegistrationRequest struct {
	Email     string `json:"email" validate:"required,email"`
	Password  string `json:"password" validate:"required,min=8"`
	FirstName string `json:"first_name" validate:"required,min=1"`
	LastName  string `json:"last_name" validate:"required,min=1"`
}

// UserLoginRequest represents a user login request
type UserLoginRequest struct {
	Email      string  `json:"email" validate:"required,email"`
	Password   string  `json:"password" validate:"required"`
	TOTPCode   *string `json:"totp_code"`   // Optional TOTP code for 2FA
	RememberMe bool    `json:"remember_me"` // Extend session duration
}

// MasterKeyLoginRequest represents a master API key login request
type MasterKeyLoginRequest struct {
	APIKey string `json:"api_key" validate:"required"`
}

// UserLoginResponse represents a successful login response
type UserLoginResponse struct {
	Success      bool   `json:"success"`
	SessionToken string `json:"session_token"`
	User         *User  `json:"user"`
	Message      string `json:"message"`
	RequiresTOTP bool   `json:"requires_totp,omitempty"` // If 2FA is enabled but code not provided
}

// EmailVerificationRequest represents an email verification request
type EmailVerificationRequest struct {
	Token string `json:"token" validate:"required"`
}

// TwoFactorSetupRequest represents a request to set up 2FA
type TwoFactorSetupRequest struct {
	TOTPCode string `json:"totp_code" validate:"required"`
}

// TwoFactorSetupResponse represents the response for 2FA setup
type TwoFactorSetupResponse struct {
	Success     bool     `json:"success"`
	Secret      string   `json:"secret"`
	QRCodeURL   string   `json:"qr_code_url"`
	BackupCodes []string `json:"backup_codes"`
	Message     string   `json:"message"`
}

// TwoFactorDisableRequest represents a request to disable 2FA
type TwoFactorDisableRequest struct {
	Password string `json:"password" validate:"required"`
	TOTPCode string `json:"totp_code" validate:"required"`
}

// PasswordChangeRequest represents a password change request
type PasswordChangeRequest struct {
	CurrentPassword string `json:"current_password" validate:"required"`
	NewPassword     string `json:"new_password" validate:"required,min=8"`
}

// ForgotPasswordRequest represents a forgot password request
type ForgotPasswordRequest struct {
	Email string `json:"email" validate:"required,email"`
}

// ResetPasswordRequest represents a password reset request
type ResetPasswordRequest struct {
	Token       string `json:"token" validate:"required"`
	NewPassword string `json:"new_password" validate:"required,min=8"`
}

// Validate validates a UserRegistrationRequest
func (u *UserRegistrationRequest) Validate() error {
	if u.Email == "" {
		return fmt.Errorf("email is required")
	}

	// Validate email format
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(u.Email) {
		return fmt.Errorf("invalid email format")
	}

	if u.Password == "" {
		return fmt.Errorf("password is required")
	}

	if len(u.Password) < 8 {
		return fmt.Errorf("password must be at least 8 characters long")
	}

	// Check password complexity
	if !isStrongPassword(u.Password) {
		return fmt.Errorf("password must contain at least one uppercase letter, one lowercase letter, one number, and one special character")
	}

	if u.FirstName == "" {
		return fmt.Errorf("first name is required")
	}

	if u.LastName == "" {
		return fmt.Errorf("last name is required")
	}

	return nil
}

// Validate validates a UserLoginRequest
func (u *UserLoginRequest) Validate() error {
	if u.Email == "" {
		return fmt.Errorf("email is required")
	}

	if u.Password == "" {
		return fmt.Errorf("password is required")
	}

	return nil
}

// isStrongPassword checks if a password meets complexity requirements
func isStrongPassword(password string) bool {
	// At least 8 characters
	if len(password) < 8 {
		return false
	}

	// Check for uppercase, lowercase, number, and special character
	hasUpper := regexp.MustCompile(`[A-Z]`).MatchString(password)
	hasLower := regexp.MustCompile(`[a-z]`).MatchString(password)
	hasNumber := regexp.MustCompile(`[0-9]`).MatchString(password)
	hasSpecial := regexp.MustCompile(`[!@#$%^&*()_+\-=\[\]{};':"\\|,.<>\/?]`).MatchString(password)

	return hasUpper && hasLower && hasNumber && hasSpecial
}

// Site represents a website to monitor
type Site struct {
	ID           int       `json:"id" db:"id"`
	URL          string    `json:"url" db:"url"`
	Name         string    `json:"name" db:"name"`
	ScanInterval string    `json:"scan_interval" db:"scan_interval"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// SiteCheck represents a monitoring check result
type SiteCheck struct {
	ID           int       `json:"id" db:"id"`
	SiteID       int       `json:"site_id" db:"site_id"`
	Status       string    `json:"status" db:"status"`
	ResponseTime *float64  `json:"response_time" db:"response_time"`
	StatusCode   *int      `json:"status_code" db:"status_code"`
	ErrorMessage *string   `json:"error_message" db:"error_message"`
	CheckedAt    time.Time `json:"checked_at" db:"checked_at"`
}

// SiteStatus represents a site with its latest check information
type SiteStatus struct {
	Site
	Status       *string    `json:"status"`
	ResponseTime *float64   `json:"response_time"`
	StatusCode   *int       `json:"status_code"`
	ErrorMessage *string    `json:"error_message"`
	CheckedAt    *time.Time `json:"checked_at"`
	TotalUp      int        `json:"total_up"`
	TotalDown    int        `json:"total_down"`
}

// Agent represents a monitoring agent
type Agent struct {
	ID             int        `json:"id" db:"id"`
	Name           string     `json:"name" db:"name"`
	APIKeyHash     string     `json:"-" db:"api_key_hash"`
	Description    *string    `json:"description" db:"description"`
	LastSeen       *time.Time `json:"last_seen" db:"last_seen"`
	Status         string     `json:"status" db:"status"`
	OS             *string    `json:"os" db:"os"`                     // Operating system (linux, windows, darwin, etc.)
	Platform       *string    `json:"platform" db:"platform"`         // Human-readable platform name
	Architecture   *string    `json:"architecture" db:"architecture"` // System architecture (amd64, arm64, etc.)
	Version        *string    `json:"version" db:"version"`           // Agent version
	RemoteIP       *string    `json:"remote_ip" db:"remote_ip"`       // Remote IP address of the agent
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	Connected      bool       `json:"connected" db:"-"`        // Real-time connection status
	UsingServerKey bool       `json:"using_server_key" db:"-"` // Whether using server's shared API key
}

// GetTruncatedAPIKeyHash returns a truncated version of the API key hash for display
func (a *Agent) GetTruncatedAPIKeyHash() string {
	if len(a.APIKeyHash) < 16 {
		return a.APIKeyHash
	}
	return a.APIKeyHash[:8] + "..." + a.APIKeyHash[len(a.APIKeyHash)-8:]
}

// MarshalJSON customizes JSON marshaling to include safe API key hash
func (a *Agent) MarshalJSON() ([]byte, error) {
	type Alias Agent
	return json.Marshal(&struct {
		*Alias
		APIKeyHashDisplay string `json:"api_key_hash"`
	}{
		Alias:             (*Alias)(a),
		APIKeyHashDisplay: a.GetTruncatedAPIKeyHash(),
	})
}

// SiteCreateRequest represents a request to create a new site
type SiteCreateRequest struct {
	URL          string `json:"url" validate:"required"`
	Name         string `json:"name" validate:"required,min=1"`
	ScanInterval string `json:"scan_interval" validate:"required"`
}

// AgentCreateRequest represents a request to create a new agent
type AgentCreateRequest struct {
	Name             string  `json:"name" validate:"required,min=1"`
	APIKey           string  `json:"api_key" validate:"required,min=64"`
	Description      *string `json:"description"`
	RegistrationType *string `json:"registration_type"` // "manual" or "auto"
}

// ManualCheckRequest represents a request for manual site checks
type ManualCheckRequest struct {
	SiteIDs []int `json:"site_ids"`
}

// MonitorStats represents monitoring statistics
type MonitorStats struct {
	TotalSites          int      `json:"total_sites"`
	SitesUp             int      `json:"sites_up"`
	SitesDown           int      `json:"sites_down"`
	AverageResponseTime *float64 `json:"average_response_time"`
	ConnectedAgents     int      `json:"connected_agents"`
}

// MonitorTask represents a monitoring task assigned to agents
type MonitorTask struct {
	ID          int       `json:"id" db:"id"`
	SiteID      int       `json:"site_id" db:"site_id"`
	MonitorType string    `json:"monitor_type" db:"monitor_type"` // "http", "ping", "tcp", "log", etc.
	URL         string    `json:"url" db:"url"`                   // URL or host to monitor (for log type: file path)
	Interval    string    `json:"interval" db:"interval"`         // e.g., "60s", "2m", "5m"
	Timeout     string    `json:"timeout" db:"timeout"`           // e.g., "10s", "30s"
	Enabled     bool      `json:"enabled" db:"enabled"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
	// Configuration for log monitoring (stored as JSON in metadata)
	LogConfig *LogMonitorConfig `json:"log_config,omitempty" db:"-"`
}

// LogMonitorConfig represents configuration for log file monitoring
type LogMonitorConfig struct {
	FilePath   string `json:"file_path"`             // Path to log file
	Format     string `json:"format"`                // "nginx", "apache", "combined", "json", "custom"
	Pattern    string `json:"pattern,omitempty"`     // Custom regex pattern for parsing
	ErrorCodes []int  `json:"error_codes,omitempty"` // HTTP status codes to consider as errors (default: 400-599)
	TailLines  int    `json:"tail_lines"`            // Number of lines to tail from end (default: 1000)
	Encoding   string `json:"encoding"`              // File encoding (default: "utf-8")
}

// LogEntry represents a parsed log entry
type LogEntry struct {
	Timestamp    time.Time `json:"timestamp"`
	Method       string    `json:"method,omitempty"`
	URL          string    `json:"url,omitempty"`
	StatusCode   int       `json:"status_code"`
	ResponseTime float64   `json:"response_time,omitempty"` // in milliseconds
	RemoteAddr   string    `json:"remote_addr,omitempty"`
	UserAgent    string    `json:"user_agent,omitempty"`
	Referrer     string    `json:"referrer,omitempty"`
	BytesSent    int64     `json:"bytes_sent,omitempty"`
	RawLine      string    `json:"raw_line"`
}

// MonitorResult represents the result of a monitoring check performed by an agent
type MonitorResult struct {
	ID           int       `json:"id" db:"id"`
	TaskID       int       `json:"task_id" db:"task_id"`
	AgentID      int       `json:"agent_id" db:"agent_id"`
	Status       string    `json:"status" db:"status"`               // "up", "down", "timeout", "error"
	ResponseTime *float64  `json:"response_time" db:"response_time"` // in milliseconds
	StatusCode   *int      `json:"status_code" db:"status_code"`     // HTTP status code (if applicable)
	ErrorMessage *string   `json:"error_message" db:"error_message"`
	Metadata     *string   `json:"metadata" db:"metadata"` // JSON metadata (headers, cert info, etc.)
	CheckedAt    time.Time `json:"checked_at" db:"checked_at"`
}

// AgentTaskAssignment represents which tasks are assigned to which agents
type AgentTaskAssignment struct {
	ID        int       `json:"id" db:"id"`
	AgentID   int       `json:"agent_id" db:"agent_id"`
	TaskID    int       `json:"task_id" db:"task_id"`
	Assigned  bool      `json:"assigned" db:"assigned"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// AgentTasksResponse represents the response sent to agents with their assigned tasks
type AgentTasksResponse struct {
	Tasks   []MonitorTask `json:"tasks"`
	AgentID int           `json:"agent_id"`
}

// MonitorResultRequest represents a monitoring result submitted by an agent
type MonitorResultRequest struct {
	TaskID       int                    `json:"task_id" validate:"required"`
	Status       string                 `json:"status" validate:"required"`
	ResponseTime *float64               `json:"response_time"`
	StatusCode   *int                   `json:"status_code"`
	ErrorMessage *string                `json:"error_message"`
	Metadata     map[string]interface{} `json:"metadata"`
	CheckedAt    time.Time              `json:"checked_at"`
}

// AgentKeyUpgradeRequest represents a request to upgrade an agent's key
type AgentKeyUpgradeRequest struct {
	AgentID     string `json:"agent_id" validate:"required"`
	CurrentKey  string `json:"current_key" validate:"required"`
	RequestedBy string `json:"requested_by"` // "agent" or "manual"
}

// AgentKeyUpgradeResponse represents the response to a key upgrade request
type AgentKeyUpgradeResponse struct {
	Success       bool   `json:"success"`
	NewAPIKey     string `json:"new_api_key,omitempty"`
	Message       string `json:"message"`
	RestartNeeded bool   `json:"restart_needed"`
}

// Validate validates a SiteCreateRequest
func (s *SiteCreateRequest) Validate() error {
	if s.URL == "" {
		return fmt.Errorf("URL is required")
	}

	if s.Name == "" {
		return fmt.Errorf("name is required")
	}

	// Validate URL format
	if err := validateURL(s.URL); err != nil {
		return err
	}

	// Validate scan interval
	if err := validateScanInterval(s.ScanInterval); err != nil {
		return err
	}

	return nil
}

// Validate validates an AgentCreateRequest
func (a *AgentCreateRequest) Validate() error {
	if a.Name == "" {
		return fmt.Errorf("name is required")
	}

	if a.APIKey == "" {
		return fmt.Errorf("API key is required")
	}

	if len(a.APIKey) < 64 {
		return fmt.Errorf("API key must be at least 64 characters long")
	}

	return nil
}

// Validate validates a MonitorResultRequest
func (m *MonitorResultRequest) Validate() error {
	if m.TaskID <= 0 {
		return fmt.Errorf("task_id is required and must be positive")
	}

	if m.Status == "" {
		return fmt.Errorf("status is required")
	}

	// Validate status values
	validStatuses := map[string]bool{
		"up":       true,
		"down":     true,
		"degraded": true,
		"timeout":  true,
		"error":    true,
	}

	if !validStatuses[m.Status] {
		return fmt.Errorf("status must be one of: up, down, degraded, timeout, error")
	}

	// Response time should be positive if provided
	if m.ResponseTime != nil && *m.ResponseTime < 0 {
		return fmt.Errorf("response_time must be positive")
	}

	// Status code should be valid HTTP status if provided
	if m.StatusCode != nil && (*m.StatusCode < 100 || *m.StatusCode > 599) {
		return fmt.Errorf("status_code must be a valid HTTP status code (100-599)")
	}

	return nil
}

// validateURL validates URL format for supported protocols
func validateURL(urlStr string) error {
	if urlStr == "" {
		return fmt.Errorf("URL is required")
	}

	// Check for supported protocols
	if strings.HasPrefix(urlStr, "http://") || strings.HasPrefix(urlStr, "https://") {
		// Validate as HTTP URL
		if _, err := url.Parse(urlStr); err != nil {
			return fmt.Errorf("invalid HTTP/HTTPS URL format: %w", err)
		}
		return nil
	}

	if strings.HasPrefix(urlStr, "ping://") {
		// Validate ping URL (domain or IP after ping://)
		host := urlStr[7:] // Remove 'ping://'
		if host == "" {
			return fmt.Errorf("ping URL requires a hostname or IP address")
		}

		// Basic validation for hostname/IP
		if !regexp.MustCompile(`^[a-zA-Z0-9.-]+$`).MatchString(host) {
			return fmt.Errorf("invalid hostname or IP address for ping")
		}
		return nil
	}

	if strings.HasPrefix(urlStr, "log://") {
		// Validate log file path
		filePath := urlStr[6:] // Remove 'log://' prefix
		if filePath == "" {
			return fmt.Errorf("log URL requires a file path")
		}

		// Basic validation for file path (must be absolute or relative)
		if filePath == "." || filePath == ".." {
			return fmt.Errorf("invalid log file path")
		}

		return nil
	}

	return fmt.Errorf("URL must start with http://, https://, ping://, or log://")
}

// validateScanInterval validates scan interval format and range
func validateScanInterval(interval string) error {
	// Parse the interval string
	re := regexp.MustCompile(`^(\d+(?:\.\d+)?)([smh])$`)
	matches := re.FindStringSubmatch(strings.ToLower(strings.TrimSpace(interval)))

	if len(matches) != 3 {
		return fmt.Errorf("invalid format. Use 's', 'm', 'h' (e.g., '30s', '5m', '1h')")
	}

	// This would be configurable, but for now use reasonable defaults
	const minSeconds = 10
	const maxSeconds = 86400 // 24 hours

	valueStr, unit := matches[1], matches[2]

	// Parse value
	var value float64
	if _, err := fmt.Sscanf(valueStr, "%f", &value); err != nil {
		return fmt.Errorf("invalid interval value: %s", valueStr)
	}

	// Convert to seconds
	var seconds float64
	switch unit {
	case "s":
		seconds = value
	case "m":
		seconds = value * 60
	case "h":
		seconds = value * 3600
	}

	// Check against limits
	if seconds < minSeconds || seconds > maxSeconds {
		return fmt.Errorf("scan interval must be between %ds and %ds", minSeconds, maxSeconds)
	}

	return nil
}
