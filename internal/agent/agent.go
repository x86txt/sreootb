package agent

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
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
func (a *Agent) Start(ctx context.Context) error {
	log.Info().
		Str("server_url", a.httpURL).
		Str("websocket_url", a.wsURL).
		Str("agent_id", a.config.Agent.AgentID).
		Msg("Starting SREootb agent")

	// Start monitoring task management
	go a.startMonitoringEngine()
	go a.startResultSubmitter()

	// Try WebSocket connection first
	if err := a.connectWebSocket(); err != nil {
		log.Warn().Err(err).Msg("WebSocket connection failed, falling back to HTTP polling")
		a.useWebSocket = false

		// Test HTTP connection as fallback
		if err := a.testConnection(); err != nil {
			return fmt.Errorf("failed to connect to server via HTTP: %w", err)
		}
		log.Info().Msg("Successfully connected to server via HTTP")
	} else {
		log.Info().Msg("Successfully connected to server via WebSocket")
	}

	// Start health endpoint
	go a.startHealthServer()

	if a.useWebSocket {
		// WebSocket mode - handle connection
		return a.handleWebSocketConnection(ctx)
	} else {
		// HTTP polling mode - legacy fallback
		return a.handleHTTPPolling(ctx)
	}
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

// parseDuration parses duration strings like "30s", "2m", "1h"
func parseDuration(s string) (time.Duration, error) {
	return time.ParseDuration(s)
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
