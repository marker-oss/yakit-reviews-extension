package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// allowedOrigin is a representative seller shop origin for CORS tests.
const allowedOrigin = "https://shop.example"

func TestCORSAllowsConfiguredOrigin(t *testing.T) {
	s := newAuthTestServer(t)
	s.cfg.AllowedOrigins = []string{allowedOrigin}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", allowedOrigin)
	rec := httptest.NewRecorder()
	s.handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("healthz status = %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != allowedOrigin {
		t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, allowedOrigin)
	}
	if vary := rec.Header().Get("Vary"); !strings.Contains(vary, "Origin") {
		t.Fatalf("Vary = %q, want it to contain Origin", vary)
	}
}

func TestCORSPreflightReturns204(t *testing.T) {
	s := newAuthTestServer(t)
	s.cfg.AllowedOrigins = []string{allowedOrigin}

	req := httptest.NewRequest(http.MethodOptions, "/api/reviews", nil)
	req.Header.Set("Origin", allowedOrigin)
	req.Header.Set("Access-Control-Request-Method", "GET")
	rec := httptest.NewRecorder()
	s.handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != allowedOrigin {
		t.Fatalf("preflight Access-Control-Allow-Origin = %q, want %q", got, allowedOrigin)
	}
	if methods := rec.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(methods, "GET") {
		t.Fatalf("Access-Control-Allow-Methods = %q, want it to contain GET", methods)
	}
}

func TestCORSRejectsUnknownOrigin(t *testing.T) {
	s := newAuthTestServer(t)
	s.cfg.AllowedOrigins = []string{allowedOrigin}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	s.handler().ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty for unknown origin", got)
	}
}

func TestCORSSkipsAdminRoutes(t *testing.T) {
	s := newAuthTestServer(t)
	s.cfg.AllowedOrigins = []string{allowedOrigin}

	req := httptest.NewRequest(http.MethodGet, "/admin/api/setup-status", nil)
	req.Header.Set("Origin", allowedOrigin)
	rec := httptest.NewRecorder()
	s.handler().ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("admin Access-Control-Allow-Origin = %q, want empty (admin is same-origin)", got)
	}
}

func TestCORSDisabledWhenNoOriginsConfigured(t *testing.T) {
	s := newAuthTestServer(t)
	// AllowedOrigins left empty — preserves prior behavior.

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", allowedOrigin)
	rec := httptest.NewRecorder()
	s.handler().ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty when CORS disabled", got)
	}
}
