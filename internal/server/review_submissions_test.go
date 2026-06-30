package server

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestReviewSubmissionCreatesPendingHiddenReview(t *testing.T) {
	s := newAuthTestServer(t)
	body, contentType := submissionBody(t, map[string]string{
		"sellerArticle":  "673320",
		"rating":         "5",
		"authorName":     "Анна",
		"authorEmail":    "anna@example.com",
		"text":           "Очень понравилось качество",
		"privacyConsent": "true",
		"termsConsent":   "true",
		"openedAt":       strconv.FormatInt(time.Now().Add(-5*time.Second).UnixMilli(), 10),
	}, true)

	req := httptest.NewRequest(http.MethodPost, "/api/review-submissions", body)
	req.Header.Set("Content-Type", contentType)
	req.RemoteAddr = "203.0.113.10:1234"
	rec := httptest.NewRecorder()
	s.handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var submitted struct {
		Status   string `json:"status"`
		ReviewID uint   `json:"reviewId"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&submitted); err != nil {
		t.Fatalf("decode submit: %v", err)
	}
	if submitted.Status != "pending" || submitted.ReviewID == 0 {
		t.Fatalf("unexpected submit response: %+v", submitted)
	}

	rec = httptest.NewRecorder()
	s.handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/reviews?apply_config=0", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("public reviews status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "Очень понравилось") {
		t.Fatalf("pending review leaked publicly: %s", rec.Body.String())
	}

	cookie := loginTestAdmin(t, s)
	req = httptest.NewRequest(http.MethodGet, "/admin/api/reviews?status=pending", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin pending status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var listed adminReviewsResponse
	if err := json.NewDecoder(rec.Body).Decode(&listed); err != nil {
		t.Fatalf("decode admin pending: %v", err)
	}
	if listed.Total != 1 || listed.Reviews[0].AuthorEmail != "anna@example.com" || len(listed.Reviews[0].Media) != 1 {
		t.Fatalf("unexpected pending list: %+v", listed)
	}

	mediaURL := listed.Reviews[0].Media[0].URL
	rec = httptest.NewRecorder()
	s.handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, mediaURL, nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("pending media without auth status = %d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodGet, mediaURL, nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	s.handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("pending media with auth status = %d", rec.Code)
	}

	if err := s.store.SetReviewStatus(context.Background(), submitted.ReviewID, "approved"); err != nil {
		t.Fatalf("approve review: %v", err)
	}
	rec = httptest.NewRecorder()
	s.handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, mediaURL, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("approved media public status = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	s.handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/reviews?apply_config=0", nil))
	if !strings.Contains(rec.Body.String(), "Очень понравилось") {
		t.Fatalf("approved review missing publicly: %s", rec.Body.String())
	}
}

func TestReviewSubmissionRejectsSpamAndRateLimits(t *testing.T) {
	s := newAuthTestServer(t)
	fields := map[string]string{
		"sellerArticle":  "673320",
		"rating":         "5",
		"authorName":     "Анна",
		"authorEmail":    "anna@example.com",
		"text":           "Очень понравилось качество",
		"privacyConsent": "true",
		"termsConsent":   "true",
		"openedAt":       strconv.FormatInt(time.Now().Add(-5*time.Second).UnixMilli(), 10),
		"website":        "bot",
	}
	body, contentType := submissionBody(t, fields, false)
	req := httptest.NewRequest(http.MethodPost, "/api/review-submissions", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	s.handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("honeypot status = %d", rec.Code)
	}

	for i := 0; i < 2; i++ {
		fields["website"] = ""
		fields["authorEmail"] = "rate" + strconv.Itoa(i) + "@example.com"
		body, contentType = submissionBody(t, fields, false)
		req = httptest.NewRequest(http.MethodPost, "/api/review-submissions", body)
		req.Header.Set("Content-Type", contentType)
		req.RemoteAddr = "203.0.113.20:1234"
		rec = httptest.NewRecorder()
		s.handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("rate setup submit %d status = %d, body=%s", i, rec.Code, rec.Body.String())
		}
	}
	fields["authorEmail"] = "rate0@example.com"
	body, contentType = submissionBody(t, fields, false)
	req = httptest.NewRequest(http.MethodPost, "/api/review-submissions", body)
	req.Header.Set("Content-Type", contentType)
	req.RemoteAddr = "203.0.113.21:1234"
	rec = httptest.NewRecorder()
	s.handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("email/article daily limit status = %d", rec.Code)
	}
}

func submissionBody(t *testing.T, fields map[string]string, withMedia bool) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write field: %v", err)
		}
	}
	if withMedia {
		part, err := writer.CreateFormFile("media", "review.png")
		if err != nil {
			t.Fatalf("create media: %v", err)
		}
		if _, err := part.Write([]byte("\x89PNG\r\n\x1a\nreview-image")); err != nil {
			t.Fatalf("write media: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	return &body, writer.FormDataContentType()
}
