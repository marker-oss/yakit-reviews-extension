package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"reviews/internal/store"
)

// levelOf returns the Level for a check ID, or "" if absent.
func levelOf(items []DiagItem, id string) string {
	for _, c := range items {
		if c.ID == id {
			return c.Level
		}
	}
	return ""
}

func getDiagnostics(t *testing.T, s *Server, cookie *http.Cookie) []DiagItem {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/admin/api/diagnostics", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Checks []DiagItem `json:"checks"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	return got.Checks
}

func TestDiagnosticsFlagsMissingShopOrigin(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)
	if got := levelOf(getDiagnostics(t, s, cookie), "cors"); got != "fail" {
		t.Fatalf("cors level = %q, want fail when origin unset", got)
	}
}

func TestDiagnosticsCorsOkWhenOriginSet(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)
	if err := s.store.SetAppSetting(context.Background(), store.SettingShopOrigin, "https://shop.example"); err != nil {
		t.Fatal(err)
	}
	if got := levelOf(getDiagnostics(t, s, cookie), "cors"); got != "ok" {
		t.Fatalf("cors level = %q, want ok", got)
	}
}

func probe(t *testing.T, s *Server, cookie *http.Cookie, body string) []DiagItem {
	t.Helper()
	csrf := getCSRFToken(t, s, cookie)
	req := httptest.NewRequest(http.MethodPost, "/admin/api/diagnostics/probe", strings.NewReader(body))
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK { // must soft-degrade, never 500
		t.Fatalf("probe status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Checks []DiagItem `json:"checks"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	return got.Checks
}

func TestProbeReachableSite(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)
	shop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer shop.Close()
	if err := s.store.SetAppSetting(context.Background(), store.SettingShopOrigin, shop.URL); err != nil {
		t.Fatal(err)
	}
	if got := levelOf(probe(t, s, cookie, `{}`), "site-reachable"); got != "ok" {
		t.Fatalf("site-reachable = %q, want ok", got)
	}
}

func TestProbeUnreachableSiteDoesNotError(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)
	if err := s.store.SetAppSetting(context.Background(), store.SettingShopOrigin, "http://127.0.0.1:1"); err != nil {
		t.Fatal(err)
	}
	if got := levelOf(probe(t, s, cookie, `{}`), "site-reachable"); got != "fail" {
		t.Fatalf("site-reachable = %q, want fail (soft-degrade)", got)
	}
}
