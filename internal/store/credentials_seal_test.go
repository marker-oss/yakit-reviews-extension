package store

import (
	"context"
	"strings"
	"testing"

	"reviews/internal/secrets"
)

const testKeyB64 = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=" // 32 zero bytes

// TestCredentialsSealedAtRest proves the SaaS at-rest flow end to end: with
// a cipher installed, a saved credential row is sealed in the database, the
// store API still returns plaintext to callers, a legacy plaintext row is
// readable, and MigrateCredentials upgrades it in place.
func TestCredentialsSealedAtRest(t *testing.T) {
	s := newTestStore(t)
	cipher, err := secrets.New(testKeyB64)
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	s.SetCredentialsCipher(cipher)
	ctx := context.Background()

	// Save through the API.
	if _, err := s.SaveMarketplaceCredential(ctx, MarketplaceCredentialPatch{
		Marketplace: "wb",
		Values:      map[string]string{"token": "wb-secret-token"},
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	// The stored row must be sealed: plaintext token nowhere in the payload.
	var stored MarketplaceCredential
	if err := s.db.Where("marketplace = ?", "wb").First(&stored).Error; err != nil {
		t.Fatalf("load raw: %v", err)
	}
	if strings.Contains(stored.Payload, "wb-secret-token") {
		t.Fatalf("payload not sealed: %s", stored.Payload)
	}
	if !strings.HasPrefix(stored.Payload, "enc:v1:") {
		t.Fatalf("payload missing marker: %s", stored.Payload)
	}

	// The API decrypts for callers.
	cred, err := s.GetMarketplaceCredential(ctx, "wb")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if cred.PayloadMap()["token"] != "wb-secret-token" {
		t.Fatalf("decrypted token = %q", cred.PayloadMap()["token"])
	}

	// Legacy plaintext row (pre-cipher install) stays readable and gets
	// upgraded by MigrateCredentials.
	legacy := MarketplaceCredential{TenantID: 1, Marketplace: "ym", Payload: `{"api_key":"legacy-key"}`}
	if err := s.db.Create(&legacy).Error; err != nil {
		t.Fatalf("seed legacy: %v", err)
	}
	got, err := s.GetMarketplaceCredential(ctx, "ym")
	if err != nil || got.PayloadMap()["api_key"] != "legacy-key" {
		t.Fatalf("legacy read = %q, err=%v", got.PayloadMap()["api_key"], err)
	}
	if err := s.MigrateCredentials(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	var upgraded MarketplaceCredential
	if err := s.db.Where("marketplace = ?", "ym").First(&upgraded).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !strings.HasPrefix(upgraded.Payload, "enc:v1:") {
		t.Fatalf("legacy row not upgraded: %s", upgraded.Payload)
	}
	// Still readable after upgrade.
	got, err = s.GetMarketplaceCredential(ctx, "ym")
	if err != nil || got.PayloadMap()["api_key"] != "legacy-key" {
		t.Fatalf("post-migration read = %q, err=%v", got.PayloadMap()["api_key"], err)
	}

	// Merging a patch into a sealed row keeps other keys.
	if _, err := s.SaveMarketplaceCredential(ctx, MarketplaceCredentialPatch{
		Marketplace: "wb",
		Values:      map[string]string{"extra": "v"},
	}); err != nil {
		t.Fatalf("merge: %v", err)
	}
	merged, err := s.GetMarketplaceCredential(ctx, "wb")
	if err != nil {
		t.Fatalf("get merged: %v", err)
	}
	if merged.PayloadMap()["token"] != "wb-secret-token" || merged.PayloadMap()["extra"] != "v" {
		t.Fatalf("merge lost keys: %+v", merged.PayloadMap())
	}
}

// TestCredentialsPlaintextWithoutCipher proves compat: without a key every
// payload stays plaintext and nothing breaks.
func TestCredentialsPlaintextWithoutCipher(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.SaveMarketplaceCredential(ctx, MarketplaceCredentialPatch{
		Marketplace: "wb",
		Values:      map[string]string{"token": "plain-token"},
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	var stored MarketplaceCredential
	if err := s.db.Where("marketplace = ?", "wb").First(&stored).Error; err != nil {
		t.Fatalf("load: %v", err)
	}
	if stored.Payload == "" || strings.HasPrefix(stored.Payload, "enc:v1:") {
		t.Fatalf("compat mode must keep plaintext, got %q", stored.Payload)
	}
	if err := s.MigrateCredentials(ctx); err != nil {
		t.Fatalf("migrate without cipher must be a no-op, got: %v", err)
	}
}
