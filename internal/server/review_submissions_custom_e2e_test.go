package server

import (
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"reviews/internal/store"
)

func publishCustomFieldsConfig(t *testing.T, s *Server, payload string) {
	t.Helper()
	cookie := loginTestAdmin(t, s)
	csrf := getCSRFToken(t, s, cookie)
	req := httptest.NewRequest(http.MethodPost, "/admin/api/widget-config/product",
		strings.NewReader(payload))
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("publish widget config status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestSubmissionConfigExposesCustomFields(t *testing.T) {
	s := newAuthTestServer(t)
	publishCustomFieldsConfig(t, s, `{"customFields":[
		{"id":"height","label":"Рост","type":"chips","options":["150-160","160-170"],"required":true},
		{"id":"junk","label":"","type":"chips","options":["a","b"]}
	]}`)

	rec := httptest.NewRecorder()
	s.handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/review-submission-config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("config status = %d", rec.Code)
	}
	var cfg submissionConfigResponse
	if err := json.NewDecoder(rec.Body).Decode(&cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if len(cfg.CustomFields) != 1 || cfg.CustomFields[0].ID != "height" || len(cfg.CustomFields[0].Options) != 2 {
		t.Fatalf("customFields mismatch: %+v", cfg.CustomFields)
	}
}

func customSubmission(t *testing.T, s *Server, customJSON string) *httptest.ResponseRecorder {
	t.Helper()
	var body strings.Builder
	writer := multipart.NewWriter(&body)
	fields := map[string]string{
		"sellerArticle":  "673320",
		"rating":         "5",
		"authorName":     "Анна",
		"authorEmail":    "custom-e2e@example.com",
		"text":           "Село отлично",
		"privacyConsent": "true",
		"openedAt":       strconv.FormatInt(time.Now().Add(-5*time.Second).UnixMilli(), 10),
	}
	if customJSON != "" {
		fields["custom"] = customJSON
	}
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write field: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/review-submissions", strings.NewReader(body.String()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.RemoteAddr = "203.0.113.77:1234"
	rec := httptest.NewRecorder()
	s.handler().ServeHTTP(rec, req)
	return rec
}

func TestReviewSubmissionStoresValidCustomAnswers(t *testing.T) {
	s := newAuthTestServer(t)
	publishCustomFieldsConfig(t, s, `{"customFields":[
		{"id":"height","label":"Рост","type":"chips","options":["150-160","160-170"],"required":true},
		{"id":"fit","label":"Как сидит","type":"chips","options":["маломерит","в размер"]}
	]}`)

	rec := customSubmission(t, s, `{"height":"160-170","fit":"в размер","bogus":"x"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit status = %d, body=%s", rec.Code, rec.Body.String())
	}

	reviews, _, err := s.store.ListReviewsWithCount(context.Background(), store.ReviewListFilter{
		ArticleSearch: "673320",
		Limit:         10,
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("want 1 review, got %d", len(reviews))
	}
	var custom map[string]string
	if err := json.Unmarshal([]byte(reviews[0].CustomData), &custom); err != nil {
		t.Fatalf("unmarshal CustomData %q: %v", reviews[0].CustomData, err)
	}
	if len(custom) != 2 || custom["height"] != "160-170" || custom["fit"] != "в размер" {
		t.Fatalf("stored custom mismatch: %+v", custom)
	}

	// Admin listing exposes the answers.
	cookie := loginTestAdmin(t, s)
	req := httptest.NewRequest(http.MethodGet, "/admin/api/reviews?status=pending", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	var listed adminReviewsResponse
	if err := json.NewDecoder(rec.Body).Decode(&listed); err != nil {
		t.Fatalf("decode admin list: %v", err)
	}
	if listed.Total != 1 || listed.Reviews[0].Custom["height"] != "160-170" {
		t.Fatalf("admin list custom mismatch: %+v", listed.Reviews[0])
	}
}

func TestReviewSubmissionRejectsInvalidCustomAnswers(t *testing.T) {
	s := newAuthTestServer(t)
	publishCustomFieldsConfig(t, s, `{"customFields":[
		{"id":"height","label":"Рост","type":"chips","options":["150-160","160-170"],"required":true}
	]}`)

	if rec := customSubmission(t, s, `{"height":"не из списка"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("off-menu value status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if rec := customSubmission(t, s, `{"fit":"в размер"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing required status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if rec := customSubmission(t, s, `not-json`); rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed custom status = %d, body=%s", rec.Code, rec.Body.String())
	}
}
