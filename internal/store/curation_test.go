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
