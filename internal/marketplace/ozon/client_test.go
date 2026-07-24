package ozon

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

// productListTransport serves /v3/product/list with the given pages and
// delegates /v1/review/list to reviewHandler.
func productListTransport(t *testing.T, pages []map[string]any, productStatus int, reviewHandler func(*http.Request) *http.Response) roundTripFunc {
	t.Helper()
	call := 0
	return func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/v3/product/list":
			if productStatus != http.StatusOK {
				return jsonResponse(productStatus, map[string]any{"code": 7, "message": "Api-Key is missing a required role for a method"}), nil
			}
			if call >= len(pages) {
				t.Fatalf("unexpected extra product list call %d", call)
			}
			page := pages[call]
			call++
			return jsonResponse(http.StatusOK, page), nil
		case "/v1/review/list":
			return reviewHandler(r), nil
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
			return nil, nil
		}
	}
}

func reviewPage(sku any) func(*http.Request) *http.Response {
	return func(*http.Request) *http.Response {
		return jsonResponse(http.StatusOK, map[string]any{
			"reviews": []map[string]any{
				{
					"id":           "r-1",
					"sku":          sku,
					"text":         "ok",
					"rating":       5,
					"status":       "UNPROCESSED",
					"published_at": "2026-02-02T10:00:00Z",
				},
			},
			"has_next": false,
			"last_id":  "",
		})
	}
}

func TestFetchReviewsResolvesSellerArticleViaOfferID(t *testing.T) {
	pages := []map[string]any{
		{"result": map[string]any{
			"items":   []map[string]any{{"product_id": 1, "offer_id": "85467", "sku": 181649408}},
			"total":   1,
			"last_id": "",
		}},
	}
	httpClient := &http.Client{Transport: productListTransport(t, pages, http.StatusOK, reviewPage(181649408))}
	client := NewWithHTTPClient(config.OzonConfig{ClientID: "client", APIKey: "key"}, "https://api-seller.test", httpClient, 100)

	reviews, _, err := client.FetchReviews(context.Background(), time.Time{}, "")
	if err != nil {
		t.Fatalf("FetchReviews: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("reviews = %d, want 1", len(reviews))
	}
	if reviews[0].SellerArticle != "85467" {
		t.Fatalf("SellerArticle = %q, want offer_id 85467", reviews[0].SellerArticle)
	}
	if reviews[0].ExternalProductID != "181649408" {
		t.Fatalf("ExternalProductID = %q, want marketplace sku preserved", reviews[0].ExternalProductID)
	}
}

func TestFetchReviewsFallsBackToSKUWithoutProductRole(t *testing.T) {
	httpClient := &http.Client{Transport: productListTransport(t, nil, http.StatusForbidden, reviewPage(181649408))}
	client := NewWithHTTPClient(config.OzonConfig{ClientID: "client", APIKey: "key"}, "https://api-seller.test", httpClient, 100)

	reviews, _, err := client.FetchReviews(context.Background(), time.Time{}, "")
	if err != nil {
		t.Fatalf("FetchReviews should not fail when product role is missing: %v", err)
	}
	if len(reviews) != 1 || reviews[0].SellerArticle != "181649408" {
		t.Fatalf("SellerArticle = %+v, want fallback to sku", reviews)
	}
}

func TestCheckProductsAccess(t *testing.T) {
	okPages := []map[string]any{{"result": map[string]any{"items": []map[string]any{}, "last_id": ""}}}
	httpClient := &http.Client{Transport: productListTransport(t, okPages, http.StatusOK, nil)}
	client := NewWithHTTPClient(config.OzonConfig{ClientID: "client", APIKey: "key"}, "https://api-seller.test", httpClient, 100)
	if err := client.CheckProductsAccess(context.Background()); err != nil {
		t.Fatalf("access with role: %v", err)
	}

	httpClient = &http.Client{Transport: productListTransport(t, nil, http.StatusForbidden, nil)}
	client = NewWithHTTPClient(config.OzonConfig{ClientID: "client", APIKey: "key"}, "https://api-seller.test", httpClient, 100)
	err := client.CheckProductsAccess(context.Background())
	if err == nil || !strings.Contains(err.Error(), "role") {
		t.Fatalf("missing role must surface the API message, got %v", err)
	}
}

func TestProductArticlesPaginates(t *testing.T) {
	pages := []map[string]any{
		{"result": map[string]any{
			"items": []map[string]any{
				{"product_id": 1, "offer_id": "A-1", "sku": 11},
				{"product_id": 2, "offer_id": "A-2", "sku": 22},
			},
			"total":   3,
			"last_id": "cursor-1",
		}},
		{"result": map[string]any{
			"items":   []map[string]any{{"product_id": 3, "offer_id": "A-3", "sku": 33}},
			"total":   3,
			"last_id": "cursor-2",
		}},
	}
	httpClient := &http.Client{Transport: productListTransport(t, pages, http.StatusOK, nil)}
	client := NewWithHTTPClient(config.OzonConfig{ClientID: "client", APIKey: "key"}, "https://api-seller.test", httpClient, 2)

	articles, err := client.ProductArticles(context.Background())
	if err != nil {
		t.Fatalf("ProductArticles: %v", err)
	}
	want := map[string]string{"11": "A-1", "22": "A-2", "33": "A-3"}
	if len(articles) != len(want) {
		t.Fatalf("articles = %v, want %v", articles, want)
	}
	for k, v := range want {
		if articles[k] != v {
			t.Fatalf("articles[%s] = %q, want %q", k, articles[k], v)
		}
	}
}

func TestFetchReviewsMapsOzonResponse(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v3/product/list" {
			return jsonResponse(http.StatusForbidden, map[string]any{"code": 7, "message": "no role"}), nil
		}
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
		if r.URL.Path == "/v3/product/list" {
			return jsonResponse(http.StatusForbidden, map[string]any{"code": 7, "message": "no role"}), nil
		}
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

func TestFetchQuestionsMapsOzonResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/question/answer/list" {
			_, _ = w.Write([]byte(`{
				"answers": [
					{"text": "Да, подходит", "author_name": "seller", "status": "published"}
				],
				"has_next": false,
				"last_id": ""
			}`))
			return
		}
		if r.URL.Path != "/v1/question/list" {
			t.Errorf("path = %q, want /v1/question/list", r.URL.Path)
		}
		if got := r.Header.Get("Client-Id"); got != "cid" {
			t.Errorf("Client-Id = %q", got)
		}
		if got := r.Header.Get("Api-Key"); got != "key" {
			t.Errorf("Api-Key = %q", got)
		}
		_, _ = w.Write([]byte(`{
			"items": [
				{
					"question_id": "qid-1",
					"sku": "987654",
					"text": "Этот товар подходит для детей?",
					"author_name": "Покупатель",
					"created_at": "2026-05-01T09:00:00Z"
				}
			],
			"has_next": false,
			"last_id": ""
		}`))
	}))
	defer srv.Close()

	c := NewWithHTTPClient(config.OzonConfig{ClientID: "cid", APIKey: "key"}, srv.URL, srv.Client(), 100)
	questions, nextCursor, err := c.FetchQuestions(context.Background(), time.Time{}, "")
	if err != nil {
		t.Fatalf("fetch questions: %v", err)
	}
	if nextCursor != "" {
		t.Fatalf("next cursor = %q, want empty", nextCursor)
	}
	if len(questions) != 1 {
		t.Fatalf("questions len = %d", len(questions))
	}
	q := questions[0]
	if q.ExternalQuestionID != "qid-1" {
		t.Errorf("ExternalQuestionID = %q", q.ExternalQuestionID)
	}
	if q.ExternalSKU != "987654" {
		t.Errorf("ExternalSKU = %q", q.ExternalSKU)
	}
	if q.ExternalProductID != "987654" {
		t.Errorf("ExternalProductID = %q", q.ExternalProductID)
	}
	if q.AuthorName != "Покупатель" {
		t.Errorf("AuthorName = %q", q.AuthorName)
	}
	if q.Text != "Этот товар подходит для детей?" {
		t.Errorf("Text = %q", q.Text)
	}
	if q.Answer == nil || q.Answer.Text != "Да, подходит" {
		t.Errorf("Answer = %+v", q.Answer)
	}
	expected := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	if !q.CreatedAtMP.Equal(expected) {
		t.Errorf("CreatedAtMP = %v", q.CreatedAtMP)
	}
}

func TestFetchQuestionsOzonPaginates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ozonQuestionListRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.LastID != "cursor-q2" {
			t.Errorf("last_id = %q, want cursor-q2", req.LastID)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[],"has_next":false,"last_id":""}`))
	}))
	defer srv.Close()

	c := NewWithHTTPClient(config.OzonConfig{ClientID: "cid", APIKey: "key"}, srv.URL, srv.Client(), 100)
	_, nextCursor, err := c.FetchQuestions(context.Background(), time.Time{}, "cursor-q2")
	if err != nil {
		t.Fatalf("fetch questions: %v", err)
	}
	if nextCursor != "" {
		t.Fatalf("next cursor = %q", nextCursor)
	}
}

func TestPublishQuestionAnswerOzon(t *testing.T) {
	var gotPath, gotClientID, gotKey, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotClientID = r.Header.Get("Client-Id")
		gotKey = r.Header.Get("Api-Key")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewWithHTTPClient(config.OzonConfig{ClientID: "cid", APIKey: "key"}, srv.URL, srv.Client(), 0)
	if err := c.PublishQuestionAnswer(context.Background(), "qid-1", "987654", "Да, подходит!"); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if gotPath != "/v1/question/answer/create" {
		t.Errorf("path = %q", gotPath)
	}
	if gotClientID != "cid" || gotKey != "key" {
		t.Errorf("headers cid=%q key=%q", gotClientID, gotKey)
	}
	if !strings.Contains(gotBody, `"question_id":"qid-1"`) {
		t.Errorf("body missing question_id: %s", gotBody)
	}
	if !strings.Contains(gotBody, `"sku":"987654"`) {
		t.Errorf("body missing sku: %s", gotBody)
	}
	if !strings.Contains(gotBody, `"text":"Да, подходит!"`) {
		t.Errorf("body missing text: %s", gotBody)
	}
}

func TestPublishQuestionAnswerOzonError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))
	defer srv.Close()
	c := NewWithHTTPClient(config.OzonConfig{ClientID: "cid", APIKey: "key"}, srv.URL, srv.Client(), 0)
	if err := c.PublishQuestionAnswer(context.Background(), "qid-1", "sku-1", "x"); err == nil {
		t.Fatal("expected error on non-2xx")
	}
}

func TestPublishReplyCreatesComment(t *testing.T) {
	var gotPath, gotBody, gotClient, gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotClient = r.Header.Get("Client-Id")
		gotKey = r.Header.Get("Api-Key")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"comment_id":555}`))
	}))
	defer srv.Close()

	c := NewWithHTTPClient(config.OzonConfig{ClientID: "cid", APIKey: "key"}, srv.URL, srv.Client(), 0)
	if err := c.PublishReply(context.Background(), "9001", "Спасибо!"); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if gotPath != "/v1/review/comment/create" {
		t.Fatalf("path = %s", gotPath)
	}
	if gotClient != "cid" || gotKey != "key" {
		t.Fatalf("headers c=%q k=%q", gotClient, gotKey)
	}
	if !strings.Contains(gotBody, `"review_id":9001`) || !strings.Contains(gotBody, `"text":"Спасибо!"`) {
		t.Fatalf("body = %s", gotBody)
	}
}
