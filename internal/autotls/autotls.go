package autotls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// Config holds auto-TLS configuration
type Config struct {
	CertDir     string   // Directory to store certificates
	CommonName  string   // Common name for the certificate
	DNSNames    []string // DNS names to include in SAN
	IPAddresses []net.IP // IP addresses to include in SAN
	CertFile    string   // Generated certificate file path
	KeyFile     string   // Generated private key file path
}

// Manager handles auto-TLS certificate generation and management
type Manager struct {
	config Config
}

// New creates a new auto-TLS manager
func New(cfg Config) *Manager {
	// Set defaults
	if cfg.CertDir == "" {
		cfg.CertDir = "./certs"
	}
	if cfg.CommonName == "" {
		cfg.CommonName = "sreootb.local"
	}
	if cfg.CertFile == "" {
		cfg.CertFile = filepath.Join(cfg.CertDir, "cert.pem")
	}
	if cfg.KeyFile == "" {
		cfg.KeyFile = filepath.Join(cfg.CertDir, "key.pem")
	}

	// Add default DNS names if none provided
	if len(cfg.DNSNames) == 0 {
		cfg.DNSNames = []string{
			cfg.CommonName,
			"localhost",
			"sreootb.local",
			"sreootb.test",
			"*.sreootb.local",
			"*.sreootb.test",
		}
	}

	// Add default IP addresses if none provided
	if len(cfg.IPAddresses) == 0 {
		cfg.IPAddresses = []net.IP{
			net.IPv4(127, 0, 0, 1), // localhost
			net.IPv6loopback,       // ::1
			net.IPv4(0, 0, 0, 0),   // 0.0.0.0 (all interfaces)
		}
	}

	return &Manager{config: cfg}
}

// GetCertificate returns a TLS certificate, generating one if needed
func (m *Manager) GetCertificate() (tls.Certificate, error) {
	// Check if certificate files exist and are valid
	if m.certificateExists() && m.certificateValid() {
		log.Info().
			Str("cert_file", m.config.CertFile).
			Str("key_file", m.config.KeyFile).
			Msg("Loading existing auto-TLS certificate")

		return tls.LoadX509KeyPair(m.config.CertFile, m.config.KeyFile)
	}

	// Generate new certificate
	log.Info().
		Str("cert_dir", m.config.CertDir).
		Str("common_name", m.config.CommonName).
		Strs("dns_names", m.config.DNSNames).
		Msg("Generating new auto-TLS certificate with ECDSA P-256 key")

	if err := m.generateCertificate(); err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to generate certificate: %w", err)
	}

	return tls.LoadX509KeyPair(m.config.CertFile, m.config.KeyFile)
}

// certificateExists checks if certificate files exist
func (m *Manager) certificateExists() bool {
	_, certErr := os.Stat(m.config.CertFile)
	_, keyErr := os.Stat(m.config.KeyFile)
	return certErr == nil && keyErr == nil
}

// certificateValid checks if the existing certificate is still valid
func (m *Manager) certificateValid() bool {
	cert, err := tls.LoadX509KeyPair(m.config.CertFile, m.config.KeyFile)
	if err != nil {
		return false
	}

	if len(cert.Certificate) == 0 {
		return false
	}

	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return false
	}

	// Check if certificate expires within 30 days
	return time.Until(x509Cert.NotAfter) > 30*24*time.Hour
}

// generateCertificate creates a new ECDSA P-256 certificate
func (m *Manager) generateCertificate() error {
	// Create certificate directory
	if err := os.MkdirAll(m.config.CertDir, 0755); err != nil {
		return fmt.Errorf("failed to create certificate directory: %w", err)
	}

	// Generate ECDSA P-256 private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate ECDSA private key: %w", err)
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:       []string{"SREootb Auto-TLS"},
			OrganizationalUnit: []string{"Development"},
			Country:            []string{"US"},
			Province:           []string{""},
			Locality:           []string{""},
			CommonName:         m.config.CommonName,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour), // 1 year
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              m.config.DNSNames,
		IPAddresses:           m.config.IPAddresses,
	}

	// Generate certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return fmt.Errorf("failed to create certificate: %w", err)
	}

	// Save certificate
	certOut, err := os.Create(m.config.CertFile)
	if err != nil {
		return fmt.Errorf("failed to create certificate file: %w", err)
	}
	defer certOut.Close()

	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		return fmt.Errorf("failed to write certificate: %w", err)
	}

	// Save private key
	keyOut, err := os.Create(m.config.KeyFile)
	if err != nil {
		return fmt.Errorf("failed to create private key file: %w", err)
	}
	defer keyOut.Close()

	// Set restrictive permissions on private key
	if err := keyOut.Chmod(0600); err != nil {
		log.Warn().Err(err).Msg("Failed to set private key permissions")
	}

	// Convert ECDSA private key to PKCS#8 format
	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("failed to marshal private key: %w", err)
	}

	if err := pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyBytes}); err != nil {
		return fmt.Errorf("failed to write private key: %w", err)
	}

	log.Info().
		Str("cert_file", m.config.CertFile).
		Str("key_file", m.config.KeyFile).
		Strs("dns_names", m.config.DNSNames).
		Time("not_after", template.NotAfter).
		Msg("Generated new ECDSA P-256 auto-TLS certificate")

	return nil
}

// GetCertificateInfo returns information about the current certificate
func (m *Manager) GetCertificateInfo() (map[string]interface{}, error) {
	if !m.certificateExists() {
		return map[string]interface{}{
			"exists":    false,
			"generated": false,
		}, nil
	}

	cert, err := tls.LoadX509KeyPair(m.config.CertFile, m.config.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load certificate: %w", err)
	}

	if len(cert.Certificate) == 0 {
		return nil, fmt.Errorf("no certificate found")
	}

	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	return map[string]interface{}{
		"exists":               true,
		"generated":            true,
		"common_name":          x509Cert.Subject.CommonName,
		"dns_names":            x509Cert.DNSNames,
		"ip_addresses":         ipAddressesToStrings(x509Cert.IPAddresses),
		"not_before":           x509Cert.NotBefore,
		"not_after":            x509Cert.NotAfter,
		"is_ca":                x509Cert.IsCA,
		"serial_number":        x509Cert.SerialNumber.String(),
		"signature_algorithm":  x509Cert.SignatureAlgorithm.String(),
		"public_key_algorithm": x509Cert.PublicKeyAlgorithm.String(),
		"cert_file":            m.config.CertFile,
		"key_file":             m.config.KeyFile,
		"expires_in_days":      int(time.Until(x509Cert.NotAfter).Hours() / 24),
	}, nil
}

// ipAddressesToStrings converts IP addresses to strings
func ipAddressesToStrings(ips []net.IP) []string {
	var result []string
	for _, ip := range ips {
		result = append(result, ip.String())
	}
	return result
}

// GetAutoTLSConfig creates auto-TLS configuration from server bind addresses
func GetAutoTLSConfig(webBind, agentBind string) Config {
	dnsNames := []string{
		"localhost",
		"sreootb.local",
		"sreootb.test",
		"*.sreootb.local",
		"*.sreootb.test",
	}

	ipAddresses := []net.IP{
		net.IPv4(127, 0, 0, 1), // localhost
		net.IPv6loopback,       // ::1
	}

	// Extract hostnames/IPs from bind addresses
	for _, bind := range []string{webBind, agentBind} {
		if bind == "" {
			continue
		}

		host, _, err := net.SplitHostPort(bind)
		if err != nil {
			continue
		}

		// If it's an IP address, add it to IP addresses
		if ip := net.ParseIP(host); ip != nil {
			// Don't add 0.0.0.0 or :: as they're already covered
			if !ip.IsUnspecified() {
				ipAddresses = append(ipAddresses, ip)
			}
		} else if host != "" && host != "localhost" {
			// Add as DNS name if it's not localhost (already included)
			found := false
			for _, existing := range dnsNames {
				if existing == host || strings.HasSuffix(existing, "."+host) {
					found = true
					break
				}
			}
			if !found {
				dnsNames = append(dnsNames, host)
			}
		}
	}

	// Add 0.0.0.0 for all interfaces binding
	ipAddresses = append(ipAddresses, net.IPv4(0, 0, 0, 0))

	return Config{
		CertDir:     "./certs",
		CommonName:  "sreootb.local",
		DNSNames:    dnsNames,
		IPAddresses: ipAddresses,
	}
}
