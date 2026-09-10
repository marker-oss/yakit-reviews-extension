package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCORSPreflightAllowedForTenantOrigin proves the cors middleware echoes
// the shop origin of the tenant resolved from public_key (Stage 2.3): a SaaS
// widget on tenant B's shop passes preflight even when the global
// env/AppSetting allowlist knows nothing about B's origin.
func TestCORSPreflightAllowedForTenantOrigin(t *testing.T) {
	s := newAuthTestServer(t)
	// No env origins and no AppSetting origin: only the tenant row has it.
	tenantB, err := s.store.CreateTenant(context.Background(), "cors-b", "https://b.example")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	preflight := func(origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodOptions, "/api/reviews?public_key="+tenantB.PublicKey, nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", "GET")
		rec := httptest.NewRecorder()
		s.handler().ServeHTTP(rec, req)
		return rec
	}

	// B's own origin: preflight passes with the CORS headers a browser needs.
	rec := preflight("https://b.example")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://b.example" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want tenant B origin", got)
	}

	// www sibling of B's origin also passes.
	rec = preflight("https://www.b.example")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://www.b.example" {
		t.Fatalf("www sibling Allow-Origin = %q, want it allowed", got)
	}

	// Any other origin gets no CORS headers (and the guard in tenantScope
	// rejects the request outright with 403).
	rec = preflight("https://evil.example")
	if rec.Code == http.StatusNoContent {
		t.Fatal("foreign-origin preflight must not pass")
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("foreign origin Allow-Origin = %q, want empty", got)
	}
}

// TestCORSTenantOriginSurvivesAdminEdit proves that saving the shop origin in
// the admin panel updates tenants.shop_origin, so key-scoped CORS follows shop
// moves without recreating the tenant.
func TestCORSTenantOriginSurvivesAdminEdit(t *testing.T) {
	s := newAuthTestServer(t)
	ctx := context.Background()

	oldOrigin := "https://old.example"
	if err := s.store.UpdateTenantShopOrigin(ctx, 1, oldOrigin); err != nil {
		t.Fatalf("seed tenant origin: %v", err)
	}
	if err := s.store.UpdateTenantShopOrigin(ctx, 1, "https://new.example"); err != nil {
		t.Fatalf("update tenant origin: %v", err)
	}

	tenant, err := s.store.TenantByID(ctx)
	if err != nil {
		t.Fatalf("load tenant: %v", err)
	}
	if tenant.ShopOrigin != "https://new.example" {
		t.Fatalf("tenant.ShopOrigin = %q, want updated origin", tenant.ShopOrigin)
	}
}
