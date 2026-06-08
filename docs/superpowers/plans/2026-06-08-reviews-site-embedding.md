# Reviews Site Embedding Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Render a seamless, native-looking per-product reviews block on `shegida.ru` (Yandex Kit) product pages, aggregated by seller article, injected via Yandex Tag Manager, plus schema.org JSON-LD for SEO.

**Architecture:** Backend exports static per-article JSON from SQLite (`reviews export`). A single self-sufficient `loader.js`, loaded once by a YTM Custom HTML tag, watches SPA navigation, reads the product's seller article from the page, fetches that article's JSON over HTTPS, and renders the existing widget inside a Shadow DOM host. JSON-LD is injected into `<head>`. Data is served as static files by Caddy (auto-TLS) on a single VPS; tested locally via a `cloudflared` tunnel.

**Tech Stack:** Go 1.x (stdlib + GORM + modernc SQLite), vanilla JS (no build step), Caddy, Yandex Tag Manager.

---

## Reference: existing code this plan reuses

- `internal/store/models.go` — `Review` (fields: `Marketplace`, `ExternalReviewID`, `ExternalProductID`, `SellerArticle`, `Rating *int`, `AuthorName`, `Text`, `Pros`, `Cons`, `CreatedAtMP`, `UpdatedAtMP *time.Time`, `MPAnswerText *string`, `MPAnswerState *string`, `Raw`, `Media []ReviewMedia`), `ReviewMedia` (`Kind`, `URL`, `PreviewURL *string`, `Position`).
- `internal/store/list.go` — `ListReviews(ctx, ReviewListFilter)` (caps at 500; **not** suitable for full export).
- `internal/server/server.go` — current JSON shape `reviewsResponse{reviews,count}` + `reviewDTO`, plus mapping helpers `toReviewDTO`, `marketplaceReviewURL`, `marketplaceProductURL`, `sellerProductURL`, `productLinkForSellerArticle`, `normalizeSellerArticle`, `sellerArticleForReview`, `urlPathEscape`, `stringValue`. **Task 1 extracts the DTO + mapping into a shared package** so `export` and `serve` produce identical JSON.
- `internal/site/shegida.go` — `LoadProductLinkMap(io.Reader) (map[string]string, error)`.
- `web/reviews-widget/reviews-widget.js` — `window.ReviewsWidget.mount(root, options)`; `options.reviews` (array) or `options.source` (URL). Renders via `root.innerHTML`.
- `web/reviews-widget/reviews-widget.css` — widget styles (currently global; Task 8 loads them into the shadow root).
- `cmd/reviews/main.go` — CLI dispatch in `run()`; per-command `flag.FlagSet` pattern.

Run all Go tests with: `go test ./...` (expected baseline: `config`, `wb`, `store` packages pass).

---

## Phase A — Backend static export

### Task 1: Extract shared review-JSON package

Move the JSON DTO and mapping out of `internal/server` into a reusable `internal/reviewjson` package so `serve` and `export` emit byte-identical review objects.

**Files:**
- Create: `internal/reviewjson/reviewjson.go`
- Create: `internal/reviewjson/reviewjson_test.go`
- Modify: `internal/server/server.go` (replace local DTO/mappers with calls into `reviewjson`)

- [ ] **Step 1: Write the failing test**

`internal/reviewjson/reviewjson_test.go`:

```go
package reviewjson

import (
	"testing"
	"time"

	"reviews/internal/store"
)

func ptrInt(v int) *int          { return &v }
func ptrStr(v string) *string    { return &v }

func TestToReview_WBMapping(t *testing.T) {
	mapper := Mapper{
		ProductURLTemplate: "https://shegida.ru/search?query={seller_article_url}",
		ProductLinks:       map[string]string{"1523": "https://shegida.ru/products/p-1523"},
	}
	r := store.Review{
		Marketplace:       "wb",
		ExternalReviewID:  "wb-1001",
		ExternalProductID: "70476012",
		SellerArticle:     "1523",
		Rating:            ptrInt(5),
		AuthorName:        "Мария",
		Text:              "Отличная ткань",
		CreatedAtMP:       time.Date(2026, 5, 28, 12, 20, 0, 0, time.UTC),
		MPAnswerText:      ptrStr("Спасибо"),
		MPAnswerState:     ptrStr("published"),
		Media: []store.ReviewMedia{
			{Kind: "photo", URL: "https://cdn/p1.jpg", Position: 0},
		},
	}

	out := mapper.ToReview(r)

	if out.SellerArticle != "1523" {
		t.Fatalf("sellerArticle = %q", out.SellerArticle)
	}
	if out.MarketplaceProductURL != "https://www.wildberries.ru/catalog/70476012/detail.aspx" {
		t.Fatalf("marketplaceProductUrl = %q", out.MarketplaceProductURL)
	}
	if out.MarketplaceReviewURL != "https://www.wildberries.ru/catalog/70476012/detail.aspx#comments" {
		t.Fatalf("marketplaceReviewUrl = %q", out.MarketplaceReviewURL)
	}
	if out.SellerProductURL != "https://shegida.ru/products/p-1523" {
		t.Fatalf("sellerProductUrl = %q", out.SellerProductURL)
	}
	if out.Answer == nil || out.Answer.Text != "Спасибо" {
		t.Fatalf("answer = %+v", out.Answer)
	}
	if len(out.Media) != 1 || out.Media[0].Kind != "photo" {
		t.Fatalf("media = %+v", out.Media)
	}
}

func TestNormalizeSellerArticle(t *testing.T) {
	if got := NormalizeSellerArticle("3467/Белый"); got != "3467" {
		t.Fatalf("normalize = %q", got)
	}
	if got := NormalizeSellerArticle("  107 "); got != "107" {
		t.Fatalf("normalize trim = %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/reviewjson/`
Expected: FAIL — package `reviewjson` does not compile (types/functions undefined).

- [ ] **Step 3: Write the implementation**

`internal/reviewjson/reviewjson.go` (move the mapping logic verbatim from `internal/server/server.go`, parameterized by `Mapper`):

```go
// Package reviewjson maps store.Review records to the public JSON shape
// shared by the HTTP API (serve) and the static export.
package reviewjson

import (
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"reviews/internal/store"
)

const marketplaceWB = "wb"

// Mapper holds the per-deployment configuration needed to compute outbound
// links. Zero value is usable (no product links, empty template).
type Mapper struct {
	ProductURLTemplate string
	ProductLinks       map[string]string
}

type Review struct {
	ID                    uint       `json:"id"`
	Marketplace           string     `json:"marketplace"`
	ExternalReviewID      string     `json:"externalReviewId"`
	ExternalProductID     string     `json:"externalProductId"`
	SellerArticle         string     `json:"sellerArticle,omitempty"`
	Rating                *int       `json:"rating"`
	AuthorName            string     `json:"authorName"`
	Text                  string     `json:"text"`
	Pros                  string     `json:"pros"`
	Cons                  string     `json:"cons"`
	CreatedAt             time.Time  `json:"createdAt"`
	UpdatedAt             *time.Time `json:"updatedAt,omitempty"`
	Answer                *Answer    `json:"answer,omitempty"`
	Media                 []Media    `json:"media"`
	MarketplaceReviewURL  string     `json:"marketplaceReviewUrl,omitempty"`
	MarketplaceProductURL string     `json:"marketplaceProductUrl,omitempty"`
	SellerProductURL      string     `json:"sellerProductUrl,omitempty"`
}

type Answer struct {
	Text  string `json:"text"`
	State string `json:"state"`
}

type Media struct {
	Kind       string `json:"kind"`
	URL        string `json:"url"`
	PreviewURL string `json:"previewUrl,omitempty"`
	Position   int    `json:"position"`
}

func (m Mapper) ToReview(review store.Review) Review {
	sellerArticle := SellerArticleForReview(review)

	var answer *Answer
	if review.MPAnswerText != nil || review.MPAnswerState != nil {
		answer = &Answer{Text: stringValue(review.MPAnswerText), State: stringValue(review.MPAnswerState)}
	}

	media := make([]Media, 0, len(review.Media))
	for _, item := range review.Media {
		media = append(media, Media{
			Kind:       item.Kind,
			URL:        item.URL,
			PreviewURL: stringValue(item.PreviewURL),
			Position:   item.Position,
		})
	}

	return Review{
		ID:                    review.ID,
		Marketplace:           review.Marketplace,
		ExternalReviewID:      review.ExternalReviewID,
		ExternalProductID:     review.ExternalProductID,
		SellerArticle:         sellerArticle,
		Rating:                review.Rating,
		AuthorName:            review.AuthorName,
		Text:                  review.Text,
		Pros:                  review.Pros,
		Cons:                  review.Cons,
		CreatedAt:             review.CreatedAtMP,
		UpdatedAt:             review.UpdatedAtMP,
		Answer:                answer,
		Media:                 media,
		MarketplaceReviewURL:  marketplaceReviewURL(review),
		MarketplaceProductURL: marketplaceProductURL(review),
		SellerProductURL:      m.sellerProductURL(review, sellerArticle),
	}
}

func marketplaceReviewURL(review store.Review) string {
	productURL := marketplaceProductURL(review)
	if productURL == "" {
		return ""
	}
	if review.Marketplace == marketplaceWB {
		return productURL + "#comments"
	}
	return productURL
}

func marketplaceProductURL(review store.Review) string {
	switch review.Marketplace {
	case marketplaceWB:
		if review.ExternalProductID == "" {
			return ""
		}
		return "https://www.wildberries.ru/catalog/" + urlPathEscape(review.ExternalProductID) + "/detail.aspx"
	default:
		return ""
	}
}

func (m Mapper) sellerProductURL(review store.Review, sellerArticle string) string {
	article := sellerArticle
	if article == "" {
		article = review.ExternalProductID
	}
	if article == "" {
		return ""
	}
	if sellerArticle != "" {
		if u, ok := m.productLinkForSellerArticle(sellerArticle); ok {
			return u
		}
	}
	if m.ProductURLTemplate == "" {
		return ""
	}
	return strings.NewReplacer(
		"{article}", article,
		"{seller_article}", sellerArticle,
		"{seller_article_url}", url.QueryEscape(sellerArticle),
		"{external_product_id}", review.ExternalProductID,
		"{external_product_id_url}", url.QueryEscape(review.ExternalProductID),
		"{marketplace}", review.Marketplace,
	).Replace(m.ProductURLTemplate)
}

func (m Mapper) productLinkForSellerArticle(article string) (string, bool) {
	if len(m.ProductLinks) == 0 {
		return "", false
	}
	if u, ok := m.ProductLinks[article]; ok {
		return u, true
	}
	normalized := NormalizeSellerArticle(article)
	if normalized == article {
		return "", false
	}
	u, ok := m.ProductLinks[normalized]
	return u, ok
}

// NormalizeSellerArticle collapses marketplace article variants (e.g.
// "3467/Белый") to their base ("3467") and trims whitespace.
func NormalizeSellerArticle(article string) string {
	article = strings.TrimSpace(article)
	if before, _, ok := strings.Cut(article, "/"); ok {
		return strings.TrimSpace(before)
	}
	return article
}

// SellerArticleForReview returns the explicit seller article, falling back to
// parsing it out of the raw WB payload.
func SellerArticleForReview(review store.Review) string {
	if review.SellerArticle != "" {
		return review.SellerArticle
	}
	if review.Raw == "" || review.Marketplace != marketplaceWB {
		return ""
	}
	var raw struct {
		ProductDetails struct {
			SupplierArticle string `json:"supplierArticle"`
		} `json:"productDetails"`
	}
	if err := json.Unmarshal([]byte(review.Raw), &raw); err != nil {
		return ""
	}
	return raw.ProductDetails.SupplierArticle
}

func urlPathEscape(value string) string {
	return strings.NewReplacer("/", "%2F", "?", "%3F", "#", "%23", "&", "%26").Replace(value)
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
```

- [ ] **Step 4: Refactor `internal/server/server.go` to use the package**

In `internal/server/server.go`: delete the local `reviewDTO`, `answerDTO`, `mediaDTO`, `toReviewDTO`, `marketplaceReviewURL`, `marketplaceProductURL`, `sellerProductURL`, `productLinkForSellerArticle`, `normalizeSellerArticle`, `sellerArticleForReview`, `urlPathEscape`, `stringValue`. Add import `"reviews/internal/reviewjson"`. Replace the body of `handleReviews` item-mapping and the `reviewsResponse` type:

```go
type reviewsResponse struct {
	Reviews []reviewjson.Review `json:"reviews"`
	Count   int                 `json:"count"`
}

func (s *Server) handleReviews(w http.ResponseWriter, r *http.Request) {
	filter, err := parseReviewFilter(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	reviews, err := s.store.ListReviews(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	mapper := reviewjson.Mapper{
		ProductURLTemplate: s.cfg.ProductURLTemplate,
		ProductLinks:       s.cfg.ProductLinks,
	}
	items := make([]reviewjson.Review, 0, len(reviews))
	for _, review := range reviews {
		items = append(items, mapper.ToReview(review))
	}
	writeJSON(w, http.StatusOK, reviewsResponse{Reviews: items, Count: len(items)})
}
```

Keep `config` import only if still used elsewhere in the file; if `goimports`/compiler flags it unused, remove it.

- [ ] **Step 5: Run tests to verify everything passes**

Run: `go build ./... && go test ./internal/reviewjson/ ./internal/server/ ./...`
Expected: PASS across the board; `go vet ./...` clean.

- [ ] **Step 6: Commit**

```bash
git add internal/reviewjson/ internal/server/server.go
git commit -m "refactor: extract shared reviewjson package from server"
```

---

### Task 2: Store method to read all reviews for export

`ListReviews` caps at 500. Export needs every review with media preloaded.

**Files:**
- Create: `internal/store/export.go`
- Modify: `internal/store/store_test.go` (add test) — or create `internal/store/export_test.go`

- [ ] **Step 1: Write the failing test**

`internal/store/export_test.go`:

```go
package store

import (
	"context"
	"testing"
	"time"
)

func TestListAllReviews_ReturnsEveryRowWithMedia(t *testing.T) {
	st := newTestStore(t) // existing helper used by store_test.go
	ctx := context.Background()

	// Insert 2 reviews via the existing upsert path used in store_test.go.
	// (Reuse whatever helper store_test.go already uses to seed reviews.)
	seedReview(t, st, "wb", "r1", "art-1", 5, time.Now().Add(-2*time.Hour))
	seedReview(t, st, "wb", "r2", "art-2", 4, time.Now().Add(-1*time.Hour))

	all, err := st.ListAllReviews(ctx)
	if err != nil {
		t.Fatalf("ListAllReviews: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("want 2 reviews, got %d", len(all))
	}
	// Newest first.
	if all[0].ExternalReviewID != "r2" {
		t.Fatalf("want r2 first, got %q", all[0].ExternalReviewID)
	}
}
```

> NOTE for implementer: open `internal/store/store_test.go` first and reuse its existing store-construction and review-seeding helpers. If they have different names than `newTestStore`/`seedReview`, adapt the test to the real helper names — do not invent a second seeding path.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestListAllReviews`
Expected: FAIL — `ListAllReviews` undefined.

- [ ] **Step 3: Write the implementation**

`internal/store/export.go`:

```go
package store

import (
	"context"

	"gorm.io/gorm"
)

// ListAllReviews returns every review with media preloaded, newest first.
// Intended for full static export, not for serving paginated API responses.
func (s *Store) ListAllReviews(ctx context.Context) ([]Review, error) {
	var reviews []Review
	err := s.db.WithContext(ctx).
		Preload("Media", func(db *gorm.DB) *gorm.DB {
			return db.Order("position asc").Order("id asc")
		}).
		Order("created_at_mp desc").
		Find(&reviews).Error
	if err != nil {
		return nil, err
	}
	return reviews, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestListAllReviews -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/export.go internal/store/export_test.go
git commit -m "feat: add ListAllReviews for static export"
```

---

### Task 3: Export builder + JSON writer

Group reviews by normalized seller article, compute aggregate, write `by-article/<article>.json` and `index.json`.

**Files:**
- Create: `internal/export/export.go`
- Create: `internal/export/export_test.go`

- [ ] **Step 1: Write the failing test**

`internal/export/export_test.go`:

```go
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

func TestBuildBundles_GroupsByNormalizedArticleAndAggregates(t *testing.T) {
	reviews := []store.Review{
		{Marketplace: "wb", ExternalReviewID: "r1", SellerArticle: "3467/Белый", Rating: ptrInt(5), CreatedAtMP: time.Unix(300, 0)},
		{Marketplace: "wb", ExternalReviewID: "r2", SellerArticle: "3467/Чёрный", Rating: ptrInt(4), CreatedAtMP: time.Unix(200, 0)},
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
	// Reviews newest-first inside a bundle.
	if b.Reviews[0].ExternalReviewID != "r1" {
		t.Fatalf("want r1 first, got %q", b.Reviews[0].ExternalReviewID)
	}
	// Article with only null ratings: RatingCount 0, RatingAvg 0.
	if bundles["999"].Aggregate.RatingCount != 0 || bundles["999"].Aggregate.RatingAvg != 0 {
		t.Fatalf("999 aggregate = %+v", bundles["999"].Aggregate)
	}
}

func TestWrite_ProducesFiles(t *testing.T) {
	dir := t.TempDir()
	reviews := []store.Review{
		{Marketplace: "wb", ExternalReviewID: "r1", SellerArticle: "107", Rating: ptrInt(5), CreatedAtMP: time.Unix(1, 0)},
	}
	bundles := BuildBundles(reviews, reviewjson.Mapper{})
	if err := Write(dir, bundles, time.Unix(42, 0).UTC()); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// by-article file exists and parses.
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

	// index file exists and lists the article.
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/export/`
Expected: FAIL — package does not compile.

- [ ] **Step 3: Write the implementation**

`internal/export/export.go`:

```go
// Package export builds static per-article review JSON artifacts for the
// embeddable site widget.
package export

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"reviews/internal/reviewjson"
	"reviews/internal/store"
)

type Aggregate struct {
	Count       int     `json:"count"`       // total reviews for the article
	RatingCount int     `json:"ratingCount"` // reviews with a non-null rating
	RatingAvg   float64 `json:"ratingAvg"`   // average over rated reviews, rounded to 0.1
}

type Bundle struct {
	Article   string              `json:"article"`
	Aggregate Aggregate           `json:"aggregate"`
	Reviews   []reviewjson.Review `json:"reviews"`
}

type IndexEntry struct {
	Count     int     `json:"count"`
	RatingAvg float64 `json:"ratingAvg"`
}

type Index struct {
	GeneratedAt time.Time             `json:"generatedAt"`
	Articles    map[string]IndexEntry `json:"articles"`
}

// BuildBundles groups reviews by normalized seller article and computes
// aggregates. Reviews without a seller article are skipped.
func BuildBundles(reviews []store.Review, mapper reviewjson.Mapper) map[string]*Bundle {
	bundles := make(map[string]*Bundle)
	for _, review := range reviews {
		article := reviewjson.NormalizeSellerArticle(reviewjson.SellerArticleForReview(review))
		if article == "" {
			continue
		}
		b := bundles[article]
		if b == nil {
			b = &Bundle{Article: article}
			bundles[article] = b
		}
		b.Reviews = append(b.Reviews, mapper.ToReview(review))
	}

	for _, b := range bundles {
		sort.SliceStable(b.Reviews, func(i, j int) bool {
			return b.Reviews[i].CreatedAt.After(b.Reviews[j].CreatedAt)
		})
		var sum, rated int
		for _, r := range b.Reviews {
			if r.Rating != nil {
				sum += *r.Rating
				rated++
			}
		}
		b.Aggregate = Aggregate{Count: len(b.Reviews), RatingCount: rated}
		if rated > 0 {
			b.Aggregate.RatingAvg = roundTenth(float64(sum) / float64(rated))
		}
	}
	return bundles
}

func roundTenth(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}

// Write emits by-article/<article>.json files and index.json into dir.
func Write(dir string, bundles map[string]*Bundle, generatedAt time.Time) error {
	byArticleDir := filepath.Join(dir, "by-article")
	if err := os.MkdirAll(byArticleDir, 0o755); err != nil {
		return err
	}

	index := Index{GeneratedAt: generatedAt, Articles: make(map[string]IndexEntry, len(bundles))}
	for article, b := range bundles {
		if err := writeJSONFile(filepath.Join(byArticleDir, articleFileName(article)), b); err != nil {
			return err
		}
		index.Articles[article] = IndexEntry{Count: b.Aggregate.Count, RatingAvg: b.Aggregate.RatingAvg}
	}
	return writeJSONFile(filepath.Join(dir, "index.json"), index)
}

// articleFileName keeps the on-disk/URL name safe. Normalized articles in the
// current dataset are short alphanumerics; replace path separators defensively.
func articleFileName(article string) string {
	safe := make([]rune, 0, len(article))
	for _, r := range article {
		switch r {
		case '/', '\\', ' ':
			safe = append(safe, '_')
		default:
			safe = append(safe, r)
		}
	}
	return string(safe) + ".json"
}

func writeJSONFile(path string, value any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}
```

> NOTE: `articleFileName` must match what `loader.js` requests (Task 9). Keep both in sync: loader uses the same `/`,`\`,space → `_` rule before `encodeURIComponent`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/export/ -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/export/
git commit -m "feat: static per-article review export builder"
```

---

### Task 4: `reviews export` CLI command

**Files:**
- Modify: `cmd/reviews/main.go` (add `case "export"`, `runExport`, usage line)

- [ ] **Step 1: Add the command dispatch**

In `run()`'s `switch`, add after the `discover-site-urls` case:

```go
	case "export":
		return runExport(ctx, args[1:], cfg, logger)
```

- [ ] **Step 2: Implement `runExport`**

Add to `cmd/reviews/main.go` (imports needed: `reviews/internal/export`, `reviews/internal/reviewjson`, and `time` is already imported):

```go
func runExport(ctx context.Context, args []string, cfg config.Config, logger *slog.Logger) int {
	flags := flag.NewFlagSet("export", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	outDir := flags.String("out", "web/reviews-data", "output directory for static JSON")
	productURLTemplate := flags.String("product-url-template", cfg.Web.ProductURLTemplate, "seller product URL template")
	if err := flags.Parse(args); err != nil {
		return exitConfigError
	}

	db, err := store.Open(cfg.DB)
	if err != nil {
		logger.Error("open database", "error", err)
		return exitConfigError
	}

	reviews, err := db.ListAllReviews(ctx)
	if err != nil {
		logger.Error("list reviews", "error", err)
		return exitRunError
	}

	mapper := reviewjson.Mapper{
		ProductURLTemplate: *productURLTemplate,
		ProductLinks:       loadProductLinks(cfg.Web.ProductLinksPath, logger),
	}
	bundles := export.BuildBundles(reviews, mapper)

	if err := export.Write(*outDir, bundles, time.Now().UTC()); err != nil {
		logger.Error("write export", "out", *outDir, "error", err)
		return exitRunError
	}

	logger.Info("export complete", "articles", len(bundles), "reviews", len(reviews), "out", *outDir)
	return exitOK
}
```

- [ ] **Step 3: Update `usage()`**

Add to the usage string (after the `discover-site-urls` line):

```go
  reviews export [--out web/reviews-data]
```

- [ ] **Step 4: Build, run against the test DB, eyeball output**

Run:

```bash
go build ./... && \
REVIEWS_DB_DSN=./reviews-e2e.db go run ./cmd/reviews export --out /tmp/reviews-data && \
ls /tmp/reviews-data /tmp/reviews-data/by-article | head && \
python3 -m json.tool /tmp/reviews-data/index.json | head -20
```

Expected: `index.json` + `by-article/*.json` created; `index.json` has an `articles` map; a sample article file (e.g. `107.json`) contains `aggregate` and `reviews`.

- [ ] **Step 5: Commit**

```bash
git add cmd/reviews/main.go
git commit -m "feat: reviews export command for static per-article JSON"
```

---

## Phase B — Frontend: shadow-mount + loader + JSON-LD

### Task 5: Capture a real product page fixture for testing

We need a saved copy of the live DOM to test injection offline (test ladder step 1).

**Files:**
- Create: `web/reviews-widget/test/fixtures/shegida-product.html`
- Create: `web/reviews-widget/test/fixtures/README.md`

- [ ] **Step 1: Download the fixture**

Run:

```bash
mkdir -p web/reviews-widget/test/fixtures && \
curl -sL -A "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36" \
  "https://shegida.ru/products/svitshot-hlopkovyiy-100954" \
  -o web/reviews-widget/test/fixtures/shegida-product.html && \
wc -c web/reviews-widget/test/fixtures/shegida-product.html
```

Expected: a ~400KB HTML file. (This product's seller article is `107`, which exists in the export.)

- [ ] **Step 2: Document the fixture**

`web/reviews-widget/test/fixtures/README.md`:

```markdown
# Test fixtures

`shegida-product.html` — a saved copy of a live Yandex Kit product page
(`https://shegida.ru/products/svitshot-hlopkovyiy-100954`, seller article `107`).
Used to test `loader.js` injection offline. The page contains `"sku":"107"`
in its hydration state. Class names are hashed CSS modules and must NOT be used
as injection anchors. Re-download if the storefront layout changes.
```

- [ ] **Step 3: Commit**

```bash
git add web/reviews-widget/test/fixtures/
git commit -m "test: add saved Yandex Kit product page fixture"
```

---

### Task 6: Make the widget mountable into a Shadow DOM root

The widget already renders via `root.innerHTML`. For Shadow DOM, styles must live inside the shadow root. Add a `mountShadow(host, options)` helper that creates a shadow root, injects the CSS, and calls `mount`.

**Files:**
- Modify: `web/reviews-widget/reviews-widget.js` (extend the public API)
- Modify: `web/reviews-widget/reviews-widget.css` (no behavioral change; confirm it has no `:root`/`body`-scoped rules that break inside shadow — if it does, scope them to the widget container class)

- [ ] **Step 1: Add `mountShadow` and expose CSS injection**

At the bottom of `reviews-widget.js`, replace the public export block with:

```js
  function mountShadow(host, options) {
    if (!host) {
      throw new Error("ReviewsWidget host is required");
    }
    const shadow = host.shadowRoot || host.attachShadow({ mode: "open" });
    shadow.innerHTML = "";

    if (options && options.styleText) {
      const style = document.createElement("style");
      style.textContent = options.styleText;
      shadow.appendChild(style);
    }

    const root = document.createElement("div");
    root.className = "reviews-widget-root";
    shadow.appendChild(root);

    mount(root, options || {});
    return shadow;
  }

  window.ReviewsWidget = {
    mount,
    mountShadow,
    sampleReviews,
  };
})();
```

- [ ] **Step 2: Verify no global-only CSS selectors break inside shadow**

Run: `grep -nE "(^|[^.#a-zA-Z-])(:root|html|body)\b" web/reviews-widget/reviews-widget.css || echo "OK: no global selectors"`
Expected: `OK: no global selectors`. If any are found, rewrite them to target `.reviews-widget-root` (the shadow container) instead. Make that edit if needed.

- [ ] **Step 3: Manual smoke test in a browser**

Create a throwaway check (do not commit): open `web/reviews-widget/demo.html` logic mentally — instead verify via a one-off file:

```bash
cat > /tmp/shadow-smoke.html <<'HTML'
<!doctype html><meta charset="utf-8">
<div id="host"></div>
<script src="reviews-widget.js"></script>
<script>
  fetch("reviews-widget.css").then(r=>r.text()).then(css=>{
    window.ReviewsWidget.mountShadow(document.getElementById("host"), {
      styleText: css,
      reviews: window.ReviewsWidget.sampleReviews,
    });
  });
</script>
HTML
cp /tmp/shadow-smoke.html web/reviews-widget/shadow-smoke.html
(cd web/reviews-widget && python3 -m http.server 8099 >/dev/null 2>&1 &) ; sleep 1
echo "Open http://127.0.0.1:8099/shadow-smoke.html and confirm reviews render inside a shadow root (inspect: #host has #shadow-root)."
```

Expected: the widget renders inside `#host`'s shadow root with styles applied. After confirming, stop the server (`pkill -f "http.server 8099"`) and remove the smoke file: `rm web/reviews-widget/shadow-smoke.html`.

- [ ] **Step 4: Commit**

```bash
git add web/reviews-widget/reviews-widget.js web/reviews-widget/reviews-widget.css
git commit -m "feat: mountShadow for Shadow DOM embedding of the widget"
```

---

### Task 7: Loader config + single-bundle data shape

Decide the runtime config the loader reads and the function that turns a `Bundle` into widget `reviews`. Keep loader logic testable by splitting pure helpers from DOM glue.

**Files:**
- Create: `web/reviews-widget/loader.js`
- Create: `web/reviews-widget/test/loader.test.html` (browser-run assertions, no framework)

This task builds the **pure helpers** of the loader; Task 8 adds DOM glue; Task 9 wires injection. Splitting keeps each step reviewable.

- [ ] **Step 1: Write `loader.js` skeleton with pure helpers + config**

`web/reviews-widget/loader.js`:

```js
/*
 * Reviews embed loader for shegida.ru (Yandex Kit).
 * Loaded ONCE by a Yandex Tag Manager "Custom HTML" tag. Self-sufficient:
 * watches SPA navigation, reads the product's seller article, fetches that
 * article's JSON, renders the widget into a Shadow DOM host, and injects
 * JSON-LD. Never relies on the tag re-firing per product.
 */
(function () {
  "use strict";

  // --- Config (overridable via window.REVIEWS_EMBED_CONFIG before load) -----
  var CFG = Object.assign(
    {
      dataBase: "https://reviews.shegida.ru/reviews-data", // no trailing slash
      widgetJsUrl: "https://reviews.shegida.ru/reviews-widget.js",
      widgetCssUrl: "https://reviews.shegida.ru/reviews-widget.css",
      productPathPrefix: "/products/",
      hostId: "reviews-embed-host",
      maxJsonLdReviews: 10,
      debug: false,
    },
    window.REVIEWS_EMBED_CONFIG || {}
  );

  function log() {
    if (CFG.debug && window.console) console.log.apply(console, ["[reviews-embed]"].concat([].slice.call(arguments)));
  }

  // --- Pure helpers (unit-tested in loader.test.html) -----------------------

  // True for a storefront product page URL.
  function isProductPath(pathname) {
    return typeof pathname === "string" && pathname.indexOf(CFG.productPathPrefix) === 0;
  }

  // Mirror of Go reviewjson.NormalizeSellerArticle: cut at "/", trim.
  function normalizeArticle(article) {
    if (!article) return "";
    var s = String(article).trim();
    var slash = s.indexOf("/");
    if (slash >= 0) s = s.slice(0, slash).trim();
    return s;
  }

  // Mirror of Go export.articleFileName (minus the .json): "/","\\"," " -> "_".
  function articleFileKey(article) {
    return String(article).replace(/[\/\\ ]/g, "_");
  }

  // Build the by-article JSON URL for a raw article string.
  function bundleUrl(rawArticle) {
    var norm = normalizeArticle(rawArticle);
    if (!norm) return "";
    return CFG.dataBase + "/by-article/" + encodeURIComponent(articleFileKey(norm)) + ".json";
  }

  // Extract the seller article from page text. The Yandex Kit hydration state
  // contains "sku":"<article>"; fall back to a visible "Артикул" label.
  function extractArticleFromHTML(html) {
    if (!html) return "";
    var m = /"sku"\s*:\s*"([^"]+)"/i.exec(html);
    if (m) return m[1];
    m = /Артикул[^0-9A-Za-zА-Яа-яЁё]{0,8}([0-9A-Za-zА-Яа-яЁё\/_.\-]+)/i.exec(html);
    return m ? m[1] : "";
  }

  // Build schema.org JSON-LD from a bundle, capping embedded reviews.
  function buildJsonLd(bundle, maxReviews) {
    if (!bundle || !bundle.aggregate || bundle.aggregate.ratingCount < 1) return null;
    var reviews = (bundle.reviews || [])
      .filter(function (r) { return r.rating != null; })
      .slice(0, maxReviews)
      .map(function (r) {
        return {
          "@type": "Review",
          author: { "@type": "Person", name: r.authorName || "Покупатель" },
          datePublished: r.createdAt,
          reviewRating: { "@type": "Rating", ratingValue: r.rating, bestRating: 5, worstRating: 1 },
          reviewBody: r.text || "",
        };
      });
    return {
      "@context": "https://schema.org",
      "@type": "AggregateRating",
      ratingValue: bundle.aggregate.ratingAvg,
      reviewCount: bundle.aggregate.count,
      ratingCount: bundle.aggregate.ratingCount,
      bestRating: 5,
      worstRating: 1,
      review: reviews,
    };
  }

  // Expose pure helpers for unit testing without running the DOM glue.
  window.__reviewsEmbedInternals = {
    isProductPath: isProductPath,
    normalizeArticle: normalizeArticle,
    articleFileKey: articleFileKey,
    bundleUrl: bundleUrl,
    extractArticleFromHTML: extractArticleFromHTML,
    buildJsonLd: buildJsonLd,
    cfg: CFG,
  };

  // DOM glue is appended in Task 8 / Task 9 below the internals export.
  window.__reviewsEmbedBoot = function boot() { /* replaced in Task 8 */ };
})();
```

- [ ] **Step 2: Write browser unit tests for the pure helpers**

`web/reviews-widget/test/loader.test.html`:

```html
<!doctype html>
<meta charset="utf-8" />
<title>loader unit tests</title>
<pre id="out"></pre>
<script>
  // Prevent the boot glue from running during pure-helper tests.
  window.REVIEWS_EMBED_CONFIG = { debug: true };
</script>
<script src="../loader.js"></script>
<script>
  var out = document.getElementById("out");
  var pass = 0, fail = 0;
  function eq(name, got, want) {
    var ok = JSON.stringify(got) === JSON.stringify(want);
    out.textContent += (ok ? "PASS " : "FAIL ") + name + (ok ? "" : "  got=" + JSON.stringify(got) + " want=" + JSON.stringify(want)) + "\n";
    ok ? pass++ : fail++;
  }
  var I = window.__reviewsEmbedInternals;

  eq("isProductPath true", I.isProductPath("/products/svitshot-100954"), true);
  eq("isProductPath false", I.isProductPath("/catalog/platya"), false);
  eq("normalizeArticle slash", I.normalizeArticle("3467/Белый"), "3467");
  eq("normalizeArticle trim", I.normalizeArticle("  107 "), "107");
  eq("articleFileKey space", I.articleFileKey("a b/c"), "a_b_c");
  eq("bundleUrl", I.bundleUrl("3467/Белый"), I.cfg.dataBase + "/by-article/3467.json");
  eq("extract sku", I.extractArticleFromHTML('x "sku":"107" y'), "107");
  eq("extract label", I.extractArticleFromHTML("Артикул: 1523"), "1523");

  var ld = I.buildJsonLd(
    { aggregate: { count: 2, ratingCount: 2, ratingAvg: 4.5 }, reviews: [
      { rating: 5, authorName: "A", createdAt: "2026-05-01T00:00:00Z", text: "t1" },
      { rating: 4, authorName: "B", createdAt: "2026-04-01T00:00:00Z", text: "t2" },
    ] },
    10
  );
  eq("jsonld ratingValue", ld.ratingValue, 4.5);
  eq("jsonld reviewCount", ld.reviewCount, 2);
  eq("jsonld reviews len", ld.review.length, 2);
  eq("jsonld null when empty", I.buildJsonLd({ aggregate: { count: 0, ratingCount: 0, ratingAvg: 0 } }, 10), null);

  out.textContent += "\n" + pass + " passed, " + fail + " failed\n";
  document.title = fail === 0 ? "ALL PASS" : "FAIL";
</script>
```

- [ ] **Step 3: Run the unit tests in a browser**

Run:

```bash
(cd web/reviews-widget && python3 -m http.server 8099 >/dev/null 2>&1 &) ; sleep 1
echo "Open http://127.0.0.1:8099/test/loader.test.html — expect title 'ALL PASS' and '0 failed'."
```

Expected: page shows all `PASS` lines and `0 failed`. (If you have the Chrome DevTools / Playwright MCP available, navigate there and read `document.title` to assert `ALL PASS`.) Stop the server afterward: `pkill -f "http.server 8099"`.

- [ ] **Step 4: Commit**

```bash
git add web/reviews-widget/loader.js web/reviews-widget/test/loader.test.html
git commit -m "feat: loader pure helpers (article parse, url, json-ld) + unit tests"
```

---

### Task 8: Loader DOM glue — find anchor, mount widget, idempotency, SPA watch

**Files:**
- Modify: `web/reviews-widget/loader.js` (replace the `__reviewsEmbedBoot` placeholder and add the SPA watcher + bootstrap)

- [ ] **Step 1: Replace the boot placeholder with real DOM glue**

In `loader.js`, replace the final two lines (the `window.__reviewsEmbedBoot = ...` placeholder and nothing after the internals export) with:

```js
  // --- DOM glue -------------------------------------------------------------

  var widgetLoading = null; // Promise<void> resolving when widget JS+CSS ready
  var widgetCssText = "";
  var currentArticle = null; // normalized article currently rendered

  function loadWidgetAssets() {
    if (widgetLoading) return widgetLoading;
    widgetLoading = new Promise(function (resolve, reject) {
      var cssDone = fetch(CFG.widgetCssUrl).then(function (r) { return r.text(); }).then(function (t) { widgetCssText = t; });
      var s = document.createElement("script");
      s.src = CFG.widgetJsUrl;
      s.onload = function () { cssDone.then(resolve, reject); };
      s.onerror = function () { reject(new Error("widget js failed")); };
      document.head.appendChild(s);
    });
    return widgetLoading;
  }

  // Find a stable insertion anchor WITHOUT relying on hashed CSS-module class
  // names. Strategy: insert at the end of the main product content column.
  // We anchor off the product <h1> (stable semantic element), then walk up to
  // a reasonably-sized container and append after it. Returns the element our
  // host should be inserted AFTER, or null.
  function findAnchor() {
    var h1 = document.querySelector("main h1, h1");
    if (!h1) return null;
    var node = h1;
    // Walk up to a container that spans a meaningful chunk of the page.
    for (var i = 0; i < 6 && node.parentElement; i++) {
      var parent = node.parentElement;
      if (parent.tagName === "MAIN" || parent === document.body) break;
      node = parent;
    }
    return node;
  }

  function removeHost() {
    var existing = document.getElementById(CFG.hostId);
    if (existing && existing.parentNode) existing.parentNode.removeChild(existing);
    var ld = document.getElementById(CFG.hostId + "-jsonld");
    if (ld && ld.parentNode) ld.parentNode.removeChild(ld);
  }

  function injectJsonLd(bundle) {
    var data = buildJsonLd(bundle, CFG.maxJsonLdReviews);
    if (!data) return;
    var script = document.createElement("script");
    script.type = "application/ld+json";
    script.id = CFG.hostId + "-jsonld";
    script.textContent = JSON.stringify(data);
    document.head.appendChild(script);
  }

  function render(bundle, normArticle) {
    removeHost();
    var anchor = findAnchor();
    if (!anchor || !anchor.parentNode) { log("no anchor"); return; }

    var host = document.createElement("div");
    host.id = CFG.hostId;
    host.setAttribute("data-article", normArticle);
    anchor.parentNode.insertBefore(host, anchor.nextSibling);

    window.ReviewsWidget.mountShadow(host, { styleText: widgetCssText, reviews: bundle.reviews || [] });
    injectJsonLd(bundle);
    currentArticle = normArticle;
    log("rendered", normArticle, (bundle.reviews || []).length, "reviews");
  }

  // Called whenever the page may have settled on a (possibly new) product.
  function handleNavigation() {
    if (!isProductPath(location.pathname)) {
      if (currentArticle !== null) { removeHost(); currentArticle = null; }
      return;
    }
    var rawArticle = extractArticleFromHTML(document.documentElement.innerHTML);
    var norm = normalizeArticle(rawArticle);
    if (!norm) { log("no article on page yet"); return; }
    if (norm === currentArticle && document.getElementById(CFG.hostId)) return; // idempotent

    var url = bundleUrl(rawArticle);
    log("fetch", url);
    loadWidgetAssets()
      .then(function () { return fetch(url, { headers: { Accept: "application/json" } }); })
      .then(function (r) { if (!r.ok) throw new Error("bundle " + r.status); return r.json(); })
      .then(function (bundle) { render(bundle, norm); })
      .catch(function (e) {
        // No reviews for this article (404) or transient error: stay silent in
        // the page, only log in debug. Do not throw into the host page.
        if (currentArticle !== null) { removeHost(); currentArticle = null; }
        log("skip", url, e && e.message);
      });
  }

  // Debounced trigger so a burst of DOM mutations collapses into one run.
  var debounceTimer = null;
  function scheduleHandle() {
    if (debounceTimer) clearTimeout(debounceTimer);
    debounceTimer = setTimeout(handleNavigation, 400);
  }

  function installSpaWatch() {
    // 1) history.pushState / replaceState hooks for SPA route changes.
    ["pushState", "replaceState"].forEach(function (fn) {
      var orig = history[fn];
      history[fn] = function () {
        var ret = orig.apply(this, arguments);
        scheduleHandle();
        return ret;
      };
    });
    window.addEventListener("popstate", scheduleHandle);

    // 2) MutationObserver: SPA re-renders product content without a URL change
    // (e.g. variant switches) and can wipe our host node.
    var mo = new MutationObserver(function () {
      if (isProductPath(location.pathname) && !document.getElementById(CFG.hostId)) {
        scheduleHandle();
      }
    });
    mo.observe(document.body, { childList: true, subtree: true });
  }

  function boot() {
    if (window.__reviewsEmbedBooted) return;
    window.__reviewsEmbedBooted = true;
    installSpaWatch();
    if (document.readyState === "loading") {
      document.addEventListener("DOMContentLoaded", scheduleHandle);
    } else {
      scheduleHandle();
    }
  }

  window.__reviewsEmbedBoot = boot;
  boot();
})();
```

> NOTE: the placeholder line `window.__reviewsEmbedBoot = function boot() { /* replaced in Task 8 */ };` from Task 7 is removed and replaced by the block above. The internals export from Task 7 stays.

- [ ] **Step 2: Offline render test against the saved fixture**

Build a test page that loads the fixture's DOM, points the loader at local data, and asserts the host node appears. First generate local data and a fixture-driven harness:

```bash
# Ensure local export exists (from Task 4) at web/reviews-data.
REVIEWS_DB_DSN=./reviews-e2e.db go run ./cmd/reviews export --out web/reviews-data >/dev/null

cat > web/reviews-widget/test/fixture-render.html <<'HTML'
<!doctype html><meta charset="utf-8"><title>fixture render</title>
<script>
  // Point loader at local static data + local widget assets; disable history.
  window.REVIEWS_EMBED_CONFIG = {
    dataBase: "/reviews-data",
    widgetJsUrl: "/reviews-widget.js",
    widgetCssUrl: "/reviews-widget.css",
    debug: true,
  };
  // Pretend we're on a product URL (history API is same-origin localhost).
  history.replaceState({}, "", "/products/svitshot-hlopkovyiy-100954");
</script>
<!-- Inline the fixture body so the article "sku":"107" is present in the DOM. -->
<div id="fixture-mount"></div>
<script>
  fetch("/test/fixtures/shegida-product.html")
    .then(function (r) { return r.text(); })
    .then(function (html) {
      // Drop the fixture's own <script>/<head> noise; keep body markup.
      var m = /<body[^>]*>([\s\S]*?)<\/body>/i.exec(html);
      document.getElementById("fixture-mount").innerHTML = m ? m[1] : html;
      // Now load the loader AFTER the fixture DOM exists.
      var s = document.createElement("script");
      s.src = "/loader.js";
      document.body.appendChild(s);
    });
</script>
HTML
```

Run:

```bash
(cd web/reviews-widget && python3 -m http.server 8099 >/dev/null 2>&1 &) ; sleep 1
echo "Open http://127.0.0.1:8099/test/fixture-render.html"
echo "Expect: a #reviews-embed-host div with a shadow root containing review cards for article 107,"
echo "and a <script id='reviews-embed-host-jsonld' type='application/ld+json'> in <head>."
```

Expected: the reviews block renders inside the fixture; `document.getElementById('reviews-embed-host')` exists with a shadow root; the JSON-LD script is present. Stop server (`pkill -f "http.server 8099"`). Remove the throwaway harness afterward if you don't want it committed, or keep it (see Step 3).

- [ ] **Step 3: Commit (keep the harness as a committed manual test)**

```bash
git add web/reviews-widget/loader.js web/reviews-widget/test/fixture-render.html
git commit -m "feat: loader DOM glue with SPA watch, shadow mount, json-ld"
```

> `web/reviews-data/` is generated output — ensure it is gitignored (Task 11 adds the ignore rule). Do not commit it here.

---

### Task 9: Automated live-DOM check via browser MCP (optional but recommended)

If the Chrome DevTools or Playwright MCP is available, add a scripted check that runs the loader against the live site (test ladder step 2, automated). If no MCP is available, this task is the manual bookmarklet in Step 2.

**Files:**
- Create: `web/reviews-widget/test/bookmarklet.md`

- [ ] **Step 1: Write the bookmarklet doc**

`web/reviews-widget/test/bookmarklet.md`:

```markdown
# Live-site test bookmarklet

Paste this in the browser console on a live `https://shegida.ru/products/...`
page to run the embed exactly as YTM will (uses production data host):

```js
(function(){
  window.REVIEWS_EMBED_CONFIG = { debug: true };
  var s = document.createElement('script');
  s.src = 'https://reviews.shegida.ru/loader.js?ts=' + Date.now();
  document.body.appendChild(s);
})();
```

To test against a **local tunnel** instead, set `dataBase`, `widgetJsUrl`,
`widgetCssUrl`, and `s.src` to your `cloudflared` HTTPS URL.

Expected: a reviews block appears below the product content; navigating to
another product updates it; `<head>` gains a JSON-LD script.
```

- [ ] **Step 2: (If MCP available) automate the check**

Using the Playwright or Chrome DevTools MCP:
1. Navigate to `https://shegida.ru/products/svitshot-hlopkovyiy-100954`.
2. Evaluate the bookmarklet body (pointed at your tunnel URL, since prod host may not exist yet).
3. Wait for `#reviews-embed-host` to exist; screenshot.
4. Assert `document.querySelector('#reviews-embed-host').shadowRoot` is non-null.
5. Assert `document.querySelector('#reviews-embed-host-jsonld')` exists.
6. Navigate to a second product; assert the host's `data-article` attribute changes.

Record pass/fail in the task notes. (No code file to commit beyond the doc; this is a verification step.)

- [ ] **Step 3: Commit**

```bash
git add web/reviews-widget/test/bookmarklet.md
git commit -m "test: live-site embed bookmarklet + automated MCP check notes"
```

---

## Phase C — Deploy scaffolding

### Task 10: Caddyfile + cross-compile + deploy notes

**Files:**
- Create: `deploy/Caddyfile`
- Create: `deploy/README.md`
- Create: `deploy/build.sh`

- [ ] **Step 1: Write the Caddyfile**

`deploy/Caddyfile` (serves static JSON + widget assets with CORS and auto-TLS):

```caddyfile
reviews.shegida.ru {
	encode zstd gzip

	# Static review data + embed assets.
	root * /srv/reviews/web

	# CORS: the widget is fetched cross-origin from the storefront.
	header {
		Access-Control-Allow-Origin "*"
		Access-Control-Allow-Methods "GET, OPTIONS"
		Cache-Control "public, max-age=300"
	}

	@options method OPTIONS
	respond @options 204

	# Serve loader.js, reviews-widget.js/.css from web/reviews-widget,
	# and JSON from web/reviews-data. Symlink/lay them out under /srv/reviews/web.
	file_server
}
```

- [ ] **Step 2: Write the cross-compile script**

`deploy/build.sh`:

```bash
#!/usr/bin/env sh
# Cross-compile the linux/amd64 binary locally to avoid building on a 1GB VPS.
set -eu
OUT="${1:-./dist/reviews-linux-amd64}"
mkdir -p "$(dirname "$OUT")"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o "$OUT" ./cmd/reviews
echo "built $OUT"
ls -lh "$OUT"
```

- [ ] **Step 3: Write deploy notes**

`deploy/README.md`:

```markdown
# Deploy (single VPS, 1 CPU / 1 GB / 20 GB)

## Layout on the VPS
```
/srv/reviews/
  reviews                      # the linux binary (from deploy/build.sh)
  reviews.db                   # SQLite database
  data/shegida-product-links.json
  web/                         # what Caddy serves at reviews.shegida.ru
    loader.js                  # copy of web/reviews-widget/loader.js
    reviews-widget.js          # copy of web/reviews-widget/reviews-widget.js
    reviews-widget.css         # copy of web/reviews-widget/reviews-widget.css
    reviews-data/              # output of `reviews export` (by-article/*.json, index.json)
```

## Build & ship
```
./deploy/build.sh ./dist/reviews-linux-amd64
scp ./dist/reviews-linux-amd64 vps:/srv/reviews/reviews
scp web/reviews-widget/loader.js web/reviews-widget/reviews-widget.js \
    web/reviews-widget/reviews-widget.css vps:/srv/reviews/web/
```

## Hourly sync + export (cron on the VPS)
```
0 * * * * cd /srv/reviews && REVIEWS_DB_DSN=./reviews.db ./reviews sync --once && \
          REVIEWS_DB_DSN=./reviews.db ./reviews export --out ./web/reviews-data
```

## TLS
Caddy obtains a Let's Encrypt cert automatically for `reviews.shegida.ru`.
Point an A record for that subdomain at the VPS IP (DNS is independent of the
Yandex Kit storefront hosting).

## Yandex Tag Manager
In the Metrika counter for shegida.ru, enable Tag Manager, add a **Custom HTML**
tag firing on page view:
```
<script src="https://reviews.shegida.ru/loader.js" async></script>
```

## HTTPS is mandatory
The storefront is HTTPS; the loader and all data URLs must be HTTPS or the
browser blocks them as mixed content. For pre-DNS testing use a `cloudflared`
tunnel and override `window.REVIEWS_EMBED_CONFIG` accordingly.
```

- [ ] **Step 4: Make build.sh executable, verify cross-compile works**

Run:

```bash
chmod +x deploy/build.sh && ./deploy/build.sh ./dist/reviews-linux-amd64 && file ./dist/reviews-linux-amd64
```

Expected: builds successfully; `file` reports `ELF 64-bit LSB executable, x86-64`.

- [ ] **Step 5: Commit**

```bash
git add deploy/
git commit -m "chore: Caddy config, cross-compile script, deploy runbook"
```

---

### Task 11: Gitignore generated output + docs touch-up

**Files:**
- Modify: `.gitignore`
- Modify: `README.md`

- [ ] **Step 1: Ignore generated artifacts**

Append to `.gitignore`:

```
/web/reviews-data/
/dist/
```

- [ ] **Step 2: Document the new command + embed flow in README**

Add to `README.md` under "Expected Binary Commands":

```sh
reviews export --out web/reviews-data
```

And a short "Site embedding" subsection linking the design and plan docs and noting: static per-article JSON served by Caddy over HTTPS, embedded on Yandex Kit via a Yandex Tag Manager Custom HTML tag that loads `loader.js`.

- [ ] **Step 3: Commit**

```bash
git add .gitignore README.md
git commit -m "docs: gitignore export output; document export + embedding"
```

---

## Self-Review checklist (run before execution)

- **Spec coverage:**
  - §1 product-card reviews by article → Tasks 3, 8.
  - §2 read article from page / hashed-class avoidance → Tasks 6 (`findAnchor`), 8.
  - §3 YTM single-load loader, sandbox-risk → Task 10 (tag), risk verified in Task 9.
  - §4.1 `reviews export` → Tasks 2–4.
  - §4.2 loader.js (SPA watch, fetch, shadow mount, json-ld) → Tasks 7–8.
  - §4.3 widget reuse in shadow root → Task 6.
  - §5 data flow (export → static JSON → loader) → Tasks 3, 4, 10.
  - §6 transport HTTPS/CORS, Caddy, cross-compile, tunnel → Task 10.
  - §7 JSON-LD ≥1 review + cap; article normalization reuse; silent on miss → Tasks 3, 7 (`buildJsonLd`), 8 (`catch` stays silent).
  - §8 test ladder: fixture (5), shadow smoke (6), pure-helper unit (7), offline fixture render (8), live bookmarklet/MCP (9).
- **Type consistency:** `Bundle`/`Aggregate`/`Index` names identical across Tasks 3–4 and loader JSON keys (`aggregate.count`, `aggregate.ratingCount`, `aggregate.ratingAvg`, `reviews`). `articleFileName` (Go) ↔ `articleFileKey` (JS) use the same `/`,`\`,space→`_` rule.
- **Placeholder scan:** none — every code step shows full code. The only intentional stub (`__reviewsEmbedBoot` in Task 7) is explicitly replaced in Task 8.

## Open items deferred to execution

- Final prod subdomain (`reviews.shegida.ru` assumed; change in `deploy/Caddyfile` + `loader.js` CFG if different).
- Confirm YTM "Custom HTML" runs DOM-accessing JS unsandboxed (Task 9); if blocked, fall back to the iframe variant from the design's §7 (not planned here — would be a follow-up plan).
</content>
