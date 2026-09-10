package store

import (
	"context"
	"testing"
	"time"

	"reviews/internal/marketplace"
)

func seedRemapReview(t *testing.T, s *Store, mp, externalID, productID, article string) {
	t.Helper()
	rating := 5
	if _, err := s.UpsertReview(context.Background(), marketplace.Review{
		Marketplace:       mp,
		ExternalReviewID:  externalID,
		ExternalProductID: productID,
		SellerArticle:     article,
		Rating:            &rating,
		Text:              "t",
		CreatedAtMP:       time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("seed review: %v", err)
	}
}

func TestRemapSellerArticlesUpdatesOnlyMatchingRows(t *testing.T) {
	s := newTestStore(t)
	// Unmapped ozon review: sellerArticle fell back to the marketplace sku.
	seedRemapReview(t, s, "ozon", "oz-1", "181649408", "181649408")
	// Already-correct ozon review must stay untouched.
	seedRemapReview(t, s, "ozon", "oz-2", "222", "85999")
	// Same external id on another marketplace must not be touched.
	seedRemapReview(t, s, "wb", "wb-1", "181649408", "99166")

	updated, err := s.RemapSellerArticles(context.Background(), "ozon", map[string]string{
		"181649408": "85467",
		"222":       "85999", // no-op: already equal
		"333":       "77777", // no matching review
	})
	if err != nil {
		t.Fatalf("RemapSellerArticles: %v", err)
	}
	if updated != 1 {
		t.Fatalf("updated = %d, want 1", updated)
	}

	var articles []string
	if err := s.db.Model(&Review{}).Order("external_review_id").Pluck("seller_article", &articles).Error; err != nil {
		t.Fatalf("pluck: %v", err)
	}
	// oz-1 remapped, oz-2 kept, wb-1 untouched.
	want := []string{"85467", "85999", "99166"}
	for i := range want {
		if articles[i] != want[i] {
			t.Fatalf("articles = %v, want %v", articles, want)
		}
	}
}

func TestRemapSellerArticlesEmptyMapIsNoop(t *testing.T) {
	s := newTestStore(t)
	seedRemapReview(t, s, "ozon", "oz-1", "111", "111")
	updated, err := s.RemapSellerArticles(context.Background(), "ozon", nil)
	if err != nil {
		t.Fatalf("RemapSellerArticles: %v", err)
	}
	if updated != 0 {
		t.Fatalf("updated = %d, want 0", updated)
	}
}
