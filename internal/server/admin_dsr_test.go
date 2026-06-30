package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"reviews/internal/store"
)

func seedSiteReview(t *testing.T, s *Server, email string) {
	t.Helper()
	if _, err := s.store.CreateSiteReview(context.Background(), store.SiteReviewInput{
		ExternalReviewID: "site-dsr", SellerArticle: "a", Rating: 5,
		AuthorName: "X", AuthorEmail: email, Text: "hi",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func TestDSRLookupExportDelete(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)
	seedSiteReview(t, s, "subject@example.com")

	// Lookup
	req := httptest.NewRequest(http.MethodGet, "/admin/api/dsr/lookup?email=subject@example.com", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("lookup status %d: %s", rec.Code, rec.Body.String())
	}
	var look struct {
		Reviews []map[string]any `json:"reviews"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&look)
	if len(look.Reviews) != 1 {
		t.Fatalf("expected 1 review, got %d", len(look.Reviews))
	}

	// Export sets attachment header
	req = httptest.NewRequest(http.MethodGet, "/admin/api/dsr/export?email=subject@example.com", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("export bad: %d %q", rec.Code, rec.Header().Get("Content-Disposition"))
	}

	// Delete requires CSRF
	csrf := getCSRFToken(t, s, cookie)
	req = httptest.NewRequest(http.MethodPost, "/admin/api/dsr/delete", strings.NewReader(`{"email":"subject@example.com"}`))
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rec = httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status %d: %s", rec.Code, rec.Body.String())
	}
	var del struct {
		Deleted int `json:"deleted"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&del)
	if del.Deleted != 1 {
		t.Fatalf("deleted = %d, want 1", del.Deleted)
	}
}

func TestDSRRequiresAuth(t *testing.T) {
	s := newAuthTestServer(t)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/api/dsr/lookup?email=a@b.co", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
