package utils

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"fmt"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

// HashPassword hashes a password using bcrypt with a strong cost
func HashPassword(password string) (string, error) {
	// Use cost 12 for strong security (takes ~300ms on modern hardware)
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(hash), nil
}

// VerifyPassword verifies a password against its hash
func VerifyPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// GenerateSecureToken generates a cryptographically secure random token
func GenerateSecureToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate secure token: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// GenerateSessionToken generates a secure session token
func GenerateSessionToken() (string, error) {
	return GenerateSecureToken(32) // 64 character hex string
}

// HashSessionToken hashes a session token for storage
func HashSessionToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// GenerateEmailVerificationToken generates a secure email verification token
func GenerateEmailVerificationToken() (string, error) {
	return GenerateSecureToken(32) // 64 character hex string
}

// GenerateTOTPSecret generates a new TOTP secret for 2FA
func GenerateTOTPSecret() (string, error) {
	secret := make([]byte, 20) // 160 bits
	if _, err := rand.Read(secret); err != nil {
		return "", fmt.Errorf("failed to generate TOTP secret: %w", err)
	}
	return base32.StdEncoding.EncodeToString(secret), nil
}

// GenerateTOTPQRCodeURL generates a QR code URL for TOTP setup
func GenerateTOTPQRCodeURL(secret, email, issuer string) (string, error) {
	key, err := otp.NewKeyFromURL(fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s",
		issuer, email, secret, issuer))
	if err != nil {
		return "", fmt.Errorf("failed to generate TOTP key: %w", err)
	}
	return key.URL(), nil
}

// ValidateTOTPCode validates a TOTP code against a secret
func ValidateTOTPCode(code, secret string) bool {
	return totp.Validate(code, secret)
}

// GenerateBackupCodes generates backup codes for 2FA
func GenerateBackupCodes(count int) ([]string, error) {
	codes := make([]string, count)
	for i := 0; i < count; i++ {
		// Generate 8-character backup codes
		code, err := GenerateSecureToken(4) // 8 character hex string
		if err != nil {
			return nil, fmt.Errorf("failed to generate backup code: %w", err)
		}
		codes[i] = code
	}
	return codes, nil
}

// HashBackupCode hashes a backup code for storage
func HashBackupCode(code string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(code), 10) // Lower cost for backup codes
	if err != nil {
		return "", fmt.Errorf("failed to hash backup code: %w", err)
	}
	return string(hash), nil
}

// VerifyBackupCode verifies a backup code against its hash
func VerifyBackupCode(code, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(code))
	return err == nil
}
