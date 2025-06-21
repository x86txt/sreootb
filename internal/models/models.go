package models

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

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
	ID          int        `json:"id" db:"id"`
	Name        string     `json:"name" db:"name"`
	APIKeyHash  string     `json:"-" db:"api_key_hash"`
	Description *string    `json:"description" db:"description"`
	LastSeen    *time.Time `json:"last_seen" db:"last_seen"`
	Status      string     `json:"status" db:"status"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	Connected   bool       `json:"connected" db:"-"` // Real-time connection status
}

// SiteCreateRequest represents a request to create a new site
type SiteCreateRequest struct {
	URL          string `json:"url" validate:"required"`
	Name         string `json:"name" validate:"required,min=1"`
	ScanInterval string `json:"scan_interval" validate:"required"`
}

// AgentCreateRequest represents a request to create a new agent
type AgentCreateRequest struct {
	Name        string  `json:"name" validate:"required,min=1"`
	APIKey      string  `json:"api_key" validate:"required,min=64"`
	Description *string `json:"description"`
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

	return fmt.Errorf("URL must start with http://, https://, or ping://")
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
