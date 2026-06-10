package store

import (
	"context"
	"testing"
	"time"
)

func seedReview(t *testing.T, st *Store, mp, extID string, rating int) Review {
	t.Helper()
	r := Review{
		TenantID:          DefaultTenantID,
		Marketplace:       mp,
		ExternalReviewID:  extID,
		ExternalProductID: "p1",
		Rating:            &rating,
		Text:              "good",
		CreatedAtMP:       time.Now(),
		Visibility:        "visible",
		FetchedAt:         time.Now(),
	}
	if err := st.db.WithContext(context.Background()).Create(&r).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	return r
}

func TestSetVisibilityAndPinned(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	r := seedReview(t, st, "wb", "w1", 5)

	if err := st.SetReviewVisibility(ctx, r.ID, "hidden"); err != nil {
		t.Fatalf("hide: %v", err)
	}
	if err := st.SetReviewPinned(ctx, r.ID, true); err != nil {
		t.Fatalf("pin: %v", err)
	}

	hidden, err := st.ListReviews(ctx, ReviewListFilter{Visibility: "hidden"})
	if err != nil {
		t.Fatalf("list hidden: %v", err)
	}
	if len(hidden) != 1 || !hidden[0].Pinned {
		t.Fatalf("expected 1 hidden pinned review, got %+v", hidden)
	}
}

func TestListReviewsWithCount(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedReview(t, st, "wb", "w1", 5)
	seedReview(t, st, "ym", "y1", 3)

	items, total, err := st.ListReviewsWithCount(ctx, ReviewListFilter{Limit: 1})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected total 2, got %d", total)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item (limit), got %d", len(items))
	}
}

func TestDashboardStats(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedReview(t, st, "wb", "w1", 5)
	seedReview(t, st, "wb", "w2", 3)
	seedReview(t, st, "ym", "y1", 4)

	stats, err := st.DashboardStats(ctx)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.TotalReviews != 3 {
		t.Fatalf("expected 3 total, got %d", stats.TotalReviews)
	}
	if stats.ByMarketplace["wb"] != 2 || stats.ByMarketplace["ym"] != 1 {
		t.Fatalf("unexpected per-marketplace counts: %+v", stats.ByMarketplace)
	}
	if stats.AverageRating < 3.9 || stats.AverageRating > 4.1 {
		t.Fatalf("expected avg ~4.0, got %v", stats.AverageRating)
	}
}

func TestShowcaseRuleAndReviews(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	oldNow := timeNowUTC
	timeNowUTC = func() time.Time { return now }
	defer func() { timeNowUTC = oldNow }()

	defaultRule, err := st.GetShowcaseRule(ctx)
	if err != nil {
		t.Fatalf("default rule: %v", err)
	}
	if defaultRule.MinRating != 4 || defaultRule.Limit != 12 || defaultRule.SortBy != "recent" {
		t.Fatalf("unexpected default rule: %+v", defaultRule)
	}

	if err := st.SaveShowcaseRule(ctx, ShowcaseRule{
		MinRating:    4,
		RequirePhoto: true,
		MinTextLen:   4,
		MaxAgeDays:   30,
		SortBy:       "rating",
		Limit:        2,
	}); err != nil {
		t.Fatalf("save rule: %v", err)
	}
	rule, err := st.GetShowcaseRule(ctx)
	if err != nil {
		t.Fatalf("get saved rule: %v", err)
	}

	pinned := seedReview(t, st, "wb", "pin", 4)
	if err := st.SetReviewPinned(ctx, pinned.ID, true); err != nil {
		t.Fatalf("pin: %v", err)
	}
	top := seedReview(t, st, "wb", "top", 5)
	low := seedReview(t, st, "wb", "low", 3)
	if err := st.SetReviewVisibility(ctx, low.ID, "hidden"); err != nil {
		t.Fatalf("hide low: %v", err)
	}
	for _, reviewID := range []uint{pinned.ID, top.ID, low.ID} {
		if err := st.db.WithContext(ctx).Create(&ReviewMedia{
			ReviewID: reviewID,
			Kind:     "photo",
			URL:      "https://cdn.example/photo.jpg",
		}).Error; err != nil {
			t.Fatalf("seed media: %v", err)
		}
	}

	items, err := st.ShowcaseReviews(ctx, rule)
	if err != nil {
		t.Fatalf("showcase: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 showcase reviews, got %d", len(items))
	}
	if items[0].ExternalReviewID != "pin" || items[1].ExternalReviewID != "top" {
		t.Fatalf("unexpected showcase order: %+v", items)
	}
}
