package wb

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

func TestPublishReplyPostsAnswer(t *testing.T) {
	var gotBody string
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/feedbacks/answer" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewWithHTTPClient(config.WBConfig{Token: "tok"}, srv.URL, srv.Client(), 0)
	if err := c.PublishReply(context.Background(), "fb-1", "Спасибо!"); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if gotAuth != "tok" {
		t.Fatalf("auth = %q", gotAuth)
	}
	if !strings.Contains(gotBody, `"id":"fb-1"`) || !strings.Contains(gotBody, `"text":"Спасибо!"`) {
		t.Fatalf("body = %s", gotBody)
	}
}

func TestFetchQuestionsMapsWBResponse(t *testing.T) {
	var call int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if r.URL.Path != "/api/v1/questions" {
			t.Errorf("path = %q, want /api/v1/questions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "tok" {
			t.Errorf("Authorization = %q", got)
		}
		wantAnswered := "false"
		if call == 2 {
			wantAnswered = "true"
		}
		if got := r.URL.Query().Get("isAnswered"); got != wantAnswered {
			t.Errorf("isAnswered = %q, want %q", got, wantAnswered)
		}
		if got := r.URL.Query().Get("take"); got != "10" {
			t.Errorf("take = %q", got)
		}
		if got := r.URL.Query().Get("skip"); got != "0" {
			t.Errorf("skip = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"questions": [
					{
						"id": "qst-1",
						"text": "Есть ли в наличии?",
						"createdDate": "2024-10-01T12:00:00+03:00",
						"userName": "Пользователь",
						"answer": {
							"text": "Да, есть"
						},
						"productDetails": {
							"nmId": 123456,
							"supplierArticle": "MY-ART"
						}
					}
				]
			},
			"error": false,
			"errorText": ""
		}`))
	}))
	defer srv.Close()

	c := NewWithHTTPClient(config.WBConfig{Token: "tok"}, srv.URL, srv.Client(), 10)
	questions, nextCursor, err := c.FetchQuestions(context.Background(), time.Time{}, "")
	if err != nil {
		t.Fatalf("fetch questions: %v", err)
	}
	if nextCursor != "answered:0" {
		t.Fatalf("next cursor = %q, want answered stage", nextCursor)
	}
	if len(questions) != 1 {
		t.Fatalf("questions len = %d", len(questions))
	}
	q := questions[0]
	if q.ExternalQuestionID != "qst-1" {
		t.Errorf("ExternalQuestionID = %q", q.ExternalQuestionID)
	}
	if q.ExternalProductID != "123456" {
		t.Errorf("ExternalProductID = %q", q.ExternalProductID)
	}
	if q.SellerArticle != "MY-ART" {
		t.Errorf("SellerArticle = %q", q.SellerArticle)
	}
	if q.AuthorName != "Пользователь" {
		t.Errorf("AuthorName = %q", q.AuthorName)
	}
	if q.Text != "Есть ли в наличии?" {
		t.Errorf("Text = %q", q.Text)
	}
	if q.Answer == nil || q.Answer.Text != "Да, есть" {
		t.Errorf("Answer = %+v", q.Answer)
	}

	_, nextCursor, err = c.FetchQuestions(context.Background(), time.Time{}, nextCursor)
	if err != nil {
		t.Fatalf("fetch answered questions: %v", err)
	}
	if nextCursor != "" {
		t.Fatalf("final cursor = %q", nextCursor)
	}
}

func TestPublishQuestionAnswerWB(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewWithHTTPClient(config.WBConfig{Token: "tok"}, srv.URL, srv.Client(), 0)
	if err := c.PublishQuestionAnswer(context.Background(), "qst-1", "", "Да, в наличии!"); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if gotMethod != http.MethodPatch {
		t.Errorf("method = %q, want PATCH", gotMethod)
	}
	if gotPath != "/api/v1/questions" {
		t.Errorf("path = %q", gotPath)
	}
	if gotAuth != "tok" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if !strings.Contains(gotBody, `"id":"qst-1"`) {
		t.Errorf("body missing id: %s", gotBody)
	}
	if !strings.Contains(gotBody, `"text":"Да, в наличии!"`) {
		t.Errorf("body missing answer text: %s", gotBody)
	}
	if !strings.Contains(gotBody, `"state":"wbRu"`) {
		t.Errorf("body missing state: %s", gotBody)
	}
}

func TestPublishQuestionAnswerWBError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	c := NewWithHTTPClient(config.WBConfig{Token: "tok"}, srv.URL, srv.Client(), 0)
	if err := c.PublishQuestionAnswer(context.Background(), "qst-1", "", "x"); err == nil {
		t.Fatal("expected error on non-2xx")
	}
}

func TestPublishReplyError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errorText":"bad"}`))
	}))
	defer srv.Close()
	c := NewWithHTTPClient(config.WBConfig{Token: "tok"}, srv.URL, srv.Client(), 0)
	if err := c.PublishReply(context.Background(), "fb-1", "x"); err == nil {
		t.Fatal("expected error on non-204")
	}
}
