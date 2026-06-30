package ozon

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"reviews/internal/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func jsonResponse(status int, payload any) *http.Response {
	var body strings.Builder
	_ = json.NewEncoder(&body).Encode(payload)
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body.String())),
	}
}

func TestFetchReviewsMapsOzonResponse(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.Method; got != http.MethodPost {
			t.Fatalf("method = %q", got)
		}
		if got := r.Header.Get("Client-Id"); got != "client" {
			t.Fatalf("client id header = %q", got)
		}
		if got := r.Header.Get("Api-Key"); got != "key" {
			t.Fatalf("api key header = %q", got)
		}
		if got := r.URL.Path; got != "/v1/review/list" {
			t.Fatalf("path = %q", got)
		}

		var req ozonReviewListRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Limit != 100 || req.SortDir != "DESC" || req.LastID != "" {
			t.Fatalf("request = %+v", req)
		}

		return jsonResponse(http.StatusOK, map[string]any{
			"reviews": []map[string]any{
				{
					"id":              "017c0ddf-8b43",
					"sku":             181649408,
					"text":            "Отличный товар!",
					"rating":          5,
					"status":          "UNPROCESSED",
					"published_at":    "2026-02-02T10:00:00Z",
					"order_status":    "DELIVERED",
					"comments_amount": 0,
					"photos_amount":   2,
					"videos_amount":   0,
				},
			},
			"has_next": true,
			"last_id":  "cursor-2",
		}), nil
	})}

	client := NewWithHTTPClient(config.OzonConfig{ClientID: "client", APIKey: "key"}, "https://api-seller.test", httpClient, 100)
	reviews, nextCursor, err := client.FetchReviews(context.Background(), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "")
	if err != nil {
		t.Fatalf("fetch reviews: %v", err)
	}
	if nextCursor != "cursor-2" {
		t.Fatalf("next cursor = %q", nextCursor)
	}
	if len(reviews) != 1 {
		t.Fatalf("reviews len = %d", len(reviews))
	}

	review := reviews[0]
	if review.Marketplace != "ozon" {
		t.Fatalf("marketplace = %q", review.Marketplace)
	}
	if review.ExternalReviewID != "017c0ddf-8b43" {
		t.Fatalf("external review id = %q", review.ExternalReviewID)
	}
	if review.ExternalProductID != "181649408" || review.SellerArticle != "181649408" {
		t.Fatalf("product ids = %q / %q", review.ExternalProductID, review.SellerArticle)
	}
	if review.Rating == nil || *review.Rating != 5 {
		t.Fatalf("rating = %v", review.Rating)
	}
	if review.Text != "Отличный товар!" {
		t.Fatalf("text = %q", review.Text)
	}
	if !review.CreatedAtMP.Equal(time.Date(2026, 2, 2, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("createdAt = %v", review.CreatedAtMP)
	}
	if len(review.Media) != 0 {
		t.Fatalf("media should be empty until Ozon media URLs are mapped, got %+v", review.Media)
	}
	if len(review.Raw) == 0 {
		t.Fatalf("raw payload should be stored")
	}
}

func TestFetchReviewsUsesLastIDCursor(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var req ozonReviewListRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.LastID != "cursor-2" {
			t.Fatalf("last_id = %q", req.LastID)
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"reviews": []map[string]any{
				{"id": "review-2", "sku": "SKU-2", "rating": 4, "published_at": "2026-02-03T10:00:00Z"},
			},
			"has_next": false,
			"last_id":  "",
		}), nil
	})}

	client := NewWithHTTPClient(config.OzonConfig{ClientID: "client", APIKey: "key"}, "https://api-seller.test", httpClient, 100)
	reviews, nextCursor, err := client.FetchReviews(context.Background(), time.Time{}, "cursor-2")
	if err != nil {
		t.Fatalf("fetch reviews: %v", err)
	}
	if nextCursor != "" {
		t.Fatalf("next cursor = %q", nextCursor)
	}
	if len(reviews) != 1 || reviews[0].ExternalReviewID != "review-2" || reviews[0].SellerArticle != "SKU-2" {
		t.Fatalf("reviews = %+v", reviews)
	}
}

func TestFetchReviewsStopsAtSinceBoundary(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{
			"reviews": []map[string]any{
				{"id": "new", "sku": "NEW", "rating": 5, "published_at": "2026-06-05T00:00:00Z"},
				{"id": "old", "sku": "OLD", "rating": 3, "published_at": "2026-01-01T00:00:00Z"},
			},
			"has_next": true,
			"last_id":  "older-page",
		}), nil
	})}

	client := NewWithHTTPClient(config.OzonConfig{ClientID: "client", APIKey: "key"}, "https://api-seller.test", httpClient, 100)
	reviews, nextCursor, err := client.FetchReviews(context.Background(), time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), "")
	if err != nil {
		t.Fatalf("fetch reviews: %v", err)
	}
	if nextCursor != "" {
		t.Fatalf("next cursor should stop at old reviews, got %q", nextCursor)
	}
	if len(reviews) != 1 || reviews[0].ExternalReviewID != "new" {
		t.Fatalf("expected only recent review, got %+v", reviews)
	}
}

func TestFetchReviewsErrorsOnNon2xx(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusForbidden, map[string]any{
			"message": "premium method is unavailable",
		}), nil
	})}

	client := NewWithHTTPClient(config.OzonConfig{ClientID: "client", APIKey: "key"}, "https://api-seller.test", httpClient, 100)
	_, _, err := client.FetchReviews(context.Background(), time.Time{}, "")
	if err == nil {
		t.Fatal("expected an error on HTTP 403")
	}
	if !strings.Contains(err.Error(), "premium method is unavailable") {
		t.Fatalf("error should surface the API message, got %v", err)
	}
}
