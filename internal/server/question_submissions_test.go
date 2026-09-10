package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestQuestionSubmissionCreatesPendingHidden(t *testing.T) {
	s := newAuthTestServer(t)

	fields := url.Values{
		"sellerArticle":  {"A100"},
		"authorName":     {"Иван"},
		"authorEmail":    {"ivan@example.com"},
		"text":           {"Когда будет в наличии?"},
		"privacyConsent": {"true"},
		"openedAt":       {strconv.FormatInt(time.Now().Add(-6*time.Second).UnixMilli(), 10)},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/questions", strings.NewReader(fields.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "203.0.113.30:1234"
	rec := httptest.NewRecorder()
	s.handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("submit status = %d, body=%s", rec.Code, rec.Body.String())
	}

	var submitted struct {
		Status     string `json:"status"`
		QuestionID uint   `json:"questionId"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&submitted); err != nil {
		t.Fatalf("decode submit: %v", err)
	}
	if submitted.Status != "pending" || submitted.QuestionID == 0 {
		t.Fatalf("unexpected submit response: %+v", submitted)
	}

	// Verify the question is stored hidden/pending and does NOT appear in public list.
	rec2 := httptest.NewRecorder()
	s.handler().ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/questions?article=A100", nil))
	if rec2.Code != http.StatusOK {
		t.Fatalf("public questions status = %d", rec2.Code)
	}
	if strings.Contains(rec2.Body.String(), "Когда будет в наличии") {
		t.Fatalf("pending question leaked publicly: %s", rec2.Body.String())
	}
}

func TestQuestionSubmissionRequiresConsent(t *testing.T) {
	s := newAuthTestServer(t)

	fields := url.Values{
		"sellerArticle": {"A100"},
		"authorName":    {"Иван"},
		"authorEmail":   {"ivan@example.com"},
		"text":          {"Когда будет в наличии?"},
		// privacyConsent omitted
		"openedAt": {strconv.FormatInt(time.Now().Add(-6*time.Second).UnixMilli(), 10)},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/questions", strings.NewReader(fields.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("no consent: expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestQuestionSubmissionRejectsHoneypot(t *testing.T) {
	s := newAuthTestServer(t)

	fields := url.Values{
		"sellerArticle":  {"A100"},
		"authorName":     {"Иван"},
		"authorEmail":    {"ivan@example.com"},
		"text":           {"Когда будет в наличии?"},
		"privacyConsent": {"true"},
		"openedAt":       {strconv.FormatInt(time.Now().Add(-6*time.Second).UnixMilli(), 10)},
		"website":        {"bot-fill"},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/questions", strings.NewReader(fields.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("honeypot: expected 429, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestQuestionSubmissionNotInPublicListUntilAnswered(t *testing.T) {
	s := newAuthTestServer(t)

	// Submit a question.
	fields := url.Values{
		"sellerArticle":  {"B200"},
		"authorName":     {"Мария"},
		"authorEmail":    {"maria@example.com"},
		"text":           {"Есть размер XL?"},
		"privacyConsent": {"true"},
		"openedAt":       {strconv.FormatInt(time.Now().Add(-6*time.Second).UnixMilli(), 10)},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/questions", strings.NewReader(fields.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "203.0.113.31:1234"
	rec := httptest.NewRecorder()
	s.handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit: %d %s", rec.Code, rec.Body.String())
	}
	var submitted struct {
		QuestionID uint `json:"questionId"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&submitted)

	// Not in public list yet.
	rec2 := httptest.NewRecorder()
	s.handler().ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/questions?article=B200", nil))
	var pub1 struct {
		Questions []any `json:"questions"`
	}
	_ = json.NewDecoder(rec2.Body).Decode(&pub1)
	if len(pub1.Questions) != 0 {
		t.Fatalf("unanswered question must not be visible, got %d", len(pub1.Questions))
	}

	// Answer it via the store directly.
	if err := s.store.SetQuestionAnswer(context.Background(), submitted.QuestionID, "Да, есть XL"); err != nil {
		t.Fatalf("set answer: %v", err)
	}

	// Now it appears in public list.
	rec3 := httptest.NewRecorder()
	s.handler().ServeHTTP(rec3, httptest.NewRequest(http.MethodGet, "/api/questions?article=B200", nil))
	var pub2 struct {
		Questions []struct {
			Question string `json:"question"`
			Answer   string `json:"answer"`
		} `json:"questions"`
	}
	if err := json.NewDecoder(rec3.Body).Decode(&pub2); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(pub2.Questions) != 1 || pub2.Questions[0].Answer != "Да, есть XL" {
		t.Fatalf("answered question missing from public list: %+v", pub2)
	}
}

func TestQuestionSubmissionConfig(t *testing.T) {
	s := newAuthTestServer(t)
	rec := httptest.NewRecorder()
	s.handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/question-submission-config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("config status = %d", rec.Code)
	}
	var cfg struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if !cfg.Enabled {
		t.Fatalf("expected enabled=true")
	}
}
