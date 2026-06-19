package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"reviews/internal/marketplace"
)

func loginTestAdmin(t *testing.T, s *Server) *http.Cookie {
	t.Helper()
	mux := s.adminMux()
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/admin/api/setup",
		strings.NewReader(`{"login":"admin","password":"password1"}`)))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/api/login",
		strings.NewReader(`{"login":"admin","password":"password1"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body=%s", rec.Code, rec.Body.String())
	}
	cookie := firstCookie(rec, sessionCookieName)
	if cookie == nil {
		t.Fatal("missing session cookie")
	}
	return cookie
}

func seedAdminReview(t *testing.T, s *Server, extID string, rating int) uint {
	t.Helper()
	result, err := s.store.UpsertReview(context.Background(), marketplace.Review{
		Marketplace:       "wb",
		ExternalReviewID:  extID,
		ExternalProductID: "p1",
		Rating:            &rating,
		Text:              "good",
		CreatedAtMP:       time.Now(),
	})
	if err != nil {
		t.Fatalf("upsert review: %v", err)
	}
	return result.Review.ID
}

func TestAdminReviewsRequiresAuth(t *testing.T) {
	s := newAuthTestServer(t)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/api/reviews", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without session, got %d", rec.Code)
	}
}

func TestAdminReviewsListAndModerate(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)
	reviewID := seedAdminReview(t, s, "w1", 5)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/reviews", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var listed adminReviewsResponse
	if err := json.NewDecoder(rec.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if listed.Total != 1 || len(listed.Reviews) != 1 {
		t.Fatalf("unexpected list response: %+v", listed)
	}
	if listed.Reviews[0].Visibility != "visible" || listed.Reviews[0].Pinned {
		t.Fatalf("unexpected curation fields: %+v", listed.Reviews[0])
	}

	csrf := getCSRFToken(t, s, cookie)
	req = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/admin/api/reviews/%d", reviewID),
		strings.NewReader(`{"visibility":"hidden","pinned":true}`))
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rec = httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("moderate status = %d, body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/api/reviews?visibility=hidden", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("hidden list status = %d, body=%s", rec.Code, rec.Body.String())
	}
	listed = adminReviewsResponse{}
	if err := json.NewDecoder(rec.Body).Decode(&listed); err != nil {
		t.Fatalf("decode hidden list: %v", err)
	}
	if listed.Total != 1 || !listed.Reviews[0].Pinned || listed.Reviews[0].Visibility != "hidden" {
		t.Fatalf("review was not moderated: %+v", listed)
	}
}

func TestAdminReviewsSortAndPaginate(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)
	seedAdminReview(t, s, "low", 2)
	seedAdminReview(t, s, "high", 5)
	seedAdminReview(t, s, "mid", 3)

	// Sort by highest rating.
	req := httptest.NewRequest(http.MethodGet, "/admin/api/reviews?sort=highest", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("sorted list status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var listed adminReviewsResponse
	if err := json.NewDecoder(rec.Body).Decode(&listed); err != nil {
		t.Fatalf("decode sorted list: %v", err)
	}
	if listed.Total != 3 {
		t.Fatalf("expected total 3, got %d", listed.Total)
	}
	if got := listed.Reviews[0].Rating; got == nil || *got != 5 {
		t.Fatalf("highest sort: first rating = %v, want 5", got)
	}

	// Paginate: limit 1, offset 1 still reports the full total.
	req = httptest.NewRequest(http.MethodGet, "/admin/api/reviews?sort=highest&limit=1&offset=1", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	listed = adminReviewsResponse{}
	if err := json.NewDecoder(rec.Body).Decode(&listed); err != nil {
		t.Fatalf("decode page: %v", err)
	}
	if listed.Total != 3 {
		t.Fatalf("expected total 3 with pagination, got %d", listed.Total)
	}
	if len(listed.Reviews) != 1 {
		t.Fatalf("expected 1 item on page, got %d", len(listed.Reviews))
	}
	if got := listed.Reviews[0].Rating; got == nil || *got != 3 {
		t.Fatalf("second page item rating = %v, want 3 (mid)", got)
	}
}

func TestAdminReviewsBulkModerate(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)
	id1 := seedAdminReview(t, s, "b1", 5)
	id2 := seedAdminReview(t, s, "b2", 4)
	seedAdminReview(t, s, "b3", 3)

	csrf := getCSRFToken(t, s, cookie)
	body := fmt.Sprintf(`{"ids":[%d,%d],"visibility":"hidden","pinned":true}`, id1, id2)
	req := httptest.NewRequest(http.MethodPost, "/admin/api/reviews/bulk", strings.NewReader(body))
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("bulk status = %d, body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/api/reviews?visibility=hidden", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	var listed adminReviewsResponse
	if err := json.NewDecoder(rec.Body).Decode(&listed); err != nil {
		t.Fatalf("decode hidden list: %v", err)
	}
	if listed.Total != 2 {
		t.Fatalf("expected 2 hidden after bulk, got %d", listed.Total)
	}
	for _, rv := range listed.Reviews {
		if !rv.Pinned {
			t.Fatalf("bulk-moderated review should be pinned: %+v", rv)
		}
	}

	// Empty ids is a client error.
	req = httptest.NewRequest(http.MethodPost, "/admin/api/reviews/bulk", strings.NewReader(`{"ids":[],"pinned":true}`))
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rec = httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty ids should be 400, got %d", rec.Code)
	}
}

func TestAdminReviewDeleteRestoreAndReply(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)
	reviewID := seedAdminReview(t, s, "reply-delete", 5)
	csrf := getCSRFToken(t, s, cookie)

	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/admin/api/reviews/%d/reply", reviewID),
		strings.NewReader(`{"text":"Спасибо!"}`))
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("reply status = %d, body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/admin/api/reviews/%d", reviewID), nil)
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rec = httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/api/reviews", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	var listed adminReviewsResponse
	if err := json.NewDecoder(rec.Body).Decode(&listed); err != nil {
		t.Fatalf("decode active list: %v", err)
	}
	if listed.Total != 0 {
		t.Fatalf("deleted review should be hidden by default, got %+v", listed)
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/api/reviews?status=deleted", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	listed = adminReviewsResponse{}
	if err := json.NewDecoder(rec.Body).Decode(&listed); err != nil {
		t.Fatalf("decode deleted list: %v", err)
	}
	if listed.Total != 1 || listed.Reviews[0].Status != "deleted" || listed.Reviews[0].AdminReply == nil {
		t.Fatalf("expected deleted review with reply, got %+v", listed)
	}

	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/admin/api/reviews/%d/restore", reviewID), nil)
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rec = httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("restore status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminArticlePins(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)
	id1 := seedAdminReview(t, s, "pin-api-1", 5)
	id2 := seedAdminReview(t, s, "pin-api-2", 4)
	csrf := getCSRFToken(t, s, cookie)

	body := fmt.Sprintf(`{"reviewIds":[%d,%d,%d]}`, id1, id2, id1)
	req := httptest.NewRequest(http.MethodPut, "/admin/api/articles/107/pins", strings.NewReader(body))
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("replace pins status = %d, body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/api/articles/107/pins", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list pins status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var pins articlePinsResponse
	if err := json.NewDecoder(rec.Body).Decode(&pins); err != nil {
		t.Fatalf("decode pins: %v", err)
	}
	if len(pins.ReviewIDs) != 2 || pins.ReviewIDs[0] != id1 || pins.ReviewIDs[1] != id2 {
		t.Fatalf("unexpected pins: %+v", pins)
	}

	req = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/admin/api/articles/107/pins/%d", id1), nil)
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rec = httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("remove pin status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func getCSRFToken(t *testing.T, s *Server, session *http.Cookie) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/admin/api/csrf", nil)
	req.AddCookie(session)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("csrf status = %d, body=%s", rec.Code, rec.Body.String())
	}
	cookie := firstCookie(rec, csrfCookieName)
	if cookie == nil {
		t.Fatal("missing csrf cookie")
	}
	return cookie.Value
}
