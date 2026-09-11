package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"reviews/internal/store"
)

// TestTenantRateLimitPerTenant proves the limiter: a keyed public route 429s
// after the window fills, a second tenant has its own window, and admin
// routes never hit the limiter.
func TestTenantRateLimitPerTenant(t *testing.T) {
	restore := store.SetStrictTenantModeForTest(true)
	defer restore()
	s := newAuthTestServer(t)
	s.tenantLimiter = newTenantRateLimiter(3) // tiny window for the test
	handler := s.handler()

	ctx := t.Context()
	tenantA, err := s.store.CreateTenant(ctx, "rl-a", "https://a.example")
	if err != nil {
		t.Fatalf("create tenant A: %v", err)
	}
	tenantB, err := s.store.CreateTenant(ctx, "rl-b", "https://b.example")
	if err != nil {
		t.Fatalf("create tenant B: %v", err)
	}

	hit := func(path string) int {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec.Code
	}

	// Tenant A fills its window.
	for i := 0; i < 3; i++ {
		if code := hit("/api/reviews?public_key=" + tenantA.PublicKey); code != http.StatusOK {
			t.Fatalf("tenant A hit %d = %d, want 200", i, code)
		}
	}
	if code := hit("/api/reviews?public_key=" + tenantA.PublicKey); code != http.StatusTooManyRequests {
		t.Fatalf("tenant A 4th hit = %d, want 429", code)
	}

	// Tenant B has its own window and is not starved by A's traffic.
	for i := 0; i < 3; i++ {
		if code := hit("/api/reviews?public_key=" + tenantB.PublicKey); code != http.StatusOK {
			t.Fatalf("tenant B hit %d = %d, want 200", i, code)
		}
	}
	if code := hit("/api/reviews?public_key=" + tenantB.PublicKey); code != http.StatusTooManyRequests {
		t.Fatalf("tenant B 4th hit = %d, want 429", code)
	}

	// Admin routes are not rate-limited.
	for i := 0; i < 5; i++ {
		if code := hit("/admin/api/setup-status"); code == http.StatusTooManyRequests {
			t.Fatal("admin route must not be rate-limited")
		}
	}
}
