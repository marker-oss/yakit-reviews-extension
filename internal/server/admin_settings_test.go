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

func TestAdminSettingsRequiresAuth(t *testing.T) {
	s := newAuthTestServer(t)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/api/settings", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without session, got %d", rec.Code)
	}
}

func TestAdminSettingsRoundTrip(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)

	// Defaults to empty before anything is stored.
	req := httptest.NewRequest(http.MethodGet, "/admin/api/settings", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var got settingsResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.AgreementURL != "" {
		t.Fatalf("agreement url = %q, want empty", got.AgreementURL)
	}

	// Save a URL.
	csrf := getCSRFToken(t, s, cookie)
	req = httptest.NewRequest(http.MethodPut, "/admin/api/settings",
		strings.NewReader(`{"agreementUrl":"https://shop.example/agreement"}`))
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rec = httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put status = %d, body=%s", rec.Code, rec.Body.String())
	}

	// Public submission config now reflects the stored URL.
	rec = httptest.NewRecorder()
	s.handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/review-submission-config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("config status = %d", rec.Code)
	}
	var cfg submissionConfigResponse
	if err := json.NewDecoder(rec.Body).Decode(&cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if cfg.PrivacyURL != "https://shop.example/agreement" {
		t.Fatalf("submission config privacyUrl = %q", cfg.PrivacyURL)
	}
}

func TestAdminSettingsReviewTermsURL(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)
	csrf := getCSRFToken(t, s, cookie)

	req := httptest.NewRequest(http.MethodPut, "/admin/api/settings",
		strings.NewReader(`{"reviewTermsUrl":"https://shop.example/review-terms"}`))
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var got settingsResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ReviewTermsURL != "https://shop.example/review-terms" {
		t.Fatalf("review terms url = %q, want stored value", got.ReviewTermsURL)
	}

	rec = httptest.NewRecorder()
	s.handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/review-submission-config", nil))
	var cfg submissionConfigResponse
	if err := json.NewDecoder(rec.Body).Decode(&cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if cfg.ReviewTermsURL != "https://shop.example/review-terms" {
		t.Fatalf("submission config reviewTermsUrl = %q, want stored value", cfg.ReviewTermsURL)
	}
}

func TestReviewTermsURLFallsBackToEnv(t *testing.T) {
	s := newAuthTestServer(t)
	s.cfg.ReviewTermsURL = "https://env.example/terms"

	rec := httptest.NewRecorder()
	s.handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/review-submission-config", nil))
	var cfg submissionConfigResponse
	if err := json.NewDecoder(rec.Body).Decode(&cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if cfg.ReviewTermsURL != "https://env.example/terms" {
		t.Fatalf("submission config reviewTermsUrl = %q, want env fallback", cfg.ReviewTermsURL)
	}
}

func TestAdminSettingsCatalogSource(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)
	csrf := getCSRFToken(t, s, cookie)

	req := httptest.NewRequest(http.MethodPut, "/admin/api/settings",
		strings.NewReader(`{"shopOrigin":"https://shop.example","sitemapUrl":"https://shop.example/custom-sitemap.xml"}`))
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var got settingsResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ShopOrigin != "https://shop.example" || got.SitemapURL != "https://shop.example/custom-sitemap.xml" {
		t.Fatalf("unexpected settings: %+v", got)
	}
}

func TestEffectiveSitemapURL(t *testing.T) {
	s := newAuthTestServer(t)
	ctx := context.Background()
	s.cfg.SitemapURL = "https://env-default.example/sitemap.xml"

	// Falls back to env when nothing is stored.
	if got := s.effectiveSitemapURL(ctx); got != "https://env-default.example/sitemap.xml" {
		t.Fatalf("env fallback = %q", got)
	}

	// Shop origin derives <origin>/sitemap.xml.
	if err := s.store.SetAppSetting(ctx, store.SettingShopOrigin, "https://shop.example/"); err != nil {
		t.Fatalf("set origin: %v", err)
	}
	if got := s.effectiveSitemapURL(ctx); got != "https://shop.example/sitemap.xml" {
		t.Fatalf("origin-derived = %q", got)
	}

	// Explicit sitemap URL wins over the derived origin.
	if err := s.store.SetAppSetting(ctx, store.SettingSitemapURL, "https://shop.example/custom.xml"); err != nil {
		t.Fatalf("set sitemap: %v", err)
	}
	if got := s.effectiveSitemapURL(ctx); got != "https://shop.example/custom.xml" {
		t.Fatalf("explicit sitemap = %q", got)
	}
}

func TestAdminSettingsPublishToggles(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)
	csrf := getCSRFToken(t, s, cookie)

	// PUT all three toggles.
	req := httptest.NewRequest(http.MethodPut, "/admin/api/settings",
		strings.NewReader(`{"publish_replies_wb":"true","publish_replies_ym":"false","publish_replies_ozon":"true"}`))
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put status = %d, body=%s", rec.Code, rec.Body.String())
	}

	// GET and verify they round-trip.
	req2 := httptest.NewRequest(http.MethodGet, "/admin/api/settings", nil)
	req2.AddCookie(cookie)
	rec2 := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("get status = %d, body=%s", rec2.Code, rec2.Body.String())
	}
	var got settingsResponse
	if err := json.NewDecoder(rec2.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.PublishRepliesWb != "true" {
		t.Fatalf("publishRepliesWb = %q, want true", got.PublishRepliesWb)
	}
	if got.PublishRepliesYm != "false" {
		t.Fatalf("publishRepliesYm = %q, want false", got.PublishRepliesYm)
	}
	if got.PublishRepliesOzon != "true" {
		t.Fatalf("publishRepliesOzon = %q, want true", got.PublishRepliesOzon)
	}
}

func TestAdminSettingsRejectsNonURL(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)
	csrf := getCSRFToken(t, s, cookie)
	req := httptest.NewRequest(http.MethodPut, "/admin/api/settings",
		strings.NewReader(`{"agreementUrl":"javascript:alert(1)"}`))
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-http URL, got %d body=%s", rec.Code, rec.Body.String())
	}
}
