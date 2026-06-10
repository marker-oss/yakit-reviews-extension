package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reviews/internal/config"
	"reviews/internal/store"
)

func newAuthTestServer(t *testing.T) *Server {
	t.Helper()
	st, err := store.Open(config.DBConfig{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "reviews.db")})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(st, Config{SessionTTL: time.Hour}, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestSetupThenLoginFlow(t *testing.T) {
	s := newAuthTestServer(t)
	mux := s.adminMux()

	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"login":"admin","password":"s3cret-pass"}`)
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/api/setup", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup status = %d, body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/api/setup",
		strings.NewReader(`{"login":"x","password":"yyyyyyyy"}`)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("second setup status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/api/login",
		strings.NewReader(`{"login":"admin","password":"s3cret-pass"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body=%s", rec.Code, rec.Body.String())
	}
	sessionCookie := firstCookie(rec, sessionCookieName)
	if sessionCookie == nil {
		t.Fatalf("expected session cookie, got %q", rec.Header().Get("Set-Cookie"))
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/api/me", nil)
	req.AddCookie(sessionCookie)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("me status = %d, body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/admin/api/csrf", nil)
	req.AddCookie(sessionCookie)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("csrf status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if firstCookie(rec, csrfCookieName) == nil {
		t.Fatalf("expected csrf cookie, got %q", rec.Header().Get("Set-Cookie"))
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/api/login",
		strings.NewReader(`{"login":"admin","password":"wrong"}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad login status = %d", rec.Code)
	}
}

func firstCookie(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}
