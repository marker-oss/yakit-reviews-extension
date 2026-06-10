package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAdminDashboardAndMarketplaces(t *testing.T) {
	s := newAuthTestServer(t)
	s.cfg.Marketplaces = []MarketplaceStatus{
		{ID: "wb", Enabled: true, Configured: true},
		{ID: "ym", Enabled: false, Configured: false},
	}
	cookie := loginTestAdmin(t, s)
	seedAdminReview(t, s, "w1", 5)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/dashboard", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var dash struct {
		TotalReviews int64 `json:"total_reviews"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&dash); err != nil {
		t.Fatalf("decode dashboard: %v", err)
	}
	if dash.TotalReviews != 1 {
		t.Fatalf("total reviews = %d", dash.TotalReviews)
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/api/marketplaces", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("marketplaces status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if !json.Valid(rec.Body.Bytes()) {
		t.Fatalf("invalid marketplaces json: %s", rec.Body.String())
	}
}

func TestAdminTriggerSync(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)
	called := make(chan []string, 1)
	s.cfg.TriggerSync = func(marketplaces []string) {
		called <- marketplaces
	}
	csrf := getCSRFToken(t, s, cookie)

	req := httptest.NewRequest(http.MethodPost, "/admin/api/sync?marketplace=wb", nil)
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("sync status = %d, body=%s", rec.Code, rec.Body.String())
	}

	select {
	case got := <-called:
		if len(got) != 1 || got[0] != "wb" {
			t.Fatalf("trigger marketplaces = %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("sync trigger was not called")
	}
}

func TestAdminTriggerSyncDisabled(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)
	csrf := getCSRFToken(t, s, cookie)

	req := httptest.NewRequest(http.MethodPost, "/admin/api/sync", nil)
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("sync disabled status = %d, body=%s", rec.Code, rec.Body.String())
	}
}
