package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"reviews/internal/marketplace"
)

func TestWidgetConfigPublishAndPublicFetch(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)
	csrf := getCSRFToken(t, s, cookie)

	body := `{"theme":{"accent":"#245f4f"},"layout":{"mode":"compact"}}`
	req := httptest.NewRequest(http.MethodPost, "/admin/api/widget-config/product", strings.NewReader(body))
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("publish status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var published struct {
		Version int `json:"version"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&published); err != nil {
		t.Fatalf("decode publish: %v", err)
	}
	if published.Version != 1 {
		t.Fatalf("expected version 1, got %d", published.Version)
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/api/widget-config/product", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"#245f4f"`) {
		t.Fatalf("admin get status = %d, body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/widget-config?context=product", nil)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/widget-config", s.handlePublicWidgetConfig)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"compact"`) {
		t.Fatalf("public get status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestWidgetConfigRollback(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)
	csrf := getCSRFToken(t, s, cookie)
	mux := s.adminMux()

	for _, body := range []string{`{"theme":{"accent":"#111111"}}`, `{"theme":{"accent":"#222222"}}`} {
		req := httptest.NewRequest(http.MethodPost, "/admin/api/widget-config/homepage", strings.NewReader(body))
		req.AddCookie(cookie)
		req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
		req.Header.Set(csrfHeaderName, csrf)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("publish status = %d, body=%s", rec.Code, rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/api/widget-config/homepage/rollback/1", nil)
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("rollback status = %d, body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/api/widget-config/homepage", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"#111111"`) {
		t.Fatalf("expected v1 active, status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestPublicReviewsApplyWidgetRules(t *testing.T) {
	s := newAuthTestServer(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)

	seedPublicReview(t, s, marketplace.Review{
		Marketplace:       "wb",
		ExternalReviewID:  "plain-new",
		ExternalProductID: "p1",
		Rating:            intPtr(5),
		Text:              "Свежий отзыв без фото",
		CreatedAtMP:       now,
	})
	seedPublicReview(t, s, marketplace.Review{
		Marketplace:       "wb",
		ExternalReviewID:  "photo-text",
		ExternalProductID: "p1",
		Rating:            intPtr(4),
		Text:              "Подробный отзыв с фото",
		CreatedAtMP:       now.Add(-time.Hour),
		Media: []marketplace.Media{{
			Kind: "photo",
			URL:  "https://cdn.example/review.jpg",
		}},
	})
	seedPublicReview(t, s, marketplace.Review{
		Marketplace:       "wb",
		ExternalReviewID:  "low",
		ExternalProductID: "p1",
		Rating:            intPtr(3),
		Text:              "Низкая оценка",
		CreatedAtMP:       now.Add(-2 * time.Hour),
		Media: []marketplace.Media{{
			Kind: "photo",
			URL:  "https://cdn.example/low.jpg",
		}},
	})

	_, err := s.store.PublishWidgetConfig(ctx, "product", `{
		"defaults":{"minRating":4,"requireText":true,"initialSort":"relevance","photoFirst":true,"textFirst":true},
		"ranking":[
			{"field":"hasPhoto","direction":"desc"},
			{"field":"hasText","direction":"desc"},
			{"field":"rating","direction":"desc"},
			{"field":"createdAt","direction":"desc"}
		]
	}`)
	if err != nil {
		t.Fatalf("publish widget config: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/reviews?context=product&limit=10", nil)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/reviews", s.handleReviews)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("reviews status = %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Reviews []struct {
			ExternalReviewID string `json:"externalReviewId"`
		} `json:"reviews"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode reviews: %v", err)
	}
	if len(payload.Reviews) != 2 {
		t.Fatalf("expected 2 filtered reviews, got %+v", payload.Reviews)
	}
	if payload.Reviews[0].ExternalReviewID != "photo-text" || payload.Reviews[1].ExternalReviewID != "plain-new" {
		t.Fatalf("unexpected review order: %+v", payload.Reviews)
	}
}

func seedPublicReview(t *testing.T, s *Server, review marketplace.Review) {
	t.Helper()
	if _, err := s.store.UpsertReview(context.Background(), review); err != nil {
		t.Fatalf("upsert review %s: %v", review.ExternalReviewID, err)
	}
}

func intPtr(value int) *int {
	return &value
}
