package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/x86txt/sreootb/internal/config"
)

// Agent represents the monitoring agent instance
type Agent struct {
	config     *config.Config
	httpClient *http.Client
}

// New creates a new agent instance
func New(cfg *config.Config) (*Agent, error) {
	if cfg.Agent.ServerURL == "" {
		return nil, fmt.Errorf("server URL is required for agent mode")
	}

	if cfg.Agent.APIKey == "" {
		return nil, fmt.Errorf("API key is required for agent mode")
	}

	// Create HTTP client with timeouts
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	return &Agent{
		config:     cfg,
		httpClient: client,
	}, nil
}

// Start starts the agent
func (a *Agent) Start(ctx context.Context) error {
	log.Info().
		Str("server_url", a.config.Agent.ServerURL).
		Str("agent_id", a.config.Agent.AgentID).
		Msg("Starting SREootb agent")

	// Test connection to server
	if err := a.testConnection(); err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}

	log.Info().Msg("Successfully connected to server")

	// Start health endpoint
	go a.startHealthServer()

	// Main agent loop
	ticker := time.NewTicker(a.config.Agent.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Agent context cancelled, shutting down")
			return nil
		case <-ticker.C:
			if err := a.checkIn(); err != nil {
				log.Error().Err(err).Msg("Failed to check in with server")
			}
		}
	}
}

// testConnection tests the connection to the server
func (a *Agent) testConnection() error {
	req, err := http.NewRequest("GET", a.config.Agent.ServerURL+"/api/health", nil)
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", a.config.Agent.UserAgent)
	req.Header.Set("X-Agent-ID", a.config.Agent.AgentID)
	req.Header.Set("Authorization", "Bearer "+a.config.Agent.APIKey)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	return nil
}

// checkIn performs a regular check-in with the server
func (a *Agent) checkIn() error {
	checkinData := map[string]interface{}{
		"agent_id":  a.config.Agent.AgentID,
		"timestamp": time.Now().Unix(),
		"status":    "online",
	}

	jsonData, err := json.Marshal(checkinData)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", a.config.Agent.ServerURL+"/api/agents/checkin",
		bytes.NewReader(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", a.config.Agent.UserAgent)
	req.Header.Set("X-Agent-ID", a.config.Agent.AgentID)
	req.Header.Set("Authorization", "Bearer "+a.config.Agent.APIKey)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	log.Debug().Msg("Successfully checked in with server")
	return nil
}

// startHealthServer starts a simple health check server
func (a *Agent) startHealthServer() {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"status":    "healthy",
			"agent_id":  a.config.Agent.AgentID,
			"timestamp": time.Now().Unix(),
			"version":   "2.0",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	server := &http.Server{
		Addr:    a.config.Agent.Bind,
		Handler: mux,
	}

	log.Info().Str("addr", a.config.Agent.Bind).Msg("Starting agent health server")

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error().Err(err).Msg("Agent health server error")
	}
}
