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

func TestShowcaseRuleEndpoints(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/showcase-rule", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get rule status = %d, body=%s", rec.Code, rec.Body.String())
	}

	csrf := getCSRFToken(t, s, cookie)
	req = httptest.NewRequest(http.MethodPut, "/admin/api/showcase-rule",
		strings.NewReader(`{"MinRating":5,"RequirePhoto":true,"Limit":4,"SortBy":"rating"}`))
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rec = httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put rule status = %d, body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/api/showcase-rule", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get saved rule status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var rule struct {
		MinRating int
		Limit     int
	}
	if err := json.NewDecoder(rec.Body).Decode(&rule); err != nil {
		t.Fatalf("decode rule: %v", err)
	}
	if rule.MinRating != 5 || rule.Limit != 4 {
		t.Fatalf("unexpected saved rule: %+v", rule)
	}
}

func TestPublicShowcaseEndpoint(t *testing.T) {
	s := newAuthTestServer(t)
	seedAdminReview(t, s, "w1", 5)
	seedAdminReview(t, s, "w2", 3)
	if err := s.store.SaveShowcaseRule(context.Background(), store.ShowcaseRule{
		MinRating: 4,
		Limit:     1,
		SortBy:    "recent",
	}); err != nil {
		t.Fatalf("save showcase rule: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/showcase", nil)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/showcase", s.handleShowcase)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("showcase status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var response showcaseResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode showcase: %v", err)
	}
	if response.Count != 1 || len(response.Reviews) != 1 {
		t.Fatalf("expected one showcased review: %+v", response)
	}
	if response.Aggregate.TotalReviews != 1 || response.Aggregate.RatingCount != 1 {
		t.Fatalf("aggregate must match showcased reviews: %+v", response.Aggregate)
	}
	if response.Aggregate.AverageRating != 5 {
		t.Fatalf("average rating = %v, want 5", response.Aggregate.AverageRating)
	}
}
