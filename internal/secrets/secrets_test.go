package secrets

import (
	"strings"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	c, err := New("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=") // 32 zero bytes
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	for _, plain := range []string{"{\"token\":\"wb-secret\"}", "x", "юникод-токен"} {
		sealed, err := c.Encrypt(plain)
		if err != nil {
			t.Fatalf("encrypt %q: %v", plain, err)
		}
		if !strings.HasPrefix(sealed, marker) {
			t.Fatalf("sealed payload %q missing marker", sealed)
		}
		if sealed == plain {
			t.Fatal("payload not encrypted")
		}
		got, err := c.Decrypt(sealed)
		if err != nil || got != plain {
			t.Fatalf("decrypt %q = %q, err=%v", plain, got, err)
		}
	}
}

func TestEmptyStaysEmpty(t *testing.T) {
	c, _ := New("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	sealed, err := c.Encrypt("")
	if err != nil || sealed != "" {
		t.Fatalf("encrypt empty = %q, err=%v", sealed, err)
	}
	got, err := c.Decrypt("")
	if err != nil || got != "" {
		t.Fatalf("decrypt empty = %q, err=%v", got, err)
	}
}

func TestLegacyPlaintextPassesThrough(t *testing.T) {
	c, _ := New("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	got, err := c.Decrypt(`{"token":"legacy"}`)
	if err != nil || got != `{"token":"legacy"}` {
		t.Fatalf("legacy = %q, err=%v", got, err)
	}
	if !NeedsMigration(`{"token":"legacy"}`) {
		t.Fatal("plaintext must be flagged for migration")
	}
	if NeedsMigration(marker + "sealed") {
		t.Fatal("sealed payload must not be flagged")
	}
}

func TestWrongKeyFails(t *testing.T) {
	c1, _ := New("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	c2, _ := New("BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB=")
	sealed, err := c1.Encrypt("secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := c2.Decrypt(sealed); err == nil {
		t.Fatal("decrypt with wrong key must fail")
	}
}

func TestBadKeysRejected(t *testing.T) {
	if _, err := New("not-base64!!!"); err == nil {
		t.Fatal("invalid base64 must be rejected")
	}
	if _, err := New("AAAA"); err == nil {
		t.Fatal("short key must be rejected")
	}
	// Empty key is allowed: single-tenant compat without the env var.
	c, err := New("")
	if err != nil || c != nil {
		t.Fatalf("empty key = %v, %v; want nil, nil", c, err)
	}
}
