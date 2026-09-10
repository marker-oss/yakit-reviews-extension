package server

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"reviews/internal/store"
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

func TestSaveMarketplaceCredentials(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)
	csrf := getCSRFToken(t, s, cookie)

	enabled := true
	req := httptest.NewRequest(http.MethodPut, "/admin/api/marketplaces/ym/credentials",
		strings.NewReader(`{"enabled":true,"values":{"api_key":"api-1","business_id":"biz-1"}}`))
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("save credentials status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var saved struct {
		Marketplace MarketplaceStatus `json:"marketplace"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&saved); err != nil {
		t.Fatalf("decode saved: %v", err)
	}
	if saved.Marketplace.ID != "ym" || saved.Marketplace.Enabled != enabled || !saved.Marketplace.Configured {
		t.Fatalf("unexpected saved status: %+v", saved.Marketplace)
	}
	if !saved.Marketplace.Fields["api_key"] || !saved.Marketplace.Fields["business_id"] {
		t.Fatalf("expected masked fields set: %+v", saved.Marketplace.Fields)
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/api/marketplaces", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("marketplaces status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "api-1") || !strings.Contains(rec.Body.String(), `"configured":true`) {
		t.Fatalf("marketplaces leaked secret or missing configured status: %s", rec.Body.String())
	}
}

func TestAdminTriggerSync(t *testing.T) {
	csrfSetup := func(t *testing.T) (*Server, *http.Cookie, string) {
		s := newAuthTestServer(t)
		cookie := loginTestAdmin(t, s)
		return s, cookie, getCSRFToken(t, s, cookie)
	}
	doSync := func(t *testing.T, s *Server, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/admin/api/sync?marketplace=wb", nil)
		req.AddCookie(cookie)
		req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
		req.Header.Set(csrfHeaderName, csrf)
		rec := httptest.NewRecorder()
		s.adminMux().ServeHTTP(rec, req)
		return rec
	}

	tests := []struct {
		name    string
		trigger TriggerSyncFunc
		want    int
		body    string
	}{
		{
			name: "started returns 202 with arrays",
			trigger: func([]string) (SyncDispatch, error) {
				return SyncDispatch{Started: []string{"wb"}}, nil
			},
			want: http.StatusAccepted,
			body: `"started":["wb"]`,
		},
		{
			name: "partial start still 202",
			trigger: func([]string) (SyncDispatch, error) {
				return SyncDispatch{Started: []string{"ym"}, Busy: []string{"wb"}}, nil
			},
			want: http.StatusAccepted,
			body: `"busy":["wb"]`,
		},
		{
			name: "all busy returns 409",
			trigger: func([]string) (SyncDispatch, error) {
				return SyncDispatch{Busy: []string{"wb"}}, nil
			},
			want: http.StatusConflict,
			body: "Синхронизация уже выполняется",
		},
		{
			name: "invalid marketplace returns 400",
			trigger: func([]string) (SyncDispatch, error) {
				return SyncDispatch{}, errors.New("unknown marketplace")
			},
			want: http.StatusBadRequest,
			body: "unknown marketplace",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, cookie, csrf := csrfSetup(t)
			s.cfg.TriggerSync = tt.trigger
			rec := doSync(t, s, cookie, csrf)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d, body=%s", rec.Code, tt.want, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.body) {
				t.Fatalf("body %q does not contain %q", rec.Body.String(), tt.body)
			}
		})
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

// wbJWT builds an unsigned JWT-shaped fixture; validation is metadata-only
// (no signature verification), so tests construct these directly.
func wbJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	encode := func(v any) string {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	return encode(map[string]string{"alg": "none"}) + "." + encode(claims) + ".sig"
}

func TestSaveWBCredentialsRejectsBaseTokenAndPreservesStoredValue(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)
	csrf := getCSRFToken(t, s, cookie)

	personalToken := wbJWT(t, map[string]any{"acc": 3, "exp": time.Now().Add(time.Hour).Unix()})
	enabled := true
	if _, err := s.store.SaveMarketplaceCredential(t.Context(), store.MarketplaceCredentialPatch{
		Marketplace: "wb",
		Enabled:     &enabled,
		Values:      map[string]string{"token": personalToken},
	}); err != nil {
		t.Fatalf("seed personal token: %v", err)
	}

	baseToken := wbJWT(t, map[string]any{"acc": 1, "exp": time.Now().Add(time.Hour).Unix()})
	body := `{"enabled":true,"values":{"token":"` + baseToken + `"}}`
	req := httptest.NewRequest(http.MethodPut, "/admin/api/marketplaces/wb/credentials", strings.NewReader(body))
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("save base token status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "персональный WB API-токен") {
		t.Fatalf("expected Russian personal-token message, got: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), personalToken) || strings.Contains(rec.Body.String(), baseToken) {
		t.Fatalf("response leaked a token: %s", rec.Body.String())
	}

	stored, err := s.store.GetMarketplaceCredential(t.Context(), "wb")
	if err != nil {
		t.Fatalf("get stored credential: %v", err)
	}
	if stored.PayloadMap()["token"] != personalToken {
		t.Fatalf("stored token changed after rejected save: %+v", stored.PayloadMap())
	}
}

func TestMarketplaceStatusWBBaseTokenUnconfiguredWithWarning(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)

	baseToken := wbJWT(t, map[string]any{"acc": 1, "exp": time.Now().Add(time.Hour).Unix()})
	enabled := true
	if _, err := s.store.SaveMarketplaceCredential(t.Context(), store.MarketplaceCredentialPatch{
		Marketplace: "wb",
		Enabled:     &enabled,
		Values:      map[string]string{"token": baseToken},
	}); err != nil {
		t.Fatalf("seed base token: %v", err)
	}
	if _, err := s.store.SaveMarketplaceCredential(t.Context(), store.MarketplaceCredentialPatch{
		Marketplace: "ym",
		Enabled:     &enabled,
		Values:      map[string]string{"api_key": "ym-key", "business_id": "biz-1"},
	}); err != nil {
		t.Fatalf("seed ym credential: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/api/marketplaces", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("marketplaces status = %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Marketplaces []MarketplaceStatus `json:"marketplaces"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}

	var wb, ym *MarketplaceStatus
	for i := range payload.Marketplaces {
		switch payload.Marketplaces[i].ID {
		case "wb":
			wb = &payload.Marketplaces[i]
		case "ym":
			ym = &payload.Marketplaces[i]
		}
	}
	if wb == nil {
		t.Fatalf("no wb status: %+v", payload.Marketplaces)
	}
	if wb.Configured {
		t.Fatalf("wb with base token should be unconfigured: %+v", wb)
	}
	if !wb.Fields["token"] {
		t.Fatalf("wb token presence should stay masked true: %+v", wb)
	}
	if wb.Warning == "" {
		t.Fatalf("wb with base token should carry a warning")
	}
	if strings.Contains(rec.Body.String(), baseToken) {
		t.Fatalf("response leaked the token")
	}

	if ym == nil {
		t.Fatalf("no ym status: %+v", payload.Marketplaces)
	}
	if !ym.Configured || ym.Warning != "" {
		t.Fatalf("ym status should remain unaffected: %+v", ym)
	}
}
