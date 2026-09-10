package store

import (
	"context"
	"testing"
	"time"

	"reviews/internal/config"
	"reviews/internal/marketplace"
)

func TestUpsertReviewIsIdempotentAndSnapshotsMedia(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	createdAt := time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC)
	rating := 4

	first, err := s.UpsertReview(ctx, marketplace.Review{
		Marketplace:       "wb",
		ExternalReviewID:  "review-1",
		ExternalProductID: "100500",
		Rating:            &rating,
		AuthorName:        "Alice",
		Text:              "Good",
		Pros:              "Fabric",
		Cons:              "Slow delivery",
		CreatedAtMP:       createdAt,
		Media: []marketplace.Media{
			{Kind: "photo", URL: "https://cdn.example/photo-1.jpg", Position: 1},
			{Kind: "photo", URL: "https://cdn.example/photo-2.jpg", Position: 2},
		},
		Raw: []byte(`{"id":"review-1"}`),
	})
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if !first.Created {
		t.Fatalf("first upsert should create review")
	}

	assertCount(t, s, &Review{}, 1)
	assertCount(t, s, &ReviewMedia{}, 2)

	updatedAt := createdAt.Add(2 * time.Hour)
	nextRating := 5
	second, err := s.UpsertReview(ctx, marketplace.Review{
		Marketplace:       "wb",
		ExternalReviewID:  "review-1",
		ExternalProductID: "100500",
		Rating:            &nextRating,
		AuthorName:        "Alice",
		Text:              "Great after update",
		Pros:              "Fabric",
		Cons:              "",
		CreatedAtMP:       createdAt,
		UpdatedAtMP:       &updatedAt,
		Answer:            &marketplace.Answer{Text: "Thanks", State: "published"},
		Media: []marketplace.Media{
			{Kind: "photo", URL: "https://cdn.example/photo-1.jpg", PreviewURL: "https://cdn.example/preview-1.jpg", Position: 3},
		},
		Raw: []byte(`{"id":"review-1","updated":true}`),
	})
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if second.Created {
		t.Fatalf("second upsert should update review")
	}

	assertCount(t, s, &Review{}, 1)
	assertCount(t, s, &ReviewMedia{}, 1)

	var review Review
	if err := s.db.First(&review, "marketplace = ? AND external_review_id = ?", "wb", "review-1").Error; err != nil {
		t.Fatalf("load review: %v", err)
	}
	if review.Text != "Great after update" {
		t.Fatalf("review text = %q", review.Text)
	}
	if review.Rating == nil || *review.Rating != 5 {
		t.Fatalf("rating = %v", review.Rating)
	}
	if review.MPAnswerText == nil || *review.MPAnswerText != "Thanks" {
		t.Fatalf("answer text = %v", review.MPAnswerText)
	}

	var media ReviewMedia
	if err := s.db.First(&media, "review_id = ?", review.ID).Error; err != nil {
		t.Fatalf("load media: %v", err)
	}
	if media.URL != "https://cdn.example/photo-1.jpg" || media.Position != 3 {
		t.Fatalf("media snapshot mismatch: %+v", media)
	}
	if media.PreviewURL == nil || *media.PreviewURL != "https://cdn.example/preview-1.jpg" {
		t.Fatalf("preview url = %v", media.PreviewURL)
	}
}

func TestUpsertReviewPersistsNormalizedFields(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	createdAt := time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(30 * time.Minute)
	rating := 5

	_, err := s.UpsertReview(ctx, marketplace.Review{
		Marketplace:       "ym",
		ExternalReviewID:  "feedback-123",
		ExternalProductID: "offer-777",
		SellerArticle:     "seller-777",
		Rating:            &rating,
		AuthorName:        "Maria",
		Text:              "Looks good",
		Pros:              "Soft fabric",
		Cons:              "Runs small",
		CreatedAtMP:       createdAt,
		UpdatedAtMP:       &updatedAt,
		Answer:            &marketplace.Answer{Text: "Thank you", State: "published"},
		Raw:               []byte(`{"feedbackId":"feedback-123"}`),
	})
	if err != nil {
		t.Fatalf("upsert review: %v", err)
	}

	var saved Review
	if err := s.db.First(&saved, "marketplace = ? AND external_review_id = ?", "ym", "feedback-123").Error; err != nil {
		t.Fatalf("load review: %v", err)
	}

	if saved.Marketplace != "ym" {
		t.Fatalf("marketplace = %q", saved.Marketplace)
	}
	if saved.ExternalReviewID != "feedback-123" {
		t.Fatalf("external review id = %q", saved.ExternalReviewID)
	}
	if saved.ExternalProductID != "offer-777" {
		t.Fatalf("external product id = %q", saved.ExternalProductID)
	}
	if saved.SellerArticle != "seller-777" {
		t.Fatalf("seller article = %q", saved.SellerArticle)
	}
	if saved.ProductID != nil {
		t.Fatalf("product id should be nil for orphan, got %v", *saved.ProductID)
	}
	if saved.Rating == nil || *saved.Rating != 5 {
		t.Fatalf("rating = %v", saved.Rating)
	}
	if saved.AuthorName != "Maria" || saved.Text != "Looks good" || saved.Pros != "Soft fabric" || saved.Cons != "Runs small" {
		t.Fatalf("text fields mismatch: %+v", saved)
	}
	if !saved.CreatedAtMP.Equal(createdAt) {
		t.Fatalf("created at mp = %s", saved.CreatedAtMP)
	}
	if saved.UpdatedAtMP == nil || !saved.UpdatedAtMP.Equal(updatedAt) {
		t.Fatalf("updated at mp = %v", saved.UpdatedAtMP)
	}
	if saved.MPAnswerText == nil || *saved.MPAnswerText != "Thank you" {
		t.Fatalf("answer text = %v", saved.MPAnswerText)
	}
	if saved.MPAnswerState == nil || *saved.MPAnswerState != "published" {
		t.Fatalf("answer state = %v", saved.MPAnswerState)
	}
	if saved.Status != "imported" {
		t.Fatalf("status = %q", saved.Status)
	}
	// Raw is intentionally dropped at ingestion (personal-data minimization).
	if saved.Raw != "" {
		t.Fatalf("raw should be empty, got %q", saved.Raw)
	}
	if saved.FetchedAt.IsZero() {
		t.Fatalf("fetched_at should be set")
	}
}

func TestUpsertReviewResolvesProductLinkWhenAvailable(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	createdAt := time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC)

	_, err := s.UpsertReview(ctx, marketplace.Review{
		Marketplace:       "ym",
		ExternalReviewID:  "review-2",
		ExternalProductID: "sku-1",
		CreatedAtMP:       createdAt,
	})
	if err != nil {
		t.Fatalf("upsert orphan: %v", err)
	}

	var orphan Review
	if err := s.db.First(&orphan, "marketplace = ? AND external_review_id = ?", "ym", "review-2").Error; err != nil {
		t.Fatalf("load orphan: %v", err)
	}
	if orphan.ProductID != nil {
		t.Fatalf("product id should be nil for orphan, got %v", *orphan.ProductID)
	}

	product := Product{}
	if err := s.db.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}
	link := ProductMarketplaceLink{
		ProductID:         product.ID,
		Marketplace:       "ym",
		ExternalProductID: "sku-1",
	}
	if err := s.db.Create(&link).Error; err != nil {
		t.Fatalf("create link: %v", err)
	}

	_, err = s.UpsertReview(ctx, marketplace.Review{
		Marketplace:       "ym",
		ExternalReviewID:  "review-2",
		ExternalProductID: "sku-1",
		CreatedAtMP:       createdAt,
	})
	if err != nil {
		t.Fatalf("upsert linked: %v", err)
	}

	var linked Review
	if err := s.db.First(&linked, "marketplace = ? AND external_review_id = ?", "ym", "review-2").Error; err != nil {
		t.Fatalf("load linked: %v", err)
	}
	if linked.ProductID == nil || *linked.ProductID != product.ID {
		t.Fatalf("product id = %v, want %d", linked.ProductID, product.ID)
	}
}

func TestListReviewsReturnsMediaNewestFirst(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	oldTime := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC)
	rating := 5

	_, err := s.UpsertReview(ctx, marketplace.Review{
		Marketplace:       "wb",
		ExternalReviewID:  "old-review",
		ExternalProductID: "100",
		Rating:            &rating,
		CreatedAtMP:       oldTime,
	})
	if err != nil {
		t.Fatalf("upsert old review: %v", err)
	}

	_, err = s.UpsertReview(ctx, marketplace.Review{
		Marketplace:       "wb",
		ExternalReviewID:  "new-review",
		ExternalProductID: "100",
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

	reviews, err := s.ListReviews(ctx, ReviewListFilter{Marketplace: "wb", Limit: 10})
	if err != nil {
		t.Fatalf("list reviews: %v", err)
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

func TestSyncStateUpsert(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	now := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)

	state, err := s.GetSyncState(ctx, "wb")
	if err != nil {
		t.Fatalf("get empty sync state: %v", err)
	}
	if state.Marketplace != "wb" || state.LastSyncedAt != nil || state.Backfilled {
		t.Fatalf("empty sync state mismatch: %+v", state)
	}

	if err := s.SaveSyncState(ctx, SyncState{Marketplace: "wb", LastSyncedAt: &now, Backfilled: true}); err != nil {
		t.Fatalf("save sync state: %v", err)
	}

	state, err = s.GetSyncState(ctx, "wb")
	if err != nil {
		t.Fatalf("get saved sync state: %v", err)
	}
	if state.LastSyncedAt == nil || !state.LastSyncedAt.Equal(now) || !state.Backfilled {
		t.Fatalf("saved sync state mismatch: %+v", state)
	}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	sqlDB, err := s.db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return s
}

func assertCount(t *testing.T, s *Store, model any, want int64) {
	t.Helper()
	var got int64
	if err := s.db.Model(model).Count(&got).Error; err != nil {
		t.Fatalf("count %T: %v", model, err)
	}
	if got != want {
		t.Fatalf("count %T = %d, want %d", model, got, want)
	}
}
