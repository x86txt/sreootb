package database

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"

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
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_site_checks_site_id ON site_checks(site_id)`,
		`CREATE INDEX IF NOT EXISTS idx_site_checks_checked_at ON site_checks(checked_at)`,
		`CREATE INDEX IF NOT EXISTS idx_agents_api_key_hash ON agents(api_key_hash)`,
	}

	for _, query := range queries {
		if _, err := db.conn.Exec(query); err != nil {
			return fmt.Errorf("failed to execute query: %s: %w", query, err)
		}
	}

	return nil
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
	query := `SELECT id, name, description, last_seen, status, created_at FROM agents ORDER BY name`

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get agents: %w", err)
	}
	defer rows.Close()

	var agents []*models.Agent
	for rows.Next() {
		var agent models.Agent
		err := rows.Scan(&agent.ID, &agent.Name, &agent.Description, &agent.LastSeen, &agent.Status, &agent.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan agent: %w", err)
		}
		agents = append(agents, &agent)
	}

	return agents, nil
}

// GetAgentByKeyHash returns an agent by API key hash
func (db *DB) GetAgentByKeyHash(keyHash string) (*models.Agent, error) {
	query := `SELECT id, name, description, last_seen, status, created_at FROM agents WHERE api_key_hash = ?`

	var agent models.Agent
	err := db.conn.QueryRow(query, keyHash).Scan(&agent.ID, &agent.Name, &agent.Description, &agent.LastSeen, &agent.Status, &agent.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}

	return &agent, nil
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
