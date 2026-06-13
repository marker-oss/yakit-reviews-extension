package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWidgetConfigPublishAndPublicFetch(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)
	csrf := getCSRFToken(t, s, cookie)

	body := `{"theme":{"accent":"#245f4f"},"layout":{"mode":"compact"}}`
	req := httptest.NewRequest(http.MethodPost, "/admin/api/widget-config/product", strings.NewReader(body))
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("publish status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var published struct {
		Version int `json:"version"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&published); err != nil {
		t.Fatalf("decode publish: %v", err)
	}
	if published.Version != 1 {
		t.Fatalf("expected version 1, got %d", published.Version)
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/api/widget-config/product", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"#245f4f"`) {
		t.Fatalf("admin get status = %d, body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/widget-config?context=product", nil)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/widget-config", s.handlePublicWidgetConfig)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"compact"`) {
		t.Fatalf("public get status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestWidgetConfigRollback(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)
	csrf := getCSRFToken(t, s, cookie)
	mux := s.adminMux()

	for _, body := range []string{`{"theme":{"accent":"#111111"}}`, `{"theme":{"accent":"#222222"}}`} {
		req := httptest.NewRequest(http.MethodPost, "/admin/api/widget-config/homepage", strings.NewReader(body))
		req.AddCookie(cookie)
		req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
		req.Header.Set(csrfHeaderName, csrf)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("publish status = %d, body=%s", rec.Code, rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/api/widget-config/homepage/rollback/1", nil)
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("rollback status = %d, body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/api/widget-config/homepage", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"#111111"`) {
		t.Fatalf("expected v1 active, status = %d, body=%s", rec.Code, rec.Body.String())
	}
}
