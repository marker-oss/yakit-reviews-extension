// Package secrets encrypts marketplace credentials at rest. In SaaS the
// database holds API tokens of many sellers' shops; a database dump must not
// leak them.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// marker prefixes an encrypted payload so plaintext rows and empty payloads
// stay distinguishable during the migration window.
const marker = "enc:v1:"

// Cipher encrypts and decrypts credential payloads with AES-256-GCM.
type Cipher struct {
	aead cipher.AEAD
}

// New decodes a base64 32-byte key. Empty key returns a nil cipher: the
// instance keeps plaintext payloads (single-tenant installs without the env
// var must not break).
func New(keyB64 string) (*Cipher, error) {
	keyB64 = strings.TrimSpace(keyB64)
	if keyB64 == "" {
		return nil, nil
	}
	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return nil, fmt.Errorf("secrets: REVIEWS_CREDENTIALS_KEY is not valid base64: %w", err)
	}
	if len(key) != 32 {
		return nil, errors.New("secrets: REVIEWS_CREDENTIALS_KEY must be 32 bytes base64-encoded (generate: openssl rand -base64 32)")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt seals plaintext. Returns "" for empty input (nothing to protect).
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := c.aead.Seal(nil, nonce, []byte(plaintext), nil)
	return marker + base64.StdEncoding.EncodeToString(append(nonce, sealed...)), nil
}

// Decrypt opens a sealed payload. A value without the marker is returned as
// is (legacy plaintext row not yet migrated).
func (c *Cipher) Decrypt(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if !strings.HasPrefix(value, marker) {
		return value, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, marker))
	if err != nil {
		return "", fmt.Errorf("secrets: malformed encrypted payload: %w", err)
	}
	ns := c.aead.NonceSize()
	if len(raw) < ns+1 {
		return "", errors.New("secrets: encrypted payload too short")
	}
	plaintext, err := c.aead.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", fmt.Errorf("secrets: decrypt payload: %w", err)
	}
	return string(plaintext), nil
}

// NeedsMigration reports whether a stored payload is still plaintext.
func NeedsMigration(value string) bool {
	return value != "" && !strings.HasPrefix(value, marker)
}
