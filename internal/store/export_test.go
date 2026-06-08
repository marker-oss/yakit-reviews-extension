package store

import (
	"context"
	"testing"
	"time"

	"reviews/internal/marketplace"
)

func TestListAllReviewsReturnsEveryRowWithMedia(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	oldTime := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC)
	rating := 5

	_, err := s.UpsertReview(ctx, marketplace.Review{
		Marketplace:       "wb",
		ExternalReviewID:  "old-review",
		ExternalProductID: "100",
		SellerArticle:     "old-article",
		Rating:            &rating,
		CreatedAtMP:       oldTime,
	})
	if err != nil {
		t.Fatalf("upsert old review: %v", err)
	}

	_, err = s.UpsertReview(ctx, marketplace.Review{
		Marketplace:       "wb",
		ExternalReviewID:  "new-review",
		ExternalProductID: "101",
		SellerArticle:     "new-article",
		Rating:            &rating,
		CreatedAtMP:       newTime,
		Media: []marketplace.Media{
			{Kind: "photo", URL: "https://cdn.example/two.jpg", Position: 2},
			{Kind: "photo", URL: "https://cdn.example/one.jpg", Position: 1},
		},
	})
	if err != nil {
		t.Fatalf("upsert new review: %v", err)
	}

	reviews, err := s.ListAllReviews(ctx)
	if err != nil {
		t.Fatalf("list all reviews: %v", err)
	}
	if len(reviews) != 2 {
		t.Fatalf("reviews len = %d", len(reviews))
	}
	if reviews[0].ExternalReviewID != "new-review" {
		t.Fatalf("first review = %q", reviews[0].ExternalReviewID)
	}
	if len(reviews[0].Media) != 2 {
		t.Fatalf("media len = %d", len(reviews[0].Media))
	}
	if reviews[0].Media[0].URL != "https://cdn.example/one.jpg" {
		t.Fatalf("first media url = %q", reviews[0].Media[0].URL)
	}
}
