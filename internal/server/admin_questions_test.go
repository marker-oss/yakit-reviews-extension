package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"reviews/internal/marketplace"
	"reviews/internal/store"
)

func TestAdminQuestionsRequiresAuth(t *testing.T) {
	s := newAuthTestServer(t)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/api/questions", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without session, got %d", rec.Code)
	}
}

func TestAdminQuestionAnswerFlow(t *testing.T) {
	s := newAuthTestServer(t)
	pub := &fakeQuestionPublisher{}
	s.questionAnswerPublishers = map[string]marketplace.QuestionAnswerPublisher{"wb": pub}

	ctx := context.Background()
	q, err := s.store.UpsertQuestion(ctx, store.QuestionInput{
		Marketplace:        "wb",
		ExternalQuestionID: "wbq-1",
		ExternalProductID:  "p1",
		SellerArticle:      "art1",
		AuthorName:         "Иван",
		Text:               "Есть в наличии?",
		CreatedAtMP:        testTime(),
	})
	if err != nil {
		t.Fatalf("upsert question: %v", err)
	}

	cookie := loginTestAdmin(t, s)
	csrf := getCSRFToken(t, s, cookie)

	// PUT /admin/api/questions/{id}/answer
	reqBody := strings.NewReader(`{"text":"Да, есть в наличии!"}`)
	req := httptest.NewRequest(http.MethodPut, "/admin/api/questions/"+strconv.FormatUint(uint64(q.ID), 10)+"/answer", reqBody)
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT answer status %d, body=%s", rec.Code, rec.Body.String())
	}

	// Verify question is now answered+visible and publish was attempted
	got, err := s.store.QuestionByID(ctx, q.ID)
	if err != nil {
		t.Fatalf("QuestionByID: %v", err)
	}
	if got.Status != "answered" {
		t.Fatalf("expected answered, got %q", got.Status)
	}
	if got.Visibility != "visible" {
		t.Fatalf("expected visible, got %q", got.Visibility)
	}
	if pub.calls != 1 {
		t.Fatalf("expected 1 publisher call, got %d", pub.calls)
	}
	if got.AnswerPublishState == nil || *got.AnswerPublishState != "published" {
		t.Fatalf("expected published state, got %v", got.AnswerPublishState)
	}

	// GET /admin/api/questions returns the question
	req2 := httptest.NewRequest(http.MethodGet, "/admin/api/questions", nil)
	req2.AddCookie(cookie)
	rec2 := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("GET questions status %d, body=%s", rec2.Code, rec2.Body.String())
	}
	var listResp struct {
		Questions []map[string]any `json:"questions"`
	}
	if err := json.NewDecoder(rec2.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listResp.Questions) != 1 {
		t.Fatalf("expected 1 question in list, got %d", len(listResp.Questions))
	}
}

func TestAdminQuestionAnswerRetry409WhenPublished(t *testing.T) {
	s := newAuthTestServer(t)
	pub := &fakeQuestionPublisher{}
	s.questionAnswerPublishers = map[string]marketplace.QuestionAnswerPublisher{"wb": pub}

	ctx := context.Background()
	q, _ := s.store.UpsertQuestion(ctx, store.QuestionInput{
		Marketplace:        "wb",
		ExternalQuestionID: "wbq-2",
		ExternalProductID:  "p1",
		SellerArticle:      "art1",
		AuthorName:         "Петр",
		Text:               "Какой размер?",
		CreatedAtMP:        testTime(),
	})

	// Set answer and publish so state becomes "published"
	_ = s.store.SetQuestionAnswer(ctx, q.ID, "Размер S")
	updatedQ, _ := s.store.QuestionByID(ctx, q.ID)
	s.publishQuestionAnswer(ctx, updatedQ)

	got, _ := s.store.QuestionByID(ctx, q.ID)
	if got.AnswerPublishState == nil || *got.AnswerPublishState != "published" {
		t.Fatalf("expected published, got %v", got.AnswerPublishState)
	}

	cookie := loginTestAdmin(t, s)
	csrf := getCSRFToken(t, s, cookie)

	// Retry must return 409
	req := httptest.NewRequest(http.MethodPost, "/admin/api/questions/"+strconv.FormatUint(uint64(q.ID), 10)+"/answer/retry", strings.NewReader(""))
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d body=%s", rec.Code, rec.Body.String())
	}
}
