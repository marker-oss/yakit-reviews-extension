package export

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"reviews/internal/reviewjson"
	"reviews/internal/store"
)

func ptrInt(v int) *int { return &v }

func TestBuildBundlesGroupsByNormalizedArticleAndAggregates(t *testing.T) {
	reviews := []store.Review{
		{Marketplace: "wb", ExternalReviewID: "r1", SellerArticle: "3467/white", Rating: ptrInt(5), CreatedAtMP: time.Unix(300, 0)},
		{Marketplace: "wb", ExternalReviewID: "r2", SellerArticle: "3467/black", Rating: ptrInt(4), CreatedAtMP: time.Unix(200, 0)},
		{Marketplace: "wb", ExternalReviewID: "r3", SellerArticle: "999", Rating: nil, CreatedAtMP: time.Unix(100, 0)},
	}

	bundles := BuildBundles(reviews, reviewjson.Mapper{})

	if len(bundles) != 2 {
		t.Fatalf("want 2 articles, got %d", len(bundles))
	}
	b := bundles["3467"]
	if b == nil {
		t.Fatalf("missing article 3467")
	}
	if b.Aggregate.Count != 2 || b.Aggregate.RatingCount != 2 {
		t.Fatalf("aggregate counts = %+v", b.Aggregate)
	}
	if b.Aggregate.RatingAvg != 4.5 {
		t.Fatalf("ratingAvg = %v, want 4.5", b.Aggregate.RatingAvg)
	}
	if b.Reviews[0].ExternalReviewID != "r1" {
		t.Fatalf("want r1 first, got %q", b.Reviews[0].ExternalReviewID)
	}
	if bundles["999"].Aggregate.RatingCount != 0 || bundles["999"].Aggregate.RatingAvg != 0 {
		t.Fatalf("999 aggregate = %+v", bundles["999"].Aggregate)
	}
}

func TestBuildBundlesUsesKnownSiteArticlePrefixes(t *testing.T) {
	reviews := []store.Review{
		{Marketplace: "wb", ExternalReviewID: "r1", SellerArticle: "6202бежевый", Rating: ptrInt(5), CreatedAtMP: time.Unix(300, 0)},
		{Marketplace: "wb", ExternalReviewID: "r2", SellerArticle: "6202красный", Rating: ptrInt(4), CreatedAtMP: time.Unix(200, 0)},
	}
	mapper := reviewjson.Mapper{
		ProductLinks: map[string]string{"6202": "https://shegida.ru/products/p-6202"},
	}

	bundles := BuildBundles(reviews, mapper)

	if len(bundles) != 1 {
		t.Fatalf("want 1 article, got %d", len(bundles))
	}
	b := bundles["6202"]
	if b == nil {
		t.Fatalf("missing article 6202")
	}
	if b.Aggregate.Count != 2 || b.Aggregate.RatingAvg != 4.5 {
		t.Fatalf("aggregate = %+v", b.Aggregate)
	}
}

func TestWriteProducesFiles(t *testing.T) {
	dir := t.TempDir()
	reviews := []store.Review{
		{Marketplace: "wb", ExternalReviewID: "r1", SellerArticle: "107", Rating: ptrInt(5), CreatedAtMP: time.Unix(1, 0)},
	}
	bundles := BuildBundles(reviews, reviewjson.Mapper{})
	if err := Write(dir, bundles, time.Unix(42, 0).UTC()); err != nil {
		t.Fatalf("Write: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "by-article", "107.json"))
	if err != nil {
		t.Fatalf("read by-article: %v", err)
	}
	var bundle Bundle
	if err := json.Unmarshal(raw, &bundle); err != nil {
		t.Fatalf("parse bundle: %v", err)
	}
	if bundle.Article != "107" || len(bundle.Reviews) != 1 {
		t.Fatalf("bundle = %+v", bundle)
	}

	idxRaw, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	var idx Index
	if err := json.Unmarshal(idxRaw, &idx); err != nil {
		t.Fatalf("parse index: %v", err)
	}
	if idx.Articles["107"].Count != 1 {
		t.Fatalf("index = %+v", idx)
	}
}
