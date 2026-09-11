package main

import (
	"context"
	"testing"

	"reviews/internal/config"
	"reviews/internal/marketplace"
	"reviews/internal/marketplace/apihttp"
	"reviews/internal/store"
)

// TestDispatchSyncIsolatedPerTenant proves the SaaS sync loop serves every
// tenant: two tenants dispatch the same marketplace independently, their
// coordinator slots don't collide (tenant B starts while tenant A is busy),
// and each tenant's background work stamps its own tenant into sync_runs.
func TestDispatchSyncIsolatedPerTenant(t *testing.T) {
	base := config.Config{Marketplaces: config.MarketplaceConfig{
		WB: config.WBConfig{Enabled: true, Token: personalWBToken(t)},
	}}
	o, db := newOps(t, base)
	ctxA := store.WithTenant(context.Background(), store.DefaultTenantID)
	tenantB, err := db.CreateTenant(ctxA, "smoke-b", "https://b.example")
	if err != nil {
		t.Fatalf("create tenant B: %v", err)
	}
	ctxB := store.WithTenant(context.Background(), tenantB.ID)
	// Both tenants need a WB credential row of their own: effective config
	// resolves credentials per tenant from the database.
	saveWBCredential(t, db, personalWBToken(t), true)
	if _, err := db.SaveMarketplaceCredential(ctxB, store.MarketplaceCredentialPatch{
		Marketplace: config.MarketplaceWB,
		Values:      map[string]string{"token": personalWBToken(t)},
	}); err != nil {
		t.Fatalf("save wb credential B: %v", err)
	}

	// Each dispatch builds a fresh blocking adapter; both stay in-flight
	// until the test releases them.
	var adapters []*blockingAdapter
	adapterMu := make(chan struct{}, 1)
	o.newAdapter = func(cfg config.Config, id string, _ *apihttp.Executor) (marketplace.Adapter, error) {
		a := newBlockingAdapter(id)
		adapters = append(adapters, a)
		adapterMu <- struct{}{}
		return a, nil
	}
	waitAdapter := func(i int) *blockingAdapter {
		for len(adapters) <= i {
			<-adapterMu
		}
		return adapters[i]
	}

	first, err := o.DispatchSync(ctxA, []string{"wb"}, nil)
	if err != nil || len(first.Started) != 1 {
		t.Fatalf("dispatch A = %+v, err=%v", first, err)
	}
	<-waitAdapter(0).entered

	// Tenant B must NOT be blocked by A's in-flight WB sync.
	second, err := o.DispatchSync(ctxB, []string{"wb"}, nil)
	if err != nil {
		t.Fatalf("dispatch B: %v", err)
	}
	if len(second.Started) != 1 || second.Started[0] != "wb" {
		t.Fatalf("dispatch B = %+v, want Started:[wb] — per-tenant slot must not collide", second)
	}
	<-waitAdapter(1).entered

	// The same tenant's second dispatch must report busy.
	third, err := o.DispatchSync(ctxB, []string{"wb"}, nil)
	if err != nil {
		t.Fatalf("dispatch B2: %v", err)
	}
	if len(third.Busy) != 1 {
		t.Fatalf("dispatch B2 = %+v, want Busy:[wb]", third)
	}

	// Unbind both syncs and verify each tenant's sync_runs were stamped with
	// the right tenant: A sees its own run (and only its own), B sees its own.
	close(waitAdapter(0).unblock)
	close(waitAdapter(1).unblock)

	runsA, err := db.RecentSyncRuns(ctxA, 10)
	if err != nil {
		t.Fatalf("recent runs A: %v", err)
	}
	if len(runsA) == 0 {
		t.Fatal("no sync_runs for tenant A")
	}
	for _, r := range runsA {
		if r.TenantID != store.DefaultTenantID {
			t.Fatalf("run %+v belongs to tenant %d, want %d", r, r.TenantID, store.DefaultTenantID)
		}
	}

	runsB, err := db.RecentSyncRuns(ctxB, 10)
	if err != nil {
		t.Fatalf("recent runs B: %v", err)
	}
	if len(runsB) == 0 {
		t.Fatal("no sync_runs for tenant B — background work did not run in B's tenant context")
	}
	for _, r := range runsB {
		if r.TenantID != tenantB.ID {
			t.Fatalf("run %+v belongs to tenant %d, want %d", r, r.TenantID, tenantB.ID)
		}
	}
}
