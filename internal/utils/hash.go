package utils

import (
	"crypto/sha256"
	"encoding/hex"
)

// HashAPIKey creates a SHA-256 hash of an API key
func HashAPIKey(apiKey string) string {
	hash := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(hash[:])
}
