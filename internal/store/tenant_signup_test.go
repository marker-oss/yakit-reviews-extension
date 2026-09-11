package store

import (
	"context"
	"testing"
	"time"
)

// TestPauseExpiredTrials proves the trial job's store method flips only
// expired trials and is idempotent. Uses the raw db handle (store-package
// tests may) to backdate one tenant's trial window.
func TestPauseExpiredTrials(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	expired, err := s.CreateTenant(ctx, "expired", "https://e.example")
	if err != nil {
		t.Fatalf("create expired: %v", err)
	}
	active, err := s.CreateTenant(ctx, "active", "https://a.example")
	if err != nil {
		t.Fatalf("create active: %v", err)
	}
	if err := s.db.Model(&Tenant{}).Where("id = ?", expired.ID).
		Update("trial_ends_at", time.Now().UTC().Add(-time.Hour)).Error; err != nil {
		t.Fatalf("backdate trial: %v", err)
	}

	n, err := s.PauseExpiredTrials(ctx)
	if err != nil {
		t.Fatalf("pause expired: %v", err)
	}
	if n != 1 {
		t.Fatalf("paused %d tenants, want 1", n)
	}
	// Idempotent: the second run matches nothing.
	n2, err := s.PauseExpiredTrials(ctx)
	if err != nil || n2 != 0 {
		t.Fatalf("second run paused %d, err=%v; want 0", n2, err)
	}

	got, err := s.TenantByPublicKey(ctx, expired.PublicKey)
	if err != nil || got.Status != "paused" {
		t.Fatalf("expired tenant status = %q err=%v, want paused", got.Status, err)
	}
	gotActive, err := s.TenantByPublicKey(ctx, active.PublicKey)
	if err != nil || gotActive.Status != "trial" {
		t.Fatalf("active tenant status = %q err=%v, want trial", gotActive.Status, err)
	}
}

// TestCreateTenantWithAdmin proves the atomic signup: tenant + admin in one
// transaction, admin scoped to the new tenant, globally unique login.
func TestCreateTenantWithAdmin(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	result, err := s.CreateTenantWithAdmin(ctx, "seller1", "hash-1", "https://shop1.example")
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	if result.AdminID == 0 || result.Tenant.ID == 0 {
		t.Fatalf("result = %+v, want nonzero ids", result)
	}
	if result.Tenant.Status != "trial" {
		t.Fatalf("status = %q, want trial", result.Tenant.Status)
	}
	if result.Tenant.TrialEndsAt.Before(time.Now().UTC()) {
		t.Fatal("trial window must start in the future")
	}

	// The admin belongs to the new tenant, not tenant 1.
	admin, err := s.GetAdminUserByLogin(ctx, "seller1")
	if err != nil {
		t.Fatalf("lookup admin: %v", err)
	}
	if admin.TenantID != result.Tenant.ID {
		t.Fatalf("admin tenant = %d, want %d", admin.TenantID, result.Tenant.ID)
	}

	// Duplicate login fails the transaction; no tenant row leaks.
	if _, err := s.CreateTenantWithAdmin(ctx, "seller1", "hash-2", "https://shop2.example"); err == nil {
		t.Fatal("duplicate login must fail")
	}
	var count int64
	s.db.Model(&Tenant{}).Where("slug = ?", "seller1").Count(&count)
	if count != 1 {
		t.Fatalf("tenant rows with slug seller1 = %d, want 1 (transaction must roll back)", count)
	}
}
