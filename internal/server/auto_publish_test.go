package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"reviews/internal/marketplace"
)

func TestRunAutoPublishOncePublishesWhenDirty(t *testing.T) {
	s := newAuthTestServer(t)
	s.cfg.StaticDir = t.TempDir()

	rating := 5
	if _, err := s.store.UpsertReview(context.Background(), marketplace.Review{
		Marketplace:       "wb",
		ExternalReviewID:  "wb-1",
		ExternalProductID: "100",
		SellerArticle:     "100",
		Rating:            &rating,
		Text:              "отлично",
		CreatedAtMP:       time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := s.store.MarkExportDirty(context.Background()); err != nil {
		t.Fatalf("mark dirty: %v", err)
	}

	published, err := s.runAutoPublishOnce(context.Background())
	if err != nil {
		t.Fatalf("runAutoPublishOnce: %v", err)
	}
	if !published {
		t.Fatal("expected a publish while dirty")
	}
	if _, err := os.Stat(filepath.Join(s.cfg.StaticDir, "reviews-data", "index.json")); err != nil {
		t.Fatalf("export not written: %v", err)
	}
	if _, dirty, err := s.store.ExportDirtySince(context.Background()); err != nil || dirty {
		t.Fatalf("export still dirty after publish (dirty=%v err=%v)", dirty, err)
	}

	published, err = s.runAutoPublishOnce(context.Background())
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if published {
		t.Fatal("clean state must not republish")
	}
}
