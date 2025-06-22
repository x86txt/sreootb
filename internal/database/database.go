package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog/log"

	"github.com/x86txt/sreootb/internal/models"
)

// DB wraps a SQLite database connection
type DB struct {
	conn *sql.DB
}

// New creates a new database connection
func New(dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db := &DB{conn: conn}

	// Initialize tables
	if err := db.init(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	return db, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.conn.Close()
}

// init creates the database tables if they don't exist
func (db *DB) init() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS sites (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			url TEXT UNIQUE NOT NULL,
			name TEXT NOT NULL,
			scan_interval TEXT DEFAULT '60s',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS site_checks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			site_id INTEGER NOT NULL,
			status TEXT NOT NULL,
			response_time REAL,
			status_code INTEGER,
			error_message TEXT,
			checked_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (site_id) REFERENCES sites (id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS agents (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			api_key_hash TEXT UNIQUE NOT NULL,
			description TEXT,
			last_seen TIMESTAMP,
			status TEXT DEFAULT 'offline',
			os TEXT,
			platform TEXT,
			architecture TEXT,
			version TEXT,
			remote_ip TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS monitor_tasks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			site_id INTEGER NOT NULL,
			monitor_type TEXT NOT NULL,
			url TEXT NOT NULL,
			interval TEXT NOT NULL DEFAULT '60s',
			timeout TEXT NOT NULL DEFAULT '10s',
			enabled BOOLEAN NOT NULL DEFAULT 1,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (site_id) REFERENCES sites (id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS monitor_results (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id INTEGER NOT NULL,
			agent_id INTEGER NOT NULL,
			status TEXT NOT NULL,
			response_time REAL,
			status_code INTEGER,
			error_message TEXT,
			metadata TEXT,
			checked_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (task_id) REFERENCES monitor_tasks (id) ON DELETE CASCADE,
			FOREIGN KEY (agent_id) REFERENCES agents (id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS agent_task_assignments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			agent_id INTEGER NOT NULL,
			task_id INTEGER NOT NULL,
			assigned BOOLEAN NOT NULL DEFAULT 1,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (agent_id) REFERENCES agents (id) ON DELETE CASCADE,
			FOREIGN KEY (task_id) REFERENCES monitor_tasks (id) ON DELETE CASCADE,
			UNIQUE(agent_id, task_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_site_checks_site_id ON site_checks(site_id)`,
		`CREATE INDEX IF NOT EXISTS idx_site_checks_checked_at ON site_checks(checked_at)`,
		`CREATE INDEX IF NOT EXISTS idx_agents_api_key_hash ON agents(api_key_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_monitor_tasks_site_id ON monitor_tasks(site_id)`,
		`CREATE INDEX IF NOT EXISTS idx_monitor_tasks_enabled ON monitor_tasks(enabled)`,
		`CREATE INDEX IF NOT EXISTS idx_monitor_results_task_id ON monitor_results(task_id)`,
		`CREATE INDEX IF NOT EXISTS idx_monitor_results_agent_id ON monitor_results(agent_id)`,
		`CREATE INDEX IF NOT EXISTS idx_monitor_results_checked_at ON monitor_results(checked_at)`,
		`CREATE INDEX IF NOT EXISTS idx_agent_task_assignments_agent_id ON agent_task_assignments(agent_id)`,
		`CREATE INDEX IF NOT EXISTS idx_agent_task_assignments_task_id ON agent_task_assignments(task_id)`,
	}

	for _, query := range queries {
		if _, err := db.conn.Exec(query); err != nil {
			return fmt.Errorf("failed to execute query: %s: %w", query, err)
		}
	}

	// Run migrations for existing databases
	if err := db.migrate(); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

// migrate applies database migrations for existing databases
func (db *DB) migrate() error {
	// Check if we need to add OS information columns to agents table
	if err := db.addOSInfoColumns(); err != nil {
		return fmt.Errorf("failed to add OS info columns: %w", err)
	}

	// Check if we need to create monitoring tasks for existing sites
	if err := db.createMonitoringTasksForExistingSites(); err != nil {
		return fmt.Errorf("failed to create monitoring tasks for existing sites: %w", err)
	}

	return nil
}

// addOSInfoColumns adds OS information columns to the agents table if they don't exist
func (db *DB) addOSInfoColumns() error {
	// Check if os column exists
	var count int
	err := db.conn.QueryRow("SELECT COUNT(*) FROM pragma_table_info('agents') WHERE name='os'").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check for os column: %w", err)
	}

	// If os column doesn't exist, add all the new columns
	if count == 0 {
		migrations := []string{
			"ALTER TABLE agents ADD COLUMN os TEXT",
			"ALTER TABLE agents ADD COLUMN platform TEXT",
			"ALTER TABLE agents ADD COLUMN architecture TEXT",
			"ALTER TABLE agents ADD COLUMN version TEXT",
		}

		for _, migration := range migrations {
			if _, err := db.conn.Exec(migration); err != nil {
				return fmt.Errorf("failed to execute migration '%s': %w", migration, err)
			}
		}

		fmt.Println("✅ Added OS information columns to agents table")
	}

	// Check if remote_ip column exists (separate migration)
	err = db.conn.QueryRow("SELECT COUNT(*) FROM pragma_table_info('agents') WHERE name='remote_ip'").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check for remote_ip column: %w", err)
	}

	if count == 0 {
		if _, err := db.conn.Exec("ALTER TABLE agents ADD COLUMN remote_ip TEXT"); err != nil {
			return fmt.Errorf("failed to add remote_ip column: %w", err)
		}
		fmt.Println("✅ Added remote_ip column to agents table")
	}

	return nil
}

// createMonitoringTasksForExistingSites creates monitoring tasks for existing sites that don't have them
func (db *DB) createMonitoringTasksForExistingSites() error {
	// Get all sites that don't have monitoring tasks
	query := `
		SELECT s.id, s.url, s.scan_interval 
		FROM sites s 
		WHERE NOT EXISTS (
			SELECT 1 FROM monitor_tasks mt WHERE mt.site_id = s.id
		)
	`

	rows, err := db.conn.Query(query)
	if err != nil {
		return fmt.Errorf("failed to query sites without monitoring tasks: %w", err)
	}
	defer rows.Close()

	var sitesToMigrate []struct {
		ID           int
		URL          string
		ScanInterval string
	}

	for rows.Next() {
		var site struct {
			ID           int
			URL          string
			ScanInterval string
		}
		if err := rows.Scan(&site.ID, &site.URL, &site.ScanInterval); err != nil {
			return fmt.Errorf("failed to scan site: %w", err)
		}
		sitesToMigrate = append(sitesToMigrate, site)
	}

	// Create monitoring tasks for these sites
	for _, site := range sitesToMigrate {
		if err := db.createMonitoringTaskForSite(site.ID, site.URL, site.ScanInterval); err != nil {
			return fmt.Errorf("failed to create monitoring task for site %d: %w", site.ID, err)
		}
	}

	if len(sitesToMigrate) > 0 {
		fmt.Printf("✅ Created monitoring tasks for %d existing sites\n", len(sitesToMigrate))
	}

	return nil
}

// createMonitoringTaskForSite creates appropriate monitoring tasks for a site based on its URL
func (db *DB) createMonitoringTaskForSite(siteID int, url, interval string) error {
	var monitorType string
	var timeout string = "10s"

	// Determine monitor type based on URL
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		monitorType = "http"
		timeout = "30s"
	} else if strings.HasPrefix(url, "ping://") {
		monitorType = "ping"
		timeout = "5s"
		// Remove ping:// prefix for the actual URL
		url = strings.TrimPrefix(url, "ping://")
	} else {
		// Default to HTTP for unknown protocols
		monitorType = "http"
		timeout = "30s"
	}

	// Create the monitoring task
	query := `INSERT INTO monitor_tasks (site_id, monitor_type, url, interval, timeout, enabled) VALUES (?, ?, ?, ?, ?, 1)`
	_, err := db.conn.Exec(query, siteID, monitorType, url, interval, timeout)
	return err
}

// Sites

// AddSite adds a new site to monitor
func (db *DB) AddSite(site *models.SiteCreateRequest) (*models.Site, error) {
	query := `INSERT INTO sites (url, name, scan_interval) VALUES (?, ?, ?) RETURNING id, created_at`

	var newSite models.Site
	newSite.URL = site.URL
	newSite.Name = site.Name
	newSite.ScanInterval = site.ScanInterval

	err := db.conn.QueryRow(query, site.URL, site.Name, site.ScanInterval).Scan(&newSite.ID, &newSite.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to add site: %w", err)
	}

	// Create monitoring task for this site
	if err := db.createMonitoringTaskForSite(newSite.ID, newSite.URL, newSite.ScanInterval); err != nil {
		log.Warn().Err(err).Int("site_id", newSite.ID).Msg("Failed to create monitoring task for new site")
	}

	return &newSite, nil
}

// GetSites returns all sites
func (db *DB) GetSites() ([]*models.Site, error) {
	query := `SELECT id, url, name, scan_interval, created_at FROM sites ORDER BY name`

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get sites: %w", err)
	}
	defer rows.Close()

	var sites []*models.Site
	for rows.Next() {
		var site models.Site
		if err := rows.Scan(&site.ID, &site.URL, &site.Name, &site.ScanInterval, &site.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan site: %w", err)
		}
		sites = append(sites, &site)
	}

	return sites, nil
}

// GetSite returns a site by ID
func (db *DB) GetSite(id int) (*models.Site, error) {
	query := `SELECT id, url, name, scan_interval, created_at FROM sites WHERE id = ?`

	var site models.Site
	err := db.conn.QueryRow(query, id).Scan(&site.ID, &site.URL, &site.Name, &site.ScanInterval, &site.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get site: %w", err)
	}

	return &site, nil
}

// DeleteSite deletes a site and all its checks
func (db *DB) DeleteSite(id int) error {
	query := `DELETE FROM sites WHERE id = ?`

	result, err := db.conn.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete site: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("site not found")
	}

	return nil
}

// Site Checks

// RecordCheck records a site check result
func (db *DB) RecordCheck(check *models.SiteCheck) error {
	query := `INSERT INTO site_checks (site_id, status, response_time, status_code, error_message) 
			  VALUES (?, ?, ?, ?, ?)`

	_, err := db.conn.Exec(query, check.SiteID, check.Status, check.ResponseTime, check.StatusCode, check.ErrorMessage)
	if err != nil {
		return fmt.Errorf("failed to record check: %w", err)
	}

	return nil
}

// GetSiteStatus returns current status of all sites with latest check information
func (db *DB) GetSiteStatus() ([]*models.SiteStatus, error) {
	query := `
		SELECT 
			s.id, s.url, s.name, s.scan_interval, s.created_at,
			sc.status, sc.response_time, sc.status_code, sc.error_message, sc.checked_at,
			(SELECT COUNT(*) FROM site_checks WHERE site_id = s.id AND status = 'up') as total_up,
			(SELECT COUNT(*) FROM site_checks WHERE site_id = s.id AND status = 'down') as total_down
		FROM sites s
		LEFT JOIN site_checks sc ON s.id = sc.site_id
		WHERE sc.checked_at = (
			SELECT MAX(checked_at) 
			FROM site_checks 
			WHERE site_id = s.id
		) OR sc.checked_at IS NULL
		ORDER BY s.name
	`

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get site status: %w", err)
	}
	defer rows.Close()

	var statuses []*models.SiteStatus
	for rows.Next() {
		var status models.SiteStatus

		err := rows.Scan(
			&status.ID, &status.URL, &status.Name, &status.ScanInterval, &status.CreatedAt,
			&status.Status, &status.ResponseTime, &status.StatusCode, &status.ErrorMessage, &status.CheckedAt,
			&status.TotalUp, &status.TotalDown,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan site status: %w", err)
		}

		statuses = append(statuses, &status)
	}

	return statuses, nil
}

// GetSiteHistory returns check history for a specific site
func (db *DB) GetSiteHistory(siteID int, limit int) ([]*models.SiteCheck, error) {
	query := `SELECT id, site_id, status, response_time, status_code, error_message, checked_at 
			  FROM site_checks WHERE site_id = ? ORDER BY checked_at DESC LIMIT ?`

	rows, err := db.conn.Query(query, siteID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get site history: %w", err)
	}
	defer rows.Close()

	var checks []*models.SiteCheck
	for rows.Next() {
		var check models.SiteCheck
		err := rows.Scan(&check.ID, &check.SiteID, &check.Status, &check.ResponseTime,
			&check.StatusCode, &check.ErrorMessage, &check.CheckedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan site check: %w", err)
		}
		checks = append(checks, &check)
	}

	return checks, nil
}

// Agents

// AddAgent adds a new agent
func (db *DB) AddAgent(agent *models.AgentCreateRequest, apiKeyHash string) (*models.Agent, error) {
	query := `INSERT INTO agents (name, api_key_hash, description) VALUES (?, ?, ?) RETURNING id, created_at`

	var newAgent models.Agent
	newAgent.Name = agent.Name
	newAgent.APIKeyHash = apiKeyHash
	newAgent.Description = agent.Description
	newAgent.Status = "offline"

	err := db.conn.QueryRow(query, agent.Name, apiKeyHash, agent.Description).Scan(&newAgent.ID, &newAgent.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to add agent: %w", err)
	}

	return &newAgent, nil
}

// GetAgents returns all agents
func (db *DB) GetAgents() ([]*models.Agent, error) {
	query := `SELECT id, name, description, last_seen, status, os, platform, architecture, version, remote_ip, created_at, api_key_hash FROM agents ORDER BY name`

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get agents: %w", err)
	}
	defer rows.Close()

	var agents []*models.Agent
	for rows.Next() {
		var agent models.Agent
		err := rows.Scan(&agent.ID, &agent.Name, &agent.Description, &agent.LastSeen, &agent.Status,
			&agent.OS, &agent.Platform, &agent.Architecture, &agent.Version, &agent.RemoteIP, &agent.CreatedAt, &agent.APIKeyHash)
		if err != nil {
			return nil, fmt.Errorf("failed to scan agent: %w", err)
		}
		agents = append(agents, &agent)
	}

	return agents, nil
}

// GetAgentByKeyHash returns an agent by API key hash
func (db *DB) GetAgentByKeyHash(keyHash string) (*models.Agent, error) {
	query := `SELECT id, name, description, last_seen, status, os, platform, architecture, version, remote_ip, created_at FROM agents WHERE api_key_hash = ?`

	var agent models.Agent
	err := db.conn.QueryRow(query, keyHash).Scan(&agent.ID, &agent.Name, &agent.Description, &agent.LastSeen, &agent.Status,
		&agent.OS, &agent.Platform, &agent.Architecture, &agent.Version, &agent.RemoteIP, &agent.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}

	return &agent, nil
}

// UpdateAgentOSInfo updates agent OS information and status
func (db *DB) UpdateAgentOSInfo(keyHash string, status string, osInfo map[string]interface{}) error {
	query := `UPDATE agents SET status = ?, last_seen = CURRENT_TIMESTAMP, os = ?, platform = ?, architecture = ?, version = ? WHERE api_key_hash = ?`

	// Extract OS information from the map
	var os, platform, architecture, version interface{}
	if osInfo != nil {
		os = osInfo["os"]
		platform = osInfo["platform"]
		architecture = osInfo["architecture"]
		version = osInfo["version"]
	}

	_, err := db.conn.Exec(query, status, os, platform, architecture, version, keyHash)
	if err != nil {
		return fmt.Errorf("failed to update agent OS info: %w", err)
	}

	return nil
}

// UpdateAgentWithRemoteIP updates agent OS information, status, and remote IP
func (db *DB) UpdateAgentWithRemoteIP(keyHash string, status string, osInfo map[string]interface{}, remoteIP string) error {
	query := `UPDATE agents SET status = ?, last_seen = CURRENT_TIMESTAMP, os = ?, platform = ?, architecture = ?, version = ?, remote_ip = ? WHERE api_key_hash = ?`

	// Extract OS information from the map
	var os, platform, architecture, version interface{}
	if osInfo != nil {
		os = osInfo["os"]
		platform = osInfo["platform"]
		architecture = osInfo["architecture"]
		version = osInfo["version"]
	}

	_, err := db.conn.Exec(query, status, os, platform, architecture, version, remoteIP, keyHash)
	if err != nil {
		return fmt.Errorf("failed to update agent with remote IP: %w", err)
	}

	return nil
}

// UpdateAgentStatus updates agent status and last seen timestamp
func (db *DB) UpdateAgentStatus(keyHash string, status string) error {
	query := `UPDATE agents SET status = ?, last_seen = CURRENT_TIMESTAMP WHERE api_key_hash = ?`

	_, err := db.conn.Exec(query, status, keyHash)
	if err != nil {
		return fmt.Errorf("failed to update agent status: %w", err)
	}

	return nil
}

// ValidateAgentAPIKey validates an agent API key hash
func (db *DB) ValidateAgentAPIKey(keyHash string) (bool, error) {
	query := `SELECT COUNT(*) FROM agents WHERE api_key_hash = ?`

	var count int
	err := db.conn.QueryRow(query, keyHash).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to validate agent API key: %w", err)
	}

	return count > 0, nil
}

// DeleteAgent deletes an agent
func (db *DB) DeleteAgent(id int) error {
	query := `DELETE FROM agents WHERE id = ?`

	result, err := db.conn.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete agent: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("agent not found")
	}

	return nil
}

// GetMonitorStats returns monitoring statistics
func (db *DB) GetMonitorStats() (*models.MonitorStats, error) {
	query := `
		SELECT 
			COUNT(*) as total_sites,
			COUNT(CASE WHEN latest_status = 'up' THEN 1 END) as sites_up,
			COUNT(CASE WHEN latest_status = 'down' THEN 1 END) as sites_down,
			AVG(CASE WHEN latest_status = 'up' THEN latest_response_time END) as avg_response_time
		FROM (
			SELECT 
				s.id,
				sc.status as latest_status,
				sc.response_time as latest_response_time
			FROM sites s
			LEFT JOIN site_checks sc ON s.id = sc.site_id
			WHERE sc.checked_at = (
				SELECT MAX(checked_at) 
				FROM site_checks 
				WHERE site_id = s.id
			) OR sc.checked_at IS NULL
		) latest_checks
	`

	var stats models.MonitorStats
	err := db.conn.QueryRow(query).Scan(&stats.TotalSites, &stats.SitesUp, &stats.SitesDown, &stats.AverageResponseTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get monitor stats: %w", err)
	}

	return &stats, nil
}

// Monitoring Tasks

// GetMonitoringTasks returns all monitoring tasks
func (db *DB) GetMonitoringTasks() ([]*models.MonitorTask, error) {
	query := `SELECT id, site_id, monitor_type, url, interval, timeout, enabled, created_at, updated_at FROM monitor_tasks ORDER BY id`

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get monitoring tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*models.MonitorTask
	for rows.Next() {
		var task models.MonitorTask
		err := rows.Scan(&task.ID, &task.SiteID, &task.MonitorType, &task.URL, &task.Interval, &task.Timeout, &task.Enabled, &task.CreatedAt, &task.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan monitoring task: %w", err)
		}
		tasks = append(tasks, &task)
	}

	return tasks, nil
}

// GetEnabledMonitoringTasks returns all enabled monitoring tasks
func (db *DB) GetEnabledMonitoringTasks() ([]*models.MonitorTask, error) {
	query := `SELECT id, site_id, monitor_type, url, interval, timeout, enabled, created_at, updated_at FROM monitor_tasks WHERE enabled = 1 ORDER BY id`

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get enabled monitoring tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*models.MonitorTask
	for rows.Next() {
		var task models.MonitorTask
		err := rows.Scan(&task.ID, &task.SiteID, &task.MonitorType, &task.URL, &task.Interval, &task.Timeout, &task.Enabled, &task.CreatedAt, &task.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan monitoring task: %w", err)
		}
		tasks = append(tasks, &task)
	}

	return tasks, nil
}

// GetTasksForAgent returns monitoring tasks assigned to a specific agent
func (db *DB) GetTasksForAgent(agentID int) ([]*models.MonitorTask, error) {
	// For now, assign all enabled tasks to all agents (can be refined later)
	query := `
		SELECT mt.id, mt.site_id, mt.monitor_type, mt.url, mt.interval, mt.timeout, mt.enabled, mt.created_at, mt.updated_at 
		FROM monitor_tasks mt 
		WHERE mt.enabled = 1 
		ORDER BY mt.id
	`

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get tasks for agent: %w", err)
	}
	defer rows.Close()

	var tasks []*models.MonitorTask
	for rows.Next() {
		var task models.MonitorTask
		err := rows.Scan(&task.ID, &task.SiteID, &task.MonitorType, &task.URL, &task.Interval, &task.Timeout, &task.Enabled, &task.CreatedAt, &task.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan monitoring task: %w", err)
		}
		tasks = append(tasks, &task)
	}

	return tasks, nil
}

// Monitoring Results

// RecordMonitorResult records a monitoring result from an agent
func (db *DB) RecordMonitorResult(result *models.MonitorResultRequest, agentID int) error {
	// Convert metadata to JSON if provided
	var metadataJSON *string
	if result.Metadata != nil && len(result.Metadata) > 0 {
		if jsonBytes, err := json.Marshal(result.Metadata); err == nil {
			metadataStr := string(jsonBytes)
			metadataJSON = &metadataStr
		}
	}

	query := `INSERT INTO monitor_results (task_id, agent_id, status, response_time, status_code, error_message, metadata, checked_at) 
			  VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := db.conn.Exec(query, result.TaskID, agentID, result.Status, result.ResponseTime, result.StatusCode, result.ErrorMessage, metadataJSON, result.CheckedAt)
	if err != nil {
		return fmt.Errorf("failed to record monitoring result: %w", err)
	}

	return nil
}

// GetMonitorResults returns monitoring results with optional filtering
func (db *DB) GetMonitorResults(limit int, agentID *int, taskID *int) ([]*models.MonitorResult, error) {
	query := `SELECT id, task_id, agent_id, status, response_time, status_code, error_message, metadata, checked_at FROM monitor_results`
	args := []interface{}{}
	conditions := []string{}

	if agentID != nil {
		conditions = append(conditions, "agent_id = ?")
		args = append(args, *agentID)
	}

	if taskID != nil {
		conditions = append(conditions, "task_id = ?")
		args = append(args, *taskID)
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	query += " ORDER BY checked_at DESC"

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get monitoring results: %w", err)
	}
	defer rows.Close()

	var results []*models.MonitorResult
	for rows.Next() {
		var result models.MonitorResult
		err := rows.Scan(&result.ID, &result.TaskID, &result.AgentID, &result.Status, &result.ResponseTime, &result.StatusCode, &result.ErrorMessage, &result.Metadata, &result.CheckedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan monitoring result: %w", err)
		}
		results = append(results, &result)
	}

	return results, nil
}

// GetLatestMonitorResults returns the latest result for each task-agent combination
func (db *DB) GetLatestMonitorResults() ([]*models.MonitorResult, error) {
	query := `
		SELECT mr1.id, mr1.task_id, mr1.agent_id, mr1.status, mr1.response_time, mr1.status_code, mr1.error_message, mr1.metadata, mr1.checked_at
		FROM monitor_results mr1
		INNER JOIN (
			SELECT task_id, agent_id, MAX(checked_at) as max_checked_at
			FROM monitor_results
			GROUP BY task_id, agent_id
		) mr2 ON mr1.task_id = mr2.task_id AND mr1.agent_id = mr2.agent_id AND mr1.checked_at = mr2.max_checked_at
		ORDER BY mr1.checked_at DESC
	`

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest monitoring results: %w", err)
	}
	defer rows.Close()

	var results []*models.MonitorResult
	for rows.Next() {
		var result models.MonitorResult
		err := rows.Scan(&result.ID, &result.TaskID, &result.AgentID, &result.Status, &result.ResponseTime, &result.StatusCode, &result.ErrorMessage, &result.Metadata, &result.CheckedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan monitoring result: %w", err)
		}
		results = append(results, &result)
	}

	return results, nil
}
