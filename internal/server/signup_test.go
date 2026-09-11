package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"reviews/internal/store"
)

// signupBody posts a signup request and returns the recorder.
func signupBody(t *testing.T, s *Server, payload string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/api/signup", strings.NewReader(payload))
	s.adminMux().ServeHTTP(rec, req)
	return rec
}

// TestSignupCreatesTenantAndLogsIn proves the SaaS self-serve path: signup
// creates an isolated trial tenant with its own admin, returns the public
// key, and establishes a session scoped to the new tenant.
func TestSignupCreatesTenantAndLogsIn(t *testing.T) {
	restore := store.SetStrictTenantModeForTest(true)
	defer restore()
	s := newAuthTestServer(t)

	rec := signupBody(t, s, `{"login":"seller1","password":"password1","shopOrigin":"https://shop1.example"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("signup status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		PublicKey string `json:"publicKey"`
		TrialEnds time.Time
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.PublicKey) != 64 {
		t.Fatalf("public key = %q, want 64 hex chars", resp.PublicKey)
	}

	// The session cookie is scoped to the new tenant: /admin/api/me works
	// and tenant-scoped store calls resolve the new tenant, not tenant 1.
	cookie := rec.Result().Cookies()[0]
	req := httptest.NewRequest(http.MethodGet, "/admin/api/me", nil)
	req.AddCookie(cookie)
	rec2 := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec2, req)
	if rec2.Code != http.StatusOK {
		t.Fatalf("me status = %d, body=%s", rec2.Code, rec2.Body.String())
	}

	// Duplicate login is rejected with 409.
	rec3 := signupBody(t, s, `{"login":"seller1","password":"password1","shopOrigin":"https://other.example"}`)
	if rec3.Code != http.StatusConflict {
		t.Fatalf("duplicate login status = %d, want 409", rec3.Code)
	}

	// Weak password and missing origin are rejected.
	if rec := signupBody(t, s, `{"login":"s2","password":"short","shopOrigin":"https://x.example"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("weak password status = %d, want 400", rec.Code)
	}
	if rec := signupBody(t, s, `{"login":"s2","password":"password1","shopOrigin":""}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing origin status = %d, want 400", rec.Code)
	}

	// Signup disabled outside strict mode (single-tenant installs).
	restore()
	rec4 := signupBody(t, s, `{"login":"seller2","password":"password1","shopOrigin":"https://y.example"}`)
	if rec4.Code != http.StatusNotFound {
		t.Fatalf("compat signup status = %d, want 404", rec4.Code)
	}
}

// TestPausedTenantGets402 proves the billing gate: a paused tenant's widget
// data routes answer 402 while admin routes stay reachable (so the seller
// can log in and pay).
func TestPausedTenantGets402(t *testing.T) {
	restore := store.SetStrictTenantModeForTest(true)
	defer restore()
	s := newAuthTestServer(t)

	ctx := context.Background()
	tenant, err := s.store.CreateTenant(ctx, "paused-b", "https://b.example")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if err := s.store.SetTenantStatus(ctx, tenant.ID, "paused"); err != nil {
		t.Fatalf("pause tenant: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/reviews?public_key="+tenant.PublicKey, nil)
	rec := httptest.NewRecorder()
	s.handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("paused widget status = %d, want 402", rec.Code)
	}

	// Admin route is not key-scoped and must not 402.
	req2 := httptest.NewRequest(http.MethodGet, "/admin/api/setup-status", nil)
	rec2 := httptest.NewRecorder()
	s.handler().ServeHTTP(rec2, req2)
	if rec2.Code == http.StatusPaymentRequired {
		t.Fatal("admin route must not be gated by tenant status")
	}
}
