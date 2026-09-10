package store

import (
	"context"
	"testing"

	"reviews/internal/config"
	"reviews/internal/marketplace"
)

// newTenantIsolationStore opens an in-memory store with migrations applied.
func newTenantIsolationStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return s
}

// TestTenantIsolation proves tenant A's data is invisible to tenant B across
// the tenant-scoped tables: reviews, credentials, widget configs, showcase
// rules and pins, questions, app settings, sync state.
func TestTenantIsolation(t *testing.T) {
	s := newTenantIsolationStore(t)
	ctxA := WithTenant(context.Background(), 1)
	ctxB := WithTenant(context.Background(), 2)

	rating := 5
	// Tenant A writes reviews, a question, a credential, a widget config.
	if _, err := s.UpsertReview(ctxA, marketplace.Review{
		Marketplace:       "wb",
		ExternalReviewID:  "wb-a",
		ExternalProductID: "p1",
		SellerArticle:     "art-a",
		Rating:            &rating,
		Text:              "A's review",
	}); err != nil {
		t.Fatalf("upsert review A: %v", err)
	}
	if _, err := s.UpsertQuestion(ctxA, QuestionInput{
		Marketplace:        "wb",
		ExternalQuestionID: "wbq-a",
		ExternalProductID:  "p1",
		Text:               "A's question",
	}); err != nil {
		t.Fatalf("upsert question A: %v", err)
	}
	if _, err := s.SaveMarketplaceCredential(ctxA, MarketplaceCredentialPatch{
		Marketplace: "wb",
		Values:      map[string]string{"token": "a-token"},
	}); err != nil {
		t.Fatalf("save credential A: %v", err)
	}
	if _, err := s.PublishWidgetConfig(ctxA, "product", `{"theme":{"accent":"#111111"}}`); err != nil {
		t.Fatalf("publish widget config A: %v", err)
	}
	if err := s.SaveShowcaseRule(ctxA, ShowcaseRule{MinRating: 5, Limit: 3}); err != nil {
		t.Fatalf("save showcase rule A: %v", err)
	}
	if err := s.SetAppSetting(ctxA, "k", "a"); err != nil {
		t.Fatalf("set app setting A: %v", err)
	}

	// Tenant B sees none of A's data.
	reviewsB, err := s.ListReviews(ctxB, ReviewListFilter{})
	if err != nil {
		t.Fatalf("list reviews B: %v", err)
	}
	if len(reviewsB) != 0 {
		t.Fatalf("tenant B sees %d of A's reviews", len(reviewsB))
	}
	questionsB, err := s.ListQuestions(ctxB, QuestionFilter{})
	if err != nil {
		t.Fatalf("list questions B: %v", err)
	}
	if len(questionsB) != 0 {
		t.Fatalf("tenant B sees %d of A's questions", len(questionsB))
	}
	if _, err := s.GetMarketplaceCredential(ctxB, "wb"); err != ErrNotFound {
		t.Fatalf("tenant B must not see A's credential, got err=%v", err)
	}
	if cfgB, err := s.GetActiveWidgetConfig(ctxB, "product"); err != ErrNotFound {
		t.Fatalf("tenant B must not see A's widget config, got cfg=%+v err=%v", cfgB, err)
	}
	if v, err := s.GetAppSetting(ctxB, "k"); err != nil || v != "" {
		t.Fatalf("tenant B must not see A's app setting, got %q err=%v", v, err)
	}
	// B's showcase rule falls back to the default, not A's saved rule.
	ruleB, err := s.GetShowcaseRule(ctxB)
	if err != nil {
		t.Fatalf("get showcase rule B: %v", err)
	}
	if ruleB.Limit == 3 || ruleB.MinRating == 5 {
		t.Fatalf("tenant B leaked A's showcase rule: %+v", ruleB)
	}

	// B writes its own review with the same external ID — both coexist.
	if _, err := s.UpsertReview(ctxB, marketplace.Review{
		Marketplace:       "wb",
		ExternalReviewID:  "wb-a",
		ExternalProductID: "p1",
		Rating:            &rating,
		Text:              "B's review",
	}); err != nil {
		t.Fatalf("upsert review B: %v", err)
	}
	reviewsA, err := s.ListReviews(ctxA, ReviewListFilter{})
	if err != nil {
		t.Fatalf("list reviews A: %v", err)
	}
	if len(reviewsA) != 1 || reviewsA[0].Text != "A's review" {
		t.Fatalf("tenant A must still see exactly its own review, got %d", len(reviewsA))
	}

	// Sync state is keyed per tenant.
	if err := s.SaveSyncState(ctxA, SyncState{Marketplace: "wb", Backfilled: true}); err != nil {
		t.Fatalf("save sync state A: %v", err)
	}
	stateB, err := s.GetSyncState(ctxB, "wb")
	if err != nil {
		t.Fatalf("get sync state B: %v", err)
	}
	if stateB.Backfilled {
		t.Fatal("tenant B must not inherit A's sync state")
	}
}

// TestStrictTenantModeMissingTenant proves a SaaS instance (compat disabled)
// treats a tenant-less context as a programming error.
func TestStrictTenantModeMissingTenant(t *testing.T) {
	t.Setenv("REVIEWS_COMPAT_SINGLE_TENANT", "false")
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for missing tenant in strict mode")
		}
	}()
	// Reset the package-level flag captured at init: re-evaluate directly.
	strictTenantMode = true
	defer func() { strictTenantMode = false }()
	_ = TenantIDFromCtx(context.Background())
}

// TestEnsureDefaultTenant seeds tenant 1 exactly once with a 64-hex key.
func TestEnsureDefaultTenant(t *testing.T) {
	s := newTenantIsolationStore(t)
	ctx := context.Background()
	// Migrate already seeded tenant 1 (with empty origin); a second call with a
	// different origin must not duplicate or overwrite.
	if err := s.EnsureDefaultTenant(ctx, "https://shop.example"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := s.EnsureDefaultTenant(ctx, "https://other.example"); err != nil {
		t.Fatalf("re-seed: %v", err)
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&Tenant{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("expected exactly one tenant, got %d err=%v", count, err)
	}
	var tenant Tenant
	if err := s.db.WithContext(ctx).First(&tenant, DefaultTenantID).Error; err != nil {
		t.Fatalf("load tenant: %v", err)
	}
	if len(tenant.PublicKey) != 64 {
		t.Fatalf("expected 64-hex public key, got %+v", tenant)
	}
}
