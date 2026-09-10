package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func marketplacesResponse(t *testing.T, s *Server) map[string]MarketplaceStatus {
	t.Helper()
	cookie := loginTestAdmin(t, s)
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
	byID := map[string]MarketplaceStatus{}
	for _, m := range payload.Marketplaces {
		byID[m.ID] = m
	}
	return byID
}

func TestMarketplacesWarnAboutMissingOzonProductsRole(t *testing.T) {
	s := newAuthTestServer(t)
	s.cfg.Marketplaces = []MarketplaceStatus{{ID: "ozon", Enabled: true, Configured: true}}
	s.cfg.OzonProductsProbe = func(ctx context.Context) error {
		return errors.New("Ozon product list: status 403: Api-Key is missing a required role for a method")
	}

	ozon := marketplacesResponse(t, s)["ozon"]
	if ozon.Warning == "" {
		t.Fatal("expected a warning when the key lacks the products role")
	}
}

func TestMarketplacesNoWarningWhenProductsAccessible(t *testing.T) {
	s := newAuthTestServer(t)
	s.cfg.Marketplaces = []MarketplaceStatus{{ID: "ozon", Enabled: true, Configured: true}}
	s.cfg.OzonProductsProbe = func(ctx context.Context) error { return nil }

	ozon := marketplacesResponse(t, s)["ozon"]
	if ozon.Warning != "" {
		t.Fatalf("unexpected warning: %q", ozon.Warning)
	}
}
