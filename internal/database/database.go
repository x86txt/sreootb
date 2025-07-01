package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog/log"

	"github.com/x86txt/sreootb/internal/config"
	"github.com/x86txt/sreootb/internal/models"
)

// DatabaseType represents the type of database being used
type DatabaseType int

const (
	SQLite DatabaseType = iota
	CockroachDB
)

// DB wraps a database connection with type information
type DB struct {
	conn   *sql.DB
	dbType DatabaseType
}

// New creates a new database connection based on configuration
func New(cfg *config.DatabaseConfig) (*DB, error) {
	var conn *sql.DB
	var dbType DatabaseType
	var err error

	switch cfg.Type {
	case "sqlite":
		conn, err = openSQLite(cfg)
		dbType = SQLite
	case "cockroachdb":
		conn, err = openCockroachDB(cfg)
		dbType = CockroachDB
	default:
		return nil, fmt.Errorf("unsupported database type: %s", cfg.Type)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db := &DB{
		conn:   conn,
		dbType: dbType,
	}

	// Test connection
	if err := db.conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Initialize tables
	if err := db.init(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	log.Info().Str("type", cfg.Type).Msg("Database connection established")
	return db, nil
}

// openSQLite opens a SQLite database connection
func openSQLite(cfg *config.DatabaseConfig) (*sql.DB, error) {
	dsn := cfg.SQLitePath + "?_foreign_keys=on&_journal_mode=WAL"
	return sql.Open("sqlite3", dsn)
}

// openCockroachDB opens a CockroachDB connection
func openCockroachDB(cfg *config.DatabaseConfig) (*sql.DB, error) {
	// Build connection string
	values := url.Values{}
	values.Set("sslmode", cfg.SSLMode)

	if cfg.SSLRootCert != "" {
		values.Set("sslrootcert", cfg.SSLRootCert)
	}
	if cfg.SSLCert != "" {
		values.Set("sslcert", cfg.SSLCert)
	}
	if cfg.SSLKey != "" {
		values.Set("sslkey", cfg.SSLKey)
	}

	dsn := fmt.Sprintf("postgresql://%s:%s@%s:%d/%s?%s",
		url.QueryEscape(cfg.User),
		url.QueryEscape(cfg.Password),
		cfg.Host,
		cfg.Port,
		cfg.Database,
		values.Encode())

	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}

	// Configure connection pool for high availability
	conn.SetMaxOpenConns(cfg.MaxOpenConns)
	conn.SetMaxIdleConns(cfg.MaxIdleConns)
	conn.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	conn.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	return conn, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.conn.Close()
}

// init creates the database tables if they don't exist
func (db *DB) init() error {
	queries := db.getInitQueries()

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

// getInitQueries returns database initialization queries based on database type
func (db *DB) getInitQueries() []string {
	switch db.dbType {
	case SQLite:
		return db.getSQLiteInitQueries()
	case CockroachDB:
		return db.getCockroachInitQueries()
	default:
		return nil
	}
}

// getSQLiteInitQueries returns SQLite-specific initialization queries
func (db *DB) getSQLiteInitQueries() []string {
	return []string{
		// User authentication tables
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			first_name TEXT NOT NULL,
			last_name TEXT NOT NULL,
			role TEXT DEFAULT 'user',
			email_verified BOOLEAN DEFAULT 0,
			two_factor_enabled BOOLEAN DEFAULT 0,
			two_factor_secret TEXT,
			last_login_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS user_sessions (
			id TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL,
			token_hash TEXT NOT NULL,
			expires_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_agent TEXT,
			ip_address TEXT,
			FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS email_verifications (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			token TEXT NOT NULL,
			expires_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			used BOOLEAN DEFAULT 0,
			FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS two_factor_backup_codes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			code_hash TEXT NOT NULL,
			used BOOLEAN DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			used_at TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS password_reset_tokens (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			token TEXT NOT NULL,
			expires_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			used BOOLEAN DEFAULT 0,
			FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
		)`,
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
			key_type TEXT DEFAULT 'permanent',
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
		// Indexes for user authentication
		`CREATE INDEX IF NOT EXISTS idx_users_email ON users(email)`,
		`CREATE INDEX IF NOT EXISTS idx_user_sessions_user_id ON user_sessions(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_user_sessions_expires_at ON user_sessions(expires_at)`,
		`CREATE INDEX IF NOT EXISTS idx_email_verifications_user_id ON email_verifications(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_email_verifications_token ON email_verifications(token)`,
		`CREATE INDEX IF NOT EXISTS idx_two_factor_backup_codes_user_id ON two_factor_backup_codes(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_user_id ON password_reset_tokens(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_token ON password_reset_tokens(token)`,
		// Indexes for monitoring
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
}

// getCockroachInitQueries returns CockroachDB-specific initialization queries
func (db *DB) getCockroachInitQueries() []string {
	return []string{
		// User authentication tables
		`CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			email STRING UNIQUE NOT NULL,
			password_hash STRING NOT NULL,
			first_name STRING NOT NULL,
			last_name STRING NOT NULL,
			role STRING DEFAULT 'user',
			email_verified BOOL DEFAULT false,
			two_factor_enabled BOOL DEFAULT false,
			two_factor_secret STRING,
			last_login_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS user_sessions (
			id STRING PRIMARY KEY,
			user_id INT NOT NULL,
			token_hash STRING NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			user_agent STRING,
			ip_address STRING,
			FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS email_verifications (
			id SERIAL PRIMARY KEY,
			user_id INT NOT NULL,
			token STRING NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			used BOOL DEFAULT false,
			FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS two_factor_backup_codes (
			id SERIAL PRIMARY KEY,
			user_id INT NOT NULL,
			code_hash STRING NOT NULL,
			used BOOL DEFAULT false,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			used_at TIMESTAMPTZ,
			FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS password_reset_tokens (
			id SERIAL PRIMARY KEY,
			user_id INT NOT NULL,
			token STRING NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			used BOOL DEFAULT false,
			FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS sites (
			id SERIAL PRIMARY KEY,
			url STRING UNIQUE NOT NULL,
			name STRING NOT NULL,
			scan_interval STRING DEFAULT '60s',
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS site_checks (
			id SERIAL PRIMARY KEY,
			site_id INT NOT NULL,
			status STRING NOT NULL,
			response_time FLOAT,
			status_code INT,
			error_message STRING,
			checked_at TIMESTAMPTZ DEFAULT NOW(),
			FOREIGN KEY (site_id) REFERENCES sites (id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS agents (
			id SERIAL PRIMARY KEY,
			name STRING NOT NULL,
			api_key_hash STRING UNIQUE NOT NULL,
			key_type STRING DEFAULT 'permanent',
			description STRING,
			last_seen TIMESTAMPTZ,
			status STRING DEFAULT 'offline',
			os STRING,
			platform STRING,
			architecture STRING,
			version STRING,
			remote_ip STRING,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS monitor_tasks (
			id SERIAL PRIMARY KEY,
			site_id INT NOT NULL,
			monitor_type STRING NOT NULL,
			url STRING NOT NULL,
			interval STRING NOT NULL DEFAULT '60s',
			timeout STRING NOT NULL DEFAULT '10s',
			enabled BOOL NOT NULL DEFAULT true,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			FOREIGN KEY (site_id) REFERENCES sites (id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS monitor_results (
			id SERIAL PRIMARY KEY,
			task_id INT NOT NULL,
			agent_id INT NOT NULL,
			status STRING NOT NULL,
			response_time FLOAT,
			status_code INT,
			error_message STRING,
			metadata STRING,
			checked_at TIMESTAMPTZ DEFAULT NOW(),
			FOREIGN KEY (task_id) REFERENCES monitor_tasks (id) ON DELETE CASCADE,
			FOREIGN KEY (agent_id) REFERENCES agents (id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS agent_task_assignments (
			id SERIAL PRIMARY KEY,
			agent_id INT NOT NULL,
			task_id INT NOT NULL,
			assigned BOOL NOT NULL DEFAULT true,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			FOREIGN KEY (agent_id) REFERENCES agents (id) ON DELETE CASCADE,
			FOREIGN KEY (task_id) REFERENCES monitor_tasks (id) ON DELETE CASCADE,
			UNIQUE(agent_id, task_id)
		)`,
		// Indexes for user authentication
		`CREATE INDEX IF NOT EXISTS idx_users_email ON users(email)`,
		`CREATE INDEX IF NOT EXISTS idx_user_sessions_user_id ON user_sessions(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_user_sessions_expires_at ON user_sessions(expires_at)`,
		`CREATE INDEX IF NOT EXISTS idx_email_verifications_user_id ON email_verifications(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_email_verifications_token ON email_verifications(token)`,
		`CREATE INDEX IF NOT EXISTS idx_two_factor_backup_codes_user_id ON two_factor_backup_codes(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_user_id ON password_reset_tokens(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_token ON password_reset_tokens(token)`,
		// Indexes for monitoring
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
}

// migrate applies database migrations for existing databases
func (db *DB) migrate() error {
	// For SQLite, run the existing migration logic
	if db.dbType == SQLite {
		// Check if we need to add OS information columns to agents table
		if err := db.addOSInfoColumns(); err != nil {
			return fmt.Errorf("failed to add OS info columns: %w", err)
		}

		// Check if we need to add key_type column to agents table
		if err := db.addKeyTypeColumn(); err != nil {
			return fmt.Errorf("failed to add key_type column: %w", err)
		}
	}

	// For both databases, create monitoring tasks for existing sites
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

// addKeyTypeColumn adds the key_type column to the agents table if it doesn't exist
func (db *DB) addKeyTypeColumn() error {
	// Check if key_type column exists
	var count int
	err := db.conn.QueryRow("SELECT COUNT(*) FROM pragma_table_info('agents') WHERE name='key_type'").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check for key_type column: %w", err)
	}

	// If key_type column doesn't exist, add it
	if count == 0 {
		if _, err := db.conn.Exec("ALTER TABLE agents ADD COLUMN key_type TEXT DEFAULT 'permanent'"); err != nil {
			return fmt.Errorf("failed to add key_type column: %w", err)
		}
		fmt.Println("✅ Added key_type column to agents table")
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

// Helper methods for database-specific SQL

// placeholder returns the appropriate placeholder for the database type
func (db *DB) placeholder(n int) string {
	switch db.dbType {
	case SQLite:
		return "?"
	case CockroachDB:
		return fmt.Sprintf("$%d", n)
	default:
		return "?"
	}
}

// currentTimestamp returns the appropriate current timestamp expression
func (db *DB) currentTimestamp() string {
	switch db.dbType {
	case SQLite:
		return "CURRENT_TIMESTAMP"
	case CockroachDB:
		return "NOW()"
	default:
		return "CURRENT_TIMESTAMP"
	}
}

// boolValue returns the appropriate boolean value representation
func (db *DB) boolValue(b bool) interface{} {
	switch db.dbType {
	case SQLite:
		if b {
			return 1
		}
		return 0
	case CockroachDB:
		return b
	default:
		return b
	}
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
	} else if strings.HasPrefix(url, "log://") {
		monitorType = "log"
		timeout = "60s"
		// Keep the log:// prefix for the agent to handle
	} else {
		// Default to HTTP for unknown protocols
		monitorType = "http"
		timeout = "30s"
	}

	// Create the monitoring task with database-specific placeholders
	var query string
	switch db.dbType {
	case SQLite:
		query = `INSERT INTO monitor_tasks (site_id, monitor_type, url, interval, timeout, enabled) VALUES (?, ?, ?, ?, ?, ?)`
		_, err := db.conn.Exec(query, siteID, monitorType, url, interval, timeout, db.boolValue(true))
		return err
	case CockroachDB:
		query = `INSERT INTO monitor_tasks (site_id, monitor_type, url, interval, timeout, enabled) VALUES ($1, $2, $3, $4, $5, $6)`
		_, err := db.conn.Exec(query, siteID, monitorType, url, interval, timeout, db.boolValue(true))
		return err
	default:
		return fmt.Errorf("unsupported database type")
	}
}

// Sites

// AddSite adds a new site to monitor
func (db *DB) AddSite(site *models.SiteCreateRequest) (*models.Site, error) {
	var newSite models.Site
	newSite.URL = site.URL
	newSite.Name = site.Name
	newSite.ScanInterval = site.ScanInterval

	var err error
	switch db.dbType {
	case SQLite:
		query := `INSERT INTO sites (url, name, scan_interval) VALUES (?, ?, ?) RETURNING id, created_at`
		err = db.conn.QueryRow(query, site.URL, site.Name, site.ScanInterval).Scan(&newSite.ID, &newSite.CreatedAt)
	case CockroachDB:
		query := `INSERT INTO sites (url, name, scan_interval) VALUES ($1, $2, $3) RETURNING id, created_at`
		err = db.conn.QueryRow(query, site.URL, site.Name, site.ScanInterval).Scan(&newSite.ID, &newSite.CreatedAt)
	default:
		return nil, fmt.Errorf("unsupported database type")
	}

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
	var query string
	switch db.dbType {
	case SQLite:
		query = `SELECT id, url, name, scan_interval, created_at FROM sites WHERE id = ?`
	case CockroachDB:
		query = `SELECT id, url, name, scan_interval, created_at FROM sites WHERE id = $1`
	default:
		return nil, fmt.Errorf("unsupported database type")
	}

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
	var query string
	switch db.dbType {
	case SQLite:
		query = `DELETE FROM sites WHERE id = ?`
	case CockroachDB:
		query = `DELETE FROM sites WHERE id = $1`
	default:
		return fmt.Errorf("unsupported database type")
	}

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
	var query string
	switch db.dbType {
	case SQLite:
		query = `INSERT INTO site_checks (site_id, status, response_time, status_code, error_message) 
			  VALUES (?, ?, ?, ?, ?)`
	case CockroachDB:
		query = `INSERT INTO site_checks (site_id, status, response_time, status_code, error_message) 
			  VALUES ($1, $2, $3, $4, $5)`
	default:
		return fmt.Errorf("unsupported database type")
	}

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
	var query string
	switch db.dbType {
	case SQLite:
		query = `SELECT id, site_id, status, response_time, status_code, error_message, checked_at 
			  FROM site_checks WHERE site_id = ? ORDER BY checked_at DESC LIMIT ?`
	case CockroachDB:
		query = `SELECT id, site_id, status, response_time, status_code, error_message, checked_at 
			  FROM site_checks WHERE site_id = $1 ORDER BY checked_at DESC LIMIT $2`
	default:
		return nil, fmt.Errorf("unsupported database type")
	}

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
	// Determine key type based on registration type
	keyType := "permanent" // default
	if agent.RegistrationType != nil && *agent.RegistrationType == "auto" {
		keyType = "bootstrap"
	}

	var newAgent models.Agent
	newAgent.Name = agent.Name
	newAgent.APIKeyHash = apiKeyHash
	newAgent.Description = agent.Description
	newAgent.Status = "offline"

	var err error
	switch db.dbType {
	case SQLite:
		query := `INSERT INTO agents (name, api_key_hash, key_type, description) VALUES (?, ?, ?, ?) RETURNING id, created_at`
		err = db.conn.QueryRow(query, agent.Name, apiKeyHash, keyType, agent.Description).Scan(&newAgent.ID, &newAgent.CreatedAt)
	case CockroachDB:
		query := `INSERT INTO agents (name, api_key_hash, key_type, description) VALUES ($1, $2, $3, $4) RETURNING id, created_at`
		err = db.conn.QueryRow(query, agent.Name, apiKeyHash, keyType, agent.Description).Scan(&newAgent.ID, &newAgent.CreatedAt)
	default:
		return nil, fmt.Errorf("unsupported database type")
	}

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
	var query string
	switch db.dbType {
	case SQLite:
		query = `SELECT id, name, description, last_seen, status, os, platform, architecture, version, remote_ip, created_at FROM agents WHERE api_key_hash = ?`
	case CockroachDB:
		query = `SELECT id, name, description, last_seen, status, os, platform, architecture, version, remote_ip, created_at FROM agents WHERE api_key_hash = $1`
	default:
		return nil, fmt.Errorf("unsupported database type")
	}

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
	var query string
	switch db.dbType {
	case SQLite:
		query = `UPDATE agents SET status = ?, last_seen = CURRENT_TIMESTAMP, os = ?, platform = ?, architecture = ?, version = ? WHERE api_key_hash = ?`
	case CockroachDB:
		query = `UPDATE agents SET status = $1, last_seen = NOW(), os = $2, platform = $3, architecture = $4, version = $5 WHERE api_key_hash = $6`
	default:
		return fmt.Errorf("unsupported database type")
	}

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
	var query string
	switch db.dbType {
	case SQLite:
		query = `UPDATE agents SET status = ?, last_seen = CURRENT_TIMESTAMP, os = ?, platform = ?, architecture = ?, version = ?, remote_ip = ? WHERE api_key_hash = ?`
	case CockroachDB:
		query = `UPDATE agents SET status = $1, last_seen = NOW(), os = $2, platform = $3, architecture = $4, version = $5, remote_ip = $6 WHERE api_key_hash = $7`
	default:
		return fmt.Errorf("unsupported database type")
	}

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
	var query string
	switch db.dbType {
	case SQLite:
		query = `UPDATE agents SET status = ?, last_seen = CURRENT_TIMESTAMP WHERE api_key_hash = ?`
	case CockroachDB:
		query = `UPDATE agents SET status = $1, last_seen = NOW() WHERE api_key_hash = $2`
	default:
		return fmt.Errorf("unsupported database type")
	}

	_, err := db.conn.Exec(query, status, keyHash)
	if err != nil {
		return fmt.Errorf("failed to update agent status: %w", err)
	}

	return nil
}

// ValidateAgentAPIKey validates an agent API key hash
func (db *DB) ValidateAgentAPIKey(keyHash string) (bool, error) {
	var query string
	switch db.dbType {
	case SQLite:
		query = `SELECT COUNT(*) FROM agents WHERE api_key_hash = ?`
	case CockroachDB:
		query = `SELECT COUNT(*) FROM agents WHERE api_key_hash = $1`
	default:
		return false, fmt.Errorf("unsupported database type")
	}

	var count int
	err := db.conn.QueryRow(query, keyHash).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to validate agent API key: %w", err)
	}

	return count > 0, nil
}

// DeleteAgent deletes an agent
func (db *DB) DeleteAgent(id int) error {
	var query string
	switch db.dbType {
	case SQLite:
		query = `DELETE FROM agents WHERE id = ?`
	case CockroachDB:
		query = `DELETE FROM agents WHERE id = $1`
	default:
		return fmt.Errorf("unsupported database type")
	}

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

	// Verify the task exists before trying to insert
	var taskExists bool
	taskCheckQuery := `SELECT EXISTS(SELECT 1 FROM monitor_tasks WHERE id = ?)`
	err := db.conn.QueryRow(taskCheckQuery, result.TaskID).Scan(&taskExists)
	if err != nil {
		return fmt.Errorf("failed to check if task exists: %w", err)
	}
	if !taskExists {
		return fmt.Errorf("task_id %d does not exist in monitor_tasks table", result.TaskID)
	}

	// Verify the agent exists before trying to insert
	var agentExists bool
	agentCheckQuery := `SELECT EXISTS(SELECT 1 FROM agents WHERE id = ?)`
	err = db.conn.QueryRow(agentCheckQuery, agentID).Scan(&agentExists)
	if err != nil {
		return fmt.Errorf("failed to check if agent exists: %w", err)
	}
	if !agentExists {
		return fmt.Errorf("agent_id %d does not exist in agents table", agentID)
	}

	query := `INSERT INTO monitor_results (task_id, agent_id, status, response_time, status_code, error_message, metadata, checked_at) 
			  VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = db.conn.Exec(query, result.TaskID, agentID, result.Status, result.ResponseTime, result.StatusCode, result.ErrorMessage, metadataJSON, result.CheckedAt)
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

// GetAnalyticsData returns time-series monitoring data for analytics
func (db *DB) GetAnalyticsData(siteIDs []int, startTime time.Time, intervalMinutes int) (map[string]interface{}, error) {
	var query string
	var args []interface{}

	// Build the query using site_checks table to include error tracking
	if len(siteIDs) > 0 {
		placeholders := make([]string, len(siteIDs))
		for i, id := range siteIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		query = fmt.Sprintf(`
			SELECT 
				mt.site_id,
				s.name as site_name,
				s.url as site_url,
				mr.response_time,
				mr.status,
				mr.status_code,
				mr.error_message,
				mr.metadata,
				mr.checked_at,
				datetime(
					(strftime('%%s', mr.checked_at) / (%d * 60)) * (%d * 60),
					'unixepoch'
				) as time_bucket
			FROM monitor_results mr
			JOIN monitor_tasks mt ON mr.task_id = mt.id
			JOIN sites s ON mt.site_id = s.id
			WHERE mt.site_id IN (%s)
			AND mr.checked_at >= ?
			ORDER BY mr.checked_at DESC
		`, intervalMinutes, intervalMinutes, strings.Join(placeholders, ","))
		args = append(args, startTime)
	} else {
		query = fmt.Sprintf(`
			SELECT 
				mt.site_id,
				s.name as site_name,
				s.url as site_url,
				mr.response_time,
				mr.status,
				mr.status_code,
				mr.error_message,
				mr.metadata,
				mr.checked_at,
				datetime(
					(strftime('%%s', mr.checked_at) / (%d * 60)) * (%d * 60),
					'unixepoch'
				) as time_bucket
			FROM monitor_results mr
			JOIN monitor_tasks mt ON mr.task_id = mt.id
			JOIN sites s ON mt.site_id = s.id
			WHERE mr.checked_at >= ?
			ORDER BY mr.checked_at DESC
		`, intervalMinutes, intervalMinutes)
		args = append(args, startTime)
	}

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get analytics data: %w", err)
	}
	defer rows.Close()

	// Data structures for organizing results
	siteInfo := make(map[int]map[string]interface{})
	timeBuckets := make(map[string]map[string]interface{})

	// Track counts for error rate calculation
	bucketCounts := make(map[string]map[string]int) // [time_bucket][site_id] = {total, errors}

	for rows.Next() {
		var siteID int
		var siteName, siteURL, status, timeBucket string
		var responseTime *float64
		var statusCode *int
		var errorMessage *string
		var metadata *string
		var checkedAt time.Time

		err := rows.Scan(&siteID, &siteName, &siteURL, &responseTime, &status, &statusCode, &errorMessage, &metadata, &checkedAt, &timeBucket)
		if err != nil {
			return nil, fmt.Errorf("failed to scan analytics row: %w", err)
		}

		// Store site information
		if _, exists := siteInfo[siteID]; !exists {
			siteInfo[siteID] = map[string]interface{}{
				"id":                 siteID,
				"name":               siteName,
				"url":                siteURL,
				"last_status":        status,
				"last_response_time": responseTime,
				"last_status_code":   statusCode,
				"last_checked_at":    checkedAt.Format(time.RFC3339),
			}
		}

		// Initialize time bucket if not exists
		if _, exists := timeBuckets[timeBucket]; !exists {
			timeBuckets[timeBucket] = map[string]interface{}{
				"timestamp":      time.Unix(0, 0).Add(time.Duration(len(timeBuckets)*intervalMinutes) * time.Minute).Format("15:04"),
				"full_timestamp": timeBucket,
			}
		}

		// Initialize bucket counts if not exists
		if _, exists := bucketCounts[timeBucket]; !exists {
			bucketCounts[timeBucket] = make(map[string]int)
		}

		siteKey := fmt.Sprintf("site_%d", siteID)
		siteErrorKey := fmt.Sprintf("site_%d_errors", siteID)
		siteTotalKey := fmt.Sprintf("site_%d_total", siteID)
		siteErrorRateKey := fmt.Sprintf("site_%d_error_rate", siteID)

		// Count total checks and errors for this site in this time bucket
		if _, exists := bucketCounts[timeBucket][siteTotalKey]; !exists {
			bucketCounts[timeBucket][siteTotalKey] = 0
			bucketCounts[timeBucket][siteErrorKey] = 0
		}
		bucketCounts[timeBucket][siteTotalKey]++

		// Check if this is a log monitoring task with metadata error rate
		var logErrorRate *float64
		var logAvgResponseTime *float64
		if metadata != nil && *metadata != "" {
			var metadataMap map[string]interface{}
			if err := json.Unmarshal([]byte(*metadata), &metadataMap); err == nil {
				if errorRate, exists := metadataMap["error_rate"]; exists {
					if rate, ok := errorRate.(float64); ok {
						logErrorRate = &rate
					}
				}
				if avgResponseTime, exists := metadataMap["avg_response_time"]; exists {
					if responseTime, ok := avgResponseTime.(float64); ok {
						logAvgResponseTime = &responseTime
					}
				}
			}
		}

		// For log monitoring, use the metadata values directly
		if logErrorRate != nil {
			// Use the actual error rate from log analysis
			timeBuckets[timeBucket][siteErrorRateKey] = *logErrorRate
			
			// Also set the average response time from log analysis
			if logAvgResponseTime != nil {
				timeBuckets[timeBucket][siteKey] = *logAvgResponseTime
			}
			
			// Debug logging for nginx log monitoring
			if strings.Contains(strings.ToLower(siteURL), "nginx") || strings.Contains(strings.ToLower(siteName), "nginx") {
				log.Debug().
					Int("site_id", siteID).
					Str("site_name", siteName).
					Str("site_url", siteURL).
					Str("status", status).
					Float64("log_error_rate", *logErrorRate).
					Interface("log_avg_response_time", logAvgResponseTime).
					Str("time_bucket", timeBucket).
					Msg("Analytics: Using log metadata values")
			}
		} else {
			// For non-log monitoring, determine error based on status/status_code
			isError := false
			if status == "down" {
				isError = true
			} else if statusCode != nil && (*statusCode >= 400 && *statusCode < 600) {
				isError = true
			}

			if isError {
				bucketCounts[timeBucket][siteErrorKey]++
			}
		}

		// Add response time for successful checks
		if responseTime != nil && status == "up" {
			// Use the latest/most recent value in the time bucket for response time
			timeBuckets[timeBucket][siteKey] = *responseTime
		}
	}

	// Calculate error rates and add to time buckets (only for non-log monitoring)
	for timeBucket, bucket := range timeBuckets {
		for siteIDStr := range bucketCounts[timeBucket] {
			if strings.HasSuffix(siteIDStr, "_total") {
				siteKey := strings.TrimSuffix(siteIDStr, "_total")
				errorKey := siteKey + "_errors"
				siteErrorRateKey := siteKey + "_error_rate"

				// Skip if error rate was already set from log metadata
				if _, exists := bucket[siteErrorRateKey]; exists {
					continue
				}

				totalChecks := bucketCounts[timeBucket][siteIDStr]
				errorChecks := bucketCounts[timeBucket][errorKey]

				if totalChecks > 0 {
					errorRate := float64(errorChecks) / float64(totalChecks) * 100.0
					bucket[siteErrorRateKey] = errorRate
				}
			}
		}
	}

	// Convert time buckets to sorted slice
	var timestamps []string
	for timestamp := range timeBuckets {
		timestamps = append(timestamps, timestamp)
	}

	// Sort timestamps
	sort.Strings(timestamps)

	// Build final data points array
	var dataPoints []map[string]interface{}
	for _, timestamp := range timestamps {
		dataPoints = append(dataPoints, timeBuckets[timestamp])
	}

	// Calculate averages for "all" view
	for _, point := range dataPoints {
		var responseTimeSum float64
		var responseTimeCount int
		var errorRateSum float64
		var errorRateCount int

		for key, value := range point {
			if strings.HasPrefix(key, "site_") && !strings.HasSuffix(key, "_error_rate") {
				// Response time values
				if val, ok := value.(float64); ok {
					responseTimeSum += val
					responseTimeCount++
				}
			} else if strings.HasSuffix(key, "_error_rate") {
				// Error rate values
				if val, ok := value.(float64); ok {
					errorRateSum += val
					errorRateCount++
				}
			}
		}

		if responseTimeCount > 0 {
			point["average"] = responseTimeSum / float64(responseTimeCount)
		} else {
			point["average"] = nil
		}

		if errorRateCount > 0 {
			point["average_error_rate"] = errorRateSum / float64(errorRateCount)
		} else {
			point["average_error_rate"] = 0.0
		}
	}

	// Convert site info to slice
	var sites []map[string]interface{}
	for _, info := range siteInfo {
		sites = append(sites, info)
	}

	// Sort sites by ID for consistency
	sort.Slice(sites, func(i, j int) bool {
		return sites[i]["id"].(int) < sites[j]["id"].(int)
	})

	return map[string]interface{}{
		"data":  dataPoints,
		"sites": sites,
	}, nil
}

// UpgradeAgentKey upgrades an agent from bootstrap to permanent key
func (db *DB) UpgradeAgentKey(currentKeyHash, newKeyHash string) error {
	// Update the agent's API key hash and set key_type to permanent
	query := `UPDATE agents SET api_key_hash = ?, key_type = 'permanent' WHERE api_key_hash = ? AND key_type = 'bootstrap'`

	result, err := db.conn.Exec(query, newKeyHash, currentKeyHash)
	if err != nil {
		return fmt.Errorf("failed to upgrade agent key: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("no bootstrap agent found with provided key")
	}

	return nil
}

// IsBootstrapKey checks if the given key hash belongs to a bootstrap agent
func (db *DB) IsBootstrapKey(keyHash string) (bool, error) {
	query := `SELECT COUNT(*) FROM agents WHERE api_key_hash = ? AND key_type = 'bootstrap'`

	var count int
	err := db.conn.QueryRow(query, keyHash).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check if key is bootstrap: %w", err)
	}

	return count > 0, nil
}

// GetAgentByKeyHashWithType returns an agent by API key hash including key type
func (db *DB) GetAgentByKeyHashWithType(keyHash string) (*models.Agent, string, error) {
	query := `SELECT id, name, description, last_seen, status, os, platform, architecture, version, remote_ip, created_at, key_type FROM agents WHERE api_key_hash = ?`

	var agent models.Agent
	var keyType string
	err := db.conn.QueryRow(query, keyHash).Scan(&agent.ID, &agent.Name, &agent.Description, &agent.LastSeen, &agent.Status,
		&agent.OS, &agent.Platform, &agent.Architecture, &agent.Version, &agent.RemoteIP, &agent.CreatedAt, &keyType)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, "", nil
		}
		return nil, "", fmt.Errorf("failed to get agent: %w", err)
	}

	return &agent, keyType, nil
}

// User authentication methods

// CreateUser creates a new user account
func (db *DB) CreateUser(req *models.UserRegistrationRequest, passwordHash string) (*models.User, error) {
	var newUser models.User
	newUser.Email = req.Email
	newUser.PasswordHash = passwordHash
	newUser.FirstName = req.FirstName
	newUser.LastName = req.LastName
	newUser.Role = "user" // Default role
	newUser.EmailVerified = false
	newUser.TwoFactorEnabled = false

	var err error
	switch db.dbType {
	case SQLite:
		query := `INSERT INTO users (email, password_hash, first_name, last_name, role, email_verified, two_factor_enabled) 
				  VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id, created_at, updated_at`
		err = db.conn.QueryRow(query, req.Email, passwordHash, req.FirstName, req.LastName, "user",
			db.boolValue(false), db.boolValue(false)).Scan(&newUser.ID, &newUser.CreatedAt, &newUser.UpdatedAt)
	case CockroachDB:
		query := `INSERT INTO users (email, password_hash, first_name, last_name, role, email_verified, two_factor_enabled) 
				  VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, created_at, updated_at`
		err = db.conn.QueryRow(query, req.Email, passwordHash, req.FirstName, req.LastName, "user",
			db.boolValue(false), db.boolValue(false)).Scan(&newUser.ID, &newUser.CreatedAt, &newUser.UpdatedAt)
	default:
		return nil, fmt.Errorf("unsupported database type")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return &newUser, nil
}

// GetUserByEmail returns a user by email address
func (db *DB) GetUserByEmail(email string) (*models.User, error) {
	var query string
	switch db.dbType {
	case SQLite:
		query = `SELECT id, email, password_hash, first_name, last_name, role, email_verified, two_factor_enabled, 
				 two_factor_secret, last_login_at, created_at, updated_at FROM users WHERE email = ?`
	case CockroachDB:
		query = `SELECT id, email, password_hash, first_name, last_name, role, email_verified, two_factor_enabled, 
				 two_factor_secret, last_login_at, created_at, updated_at FROM users WHERE email = $1`
	default:
		return nil, fmt.Errorf("unsupported database type")
	}

	var user models.User
	err := db.conn.QueryRow(query, email).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.FirstName,
		&user.LastName, &user.Role, &user.EmailVerified, &user.TwoFactorEnabled, &user.TwoFactorSecret,
		&user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &user, nil
}

// GetUserByID returns a user by ID
func (db *DB) GetUserByID(id int) (*models.User, error) {
	var query string
	switch db.dbType {
	case SQLite:
		query = `SELECT id, email, password_hash, first_name, last_name, role, email_verified, two_factor_enabled, 
				 two_factor_secret, last_login_at, created_at, updated_at FROM users WHERE id = ?`
	case CockroachDB:
		query = `SELECT id, email, password_hash, first_name, last_name, role, email_verified, two_factor_enabled, 
				 two_factor_secret, last_login_at, created_at, updated_at FROM users WHERE id = $1`
	default:
		return nil, fmt.Errorf("unsupported database type")
	}

	var user models.User
	err := db.conn.QueryRow(query, id).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.FirstName,
		&user.LastName, &user.Role, &user.EmailVerified, &user.TwoFactorEnabled, &user.TwoFactorSecret,
		&user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &user, nil
}

// CreateUserSession creates a new user session
func (db *DB) CreateUserSession(userID int, sessionID, tokenHash string, expiresAt time.Time, userAgent, ipAddress *string) error {
	var query string
	switch db.dbType {
	case SQLite:
		query = `INSERT INTO user_sessions (id, user_id, token_hash, expires_at, user_agent, ip_address) 
				 VALUES (?, ?, ?, ?, ?, ?)`
	case CockroachDB:
		query = `INSERT INTO user_sessions (id, user_id, token_hash, expires_at, user_agent, ip_address) 
				 VALUES ($1, $2, $3, $4, $5, $6)`
	default:
		return fmt.Errorf("unsupported database type")
	}

	_, err := db.conn.Exec(query, sessionID, userID, tokenHash, expiresAt, userAgent, ipAddress)
	if err != nil {
		return fmt.Errorf("failed to create user session: %w", err)
	}

	return nil
}

// GetUserSession returns a user session by token hash
func (db *DB) GetUserSession(tokenHash string) (*models.UserSession, error) {
	var query string
	switch db.dbType {
	case SQLite:
		query = `SELECT id, user_id, token_hash, expires_at, created_at, user_agent, ip_address 
				 FROM user_sessions WHERE token_hash = ? AND expires_at > ?`
	case CockroachDB:
		query = `SELECT id, user_id, token_hash, expires_at, created_at, user_agent, ip_address 
				 FROM user_sessions WHERE token_hash = $1 AND expires_at > $2`
	default:
		return nil, fmt.Errorf("unsupported database type")
	}

	var session models.UserSession
	err := db.conn.QueryRow(query, tokenHash, time.Now()).Scan(&session.ID, &session.UserID, &session.Token,
		&session.ExpiresAt, &session.CreatedAt, &session.UserAgent, &session.IPAddress)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user session: %w", err)
	}

	return &session, nil
}

// DeleteUserSession deletes a user session
func (db *DB) DeleteUserSession(sessionID string) error {
	var query string
	switch db.dbType {
	case SQLite:
		query = `DELETE FROM user_sessions WHERE id = ?`
	case CockroachDB:
		query = `DELETE FROM user_sessions WHERE id = $1`
	default:
		return fmt.Errorf("unsupported database type")
	}

	_, err := db.conn.Exec(query, sessionID)
	if err != nil {
		return fmt.Errorf("failed to delete user session: %w", err)
	}

	return nil
}

// CleanupExpiredSessions removes expired sessions
func (db *DB) CleanupExpiredSessions() error {
	var query string
	switch db.dbType {
	case SQLite:
		query = `DELETE FROM user_sessions WHERE expires_at <= ?`
	case CockroachDB:
		query = `DELETE FROM user_sessions WHERE expires_at <= $1`
	default:
		return fmt.Errorf("unsupported database type")
	}

	_, err := db.conn.Exec(query, time.Now())
	if err != nil {
		return fmt.Errorf("failed to cleanup expired sessions: %w", err)
	}

	return nil
}

// CreateEmailVerification creates an email verification token
func (db *DB) CreateEmailVerification(userID int, token string, expiresAt time.Time) error {
	var query string
	switch db.dbType {
	case SQLite:
		query = `INSERT INTO email_verifications (user_id, token, expires_at) VALUES (?, ?, ?)`
	case CockroachDB:
		query = `INSERT INTO email_verifications (user_id, token, expires_at) VALUES ($1, $2, $3)`
	default:
		return fmt.Errorf("unsupported database type")
	}

	_, err := db.conn.Exec(query, userID, token, expiresAt)
	if err != nil {
		return fmt.Errorf("failed to create email verification: %w", err)
	}

	return nil
}

// VerifyEmail marks an email as verified using the verification token
func (db *DB) VerifyEmail(token string) error {
	// First, find the verification record
	var userID int
	var query string
	switch db.dbType {
	case SQLite:
		query = `SELECT user_id FROM email_verifications WHERE token = ? AND expires_at > ? AND used = ?`
	case CockroachDB:
		query = `SELECT user_id FROM email_verifications WHERE token = $1 AND expires_at > $2 AND used = $3`
	default:
		return fmt.Errorf("unsupported database type")
	}

	err := db.conn.QueryRow(query, token, time.Now(), db.boolValue(false)).Scan(&userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("invalid or expired verification token")
		}
		return fmt.Errorf("failed to find verification token: %w", err)
	}

	// Start transaction
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Mark verification as used
	switch db.dbType {
	case SQLite:
		query = `UPDATE email_verifications SET used = ? WHERE token = ?`
	case CockroachDB:
		query = `UPDATE email_verifications SET used = $1 WHERE token = $2`
	}

	_, err = tx.Exec(query, db.boolValue(true), token)
	if err != nil {
		return fmt.Errorf("failed to mark verification as used: %w", err)
	}

	// Mark user email as verified
	switch db.dbType {
	case SQLite:
		query = `UPDATE users SET email_verified = ?, updated_at = ? WHERE id = ?`
	case CockroachDB:
		query = `UPDATE users SET email_verified = $1, updated_at = $2 WHERE id = $3`
	}

	_, err = tx.Exec(query, db.boolValue(true), time.Now(), userID)
	if err != nil {
		return fmt.Errorf("failed to mark email as verified: %w", err)
	}

	return tx.Commit()
}

// UpdateLastLogin updates the user's last login timestamp
func (db *DB) UpdateLastLogin(userID int) error {
	var query string
	switch db.dbType {
	case SQLite:
		query = `UPDATE users SET last_login_at = ?, updated_at = ? WHERE id = ?`
	case CockroachDB:
		query = `UPDATE users SET last_login_at = $1, updated_at = $2 WHERE id = $3`
	default:
		return fmt.Errorf("unsupported database type")
	}

	now := time.Now()
	_, err := db.conn.Exec(query, now, now, userID)
	if err != nil {
		return fmt.Errorf("failed to update last login: %w", err)
	}

	return nil
}

// ValidateMasterAPIKey validates the master API key (preserves emergency access)
func (db *DB) ValidateMasterAPIKey(apiKey, masterKey string) bool {
	return apiKey == masterKey
}
