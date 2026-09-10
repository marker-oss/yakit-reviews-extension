package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"reviews/internal/marketplace"
	"reviews/internal/store"
)

// TestPublicRoutesScopedByPublicKey proves the tenantScope middleware resolves
// tenants by public_key: tenant B's key returns only B's reviews, no key falls
// back to the default tenant (compat), and an unknown key is rejected.
func TestPublicRoutesScopedByPublicKey(t *testing.T) {
	s := newAuthTestServer(t)
	ctx := context.Background()

	tenantB, err := s.store.CreateTenant(ctx, "second", "https://b.example")
	if err != nil {
		t.Fatalf("create tenant B: %v", err)
	}

	rating := 5
	seed := func(ctx context.Context, externalID, text string) {
		t.Helper()
		if _, err := s.store.UpsertReview(ctx, marketplace.Review{
			Marketplace:       "wb",
			ExternalReviewID:  externalID,
			ExternalProductID: "p1",
			Rating:            &rating,
			Text:              text,
			CreatedAtMP:       time.Now().UTC(),
		}); err != nil {
			t.Fatalf("upsert %s: %v", externalID, err)
		}
	}
	seed(store.WithTenant(ctx, store.DefaultTenantID), "review-a", "tenant A review")
	seed(store.WithTenant(ctx, tenantB.ID), "review-b", "tenant B review")

	get := func(url string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		s.handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
		return rec
	}

	// Tenant B's key sees only B's reviews.
	rec := get("/api/reviews?public_key=" + tenantB.PublicKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("tenant B status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Reviews []struct {
			Text string `json:"text"`
		} `json:"reviews"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Reviews) != 1 || resp.Reviews[0].Text != "tenant B review" {
		t.Fatalf("tenant B leaked data: %+v", resp.Reviews)
	}

	// No key in compat mode serves the default tenant.
	rec = get("/api/reviews")
	if rec.Code != http.StatusOK {
		t.Fatalf("compat status = %d", rec.Code)
	}
	resp.Reviews = nil
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode compat: %v", err)
	}
	if len(resp.Reviews) != 1 || resp.Reviews[0].Text != "tenant A review" {
		t.Fatalf("compat served wrong tenant: %+v", resp.Reviews)
	}

	// Unknown key is rejected.
	if rec := get("/api/reviews?public_key=deadbeef"); rec.Code != http.StatusForbidden {
		t.Fatalf("unknown key status = %d, want 403", rec.Code)
	}
}

// TestStrictModeRequiresPublicKey proves a SaaS instance (strict mode) rejects
// public data requests without a public_key while health probes, static
// assets, and admin routes stay tenant-less or session-based.
func TestStrictModeRequiresPublicKey(t *testing.T) {
	s := newAuthTestServer(t)
	restore := store.SetStrictTenantModeForTest(true)
	defer restore()

	rec := httptest.NewRecorder()
	s.handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/reviews", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("strict public status = %d, want 403", rec.Code)
	}

	// Health probes answer for load balancers without any tenant.
	rec = httptest.NewRecorder()
	s.handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("strict healthz status = %d, want 200", rec.Code)
	}

	// Admin setup-status is exempt: login/setup must work before any key exists.
	rec = httptest.NewRecorder()
	s.handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/api/setup-status", nil))
	if rec.Code == http.StatusForbidden {
		t.Fatal("admin route must not require public_key")
	}
}

// TestCrossTenantOriginRejected proves the CORS guard: a browser Origin of
// tenant A cannot use tenant B's public key, while B's own origin (and its
// www sibling) passes.
func TestCrossTenantOriginRejected(t *testing.T) {
	s := newAuthTestServer(t)
	ctx := context.Background()
	tenantB, err := s.store.CreateTenant(ctx, "second", "https://b.example")
	if err != nil {
		t.Fatalf("create tenant B: %v", err)
	}

	get := func(origin string) int {
		req := httptest.NewRequest(http.MethodGet, "/api/reviews?public_key="+tenantB.PublicKey, nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		s.handler().ServeHTTP(rec, req)
		return rec.Code
	}

	if code := get("https://a.example"); code != http.StatusForbidden {
		t.Fatalf("foreign origin status = %d, want 403", code)
	}
	if code := get("https://b.example"); code != http.StatusOK {
		t.Fatalf("own origin status = %d, want 200", code)
	}
	if code := get("https://www.b.example"); code != http.StatusOK {
		t.Fatalf("www sibling origin status = %d, want 200", code)
	}
	// Non-browser clients (curl, no Origin header) keep working.
	if code := get(""); code != http.StatusOK {
		t.Fatalf("no origin status = %d, want 200", code)
	}
}
