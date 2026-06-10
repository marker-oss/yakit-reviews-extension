package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"reviews/internal/marketplace"
)

func loginTestAdmin(t *testing.T, s *Server) *http.Cookie {
	t.Helper()
	mux := s.adminMux()
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/admin/api/setup",
		strings.NewReader(`{"login":"admin","password":"password1"}`)))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/api/login",
		strings.NewReader(`{"login":"admin","password":"password1"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body=%s", rec.Code, rec.Body.String())
	}
	cookie := firstCookie(rec, sessionCookieName)
	if cookie == nil {
		t.Fatal("missing session cookie")
	}
	return cookie
}

func seedAdminReview(t *testing.T, s *Server, extID string, rating int) uint {
	t.Helper()
	result, err := s.store.UpsertReview(context.Background(), marketplace.Review{
		Marketplace:       "wb",
		ExternalReviewID:  extID,
		ExternalProductID: "p1",
		Rating:            &rating,
		Text:              "good",
		CreatedAtMP:       time.Now(),
	})
	if err != nil {
		t.Fatalf("upsert review: %v", err)
	}
	return result.Review.ID
}

func TestAdminReviewsRequiresAuth(t *testing.T) {
	s := newAuthTestServer(t)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/api/reviews", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without session, got %d", rec.Code)
	}
}

func TestAdminReviewsListAndModerate(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)
	reviewID := seedAdminReview(t, s, "w1", 5)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/reviews", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var listed adminReviewsResponse
	if err := json.NewDecoder(rec.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if listed.Total != 1 || len(listed.Reviews) != 1 {
		t.Fatalf("unexpected list response: %+v", listed)
	}
	if listed.Reviews[0].Visibility != "visible" || listed.Reviews[0].Pinned {
		t.Fatalf("unexpected curation fields: %+v", listed.Reviews[0])
	}

	csrf := getCSRFToken(t, s, cookie)
	req = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/admin/api/reviews/%d", reviewID),
		strings.NewReader(`{"visibility":"hidden","pinned":true}`))
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rec = httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("moderate status = %d, body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/api/reviews?visibility=hidden", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("hidden list status = %d, body=%s", rec.Code, rec.Body.String())
	}
	listed = adminReviewsResponse{}
	if err := json.NewDecoder(rec.Body).Decode(&listed); err != nil {
		t.Fatalf("decode hidden list: %v", err)
	}
	if listed.Total != 1 || !listed.Reviews[0].Pinned || listed.Reviews[0].Visibility != "hidden" {
		t.Fatalf("review was not moderated: %+v", listed)
	}
}

func getCSRFToken(t *testing.T, s *Server, session *http.Cookie) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/admin/api/csrf", nil)
	req.AddCookie(session)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("csrf status = %d, body=%s", rec.Code, rec.Body.String())
	}
	cookie := firstCookie(rec, csrfCookieName)
	if cookie == nil {
		t.Fatal("missing csrf cookie")
	}
	return cookie.Value
}
