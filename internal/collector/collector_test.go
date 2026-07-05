package collector

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"reviews/internal/config"
	"reviews/internal/marketplace"
	"reviews/internal/store"
)

// mappingAdapter fakes an Ozon-like adapter: serves one review and exposes a
// sku→offer_id article map.
type mappingAdapter struct {
	articles map[string]string
}

func (a *mappingAdapter) Marketplace() string { return "ozon" }

func (a *mappingAdapter) FetchReviews(ctx context.Context, since time.Time, cursor string) ([]marketplace.Review, string, error) {
	rating := 5
	return []marketplace.Review{{
		Marketplace:       "ozon",
		ExternalReviewID:  "oz-new",
		ExternalProductID: "555",
		SellerArticle:     "85555", // adapter already resolves fresh reviews
		Rating:            &rating,
		Text:              "fresh",
		CreatedAtMP:       time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
	}}, "", nil
}

func (a *mappingAdapter) ProductArticles(ctx context.Context) (map[string]string, error) {
	return a.articles, nil
}

func newCollectorTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return s
}

// A sync run against an adapter that can enumerate its products must also
// heal reviews stored before the mapping existed (sellerArticle == sku).
func TestRunOnceRemapsExistingSellerArticles(t *testing.T) {
	s := newCollectorTestStore(t)
	rating := 4
	if _, err := s.UpsertReview(context.Background(), marketplace.Review{
		Marketplace:       "ozon",
		ExternalReviewID:  "oz-old",
		ExternalProductID: "181649408",
		SellerArticle:     "181649408",
		Rating:            &rating,
		Text:              "legacy",
		CreatedAtMP:       time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	adapter := &mappingAdapter{articles: map[string]string{"181649408": "85467"}}
	runner := NewRunner(s, config.SyncConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)), []marketplace.Adapter{adapter})
	results := runner.RunOnce(context.Background(), []string{"ozon"})
	if len(results) != 1 || results[0].Error != nil {
		t.Fatalf("results = %+v", results)
	}

	got, _, err := s.ListReviewsWithCount(context.Background(), store.ReviewListFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	articles := map[string]string{}
	for _, r := range got {
		articles[r.ExternalReviewID] = r.SellerArticle
	}
	if articles["oz-old"] != "85467" {
		t.Fatalf("legacy review not remapped: %v", articles)
	}
	if articles["oz-new"] != "85555" {
		t.Fatalf("fresh review lost its article: %v", articles)
	}
}
