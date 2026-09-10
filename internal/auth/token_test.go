package auth

import "testing"

func TestNewSessionTokenIsUniqueAndLong(t *testing.T) {
	a, err := NewSessionToken()
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	b, err := NewSessionToken()
	if err != nil {
		t.Fatalf("token b: %v", err)
	}
	if a == b {
		t.Fatal("expected unique tokens")
	}
	if len(a) < 32 {
		t.Fatalf("token too short: %d", len(a))
	}
}
