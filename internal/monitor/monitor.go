package monitor

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/x86txt/sreootb/internal/config"
	"github.com/x86txt/sreootb/internal/database"
	"github.com/x86txt/sreootb/internal/models"
)

// Monitor handles site monitoring
type Monitor struct {
	db     *database.DB
	config *config.Config
	sites  map[int]*monitoredSite
	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

type monitoredSite struct {
	site   *models.Site
	ticker *time.Ticker
	stop   chan struct{}
}

// New creates a new monitor instance
func New(db *database.DB, cfg *config.Config) *Monitor {
	ctx, cancel := context.WithCancel(context.Background())

	return &Monitor{
		db:     db,
		config: cfg,
		sites:  make(map[int]*monitoredSite),
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start starts the monitoring process
func (m *Monitor) Start() error {
	log.Info().Msg("Starting site monitor")

	// Load existing sites from database
	sites, err := m.db.GetSites()
	if err != nil {
		return fmt.Errorf("failed to load sites: %w", err)
	}

	// Start monitoring each site
	for _, site := range sites {
		if err := m.addSiteMonitoring(site); err != nil {
			log.Error().Err(err).Int("site_id", site.ID).Str("url", site.URL).Msg("Failed to start monitoring site")
		}
	}

	log.Info().Int("sites", len(sites)).Msg("Site monitor started")
	return nil
}

// Stop stops the monitoring process
func (m *Monitor) Stop() {
	log.Info().Msg("Stopping site monitor")

	m.cancel()

	m.mu.Lock()
	for _, monitored := range m.sites {
		monitored.ticker.Stop()
		close(monitored.stop)
	}
	m.sites = make(map[int]*monitoredSite)
	m.mu.Unlock()

	m.wg.Wait()
	log.Info().Msg("Site monitor stopped")
}

// RefreshMonitoring reloads sites from database and updates monitoring
func (m *Monitor) RefreshMonitoring() error {
	sites, err := m.db.GetSites()
	if err != nil {
		return fmt.Errorf("failed to load sites: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Build map of current site IDs
	currentSites := make(map[int]bool)
	for _, site := range sites {
		currentSites[site.ID] = true
	}

	// Stop monitoring for sites that no longer exist
	for siteID, monitored := range m.sites {
		if !currentSites[siteID] {
			monitored.ticker.Stop()
			close(monitored.stop)
			delete(m.sites, siteID)
		}
	}

	// Start monitoring for new sites
	for _, site := range sites {
		if _, exists := m.sites[site.ID]; !exists {
			if err := m.addSiteMonitoringLocked(site); err != nil {
				log.Error().Err(err).Int("site_id", site.ID).Str("url", site.URL).Msg("Failed to start monitoring site")
			}
		}
	}

	return nil
}

// CheckSitesByID manually checks specific sites or all sites if siteIDs is nil
func (m *Monitor) CheckSitesByID(siteIDs []int) ([]models.SiteCheck, error) {
	var sitesToCheck []*models.Site

	if siteIDs == nil {
		// Check all sites
		sites, err := m.db.GetSites()
		if err != nil {
			return nil, fmt.Errorf("failed to get sites: %w", err)
		}
		sitesToCheck = sites
	} else {
		// Check specific sites
		for _, siteID := range siteIDs {
			site, err := m.db.GetSite(siteID)
			if err != nil {
				return nil, fmt.Errorf("failed to get site %d: %w", siteID, err)
			}
			if site != nil {
				sitesToCheck = append(sitesToCheck, site)
			}
		}
	}

	var results []models.SiteCheck
	for _, site := range sitesToCheck {
		check := m.checkSite(site)
		if err := m.db.RecordCheck(&check); err != nil {
			log.Error().Err(err).Int("site_id", site.ID).Msg("Failed to record check result")
		}
		results = append(results, check)
	}

	return results, nil
}

// GetStats returns monitoring statistics
func (m *Monitor) GetStats() (*models.MonitorStats, error) {
	return m.db.GetMonitorStats()
}

// addSiteMonitoring adds monitoring for a site (with locking)
func (m *Monitor) addSiteMonitoring(site *models.Site) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.addSiteMonitoringLocked(site)
}

// addSiteMonitoringLocked adds monitoring for a site (without locking)
func (m *Monitor) addSiteMonitoringLocked(site *models.Site) error {
	// Parse scan interval
	interval, err := parseScanInterval(site.ScanInterval)
	if err != nil {
		return fmt.Errorf("invalid scan interval for site %d: %w", site.ID, err)
	}

	// Create ticker and monitoring goroutine
	ticker := time.NewTicker(interval)
	stop := make(chan struct{})

	monitored := &monitoredSite{
		site:   site,
		ticker: ticker,
		stop:   stop,
	}

	m.sites[site.ID] = monitored

	// Start monitoring goroutine
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()

		// Initial check
		check := m.checkSite(site)
		if err := m.db.RecordCheck(&check); err != nil {
			log.Error().Err(err).Int("site_id", site.ID).Msg("Failed to record initial check")
		}

		// Periodic checks
		for {
			select {
			case <-ticker.C:
				check := m.checkSite(site)
				if err := m.db.RecordCheck(&check); err != nil {
					log.Error().Err(err).Int("site_id", site.ID).Msg("Failed to record check")
				}
			case <-stop:
				return
			case <-m.ctx.Done():
				return
			}
		}
	}()

	log.Debug().Int("site_id", site.ID).Str("url", site.URL).Dur("interval", interval).Msg("Started monitoring site")
	return nil
}

// checkSite performs a single check of a site
func (m *Monitor) checkSite(site *models.Site) models.SiteCheck {
	check := models.SiteCheck{
		SiteID:    site.ID,
		CheckedAt: time.Now(),
	}

	if strings.HasPrefix(site.URL, "ping://") {
		// Ping check
		host := site.URL[7:] // Remove 'ping://'
		if m.pingHost(host) {
			check.Status = "up"
		} else {
			check.Status = "down"
			errorMsg := "ping failed"
			check.ErrorMessage = &errorMsg
		}
	} else {
		// HTTP check
		start := time.Now()
		resp, err := m.httpCheck(site.URL)
		duration := time.Since(start)

		responseTime := duration.Seconds()
		check.ResponseTime = &responseTime

		if err != nil {
			check.Status = "down"
			errorMsg := err.Error()
			check.ErrorMessage = &errorMsg
		} else {
			statusCode := resp.StatusCode
			check.StatusCode = &statusCode

			if resp.StatusCode >= 200 && resp.StatusCode < 400 {
				check.Status = "up"
			} else {
				check.Status = "down"
				errorMsg := fmt.Sprintf("HTTP %d", resp.StatusCode)
				check.ErrorMessage = &errorMsg
			}
			resp.Body.Close()
		}
	}

	log.Debug().
		Int("site_id", site.ID).
		Str("url", site.URL).
		Str("status", check.Status).
		Interface("response_time", check.ResponseTime).
		Interface("status_code", check.StatusCode).
		Msg("Site check completed")

	return check
}

// httpCheck performs an HTTP check
func (m *Monitor) httpCheck(url string) (*http.Response, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 10 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout: 10 * time.Second,
		},
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "SREootb-Monitor/2.0")

	return client.Do(req)
}

// pingHost performs a ping check
func (m *Monitor) pingHost(host string) bool {
	// Use system ping command for better reliability
	cmd := exec.Command("ping", "-c", "1", "-W", "5", host)
	err := cmd.Run()
	return err == nil
}

// parseScanInterval parses a scan interval string into a time.Duration
func parseScanInterval(interval string) (time.Duration, error) {
	interval = strings.ToLower(strings.TrimSpace(interval))

	// Parse the format like "30s", "5m", "1h"
	if len(interval) < 2 {
		return 0, fmt.Errorf("invalid interval format")
	}

	unit := interval[len(interval)-1:]
	valueStr := interval[:len(interval)-1]

	// Parse the numeric value
	var multiplier time.Duration
	switch unit {
	case "s":
		multiplier = time.Second
	case "m":
		multiplier = time.Minute
	case "h":
		multiplier = time.Hour
	default:
		return 0, fmt.Errorf("invalid time unit: %s", unit)
	}

	// Parse the value
	var value float64
	if _, err := fmt.Sscanf(valueStr, "%f", &value); err != nil {
		return 0, fmt.Errorf("invalid numeric value: %s", valueStr)
	}

	return time.Duration(float64(multiplier) * value), nil
}
