package auth

import (
	"crypto/rand"
	"encoding/base64"
)

const sessionTokenBytes = 32

// NewSessionToken returns a URL-safe, high-entropy bearer token.
func NewSessionToken() (string, error) {
	buf := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
