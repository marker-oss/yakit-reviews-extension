package wb

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

func TestFetchReviewsMapsWBResponse(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get("Authorization"); got != "token" {
			t.Fatalf("authorization header = %q", got)
		}
		if got := r.URL.Path; got != "/api/v1/feedbacks" {
			t.Fatalf("path = %q", got)
		}
		if got := r.URL.Query().Get("isAnswered"); got != "false" {
			t.Fatalf("isAnswered = %q", got)
		}
		if got := r.URL.Query().Get("take"); got != "10" {
			t.Fatalf("take = %q", got)
		}
		if got := r.URL.Query().Get("skip"); got != "0" {
			t.Fatalf("skip = %q", got)
		}
		if got := r.URL.Query().Get("dateFrom"); got == "" {
			t.Fatalf("dateFrom should be set")
		}

		var body strings.Builder
		_ = json.NewEncoder(&body).Encode(map[string]any{
			"data": map[string]any{
				"feedbacks": []map[string]any{
					{
						"id":               "fb-1",
						"text":             "Спасибо",
						"pros":             "Крой",
						"cons":             "Нет",
						"productValuation": 5,
						"createdDate":      "2024-09-26T10:20:48+03:00",
						"userName":         "Николай",
						"answer": map[string]any{
							"text":  "Пожалуйста",
							"state": "wbRu",
						},
						"productDetails": map[string]any{
							"nmId":            987654321,
							"supplierArticle": "ART-1",
						},
						"photoLinks": []map[string]any{
							{"fullSize": "https://example.test/full.webp", "miniSize": "https://example.test/mini.webp"},
						},
						"video": map[string]any{
							"link":         "https://example.test/index.m3u8",
							"previewImage": "https://example.test/preview.webp",
						},
					},
				},
			},
			"error":     false,
			"errorText": "",
		})
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body.String())),
		}, nil
	})}

	client := NewWithHTTPClient(config.WBConfig{Token: "token"}, "https://feedbacks-api.test", httpClient, 10)
	reviews, nextCursor, err := client.FetchReviews(context.Background(), time.Now().Add(-time.Hour), "")
	if err != nil {
		t.Fatalf("fetch reviews: %v", err)
	}
	if nextCursor != "answered:0" {
		t.Fatalf("next cursor = %q", nextCursor)
	}
	if len(reviews) != 1 {
		t.Fatalf("reviews len = %d", len(reviews))
	}

	review := reviews[0]
	if review.Marketplace != "wb" || review.ExternalReviewID != "fb-1" || review.ExternalProductID != "987654321" {
		t.Fatalf("review ids mismatch: %+v", review)
	}
	if review.SellerArticle != "ART-1" {
		t.Fatalf("seller article = %q", review.SellerArticle)
	}
	if review.Rating == nil || *review.Rating != 5 {
		t.Fatalf("rating = %v", review.Rating)
	}
	if review.Answer == nil || review.Answer.Text != "Пожалуйста" || review.Answer.State != "wbRu" {
		t.Fatalf("answer = %+v", review.Answer)
	}
	if len(review.Media) != 2 {
		t.Fatalf("media len = %d", len(review.Media))
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
