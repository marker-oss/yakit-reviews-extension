package ym

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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

func TestFetchReviewsMapsYMResponse(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.Method; got != http.MethodPost {
			t.Fatalf("method = %q", got)
		}
		if got := r.Header.Get("Api-Key"); got != "key" {
			t.Fatalf("api-key header = %q", got)
		}
		if got := r.URL.Path; got != "/v2/businesses/777/goods-feedback" {
			t.Fatalf("path = %q", got)
		}
		if got := r.URL.Query().Get("limit"); got != "50" {
			t.Fatalf("limit = %q", got)
		}
		if got := r.URL.Query().Get("page_token"); got != "" {
			t.Fatalf("page_token should be empty on first page, got %q", got)
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"status": "OK",
			"result": map[string]any{
				"feedbacks": []map[string]any{
					{
						"feedbackId":   1001,
						"createdAt":    "2026-05-28T12:20:00Z",
						"needReaction": true,
						"author":       "Мария",
						"identifiers": map[string]any{
							"offerId":   "1523",
							"shopSku":   "1523",
							"marketSku": 70476012,
							"modelId":   12345,
						},
						"description": map[string]any{
							"advantages":    "Крой",
							"disadvantages": "Нет",
							"comment":       "Отличная ткань",
						},
						"media": map[string]any{
							"photos": []string{"https://cdn.test/p1.jpg"},
							"videos": []string{"https://cdn.test/v1.mp4"},
						},
						"statistics": map[string]any{
							"rating": 5,
						},
					},
				},
				"paging": map[string]any{"nextPageToken": ""},
			},
		}), nil
	})}

	client := NewWithHTTPClient(
		config.YMConfig{APIKey: "key", BusinessID: "777"},
		"https://api.partner.test", httpClient, 50,
	)
	reviews, nextCursor, err := client.FetchReviews(context.Background(), time.Now().Add(-30*24*time.Hour), "")
	if err != nil {
		t.Fatalf("fetch reviews: %v", err)
	}
	if nextCursor != "" {
		t.Fatalf("next cursor = %q", nextCursor)
	}
	if len(reviews) != 1 {
		t.Fatalf("reviews len = %d", len(reviews))
	}

	review := reviews[0]
	if review.Marketplace != "ym" {
		t.Fatalf("marketplace = %q", review.Marketplace)
	}
	if review.ExternalReviewID != "1001" {
		t.Fatalf("external review id = %q", review.ExternalReviewID)
	}
	if review.ExternalProductID != "70476012" {
		t.Fatalf("external product id = %q", review.ExternalProductID)
	}
	if review.SellerArticle != "1523" {
		t.Fatalf("seller article = %q", review.SellerArticle)
	}
	if review.Rating == nil || *review.Rating != 5 {
		t.Fatalf("rating = %v", review.Rating)
	}
	if review.Text != "Отличная ткань" || review.Pros != "Крой" || review.Cons != "Нет" {
		t.Fatalf("text/pros/cons = %q / %q / %q", review.Text, review.Pros, review.Cons)
	}
	if review.AuthorName != "Мария" {
		t.Fatalf("author = %q", review.AuthorName)
	}
	if !review.CreatedAtMP.Equal(time.Date(2026, 5, 28, 12, 20, 0, 0, time.UTC)) {
		t.Fatalf("createdAt = %v", review.CreatedAtMP)
	}
	if review.Answer != nil {
		t.Fatalf("answer should be nil in v1, got %+v", review.Answer)
	}
	if len(review.Media) != 2 {
		t.Fatalf("media len = %d", len(review.Media))
	}
	if review.Media[0].Kind != "photo" || review.Media[0].URL != "https://cdn.test/p1.jpg" {
		t.Fatalf("photo media = %+v", review.Media[0])
	}
	if review.Media[1].Kind != "video" || review.Media[1].URL != "https://cdn.test/v1.mp4" {
		t.Fatalf("video media = %+v", review.Media[1])
	}
}

func TestFetchReviewsPaginatesWithPageToken(t *testing.T) {
	// First page returns a nextPageToken; the client must surface it as the cursor.
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		token := r.URL.Query().Get("page_token")
		switch token {
		case "":
			return jsonResponse(http.StatusOK, map[string]any{
				"result": map[string]any{
					"feedbacks": []map[string]any{
						{
							"feedbackId": 1, "createdAt": "2026-06-01T00:00:00Z",
							"identifiers": map[string]any{"offerId": "A"},
							"statistics":  map[string]any{"rating": 4},
						},
					},
					"paging": map[string]any{"nextPageToken": "PAGE2"},
				},
			}), nil
		case "PAGE2":
			return jsonResponse(http.StatusOK, map[string]any{
				"result": map[string]any{
					"feedbacks": []map[string]any{
						{
							"feedbackId": 2, "createdAt": "2026-06-02T00:00:00Z",
							"identifiers": map[string]any{"offerId": "B"},
							"statistics":  map[string]any{"rating": 5},
						},
					},
					"paging": map[string]any{"nextPageToken": ""},
				},
			}), nil
		default:
			t.Fatalf("unexpected page_token %q", token)
			return nil, nil
		}
	})}

	client := NewWithHTTPClient(config.YMConfig{APIKey: "key", BusinessID: "777"}, "https://api.partner.test", httpClient, 50)

	page1, cursor1, err := client.FetchReviews(context.Background(), time.Time{}, "")
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if cursor1 != "PAGE2" {
		t.Fatalf("cursor1 = %q", cursor1)
	}
	if len(page1) != 1 || page1[0].ExternalReviewID != "1" {
		t.Fatalf("page1 = %+v", page1)
	}

	page2, cursor2, err := client.FetchReviews(context.Background(), time.Time{}, cursor1)
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if cursor2 != "" {
		t.Fatalf("cursor2 = %q", cursor2)
	}
	if len(page2) != 1 || page2[0].ExternalReviewID != "2" {
		t.Fatalf("page2 = %+v", page2)
	}
}

func TestFetchReviewsDropsReviewsOlderThanSince(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{
			"result": map[string]any{
				"feedbacks": []map[string]any{
					{
						"feedbackId": 10, "createdAt": "2026-01-01T00:00:00Z", // old → dropped
						"identifiers": map[string]any{"offerId": "OLD"},
						"statistics":  map[string]any{"rating": 3},
					},
					{
						"feedbackId": 11, "createdAt": "2026-06-05T00:00:00Z", // recent → kept
						"identifiers": map[string]any{"offerId": "NEW"},
						"statistics":  map[string]any{"rating": 5},
					},
				},
				"paging": map[string]any{"nextPageToken": ""},
			},
		}), nil
	})}

	client := NewWithHTTPClient(config.YMConfig{APIKey: "key", BusinessID: "777"}, "https://api.partner.test", httpClient, 50)
	since := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	reviews, _, err := client.FetchReviews(context.Background(), since, "")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(reviews) != 1 || reviews[0].ExternalReviewID != "11" {
		t.Fatalf("expected only the recent review, got %+v", reviews)
	}
}

func TestFetchReviewsErrorsOnNon2xx(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusForbidden, map[string]any{
			"errors": []map[string]any{{"code": "FORBIDDEN", "message": "no access"}},
		}), nil
	})}

	client := NewWithHTTPClient(config.YMConfig{APIKey: "key", BusinessID: "777"}, "https://api.partner.test", httpClient, 50)
	_, _, err := client.FetchReviews(context.Background(), time.Time{}, "")
	if err == nil {
		t.Fatal("expected an error on HTTP 403")
	}
	if !strings.Contains(err.Error(), "no access") {
		t.Fatalf("error should surface the API message, got %v", err)
	}
}

func TestPublishReplyPostsComment(t *testing.T) {
	var gotPath, gotBody, gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("Api-Key")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewWithHTTPClient(config.YMConfig{APIKey: "key", BusinessID: "42"}, srv.URL, srv.Client(), 0)
	if err := c.PublishReply(context.Background(), "1001", "Спасибо!"); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if gotPath != "/v2/businesses/42/goods-feedback/comments/update" {
		t.Fatalf("path = %s", gotPath)
	}
	if gotKey != "key" {
		t.Fatalf("api-key = %q", gotKey)
	}
	if !strings.Contains(gotBody, `"feedbackId":1001`) || !strings.Contains(gotBody, `"text":"Спасибо!"`) {
		t.Fatalf("body = %s", gotBody)
	}
}

func TestPublishReplyBadID(t *testing.T) {
	c := NewWithHTTPClient(config.YMConfig{APIKey: "key", BusinessID: "42"}, "http://unused", nil, 0)
	if err := c.PublishReply(context.Background(), "not-a-number", "x"); err == nil {
		t.Fatal("expected error for non-numeric feedback id")
	}
}
