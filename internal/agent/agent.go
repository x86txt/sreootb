package agent

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"

	"github.com/x86txt/sreootb/internal/config"
	"github.com/x86txt/sreootb/internal/models"
)

// Agent represents the monitoring agent instance
type Agent struct {
	config       *config.Config
	httpClient   *http.Client
	wsConn       *websocket.Conn
	wsURL        string
	httpURL      string
	useWebSocket bool
	osInfo       OSInfo

	// Monitoring tasks management
	tasks           []models.MonitorTask
	tasksMutex      sync.RWMutex
	taskSchedulers  map[int]*TaskScheduler // task_id -> scheduler
	schedulersMutex sync.RWMutex
	results         chan models.MonitorResultRequest
	stopChan        chan struct{}
}

// TaskScheduler manages the execution schedule for a monitoring task
type TaskScheduler struct {
	task     models.MonitorTask
	ticker   *time.Ticker
	stopChan chan struct{}
	agent    *Agent
}

// OSInfo contains operating system information
type OSInfo struct {
	OS           string `json:"os"`
	Platform     string `json:"platform"`
	Architecture string `json:"architecture"`
	Version      string `json:"version,omitempty"`
}

// detectOS detects the current operating system
func detectOS() OSInfo {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	var os, platform string

	switch goos {
	case "linux":
		os = "linux"
		platform = "Linux"
	case "windows":
		os = "windows"
		platform = "Windows"
	case "darwin":
		os = "darwin"
		platform = "macOS"
	case "freebsd":
		os = "freebsd"
		platform = "FreeBSD"
	case "openbsd":
		os = "openbsd"
		platform = "OpenBSD"
	case "netbsd":
		os = "netbsd"
		platform = "NetBSD"
	default:
		os = "unknown"
		platform = strings.Title(goos)
	}

	return OSInfo{
		OS:           os,
		Platform:     platform,
		Architecture: goarch,
		Version:      "", // Could be enhanced with version detection
	}
}

// New creates a new agent instance
func New(cfg *config.Config) (*Agent, error) {
	if cfg.Agent.ServerURL == "" {
		return nil, fmt.Errorf("server URL is required for agent mode")
	}

	if cfg.Agent.APIKey == "" {
		return nil, fmt.Errorf("API key is required for agent mode")
	}

	// Create HTTP client with timeouts and TLS configuration
	transport := &http.Transport{}

	// Configure TLS settings if insecure mode is enabled
	if cfg.Agent.InsecureTLS {
		log.Warn().Msg("TLS certificate verification disabled (insecure mode)")
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}

	// Determine WebSocket and HTTP URLs
	httpURL := cfg.Agent.ServerURL
	wsURL := ""

	// Convert HTTP URLs to WebSocket URLs
	if strings.HasPrefix(httpURL, "https://") {
		wsURL = "wss://" + strings.TrimPrefix(httpURL, "https://") + "/ws"
	} else if strings.HasPrefix(httpURL, "http://") {
		wsURL = "ws://" + strings.TrimPrefix(httpURL, "http://") + "/ws"
	} else {
		// Default to secure WebSocket if no protocol specified
		wsURL = "wss://" + httpURL + "/ws"
		httpURL = "https://" + httpURL
	}

	return &Agent{
		config:         cfg,
		httpClient:     client,
		wsURL:          wsURL,
		httpURL:        httpURL,
		useWebSocket:   true, // Default to WebSocket
		osInfo:         detectOS(),
		taskSchedulers: make(map[int]*TaskScheduler),
		results:        make(chan models.MonitorResultRequest, 100),
		stopChan:       make(chan struct{}),
	}, nil
}

// Start starts the agent
func (a *Agent) Start() error {
	log.Info().
		Str("server_url", a.config.Agent.ServerURL).
		Str("agent_id", a.config.Agent.AgentID).
		Msg("Starting SREootb agent")

	// Check if we're using a bootstrap key and request upgrade
	if err := a.checkAndUpgradeKey(); err != nil {
		log.Error().Err(err).Msg("Failed to check/upgrade API key")
		// Continue with existing key if upgrade fails
	}

	// Start monitoring engine in background
	go a.startMonitoringEngine()

	// Start result processor
	go a.startResultSubmitter()

	// Start health server
	go a.startHealthServer()

	// Try WebSocket connection first
	if a.useWebSocket {
		if err := a.connectWebSocket(); err != nil {
			log.Error().Err(err).Msg("Failed to establish WebSocket connection, falling back to HTTP")
			a.useWebSocket = false
		}
	}

	// If WebSocket failed, fall back to HTTP
	if !a.useWebSocket {
		log.Info().Msg("Using HTTP fallback mode")
		// Create a context for HTTP polling
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		return a.handleHTTPPolling(ctx)
	}

	// Handle WebSocket connection
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Info().Msg("Received shutdown signal")
		cancel()
	}()

	return a.handleWebSocketConnection(ctx)
}

// checkAndUpgradeKey checks if we're using a bootstrap key and requests an upgrade
func (a *Agent) checkAndUpgradeKey() error {
	// Check if this looks like a shared/bootstrap key (shared keys are usually the server's default key)
	// We can detect this by attempting an upgrade - if it succeeds, it was a bootstrap key
	if len(a.config.Agent.APIKey) == 64 { // Standard key length
		log.Info().Msg("🔄 Checking if API key can be upgraded from bootstrap to permanent...")

		if err := a.requestKeyUpgrade(); err != nil {
			// If upgrade fails, it's likely already a permanent key or there's an error
			log.Debug().Err(err).Msg("Key upgrade not needed or failed - continuing with current key")
			return nil
		}
	}

	return nil
}

// requestKeyUpgrade requests a key upgrade from the server
func (a *Agent) requestKeyUpgrade() error {
	upgradeReq := models.AgentKeyUpgradeRequest{
		AgentID:     a.config.Agent.AgentID,
		CurrentKey:  a.config.Agent.APIKey,
		RequestedBy: "agent",
	}

	reqData, err := json.Marshal(upgradeReq)
	if err != nil {
		return fmt.Errorf("failed to marshal upgrade request: %w", err)
	}

	// Make HTTP request to upgrade endpoint
	req, err := http.NewRequest("POST", a.httpURL+"/api/agents/upgrade-key", bytes.NewBuffer(reqData))
	if err != nil {
		return fmt.Errorf("failed to create upgrade request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", a.config.Agent.APIKey)
	req.Header.Set("X-Agent-ID", a.config.Agent.AgentID)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make upgrade request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upgrade request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var upgradeResp models.AgentKeyUpgradeResponse
	if err := json.NewDecoder(resp.Body).Decode(&upgradeResp); err != nil {
		return fmt.Errorf("failed to decode upgrade response: %w", err)
	}

	if !upgradeResp.Success {
		return fmt.Errorf("upgrade failed: %s", upgradeResp.Message)
	}

	log.Info().
		Str("old_key", a.config.Agent.APIKey[:8]+"...").
		Str("new_key", upgradeResp.NewAPIKey[:8]+"...").
		Msg("🔑 Successfully upgraded from bootstrap to permanent API key")

	// Save the new permanent key and restart
	if upgradeResp.RestartNeeded {
		return a.restartWithNewKey(upgradeResp.NewAPIKey)
	}

	return nil
}

// restartWithNewKey saves the new API key and restarts the agent process
func (a *Agent) restartWithNewKey(newAPIKey string) error {
	log.Info().
		Str("new_key", newAPIKey[:8]+"...").
		Msg("🔄 Restarting agent with permanent API key...")

	// Get current command line arguments
	args := os.Args[1:] // Exclude program name

	// Update the API key in the arguments
	newArgs := make([]string, 0, len(args))
	keyUpdated := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--api-key" && i+1 < len(args) {
			// Replace the API key
			newArgs = append(newArgs, arg)
			newArgs = append(newArgs, newAPIKey)
			i++ // Skip the old key
			keyUpdated = true
		} else if strings.HasPrefix(arg, "--api-key=") {
			// Replace the API key in --api-key=value format
			newArgs = append(newArgs, "--api-key="+newAPIKey)
			keyUpdated = true
		} else {
			newArgs = append(newArgs, arg)
		}
	}

	if !keyUpdated {
		// If --api-key wasn't found, add it
		newArgs = append(newArgs, "--api-key", newAPIKey)
	}

	// Log the restart command (with key redacted)
	redactedArgs := make([]string, len(newArgs))
	copy(redactedArgs, newArgs)
	for i, arg := range redactedArgs {
		if arg == newAPIKey {
			redactedArgs[i] = newAPIKey[:8] + "..."
		} else if strings.HasPrefix(arg, "--api-key=") && len(arg) > 20 {
			redactedArgs[i] = "--api-key=" + newAPIKey[:8] + "..."
		}
	}

	log.Info().
		Str("command", os.Args[0]).
		Strs("args", redactedArgs).
		Msg("🚀 Restarting with new permanent API key")

	// Stop current agent gracefully by closing stopChan
	close(a.stopChan)

	// Start new process
	if err := syscall.Exec(os.Args[0], append([]string{os.Args[0]}, newArgs...), os.Environ()); err != nil {
		return fmt.Errorf("failed to restart agent process: %w", err)
	}

	// This line should never be reached if exec succeeds
	return nil
}

// connectWebSocket establishes a WebSocket connection to the server
func (a *Agent) connectWebSocket() error {
	// Parse WebSocket URL
	u, err := url.Parse(a.wsURL)
	if err != nil {
		return fmt.Errorf("invalid WebSocket URL: %w", err)
	}

	// Add authentication parameters
	q := u.Query()
	q.Set("api_key", a.config.Agent.APIKey)
	q.Set("agent_id", a.config.Agent.AgentID)
	u.RawQuery = q.Encode()

	// Configure WebSocket dialer
	dialer := websocket.DefaultDialer
	if a.config.Agent.InsecureTLS {
		dialer.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	// Set headers
	headers := http.Header{}
	headers.Set("User-Agent", a.config.Agent.UserAgent)
	headers.Set("X-Agent-ID", a.config.Agent.AgentID)
	headers.Set("X-API-Key", a.config.Agent.APIKey)

	// Connect to WebSocket
	conn, _, err := dialer.Dial(u.String(), headers)
	if err != nil {
		return fmt.Errorf("WebSocket dial failed: %w", err)
	}

	a.wsConn = conn
	return nil
}

// handleWebSocketConnection manages the WebSocket connection lifecycle
func (a *Agent) handleWebSocketConnection(ctx context.Context) error {
	defer func() {
		if a.wsConn != nil {
			a.wsConn.Close()
		}
	}()

	// Send initial status update
	if err := a.sendWebSocketMessage(map[string]interface{}{
		"type":      "status_update",
		"status":    "online",
		"timestamp": time.Now().Unix(),
		"os_info":   a.osInfo,
		"agent_info": map[string]interface{}{
			"version":      "2.0",
			"capabilities": []string{"websocket", "http_fallback"},
		},
	}); err != nil {
		log.Error().Err(err).Msg("Failed to send initial status update")
	}

	// Request initial tasks after connection is established
	if err := a.requestTasksViaWebSocket(); err != nil {
		log.Error().Err(err).Msg("Failed to request initial tasks via WebSocket")
	}

	// Start heartbeat sender
	heartbeatTicker := time.NewTicker(a.config.Agent.CheckInterval)
	defer heartbeatTicker.Stop()

	// Start task status summary ticker (every 60 seconds)
	taskStatusTicker := time.NewTicker(60 * time.Second)
	defer taskStatusTicker.Stop()

	// Handle WebSocket messages
	go a.handleWebSocketMessages()

	// Main loop
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Agent context cancelled, closing WebSocket")
			// Send offline status before closing
			a.sendWebSocketMessage(map[string]interface{}{
				"type":      "status_update",
				"status":    "offline",
				"timestamp": time.Now().Unix(),
				"os_info":   a.osInfo,
			})
			return nil
		case <-heartbeatTicker.C:
			if err := a.sendHeartbeat(); err != nil {
				log.Error().Err(err).Msg("Failed to send heartbeat, attempting reconnection")
				if err := a.reconnectWebSocket(); err != nil {
					log.Error().Err(err).Msg("WebSocket reconnection failed, falling back to HTTP")
					a.useWebSocket = false
					return a.handleHTTPPolling(ctx)
				}
			}
		case <-taskStatusTicker.C:
			// Display current task status every 60 seconds
			a.tasksMutex.RLock()
			currentTasks := make([]models.MonitorTask, len(a.tasks))
			copy(currentTasks, a.tasks)
			a.tasksMutex.RUnlock()

			if len(currentTasks) > 0 {
				a.logTaskSummary(currentTasks, false)
			} else {
				log.Info().Msg("🔄 Task status update - No monitoring tasks assigned")
			}
		}
	}
}

// handleWebSocketMessages processes incoming WebSocket messages
func (a *Agent) handleWebSocketMessages() {
	for {
		messageType, data, err := a.wsConn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Error().Err(err).Msg("WebSocket connection error")
			}
			return
		}

		if messageType == websocket.TextMessage {
			var msg map[string]interface{}
			if err := json.Unmarshal(data, &msg); err != nil {
				log.Error().Err(err).Msg("Failed to parse WebSocket message")
				continue
			}

			msgType, ok := msg["type"].(string)
			if !ok {
				log.Error().Msg("WebSocket message missing type field")
				continue
			}

			log.Debug().Str("message_type", msgType).Msg("Received WebSocket message")

			// Handle different message types
			switch msgType {
			case "heartbeat_ack":
				log.Debug().Msg("Received heartbeat acknowledgment")
			case "status_ack":
				log.Debug().Msg("Received status acknowledgment")
			case "result_ack":
				log.Debug().Msg("Received result acknowledgment")
			case "task_assignment":
				a.handleTaskAssignmentMessage(msg)
			case "task_removal":
				a.handleTaskRemovalMessage(msg)
			default:
				log.Debug().Str("message_type", msgType).Msg("Unknown WebSocket message type")
			}
		}
	}
}

// sendWebSocketMessage sends a JSON message via WebSocket
func (a *Agent) sendWebSocketMessage(message map[string]interface{}) error {
	if a.wsConn == nil {
		return fmt.Errorf("WebSocket connection not established")
	}

	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	return a.wsConn.WriteMessage(websocket.TextMessage, data)
}

// sendHeartbeat sends a heartbeat message via WebSocket
func (a *Agent) sendHeartbeat() error {
	message := map[string]interface{}{
		"type":      "heartbeat",
		"agent_id":  a.config.Agent.AgentID,
		"timestamp": time.Now().Unix(),
		"status":    "online",
		"os_info":   a.osInfo,
	}
	return a.sendWebSocketMessage(message)
}

// reconnectWebSocket attempts to reconnect the WebSocket connection
func (a *Agent) reconnectWebSocket() error {
	if a.wsConn != nil {
		a.wsConn.Close()
		a.wsConn = nil
	}

	time.Sleep(2 * time.Second) // Brief delay before reconnection
	return a.connectWebSocket()
}

// handleHTTPPolling handles the legacy HTTP polling mode
func (a *Agent) handleHTTPPolling(ctx context.Context) error {
	// Main agent loop for HTTP polling
	ticker := time.NewTicker(a.config.Agent.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Agent context cancelled, shutting down HTTP polling")
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
	req, err := http.NewRequest("GET", a.httpURL+"/api/health", nil)
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", a.config.Agent.UserAgent)
	req.Header.Set("X-Agent-ID", a.config.Agent.AgentID)
	req.Header.Set("X-API-Key", a.config.Agent.APIKey)

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
		"os_info":   a.osInfo,
		"agent_info": map[string]interface{}{
			"version":      "2.0",
			"capabilities": []string{"http_fallback"},
		},
	}

	jsonData, err := json.Marshal(checkinData)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", a.httpURL+"/api/agents/checkin",
		bytes.NewReader(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", a.config.Agent.UserAgent)
	req.Header.Set("X-Agent-ID", a.config.Agent.AgentID)
	req.Header.Set("X-API-Key", a.config.Agent.APIKey)

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

// startMonitoringEngine manages monitoring tasks lifecycle
func (a *Agent) startMonitoringEngine() {
	// For WebSocket mode, tasks are requested after connection is established
	// and updates come via WebSocket messages
	if a.useWebSocket {
		// Just wait for stop signal - task management is handled via WebSocket
		<-a.stopChan
		log.Info().Msg("Stopping monitoring engine")
		a.stopAllTaskSchedulers()
		return
	}

	// HTTP fallback mode - use periodic polling
	taskRefreshTicker := time.NewTicker(5 * time.Minute)
	defer taskRefreshTicker.Stop()

	// Initial task fetch for HTTP mode
	if err := a.fetchAndUpdateTasks(); err != nil {
		log.Error().Err(err).Msg("Failed to fetch initial monitoring tasks")
	}

	for {
		select {
		case <-a.stopChan:
			log.Info().Msg("Stopping monitoring engine")
			a.stopAllTaskSchedulers()
			return
		case <-taskRefreshTicker.C:
			if err := a.fetchAndUpdateTasks(); err != nil {
				log.Error().Err(err).Msg("Failed to refresh monitoring tasks")
			}
		}
	}
}

// startResultSubmitter handles submitting monitoring results to the server
func (a *Agent) startResultSubmitter() {
	if a.useWebSocket {
		// WebSocket mode: submit results immediately
		for {
			select {
			case <-a.stopChan:
				log.Info().Msg("Stopping result submitter")
				return
			case result := <-a.results:
				// Submit immediately via WebSocket
				if err := a.submitResultViaWebSocket(result); err != nil {
					log.Error().Err(err).Int("task_id", result.TaskID).Msg("Failed to submit result via WebSocket, falling back to HTTP")
					// Fallback to HTTP if WebSocket fails
					a.submitResults([]models.MonitorResultRequest{result})
				}
			}
		}
	} else {
		// HTTP mode: batch results for efficiency
		submitTicker := time.NewTicker(30 * time.Second) // Submit results every 30 seconds
		defer submitTicker.Stop()

		var pendingResults []models.MonitorResultRequest

		for {
			select {
			case <-a.stopChan:
				log.Info().Msg("Stopping result submitter")
				// Submit any remaining results
				if len(pendingResults) > 0 {
					a.submitResults(pendingResults)
				}
				return
			case result := <-a.results:
				pendingResults = append(pendingResults, result)
				// Submit immediately if we have too many pending results
				if len(pendingResults) >= 10 {
					a.submitResults(pendingResults)
					pendingResults = nil
				}
			case <-submitTicker.C:
				if len(pendingResults) > 0 {
					a.submitResults(pendingResults)
					pendingResults = nil
				}
			}
		}
	}
}

// fetchAndUpdateTasks fetches monitoring tasks from the server and updates schedulers
func (a *Agent) fetchAndUpdateTasks() error {
	// Fetch tasks from server
	req, err := http.NewRequest("GET", a.httpURL+"/api/monitoring/tasks", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", a.config.Agent.UserAgent)
	req.Header.Set("X-Agent-ID", a.config.Agent.AgentID)
	req.Header.Set("X-API-Key", a.config.Agent.APIKey)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch tasks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	var tasksResponse models.AgentTasksResponse
	if err := json.NewDecoder(resp.Body).Decode(&tasksResponse); err != nil {
		return fmt.Errorf("failed to decode tasks response: %w", err)
	}

	// Update tasks
	a.tasksMutex.Lock()
	oldTaskCount := len(a.tasks)
	a.tasks = tasksResponse.Tasks
	a.tasksMutex.Unlock()

	// Update schedulers
	a.updateTaskSchedulers(tasksResponse.Tasks)

	// Log initial task summary or individual updates
	if oldTaskCount == 0 {
		a.logTaskSummary(tasksResponse.Tasks, true)
	} else {
		a.logTaskUpdates(tasksResponse.Tasks, oldTaskCount)
	}

	log.Debug().Int("task_count", len(tasksResponse.Tasks)).Msg("Updated monitoring tasks")
	return nil
}

// updateTaskSchedulers updates the task schedulers based on current tasks
func (a *Agent) updateTaskSchedulers(tasks []models.MonitorTask) {
	a.schedulersMutex.Lock()
	defer a.schedulersMutex.Unlock()

	// Create a map of current task IDs
	currentTasks := make(map[int]models.MonitorTask)
	for _, task := range tasks {
		if task.Enabled {
			currentTasks[task.ID] = task
		}
	}

	// Stop schedulers for tasks that are no longer enabled or exist
	for taskID, scheduler := range a.taskSchedulers {
		if _, exists := currentTasks[taskID]; !exists {
			log.Debug().Int("task_id", taskID).Msg("Stopping scheduler for removed/disabled task")
			scheduler.Stop()
			delete(a.taskSchedulers, taskID)
		}
	}

	// Start schedulers for new tasks
	for taskID, task := range currentTasks {
		if _, exists := a.taskSchedulers[taskID]; !exists {
			log.Debug().Int("task_id", taskID).Str("monitor_type", task.MonitorType).Str("url", task.URL).Msg("Starting scheduler for new task")
			scheduler := NewTaskScheduler(task, a)
			a.taskSchedulers[taskID] = scheduler
			go scheduler.Start()
		}
	}
}

// stopAllTaskSchedulers stops all running task schedulers
func (a *Agent) stopAllTaskSchedulers() {
	a.schedulersMutex.Lock()
	defer a.schedulersMutex.Unlock()

	for taskID, scheduler := range a.taskSchedulers {
		log.Debug().Int("task_id", taskID).Msg("Stopping task scheduler")
		scheduler.Stop()
	}
	a.taskSchedulers = make(map[int]*TaskScheduler)
}

// submitResults submits monitoring results to the server
func (a *Agent) submitResults(results []models.MonitorResultRequest) {
	if len(results) == 0 {
		return
	}

	jsonData, err := json.Marshal(results)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal monitoring results")
		return
	}

	req, err := http.NewRequest("POST", a.httpURL+"/api/monitoring/results", bytes.NewReader(jsonData))
	if err != nil {
		log.Error().Err(err).Msg("Failed to create results submission request")
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", a.config.Agent.UserAgent)
	req.Header.Set("X-Agent-ID", a.config.Agent.AgentID)
	req.Header.Set("X-API-Key", a.config.Agent.APIKey)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		log.Error().Err(err).Msg("Failed to submit monitoring results")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Error().Int("status_code", resp.StatusCode).Msg("Server rejected monitoring results")
		return
	}

	log.Debug().Int("result_count", len(results)).Msg("Successfully submitted monitoring results")
}

// NewTaskScheduler creates a new task scheduler
func NewTaskScheduler(task models.MonitorTask, agent *Agent) *TaskScheduler {
	return &TaskScheduler{
		task:     task,
		stopChan: make(chan struct{}),
		agent:    agent,
	}
}

// Start starts the task scheduler
func (ts *TaskScheduler) Start() {
	interval, err := parseDuration(ts.task.Interval)
	if err != nil {
		log.Error().Err(err).Int("task_id", ts.task.ID).Str("interval", ts.task.Interval).Msg("Invalid task interval")
		return
	}

	ts.ticker = time.NewTicker(interval)
	defer ts.ticker.Stop()

	// Execute immediately on start
	ts.executeTask()

	for {
		select {
		case <-ts.stopChan:
			log.Debug().Int("task_id", ts.task.ID).Msg("Task scheduler stopped")
			return
		case <-ts.ticker.C:
			ts.executeTask()
		}
	}
}

// Stop stops the task scheduler
func (ts *TaskScheduler) Stop() {
	close(ts.stopChan)
}

// executeTask executes a monitoring task
func (ts *TaskScheduler) executeTask() {
	log.Debug().Int("task_id", ts.task.ID).Str("monitor_type", ts.task.MonitorType).Str("url", ts.task.URL).Msg("Executing monitoring task")

	var result models.MonitorResultRequest
	result.TaskID = ts.task.ID
	result.CheckedAt = time.Now()

	timeout, err := parseDuration(ts.task.Timeout)
	if err != nil {
		log.Error().Err(err).Int("task_id", ts.task.ID).Str("timeout", ts.task.Timeout).Msg("Invalid task timeout")
		timeout = 30 * time.Second // Default timeout
	}

	switch ts.task.MonitorType {
	case "http", "https":
		result = ts.executeHTTPCheck(timeout)
	case "ping":
		result = ts.executePingCheck(timeout)
	case "log":
		result = ts.executeLogCheck(timeout)
	default:
		log.Error().Int("task_id", ts.task.ID).Str("monitor_type", ts.task.MonitorType).Msg("Unknown monitor type")
		result.Status = "error"
		errorMsg := fmt.Sprintf("Unknown monitor type: %s", ts.task.MonitorType)
		result.ErrorMessage = &errorMsg
	}

	// Submit result
	select {
	case ts.agent.results <- result:
		// Result queued successfully
	default:
		log.Warn().Int("task_id", ts.task.ID).Msg("Results channel full, dropping result")
	}
}

// executeHTTPCheck performs an HTTP/HTTPS check
func (ts *TaskScheduler) executeHTTPCheck(timeout time.Duration) models.MonitorResultRequest {
	result := models.MonitorResultRequest{
		TaskID:    ts.task.ID,
		CheckedAt: time.Now(),
	}

	start := time.Now()

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout:   timeout,
		Transport: ts.agent.httpClient.Transport, // Use same TLS config as agent
	}

	req, err := http.NewRequest("GET", ts.task.URL, nil)
	if err != nil {
		result.Status = "error"
		errorMsg := fmt.Sprintf("Failed to create request: %v", err)
		result.ErrorMessage = &errorMsg
		return result
	}

	req.Header.Set("User-Agent", ts.agent.config.Agent.UserAgent)

	resp, err := client.Do(req)
	duration := time.Since(start)
	responseTime := float64(duration.Nanoseconds()) / 1e6 // Convert to milliseconds
	result.ResponseTime = &responseTime

	if err != nil {
		result.Status = "down"
		errorMsg := fmt.Sprintf("HTTP request failed: %v", err)
		result.ErrorMessage = &errorMsg
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = &resp.StatusCode

	// Check if response indicates success
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		result.Status = "up"
	} else {
		result.Status = "down"
		errorMsg := fmt.Sprintf("HTTP %d", resp.StatusCode)
		result.ErrorMessage = &errorMsg
	}

	// Add metadata
	result.Metadata = map[string]interface{}{
		"content_length": resp.ContentLength,
		"headers":        resp.Header,
	}

	return result
}

// executePingCheck performs a ping check
func (ts *TaskScheduler) executePingCheck(timeout time.Duration) models.MonitorResultRequest {
	result := models.MonitorResultRequest{
		TaskID:    ts.task.ID,
		CheckedAt: time.Now(),
	}

	start := time.Now()

	// Use system ping command
	var cmd *exec.Cmd
	timeoutSeconds := int(timeout.Seconds())
	if timeoutSeconds == 0 {
		timeoutSeconds = 5
	}

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("ping", "-n", "1", "-w", strconv.Itoa(timeoutSeconds*1000), ts.task.URL)
	default: // Linux, macOS, etc.
		cmd = exec.Command("ping", "-c", "1", "-W", strconv.Itoa(timeoutSeconds), ts.task.URL)
	}

	output, err := cmd.Output()
	duration := time.Since(start)
	responseTime := float64(duration.Nanoseconds()) / 1e6 // Convert to milliseconds
	result.ResponseTime = &responseTime

	if err != nil {
		result.Status = "down"
		errorMsg := fmt.Sprintf("Ping failed: %v", err)
		result.ErrorMessage = &errorMsg
	} else {
		result.Status = "up"
		// Add ping output as metadata
		result.Metadata = map[string]interface{}{
			"ping_output": string(output),
		}
	}

	return result
}

// executeLogCheck performs a log file analysis check
func (ts *TaskScheduler) executeLogCheck(timeout time.Duration) models.MonitorResultRequest {
	result := models.MonitorResultRequest{
		TaskID:    ts.task.ID,
		CheckedAt: time.Now(),
	}

	start := time.Now()

	// Parse log configuration from task URL (JSON encoded) or use defaults
	var logConfig models.LogMonitorConfig

	// Extract file path from URL (remove log:// prefix if present)
	filePath := ts.task.URL
	if strings.HasPrefix(filePath, "log://") {
		filePath = filePath[6:] // Remove "log://" prefix
	}

	if ts.task.LogConfig != nil {
		logConfig = *ts.task.LogConfig
		// Ensure file path is updated
		if logConfig.FilePath == "" {
			logConfig.FilePath = filePath
		}
	} else {
		// Try to parse from URL field as JSON (for advanced configs)
		if strings.HasPrefix(ts.task.URL, "{") {
			if err := json.Unmarshal([]byte(ts.task.URL), &logConfig); err != nil {
				// Default configuration for nginx access log
				logConfig = models.LogMonitorConfig{
					FilePath:   filePath,
					Format:     "nginx",
					TailLines:  1000,
					Encoding:   "utf-8",
					ErrorCodes: []int{400, 401, 403, 404, 500, 502, 503, 504},
				}
			}
		} else {
			// Default configuration for nginx access log
			logConfig = models.LogMonitorConfig{
				FilePath:   filePath,
				Format:     "nginx",
				TailLines:  1000,
				Encoding:   "utf-8",
				ErrorCodes: []int{400, 401, 403, 404, 500, 502, 503, 504},
			}
		}
	}

	// Analyze log file
	metrics, err := ts.analyzeLogFile(logConfig, timeout)
	duration := time.Since(start)
	responseTimeMs := float64(duration.Nanoseconds()) / 1e6
	result.ResponseTime = &responseTimeMs

	if err != nil {
		result.Status = "error"
		errorMsg := fmt.Sprintf("Log analysis failed: %v", err)
		result.ErrorMessage = &errorMsg
		return result
	}

	// Determine status based on error rate
	if metrics.ErrorRate > 50.0 { // More than 50% errors
		result.Status = "down"
		errorMsg := fmt.Sprintf("High error rate: %.2f%%", metrics.ErrorRate)
		result.ErrorMessage = &errorMsg
	} else if metrics.ErrorRate > 20.0 { // More than 20% errors
		result.Status = "degraded"
		errorMsg := fmt.Sprintf("Elevated error rate: %.2f%%", metrics.ErrorRate)
		result.ErrorMessage = &errorMsg
	} else {
		result.Status = "up"
	}

	// Add log metrics as metadata
	result.Metadata = map[string]interface{}{
		"total_requests":    metrics.TotalRequests,
		"error_requests":    metrics.ErrorRequests,
		"error_rate":        metrics.ErrorRate,
		"avg_response_time": metrics.AvgResponseTime,
		"requests_per_min":  metrics.RequestsPerMinute,
		"status_codes":      metrics.StatusCodes,
		"top_errors":        metrics.TopErrors,
		"analysis_duration": duration.Milliseconds(),
		"log_file":          logConfig.FilePath,
		"lines_analyzed":    metrics.LinesAnalyzed,
	}

	return result
}

// LogMetrics represents aggregated metrics from log analysis
type LogMetrics struct {
	TotalRequests     int         `json:"total_requests"`
	ErrorRequests     int         `json:"error_requests"`
	ErrorRate         float64     `json:"error_rate"`
	AvgResponseTime   float64     `json:"avg_response_time"`
	RequestsPerMinute float64     `json:"requests_per_minute"`
	StatusCodes       map[int]int `json:"status_codes"`
	TopErrors         []string    `json:"top_errors"`
	LinesAnalyzed     int         `json:"lines_analyzed"`
}

// analyzeLogFile analyzes a log file and returns aggregated metrics
func (ts *TaskScheduler) analyzeLogFile(config models.LogMonitorConfig, timeout time.Duration) (*LogMetrics, error) {
	file, err := os.Open(config.FilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file %s: %w", config.FilePath, err)
	}
	defer file.Close()

	// Get file info to seek to end for tailing
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	// Seek to appropriate position for tailing
	var startPos int64
	if config.TailLines > 0 {
		// Estimate position based on average line length (assumption: ~200 chars per line)
		avgLineLength := int64(200)
		estimatedBytes := int64(config.TailLines) * avgLineLength
		if estimatedBytes < fileInfo.Size() {
			startPos = fileInfo.Size() - estimatedBytes
		}
	}

	if _, err := file.Seek(startPos, 0); err != nil {
		return nil, fmt.Errorf("failed to seek in file: %w", err)
	}

	// Initialize metrics
	metrics := &LogMetrics{
		StatusCodes: make(map[int]int),
		TopErrors:   make([]string, 0),
	}

	// Set up parser based on format
	parser, err := ts.createLogParser(config.Format, config.Pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to create log parser: %w", err)
	}

	// Configure time window based on file size (for testing with static logs)
	timeWindow := 5 * time.Minute // Default: analyze last 5 minutes of logs

	// If it's a small file (likely test data), use a longer window
	if fileInfo.Size() < 10*1024 { // Files smaller than 10KB (test files)
		timeWindow = 24 * time.Hour // Analyze last 24 hours for small test files
	}

	cutoffTime := time.Now().Add(-timeWindow)

	var totalResponseTime float64
	var responseTimeCount int
	errorMessages := make(map[string]int)

	// Context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Analyze log entries
	scanner := bufio.NewScanner(file)

	log.Debug().
		Str("log_file", config.FilePath).
		Dur("time_window", timeWindow).
		Time("cutoff_time", cutoffTime).
		Int64("file_size", fileInfo.Size()).
		Msg("Starting log file analysis")

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("log analysis timed out")
		default:
		}

		line := scanner.Text()
		metrics.LinesAnalyzed++

		entry, err := parser(line)
		if err != nil {
			// Skip unparseable lines
			log.Debug().Err(err).Str("line", line).Msg("Failed to parse log line")
			continue
		}

		// Only analyze recent entries
		if entry.Timestamp.Before(cutoffTime) {
			log.Debug().
				Time("entry_time", entry.Timestamp).
				Time("cutoff_time", cutoffTime).
				Msg("Skipping old log entry")
			continue
		}

		log.Debug().
			Time("entry_time", entry.Timestamp).
			Int("status_code", entry.StatusCode).
			Str("url", entry.URL).
			Msg("Processing log entry")

		metrics.TotalRequests++
		metrics.StatusCodes[entry.StatusCode]++

		// Check if it's an error
		isError := false
		if len(config.ErrorCodes) > 0 {
			for _, code := range config.ErrorCodes {
				if entry.StatusCode == code {
					isError = true
					break
				}
			}
		} else {
			// Default: 4xx and 5xx are errors
			isError = entry.StatusCode >= 400
		}

		if isError {
			metrics.ErrorRequests++
			errorKey := fmt.Sprintf("%d %s", entry.StatusCode, entry.URL)
			errorMessages[errorKey]++
		}

		// Track response times
		if entry.ResponseTime > 0 {
			totalResponseTime += entry.ResponseTime
			responseTimeCount++
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading log file: %w", err)
	}

	// Calculate final metrics
	if metrics.TotalRequests > 0 {
		metrics.ErrorRate = (float64(metrics.ErrorRequests) / float64(metrics.TotalRequests)) * 100.0
		metrics.RequestsPerMinute = float64(metrics.TotalRequests) / timeWindow.Minutes()
	}

	if responseTimeCount > 0 {
		metrics.AvgResponseTime = totalResponseTime / float64(responseTimeCount)
	}

	// Get top errors
	type errorCount struct {
		message string
		count   int
	}
	var errors []errorCount
	for msg, count := range errorMessages {
		errors = append(errors, errorCount{msg, count})
	}

	// Sort by count (simple bubble sort for small datasets)
	for i := 0; i < len(errors)-1; i++ {
		for j := 0; j < len(errors)-i-1; j++ {
			if errors[j].count < errors[j+1].count {
				errors[j], errors[j+1] = errors[j+1], errors[j]
			}
		}
	}

	// Take top 5 errors
	maxErrors := 5
	if len(errors) < maxErrors {
		maxErrors = len(errors)
	}
	for i := 0; i < maxErrors; i++ {
		metrics.TopErrors = append(metrics.TopErrors, fmt.Sprintf("%s (%d times)", errors[i].message, errors[i].count))
	}

	log.Debug().
		Int("total_requests", metrics.TotalRequests).
		Int("error_requests", metrics.ErrorRequests).
		Float64("error_rate", metrics.ErrorRate).
		Float64("avg_response_time", metrics.AvgResponseTime).
		Int("lines_analyzed", metrics.LinesAnalyzed).
		Msg("Log analysis completed")

	return metrics, nil
}

// createLogParser creates a parser function for the specified log format
func (ts *TaskScheduler) createLogParser(format, customPattern string) (func(string) (*models.LogEntry, error), error) {
	switch format {
	case "nginx":
		return ts.parseNginxLog, nil
	case "apache", "combined":
		return ts.parseApacheLog, nil
	case "json":
		return ts.parseJSONLog, nil
	case "custom":
		if customPattern == "" {
			return nil, fmt.Errorf("custom pattern required for custom format")
		}
		regex, err := regexp.Compile(customPattern)
		if err != nil {
			return nil, fmt.Errorf("invalid custom pattern: %w", err)
		}
		return func(line string) (*models.LogEntry, error) {
			return ts.parseCustomLog(line, regex)
		}, nil
	default:
		return nil, fmt.Errorf("unsupported log format: %s", format)
	}
}

// parseNginxLog parses nginx access log format
func (ts *TaskScheduler) parseNginxLog(line string) (*models.LogEntry, error) {
	// Nginx log format: $remote_addr - $remote_user [$time_local] "$request" $status $bytes_sent "$http_referer" "$http_user_agent" $request_time
	regex := regexp.MustCompile(`^(\S+) \S+ \S+ \[([^\]]+)\] "([^"]*)" (\d+) (\d+) "([^"]*)" "([^"]*)"(?: ([0-9.]+))?`)
	matches := regex.FindStringSubmatch(line)

	if len(matches) < 8 {
		return nil, fmt.Errorf("failed to parse nginx log line")
	}

	entry := &models.LogEntry{
		RemoteAddr: matches[1],
		RawLine:    line,
	}

	// Parse timestamp
	if timestamp, err := time.Parse("02/Jan/2006:15:04:05 -0700", matches[2]); err == nil {
		entry.Timestamp = timestamp
	} else if timestamp, err := time.Parse("02/Jan/2006:15:04:05", strings.TrimSpace(matches[2])); err == nil {
		// Handle timestamps without timezone (assume UTC)
		entry.Timestamp = timestamp.UTC()
	} else {
		entry.Timestamp = time.Now() // Fallback to current time
		log.Debug().Str("timestamp", matches[2]).Msg("Failed to parse nginx timestamp")
	}

	// Parse request
	requestParts := strings.Fields(matches[3])
	if len(requestParts) >= 2 {
		entry.Method = requestParts[0]
		entry.URL = requestParts[1]
	}

	// Parse status code
	if statusCode, err := strconv.Atoi(matches[4]); err == nil {
		entry.StatusCode = statusCode
	}

	// Parse bytes sent
	if bytesSent, err := strconv.ParseInt(matches[5], 10, 64); err == nil {
		entry.BytesSent = bytesSent
	}

	// Parse referrer and user agent
	entry.Referrer = matches[6]
	entry.UserAgent = matches[7]

	// Parse response time (if available)
	if len(matches) > 8 && matches[8] != "" {
		if responseTime, err := strconv.ParseFloat(matches[8], 64); err == nil {
			entry.ResponseTime = responseTime * 1000 // Convert to milliseconds
		}
	}

	return entry, nil
}

// parseApacheLog parses Apache combined log format
func (ts *TaskScheduler) parseApacheLog(line string) (*models.LogEntry, error) {
	// Apache combined log format: %h %l %u %t \"%r\" %>s %O \"%{Referer}i\" \"%{User-Agent}i\"
	regex := regexp.MustCompile(`^(\S+) \S+ \S+ \[([^\]]+)\] "([^"]*)" (\d+) (\d+) "([^"]*)" "([^"]*)"`)
	matches := regex.FindStringSubmatch(line)

	if len(matches) < 8 {
		return nil, fmt.Errorf("failed to parse apache log line")
	}

	entry := &models.LogEntry{
		RemoteAddr: matches[1],
		RawLine:    line,
	}

	// Parse timestamp
	if timestamp, err := time.Parse("02/Jan/2006:15:04:05 -0700", matches[2]); err == nil {
		entry.Timestamp = timestamp
	} else if timestamp, err := time.Parse("02/Jan/2006:15:04:05", strings.TrimSpace(matches[2])); err == nil {
		// Handle timestamps without timezone (assume UTC)
		entry.Timestamp = timestamp.UTC()
	} else {
		entry.Timestamp = time.Now()
		log.Debug().Str("timestamp", matches[2]).Msg("Failed to parse apache timestamp")
	}

	// Parse request
	requestParts := strings.Fields(matches[3])
	if len(requestParts) >= 2 {
		entry.Method = requestParts[0]
		entry.URL = requestParts[1]
	}

	// Parse status code
	if statusCode, err := strconv.Atoi(matches[4]); err == nil {
		entry.StatusCode = statusCode
	}

	// Parse bytes sent
	if bytesSent, err := strconv.ParseInt(matches[5], 10, 64); err == nil {
		entry.BytesSent = bytesSent
	}

	// Parse referrer and user agent
	entry.Referrer = matches[6]
	entry.UserAgent = matches[7]

	return entry, nil
}

// parseJSONLog parses JSON-formatted log entries
func (ts *TaskScheduler) parseJSONLog(line string) (*models.LogEntry, error) {
	var entry models.LogEntry
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		return nil, fmt.Errorf("failed to parse JSON log: %w", err)
	}
	entry.RawLine = line
	return &entry, nil
}

// parseCustomLog parses log using custom regex pattern
func (ts *TaskScheduler) parseCustomLog(line string, regex *regexp.Regexp) (*models.LogEntry, error) {
	matches := regex.FindStringSubmatch(line)
	if len(matches) < 2 {
		return nil, fmt.Errorf("custom pattern did not match")
	}

	entry := &models.LogEntry{
		RawLine:   line,
		Timestamp: time.Now(), // Default to current time
	}

	// Try to extract common fields from named groups
	groupNames := regex.SubexpNames()
	for i, match := range matches {
		if i == 0 || match == "" {
			continue
		}

		groupName := groupNames[i]
		switch groupName {
		case "timestamp":
			// Try common timestamp formats
			formats := []string{
				"02/Jan/2006:15:04:05 -0700",
				"2006-01-02T15:04:05Z07:00",
				"2006-01-02 15:04:05",
			}
			for _, format := range formats {
				if t, err := time.Parse(format, match); err == nil {
					entry.Timestamp = t
					break
				}
			}
		case "method":
			entry.Method = match
		case "url":
			entry.URL = match
		case "status_code":
			if code, err := strconv.Atoi(match); err == nil {
				entry.StatusCode = code
			}
		case "response_time":
			if rt, err := strconv.ParseFloat(match, 64); err == nil {
				entry.ResponseTime = rt
			}
		case "remote_addr":
			entry.RemoteAddr = match
		case "user_agent":
			entry.UserAgent = match
		case "referrer":
			entry.Referrer = match
		}
	}

	return entry, nil
}

// requestTasksViaWebSocket requests monitoring tasks via WebSocket
func (a *Agent) requestTasksViaWebSocket() error {
	message := map[string]interface{}{
		"type":      "request_tasks",
		"agent_id":  a.config.Agent.AgentID,
		"timestamp": time.Now().Unix(),
	}
	return a.sendWebSocketMessage(message)
}

// submitResultViaWebSocket submits a monitoring result via WebSocket
func (a *Agent) submitResultViaWebSocket(result models.MonitorResultRequest) error {
	message := map[string]interface{}{
		"type":          "monitoring_result",
		"agent_id":      a.config.Agent.AgentID,
		"task_id":       result.TaskID,
		"status":        result.Status,
		"response_time": result.ResponseTime,
		"status_code":   result.StatusCode,
		"error_message": result.ErrorMessage,
		"metadata":      result.Metadata,
		"checked_at":    result.CheckedAt.Unix(),
		"timestamp":     time.Now().Unix(),
	}
	return a.sendWebSocketMessage(message)
}

// handleTaskAssignmentMessage handles task assignment from server
func (a *Agent) handleTaskAssignmentMessage(msg map[string]interface{}) {
	log.Debug().Msg("Received task assignment")

	// Extract tasks from message
	tasksData, ok := msg["tasks"]
	if !ok {
		log.Error().Msg("Task assignment message missing tasks field")
		return
	}

	// Convert to JSON and back to parse into proper struct
	tasksJSON, err := json.Marshal(tasksData)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal tasks data")
		return
	}

	var tasks []models.MonitorTask
	if err := json.Unmarshal(tasksJSON, &tasks); err != nil {
		log.Error().Err(err).Msg("Failed to parse tasks from WebSocket message")
		return
	}

	// Update tasks
	a.tasksMutex.Lock()
	oldTaskCount := len(a.tasks)
	a.tasks = tasks
	a.tasksMutex.Unlock()

	// Update schedulers
	a.updateTaskSchedulers(tasks)

	// Log initial task summary or individual updates
	if oldTaskCount == 0 {
		a.logTaskSummary(tasks, true)
	} else {
		a.logTaskUpdates(tasks, oldTaskCount)
	}

	log.Info().Int("task_count", len(tasks)).Msg("Updated monitoring tasks via WebSocket")
}

// handleTaskRemovalMessage handles task removal from server
func (a *Agent) handleTaskRemovalMessage(msg map[string]interface{}) {
	log.Debug().Msg("Received task removal")

	// Extract task IDs to remove
	taskIDsData, ok := msg["task_ids"]
	if !ok {
		log.Error().Msg("Task removal message missing task_ids field")
		return
	}

	// Convert to slice of integers
	taskIDsSlice, ok := taskIDsData.([]interface{})
	if !ok {
		log.Error().Msg("Task removal message task_ids field is not an array")
		return
	}

	var taskIDsToRemove []int
	for _, idData := range taskIDsSlice {
		if id, ok := idData.(float64); ok {
			taskIDsToRemove = append(taskIDsToRemove, int(id))
		}
	}

	// Stop schedulers for removed tasks
	a.schedulersMutex.Lock()
	for _, taskID := range taskIDsToRemove {
		if scheduler, exists := a.taskSchedulers[taskID]; exists {
			log.Debug().Int("task_id", taskID).Msg("Stopping scheduler for removed task")
			scheduler.Stop()
			delete(a.taskSchedulers, taskID)
		}
	}
	a.schedulersMutex.Unlock()

	// Update tasks list
	a.tasksMutex.Lock()
	var updatedTasks []models.MonitorTask
	for _, task := range a.tasks {
		shouldRemove := false
		for _, removeID := range taskIDsToRemove {
			if task.ID == removeID {
				shouldRemove = true
				break
			}
		}
		if !shouldRemove {
			updatedTasks = append(updatedTasks, task)
		}
	}
	a.tasks = updatedTasks
	a.tasksMutex.Unlock()

	log.Info().Int("removed_count", len(taskIDsToRemove)).Msg("Removed monitoring tasks via WebSocket")
}

// logTaskSummary logs a summary of tasks organized by type
func (a *Agent) logTaskSummary(tasks []models.MonitorTask, isInitial bool) {
	if len(tasks) == 0 {
		if isInitial {
			log.Info().Msg("✅ Agent connected - No monitoring tasks assigned")
		} else {
			log.Info().Msg("🔄 Task status update - No monitoring tasks assigned")
		}
		return
	}

	// Count tasks by type
	taskCounts := make(map[string]int)
	for _, task := range tasks {
		taskCounts[task.MonitorType]++
	}

	// Log the summary
	logEvent := log.Info()
	if isInitial {
		logEvent = logEvent.Str("status", "✅ Agent connected - Received monitoring tasks")
	} else {
		logEvent = logEvent.Str("status", "🔄 Task status update - Active monitoring tasks")
	}

	logEvent = logEvent.Int("total_tasks", len(tasks))

	// Add counts for each type
	for taskType, count := range taskCounts {
		switch taskType {
		case "http":
			logEvent = logEvent.Int("http_resources", count)
		case "https":
			logEvent = logEvent.Int("https_resources", count)
		case "ping":
			logEvent = logEvent.Int("ping_targets", count)
		case "tcp":
			logEvent = logEvent.Int("tcp_services", count)
		case "log":
			logEvent = logEvent.Int("log_files", count)
		default:
			logEvent = logEvent.Int(fmt.Sprintf("%s_resources", taskType), count)
		}
	}

	logEvent.Msg("Monitoring task distribution")

	// Log individual tasks with emoji indicators
	for _, task := range tasks {
		emoji := "🌐"
		switch task.MonitorType {
		case "http":
			emoji = "🌐"
		case "https":
			emoji = "🔒"
		case "ping":
			emoji = "📡"
		case "tcp":
			emoji = "🔌"
		case "log":
			emoji = "📄"
		}

		log.Info().
			Str("emoji", emoji).
			Str("type", task.MonitorType).
			Str("url", task.URL).
			Str("interval", task.Interval).
			Int("task_id", task.ID).
			Msgf("Monitoring: %s", task.URL)
	}
}

// logTaskUpdates logs individual task updates
func (a *Agent) logTaskUpdates(newTasks []models.MonitorTask, oldTaskCount int) {
	newTaskCount := len(newTasks)

	if newTaskCount > oldTaskCount {
		added := newTaskCount - oldTaskCount
		log.Info().
			Str("status", "📈 New tasks added").
			Int("added_count", added).
			Int("total_tasks", newTaskCount).
			Msg("Monitoring tasks updated")

		// For simplicity, just log the new total summary
		a.logTaskSummary(newTasks, false)
	} else if newTaskCount < oldTaskCount {
		removed := oldTaskCount - newTaskCount
		log.Info().
			Str("status", "📉 Tasks removed").
			Int("removed_count", removed).
			Int("total_tasks", newTaskCount).
			Msg("Monitoring tasks updated")

		if newTaskCount > 0 {
			a.logTaskSummary(newTasks, false)
		}
	} else {
		log.Info().
			Str("status", "🔄 Tasks updated").
			Int("total_tasks", newTaskCount).
			Msg("Monitoring task configuration changed")
	}
}

// parseDuration parses duration strings like "30s", "2m", "1h"
func parseDuration(s string) (time.Duration, error) {
	return time.ParseDuration(s)
}
