package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"reviews/internal/config"
)

func TestMediaRouteRejectsDisallowedHost(t *testing.T) {
	s := newAuthTestServer(t)
	s.cfg.Media = config.MediaConfig{
		Allowlist:    []string{"cdn.test"},
		MaxBytes:     8 << 20,
		CacheEntries: 8,
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/media?u=https%3A%2F%2Fevil.com%2Fx.jpg", nil)
	s.handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}
