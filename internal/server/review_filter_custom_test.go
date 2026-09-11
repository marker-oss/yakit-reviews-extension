package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"reviews/internal/store"
)

func approveCustomSubmission(t *testing.T, s *Server, article string) {
	t.Helper()
	reviews, _, err := s.store.ListReviewsWithCount(context.Background(), store.ReviewListFilter{
		ArticleSearch: article,
		Limit:         10,
	})
	if err != nil || len(reviews) != 1 {
		t.Fatalf("list submissions: %v (n=%d)", err, len(reviews))
	}
	if err := s.store.SetReviewStatus(context.Background(), reviews[0].ID, "approved"); err != nil {
		t.Fatalf("approve: %v", err)
	}
}

func getReviews(t *testing.T, s *Server, rawQuery string) (int, reviewsResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/reviews?"+rawQuery, nil)
	rec := httptest.NewRecorder()
	s.handler().ServeHTTP(rec, req)
	var out reviewsResponse
	if rec.Code != http.StatusOK {
		return rec.Code, out
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return rec.Code, out
}

func TestPublicReviewsCustomFieldFilter(t *testing.T) {
	s := newAuthTestServer(t)
	publishCustomFieldsConfig(t, s, `{"customFields":[
		{"id":"height","label":"Рост","type":"chips","options":["150-160","160-170"],"required":true,"filterable":true},
		{"id":"fit","label":"Как сидит","type":"chips","options":["маломерит","в размер"]}
	]}`)
	if rec := customSubmission(t, s, `{"height":"160-170","fit":"в размер"}`); rec.Code != http.StatusCreated {
		t.Fatalf("submit: %d %s", rec.Code, rec.Body.String())
	}
	approveCustomSubmission(t, s, "673320")

	// Filterable field, configured option: exact match returned.
	code, out := getReviews(t, s, "custom_height=160-170")
	if code != http.StatusOK || out.Count != 1 {
		t.Fatalf("filter hit: code=%d count=%d", code, out.Count)
	}
	// Filterable field, other option: no results.
	if _, out := getReviews(t, s, "custom_height=150-160"); out.Count != 0 {
		t.Fatalf("non-matching option must yield 0, got %d", out.Count)
	}
	// Non-filterable field: rejected.
	if code, _ := getReviews(t, s, "custom_fit=%D0%B2+%D1%80%D0%B0%D0%B7%D0%BC%D0%B5%D1%80"); code != http.StatusBadRequest {
		t.Fatalf("non-filterable field must 400, got %d", code)
	}
	// Unknown field: rejected.
	if code, _ := getReviews(t, s, "custom_bogus=x"); code != http.StatusBadRequest {
		t.Fatalf("unknown field must 400, got %d", code)
	}
	// Configured field, unconfigured value: rejected.
	if code, _ := getReviews(t, s, "custom_height=999"); code != http.StatusBadRequest {
		t.Fatalf("invalid value must 400, got %d", code)
	}
	// Injection attempt: rejected before reaching SQL.
	if code, _ := getReviews(t, s, "custom_height=160%27+OR+1%3D1--"); code != http.StatusBadRequest {
		t.Fatalf("injection attempt must 400, got %d", code)
	}
	// Like wildcard in the value is never a configured option: rejected.
	if code, _ := getReviews(t, s, "custom_height=160%25170"); code != http.StatusBadRequest {
		t.Fatalf("wildcard value must 400, got %d", code)
	}
	// Legacy request without custom params: unchanged.
	if _, out := getReviews(t, s, "limit=100"); out.Count != 1 {
		t.Fatalf("legacy request must still return the review, got %d", out.Count)
	}
}
