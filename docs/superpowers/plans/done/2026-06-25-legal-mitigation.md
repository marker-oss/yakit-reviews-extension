# Legal Mitigation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the project's 152-ФЗ / advertising-law / attribution risks by removing personal-data storage, blurring faces in the widget via a non-storing image proxy, labeling seller replies, adding YM attribution, and shipping the legal obvязка (privacy template, deletion channel, installer consent).

**Architecture:** Personal data is anonymized at the single ingestion chokepoint (`store.UpsertReview`) and a one-time startup migration scrubs already-stored rows; the full marketplace `Raw` blob stops being persisted. Face blur is a new same-origin `internal/mediaproxy` package exposed at `GET /media?u=` that fetches marketplace-CDN images, blurs detected faces with pure-Go pigo, and streams them with an in-memory-only LRU cache (nothing written to DB or disk). The widget routes image URLs through that proxy and labels seller vs marketplace replies.

**Tech Stack:** Go 1.26.3 (module `reviews`), GORM, `github.com/esimov/pigo` (pure Go, no CGO), standard `image`/`image/jpeg`, plain unbundled JS widget.

## Global Constraints

- Go version floor: **1.26.3** (`go.mod`); module path is **`reviews`**.
- **No CGO** — the project uses pure-Go drivers deliberately; pigo is pure Go. Do not add cgo deps.
- Widget JS is **plain, unbundled, unminified** — edit `web/reviews-widget/reviews-widget.js` directly; no build step.
- Backend tests run with `go test ./...`; gate every Go task on it.
- Widget has **no CLI test runner** — verify widget logic via a pure function plus an assertion in a browser harness page and a visual fixture check.
- All admin mutation routes go through `requireSession` + `requireCSRF` (see `internal/server/middleware.go`).
- Commit after each task with the shown message.

---

### Task 1: Anonymize author name at ingestion

**Files:**
- Create: `internal/store/anonymize.go`
- Create: `internal/store/anonymize_test.go`
- Modify: `internal/store/reviews.go` (`UpsertReview`, lines 30-49 and 68-83)

**Interfaces:**
- Produces: `func AnonymizeAuthorName(full string) string` (package `store`) — used by Task 3 migration and Task 1 upsert.

- [ ] **Step 1: Write the failing test**

```go
// internal/store/anonymize_test.go
package store

import "testing"

func TestAnonymizeAuthorName(t *testing.T) {
	cases := map[string]string{
		"Анна Котова":      "Анна К.",
		"анна котова":      "анна к.",
		"Иван":             "Иван",
		"":                 "",
		"  Пётр  Сидоров ": "Пётр С.",
		"Mary Jane Watson": "Mary J.",
		"Анна К.":          "Анна К.", // already anonymized — idempotent
	}
	for in, want := range cases {
		if got := AnonymizeAuthorName(in); got != want {
			t.Errorf("AnonymizeAuthorName(%q) = %q, want %q", in, got, want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestAnonymizeAuthorName -v`
Expected: FAIL — `undefined: AnonymizeAuthorName`

- [ ] **Step 3: Write minimal implementation**

```go
// internal/store/anonymize.go
package store

import (
	"strings"
	"unicode"
)

// AnonymizeAuthorName reduces a full name to first name + surname initial
// ("Анна Котова" -> "Анна К."). One-word and empty names are returned as-is.
// It is idempotent: an already-anonymized name is returned unchanged.
func AnonymizeAuthorName(full string) string {
	fields := strings.Fields(full)
	switch len(fields) {
	case 0:
		return ""
	case 1:
		return fields[0]
	}
	first := fields[0]
	second := fields[1]
	// Already anonymized ("X.") — leave it.
	if strings.HasSuffix(second, ".") {
		return first + " " + second
	}
	r := []rune(second)
	return first + " " + string(unicode.ToUpper(r[0])) + "."
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestAnonymizeAuthorName -v`
Expected: PASS

- [ ] **Step 5: Wire into UpsertReview**

In `internal/store/reviews.go`, immediately after `now := time.Now().UTC()` (line 29) add:

```go
		authorName := AnonymizeAuthorName(input.AuthorName)
```

Change the `next` struct field (line 38) from `AuthorName: input.AuthorName,` to:

```go
			AuthorName:        authorName,
```

Change the `Raw` field (line 47) from `Raw: string(input.Raw),` to:

```go
			Raw:               "",
```

In the `updates` map, change line 73 from `"author_name": input.AuthorName,` to:

```go
				"author_name":         authorName,
```

and change line 81 from `"raw": string(input.Raw),` to:

```go
				"raw":                 "",
```

- [ ] **Step 6: Add an upsert-level regression test**

Append to `internal/store/anonymize_test.go` (uses the existing test store helper — confirm the helper name with `rg -n "func newTestStore|func testStore|OpenMemory|func setupStore" internal/store`; this plan assumes `newTestStore(t)` returning `*Store`):

```go
func TestUpsertReviewAnonymizesAndDropsRaw(t *testing.T) {
	s := newTestStore(t)
	_, err := s.UpsertReview(testCtx(t), marketplace.Review{
		Marketplace:      "wb",
		ExternalReviewID: "r1",
		AuthorName:       "Анна Котова",
		Text:             "ok",
		Raw:              []byte(`{"userName":"Анна Котова"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	var got Review
	if err := s.db.Where("external_review_id = ?", "r1").First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.AuthorName != "Анна К." {
		t.Errorf("AuthorName = %q, want %q", got.AuthorName, "Анна К.")
	}
	if got.Raw != "" {
		t.Errorf("Raw = %q, want empty", got.Raw)
	}
}
```

Add imports `"reviews/internal/marketplace"` and whatever the existing helpers need. If the store test helper / `testCtx` names differ, adapt to the existing ones found via `rg`.

- [ ] **Step 7: Run the store tests**

Run: `go test ./internal/store/ -v`
Expected: PASS (all, including the two new tests)

- [ ] **Step 8: Commit**

```bash
git add internal/store/anonymize.go internal/store/anonymize_test.go internal/store/reviews.go
git commit -m "feat(store): anonymize author name and stop storing Raw at ingestion"
```

---

### Task 2: Backfill SellerArticle from Raw helper (for migration)

**Files:**
- Modify: `internal/reviewjson/reviewjson.go` (confirm `SellerArticleForReview` signature, lines ~235-250)
- Create: `internal/store/migrate_pd.go` (extraction helper only in this task)
- Create: `internal/store/migrate_pd_test.go`

**Interfaces:**
- Produces: `func supplierArticleFromRaw(raw string) string` (package `store`, unexported) — used by Task 3.

The WB `Raw` blob holds `productDetails.supplierArticle`, the only non-PII field migration must preserve before clearing `Raw`. New ingests already populate `SellerArticle` (both adapters set it), so this is purely for legacy rows.

- [ ] **Step 1: Write the failing test**

```go
// internal/store/migrate_pd_test.go
package store

import "testing"

func TestSupplierArticleFromRaw(t *testing.T) {
	raw := `{"productDetails":{"supplierArticle":"ART-1"},"userName":"Анна Котова"}`
	if got := supplierArticleFromRaw(raw); got != "ART-1" {
		t.Errorf("got %q, want ART-1", got)
	}
	if got := supplierArticleFromRaw(`{}`); got != "" {
		t.Errorf("got %q, want empty", got)
	}
	if got := supplierArticleFromRaw(``); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestSupplierArticleFromRaw -v`
Expected: FAIL — `undefined: supplierArticleFromRaw`

- [ ] **Step 3: Write minimal implementation**

```go
// internal/store/migrate_pd.go
package store

import "encoding/json"

func supplierArticleFromRaw(raw string) string {
	if raw == "" {
		return ""
	}
	var parsed struct {
		ProductDetails struct {
			SupplierArticle string `json:"supplierArticle"`
		} `json:"productDetails"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return ""
	}
	return parsed.ProductDetails.SupplierArticle
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestSupplierArticleFromRaw -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/store/migrate_pd.go internal/store/migrate_pd_test.go
git commit -m "feat(store): add supplierArticle extraction helper for PD migration"
```

---

### Task 3: One-time PD scrub migration

**Files:**
- Modify: `internal/store/migrate_pd.go` (add `ScrubPersonalData`)
- Modify: `internal/store/migrate_pd_test.go`
- Modify: the migration/startup wiring — find where `AutoMigrate` runs with `rg -n "AutoMigrate|func Open|func New\(" internal/store` and call `ScrubPersonalData` once right after migrations succeed.

**Interfaces:**
- Produces: `func (s *Store) ScrubPersonalData(ctx context.Context) (scrubbed int, err error)` — idempotent; safe to run every startup.

- [ ] **Step 1: Write the failing test**

```go
func TestScrubPersonalData(t *testing.T) {
	s := newTestStore(t)
	// Insert a legacy row directly: full name, populated Raw, empty SellerArticle.
	s.db.Create(&Review{
		Marketplace: "wb", ExternalReviewID: "legacy1",
		AuthorName: "Анна Котова", SellerArticle: "",
		Raw: `{"productDetails":{"supplierArticle":"ART-9"},"userName":"Анна Котова"}`,
	})
	n, err := s.ScrubPersonalData(testCtx(t))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("scrubbed = %d, want 1", n)
	}
	var got Review
	s.db.Where("external_review_id = ?", "legacy1").First(&got)
	if got.AuthorName != "Анна К." {
		t.Errorf("AuthorName = %q", got.AuthorName)
	}
	if got.Raw != "" {
		t.Errorf("Raw = %q, want empty", got.Raw)
	}
	if got.SellerArticle != "ART-9" {
		t.Errorf("SellerArticle = %q, want ART-9", got.SellerArticle)
	}
	// Idempotent: second run scrubs nothing.
	n2, _ := s.ScrubPersonalData(testCtx(t))
	if n2 != 0 {
		t.Errorf("second run scrubbed = %d, want 0", n2)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestScrubPersonalData -v`
Expected: FAIL — `undefined: (*Store).ScrubPersonalData`

- [ ] **Step 3: Write minimal implementation**

Append to `internal/store/migrate_pd.go`:

```go
import (
	"context"
	// keep "encoding/json" from step above
)

// ScrubPersonalData removes stored personal data from existing rows:
// anonymizes AuthorName, backfills SellerArticle from Raw when missing, and
// clears Raw. Idempotent — rows already clean (Raw empty and name anonymized)
// are skipped, so it is safe to run on every startup.
func (s *Store) ScrubPersonalData(ctx context.Context) (int, error) {
	var rows []Review
	// Candidates: any row still holding Raw, OR a name not yet anonymized.
	if err := s.db.WithContext(ctx).
		Where("raw <> ''").
		Find(&rows).Error; err != nil {
		return 0, err
	}
	scrubbed := 0
	for _, row := range rows {
		updates := map[string]any{"raw": ""}
		anon := AnonymizeAuthorName(row.AuthorName)
		if anon != row.AuthorName {
			updates["author_name"] = anon
		}
		if row.SellerArticle == "" {
			if art := supplierArticleFromRaw(row.Raw); art != "" {
				updates["seller_article"] = art
			}
		}
		if err := s.db.WithContext(ctx).Model(&Review{}).
			Where("id = ?", row.ID).Updates(updates).Error; err != nil {
			return scrubbed, err
		}
		scrubbed++
	}
	return scrubbed, nil
}
```

> Note: rows with `raw = ''` but a still-full historical name (e.g. anonymized partially) are out of scope — once Task 1 ships, all *new* rows are clean, and all legacy rows carry a non-empty Raw, so the `raw <> ''` filter covers the real migration set. Confirm with `rg` that no other code writes `Raw` after Task 1.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestScrubPersonalData -v`
Expected: PASS

- [ ] **Step 5: Wire into startup**

Find the migration site (`rg -n "AutoMigrate" internal/store`). Right after `AutoMigrate(...)` returns nil, add (adapt receiver/var names to the actual function):

```go
	if n, err := s.ScrubPersonalData(context.Background()); err != nil {
		return nil, fmt.Errorf("scrub personal data: %w", err)
	} else if n > 0 {
		// no PD in the log line — count only
		// if a logger is available here, log "scrubbed personal data" n; otherwise omit
		_ = n
	}
```

If the constructor has no logger/ctx in scope, keep the call but drop the logging branch. Ensure `context` and `fmt` are imported.

- [ ] **Step 6: Run full store + build**

Run: `go build ./... && go test ./internal/store/ -v`
Expected: build OK, tests PASS

- [ ] **Step 7: Commit**

```bash
git add internal/store/migrate_pd.go internal/store/migrate_pd_test.go internal/store/*.go
git commit -m "feat(store): one-time PD scrub migration on startup"
```

---

### Task 4: Label seller replies vs marketplace answers

**Files:**
- Modify: `internal/reviewjson/reviewjson.go` (`Answer` struct lines 47-51; `ToReview` lines 64-69)
- Modify: `internal/reviewjson/reviewjson_test.go`

**Interfaces:**
- Produces: `Answer.Kind` JSON field with values `"seller"` | `"marketplace"` — consumed by Task 8 (widget).

- [ ] **Step 1: Write the failing test**

Append to `internal/reviewjson/reviewjson_test.go`:

```go
func TestAnswerKind(t *testing.T) {
	admin := "seller reply"
	sellerReview := store.Review{AdminReplyText: &admin}
	if a := (Mapper{}).ToReview(sellerReview).Answer; a == nil || a.Kind != "seller" {
		t.Fatalf("seller answer kind = %+v", a)
	}
	mpText := "mp reply"
	mpState := "published"
	mpReview := store.Review{MPAnswerText: &mpText, MPAnswerState: &mpState}
	if a := (Mapper{}).ToReview(mpReview).Answer; a == nil || a.Kind != "marketplace" {
		t.Fatalf("marketplace answer kind = %+v", a)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/reviewjson/ -run TestAnswerKind -v`
Expected: FAIL — `a.Kind undefined`

- [ ] **Step 3: Write minimal implementation**

In `internal/reviewjson/reviewjson.go`, extend the `Answer` struct (lines 47-51):

```go
type Answer struct {
	Text  string `json:"text"`
	State string `json:"state"`
	Kind  string `json:"kind"` // "seller" | "marketplace"
}
```

Update the answer block in `ToReview` (lines 64-69):

```go
	var answer *Answer
	if review.AdminReplyText != nil && strings.TrimSpace(*review.AdminReplyText) != "" {
		answer = &Answer{Text: *review.AdminReplyText, State: "published", Kind: "seller"}
	} else if review.MPAnswerText != nil || review.MPAnswerState != nil {
		answer = &Answer{Text: stringValue(review.MPAnswerText), State: stringValue(review.MPAnswerState), Kind: "marketplace"}
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/reviewjson/ -v`
Expected: PASS (existing answer test still green)

- [ ] **Step 5: Commit**

```bash
git add internal/reviewjson/reviewjson.go internal/reviewjson/reviewjson_test.go
git commit -m "feat(reviewjson): tag answers as seller or marketplace"
```

---

### Task 5: Yandex Market attribution link

**Files:**
- Modify: `internal/reviewjson/reviewjson.go` (`marketplaceProductURL` lines 114-124; add `marketplaceYM` const near line 16)
- Modify: `internal/reviewjson/reviewjson_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestMarketplaceProductURL_YM(t *testing.T) {
	r := store.Review{Marketplace: "ym", ExternalProductID: "12345"}
	got := (Mapper{}).ToReview(r).MarketplaceProductURL
	if got != "https://market.yandex.ru/product/12345" {
		t.Errorf("got %q", got)
	}
	empty := store.Review{Marketplace: "ym", ExternalProductID: ""}
	if got := (Mapper{}).ToReview(empty).MarketplaceProductURL; got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/reviewjson/ -run TestMarketplaceProductURL_YM -v`
Expected: FAIL — got `""`

- [ ] **Step 3: Write minimal implementation**

Add near line 16:

```go
const marketplaceYM = "ym"
```

Replace `marketplaceProductURL` (lines 114-124):

```go
func marketplaceProductURL(review store.Review) string {
	if review.ExternalProductID == "" {
		return ""
	}
	switch review.Marketplace {
	case marketplaceWB:
		return "https://www.wildberries.ru/catalog/" + urlPathEscape(review.ExternalProductID) + "/detail.aspx"
	case marketplaceYM:
		return "https://market.yandex.ru/product/" + urlPathEscape(review.ExternalProductID)
	default:
		return ""
	}
}
```

`marketplaceReviewURL` already returns the product URL for non-WB marketplaces (no `#comments`), so YM gets the product link automatically.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/reviewjson/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/reviewjson/reviewjson.go internal/reviewjson/reviewjson_test.go
git commit -m "feat(reviewjson): add Yandex Market product attribution link"
```

---

### Task 6: Media-proxy config + host allowlist

**Files:**
- Modify: `internal/config/config.go` (add fields + parsing)
- Create: `internal/mediaproxy/allowlist.go`
- Create: `internal/mediaproxy/allowlist_test.go`

**Interfaces:**
- Produces: `config.MediaConfig{ Allowlist []string; MaxBytes int64; CacheEntries int }` on `Config.Media`; `func HostAllowed(rawURL string, suffixes []string) bool` (package `mediaproxy`) — consumed by Task 7.

- [ ] **Step 1: Write the failing test**

```go
// internal/mediaproxy/allowlist_test.go
package mediaproxy

import "testing"

func TestHostAllowed(t *testing.T) {
	allow := []string{"wbbasket.ru", "images.wildberries.ru", "avatars.mds.yandex.net"}
	ok := []string{
		"https://basket-12.wbbasket.ru/vol1/part.jpg",
		"https://images.wildberries.ru/x.jpg",
		"https://avatars.mds.yandex.net/get-marketpic/1/x/orig",
	}
	for _, u := range ok {
		if !HostAllowed(u, allow) {
			t.Errorf("HostAllowed(%q) = false, want true", u)
		}
	}
	bad := []string{
		"https://evil.com/x.jpg",
		"http://wbbasket.ru.evil.com/x",          // suffix-spoof
		"https://localhost/x",                     // SSRF
		"ftp://wbbasket.ru/x",                     // wrong scheme
		"not a url",
	}
	for _, u := range bad {
		if HostAllowed(u, allow) {
			t.Errorf("HostAllowed(%q) = true, want false", u)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/mediaproxy/ -run TestHostAllowed -v`
Expected: FAIL — package/function missing

- [ ] **Step 3: Write minimal implementation**

```go
// internal/mediaproxy/allowlist.go
package mediaproxy

import (
	"net/url"
	"strings"
)

// HostAllowed reports whether rawURL is an https URL whose host equals or is a
// subdomain of one of the allowed suffixes. Guards the proxy against
// open-proxy/SSRF abuse.
func HostAllowed(rawURL string, suffixes []string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, s := range suffixes {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			continue
		}
		if host == s || strings.HasSuffix(host, "."+s) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/mediaproxy/ -run TestHostAllowed -v`
Expected: PASS

- [ ] **Step 5: Add config**

In `internal/config/config.go`, add to the `Config` struct (after `Marketplaces`):

```go
	Media MediaConfig
```

Add the type near the other config types:

```go
type MediaConfig struct {
	// Allowlist of CDN host suffixes the image proxy may fetch from.
	Allowlist []string
	// MaxBytes caps a single fetched image; larger responses are streamed
	// through unprocessed. Default 8 MiB.
	MaxBytes int64
	// CacheEntries bounds the in-memory blurred-image LRU. Default 512.
	CacheEntries int
}
```

In `LoadFromEnv`, after the `Marketplaces:` block closes (after line 121), add:

```go
	cfg.Media = MediaConfig{
		Allowlist:    envList("REVIEWS_MEDIA_ALLOWLIST"),
		MaxBytes:     8 << 20,
		CacheEntries: 512,
	}
	if len(cfg.Media.Allowlist) == 0 {
		cfg.Media.Allowlist = []string{"wbbasket.ru", "images.wildberries.ru", "avatars.mds.yandex.net"}
	}
```

- [ ] **Step 6: Build**

Run: `go build ./... && go test ./internal/mediaproxy/ ./internal/config/ -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/config/config.go internal/mediaproxy/allowlist.go internal/mediaproxy/allowlist_test.go
git commit -m "feat(mediaproxy): add CDN host allowlist and media config"
```

---

### Task 7: Face-blur image proxy handler

**Files:**
- Add dependency: `github.com/esimov/pigo`
- Vendor cascade: `internal/mediaproxy/cascade/facefinder` (binary from the pigo repo `cascade/facefinder`)
- Create: `internal/mediaproxy/blur.go`
- Create: `internal/mediaproxy/blur_test.go`
- Create: `internal/mediaproxy/proxy.go`
- Create: `internal/mediaproxy/proxy_test.go`

**Interfaces:**
- Consumes: `HostAllowed` (Task 6), `config.MediaConfig` (Task 6).
- Produces: `func NewHandler(cfg config.MediaConfig, fetch HTTPGetter) (http.Handler, error)` and the type `HTTPGetter func(url string) (*http.Response, error)` — consumed by Task 9 (server wiring). The handler serves `GET /media?u=<encoded https url>`.

- [ ] **Step 1: Add pigo + cascade**

```bash
go get github.com/esimov/pigo@latest
mkdir -p internal/mediaproxy/cascade
# Fetch the trained cascade shipped in the pigo module cache:
cp "$(go env GOMODCACHE)"/github.com/esimov/pigo@*/cascade/facefinder internal/mediaproxy/cascade/facefinder
ls -l internal/mediaproxy/cascade/facefinder   # expect a ~ 200KB binary
```

If the cascade is not under the module cache, download from the pigo GitHub repo `cascade/facefinder` (raw) into that path.

- [ ] **Step 2: Write the failing blur test**

```go
// internal/mediaproxy/blur_test.go
package mediaproxy

import (
	"image"
	"image/color"
	"testing"
)

func TestBlurRegionChangesPixels(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	for x := 0; x < 20; x++ {
		for y := 0; y < 20; y++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 12), G: 0, B: 0, A: 255})
		}
	}
	before := img.RGBAAt(5, 5)
	blurRegion(img, image.Rect(2, 2, 12, 12), 3)
	after := img.RGBAAt(5, 5)
	if before == after {
		t.Error("expected pixel inside region to change after blur")
	}
	// Pixel outside the region is untouched.
	if img.RGBAAt(18, 18) != (color.RGBA{R: 216, G: 0, B: 0, A: 255}) {
		t.Error("pixel outside region must be unchanged")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/mediaproxy/ -run TestBlurRegion -v`
Expected: FAIL — `undefined: blurRegion`

- [ ] **Step 4: Implement blur + detection**

```go
// internal/mediaproxy/blur.go
package mediaproxy

import (
	"bytes"
	_ "embed"
	"image"
	"image/draw"

	pigo "github.com/esimov/pigo/core"
)

//go:embed cascade/facefinder
var cascade []byte

type detector struct{ classifier *pigo.Pigo }

func newDetector() (*detector, error) {
	p := pigo.NewPigo()
	c, err := p.Unpack(cascade)
	if err != nil {
		return nil, err
	}
	return &detector{classifier: c}, nil
}

// detectFaces returns face bounding boxes in image coordinates.
func (d *detector) detectFaces(img *image.RGBA) []image.Rectangle {
	bounds := img.Bounds()
	cols, rows := bounds.Dx(), bounds.Dy()
	gray := pigo.RgbToGrayscale(img)
	params := pigo.CascadeParams{
		MinSize:     40,
		MaxSize:     1000,
		ShiftFactor: 0.1,
		ScaleFactor: 1.1,
		ImageParams: pigo.ImageParams{Pixels: gray, Rows: rows, Cols: cols, Dim: cols},
	}
	dets := d.classifier.RunCascade(params, 0.0)
	dets = d.classifier.ClusterDetections(dets, 0.2)
	var boxes []image.Rectangle
	for _, det := range dets {
		if det.Q < 5.0 { // confidence threshold
			continue
		}
		r := det.Scale / 2
		cx, cy := det.Col, det.Row
		box := image.Rect(cx-r, cy-r, cx+r, cy+r).Add(bounds.Min)
		boxes = append(boxes, box.Intersect(bounds))
	}
	return boxes
}

// blurRegion applies a simple box blur of the given radius inside rect only.
func blurRegion(img *image.RGBA, rect image.Rectangle, radius int) {
	rect = rect.Intersect(img.Bounds())
	if rect.Empty() || radius < 1 {
		return
	}
	src := image.NewRGBA(rect)
	draw.Draw(src, rect, img, rect.Min, draw.Src)
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			var sr, sg, sb, sa, n int
			for dy := -radius; dy <= radius; dy++ {
				for dx := -radius; dx <= radius; dx++ {
					px, py := x+dx, y+dy
					if !image.Pt(px, py).In(rect) {
						continue
					}
					c := src.RGBAAt(px, py)
					sr += int(c.R); sg += int(c.G); sb += int(c.B); sa += int(c.A)
					n++
				}
			}
			if n == 0 {
				continue
			}
			img.SetRGBA(x, y, colorFrom(sr/n, sg/n, sb/n, sa/n))
		}
	}
}
```

Add a tiny helper at the bottom of the file:

```go
import "image/color" // add to the import block

func colorFrom(r, g, b, a int) color.RGBA {
	return color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: uint8(a)}
}
```

- [ ] **Step 5: Run blur test to verify it passes**

Run: `go test ./internal/mediaproxy/ -run TestBlurRegion -v`
Expected: PASS

- [ ] **Step 6: Write the failing proxy handler test**

```go
// internal/mediaproxy/proxy_test.go
package mediaproxy

import (
	"bytes"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"reviews/internal/config"
)

func jpegBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func newTestHandler(t *testing.T, body []byte) http.Handler {
	t.Helper()
	cfg := config.MediaConfig{Allowlist: []string{"cdn.test"}, MaxBytes: 8 << 20, CacheEntries: 8}
	h, err := NewHandler(cfg, func(u string) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": {"image/jpeg"}},
			Body:       io.NopCloser(bytes.NewReader(body)),
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestProxyRejectsDisallowedHost(t *testing.T) {
	h := newTestHandler(t, jpegBytes(t))
	req := httptest.NewRequest("GET", "/media?u="+url.QueryEscape("https://evil.com/x.jpg"), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestProxyServesAllowedImage(t *testing.T) {
	h := newTestHandler(t, jpegBytes(t))
	req := httptest.NewRequest("GET", "/media?u="+url.QueryEscape("https://cdn.test/x.jpg"), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("missing no-referrer policy")
	}
	if _, _, err := image.Decode(rec.Body); err != nil {
		t.Errorf("response is not a decodable image: %v", err)
	}
}

func TestProxyMissingURL(t *testing.T) {
	h := newTestHandler(t, jpegBytes(t))
	req := httptest.NewRequest("GET", "/media", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}
```

Add `_ "image/jpeg"` import for `image.Decode` in the test if needed.

- [ ] **Step 7: Run to verify it fails**

Run: `go test ./internal/mediaproxy/ -run TestProxy -v`
Expected: FAIL — `undefined: NewHandler`

- [ ] **Step 8: Implement the handler**

```go
// internal/mediaproxy/proxy.go
package mediaproxy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"sync"

	"reviews/internal/config"
)

// HTTPGetter fetches a remote image. Injected for testing.
type HTTPGetter func(url string) (*http.Response, error)

type handler struct {
	cfg   config.MediaConfig
	fetch HTTPGetter
	det   *detector

	mu    sync.Mutex
	cache map[string][]byte
	order []string
}

func NewHandler(cfg config.MediaConfig, fetch HTTPGetter) (http.Handler, error) {
	det, err := newDetector()
	if err != nil {
		return nil, err
	}
	if fetch == nil {
		client := &http.Client{}
		fetch = func(u string) (*http.Response, error) { return client.Get(u) }
	}
	return &handler{cfg: cfg, fetch: fetch, det: det, cache: map[string][]byte{}}, nil
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("u")
	if raw == "" {
		http.Error(w, "missing u", http.StatusBadRequest)
		return
	}
	if !HostAllowed(raw, h.cfg.Allowlist) {
		http.Error(w, "host not allowed", http.StatusForbidden)
		return
	}

	key := hashKey(raw)
	if blob, ok := h.get(key); ok {
		writeImage(w, blob)
		return
	}

	resp, err := h.fetch(raw)
	if err != nil || resp.StatusCode != 200 {
		http.Error(w, "upstream error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, h.cfg.MaxBytes))
	if err != nil {
		http.Error(w, "read error", http.StatusBadGateway)
		return
	}

	out := h.process(body)
	h.put(key, out)
	writeImage(w, out)
}

// process decodes, blurs detected faces, re-encodes as JPEG. On any decode
// failure it returns the original bytes unchanged (image still renders).
func (h *handler) process(body []byte) []byte {
	src, _, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		return body
	}
	rgba := toRGBA(src)
	for _, box := range h.det.detectFaces(rgba) {
		// pad the box ~20% and blur generously
		pad := box.Dx() / 5
		region := image.Rect(box.Min.X-pad, box.Min.Y-pad, box.Max.X+pad, box.Max.Y+pad)
		blurRegion(rgba, region, 8)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, rgba, &jpeg.Options{Quality: 82}); err != nil {
		return body
	}
	return buf.Bytes()
}

func toRGBA(src image.Image) *image.RGBA {
	if r, ok := src.(*image.RGBA); ok {
		return r
	}
	b := src.Bounds()
	dst := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dst.Set(x, y, src.At(x, y))
		}
	}
	return dst
}

func writeImage(w http.ResponseWriter, blob []byte) {
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(blob)
}

func hashKey(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func (h *handler) get(key string) ([]byte, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	b, ok := h.cache[key]
	return b, ok
}

func (h *handler) put(key string, blob []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, exists := h.cache[key]; !exists {
		h.order = append(h.order, key)
	}
	h.cache[key] = blob
	for len(h.order) > h.cfg.CacheEntries && h.cfg.CacheEntries > 0 {
		oldest := h.order[0]
		h.order = h.order[1:]
		delete(h.cache, oldest)
	}
}
```

Add the needed image decoders by importing `_ "image/png"` and `_ "image/gif"` (jpeg already imported) at the top of `proxy.go` so `image.Decode` handles common formats. Note WB/YM serve `.webp` — add `golang.org/x/image/webp` decoder: `go get golang.org/x/image/webp` and import `_ "golang.org/x/image/webp"`. If a format is undecodable, `process` returns the original bytes (graceful).

- [ ] **Step 9: Run all mediaproxy tests**

Run: `go test ./internal/mediaproxy/ -v`
Expected: PASS (allowlist, blur, proxy)

- [ ] **Step 10: Commit**

```bash
git add internal/mediaproxy/ go.mod go.sum
git commit -m "feat(mediaproxy): face-blurring image proxy with in-memory cache"
```

---

### Task 8: Wire /media route into the server

**Files:**
- Modify: `internal/server/server.go` (`handler`, lines 83-93)
- Create/Modify: `internal/server/media_test.go`
- Confirm the `Server` has access to `s.cfg.Media` (added in Task 6).

- [ ] **Step 1: Write the failing test**

```go
// internal/server/media_test.go
package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMediaRouteRejectsDisallowedHost(t *testing.T) {
	srv := newTestServer(t) // use the existing server test constructor
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/media?u=https%3A%2F%2Fevil.com%2Fx.jpg", nil)
	srv.handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}
```

Confirm the server test helper name with `rg -n "func newTestServer|func testServer|func setupServer" internal/server` and adapt.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/server/ -run TestMediaRoute -v`
Expected: FAIL — route 404/unexpected status

- [ ] **Step 3: Wire the route**

In `internal/server/server.go` `handler()`, build the media handler once and register it. After line 88 (`/healthz`) add:

```go
	if mediaHandler, err := mediaproxy.NewHandler(s.cfg.Media, nil); err == nil {
		mux.Handle("GET /media", mediaHandler)
	} else {
		s.logger.Error("media proxy disabled", "error", err)
	}
```

Add the import `"reviews/internal/mediaproxy"`. The existing `securityHeaders`/`cors` wrappers still apply; the proxy's own `Referrer-Policy: no-referrer` header overrides the default `same-origin` for image responses.

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/server/ -run TestMediaRoute -v && go build ./...`
Expected: PASS + build OK

- [ ] **Step 5: Commit**

```bash
git add internal/server/server.go internal/server/media_test.go
git commit -m "feat(server): expose /media face-blur proxy route"
```

---

### Task 9: Widget — route images through proxy + label seller reply

**Files:**
- Modify: `web/reviews-widget/reviews-widget.js` (`mediaProxyURL` helper; media render lines 544-548 and 657-662; viewer src ~757; `renderAnswer` lines 792-802; normalize source capture lines 154-214)
- Modify: `web/reviews-widget/test/loader.test.html` (or add `web/reviews-widget/test/mediaproxy.test.html`) for the pure-function assertion

**Interfaces:**
- Consumes: `Answer.kind` (Task 4), `/media?u=` route (Task 8).

- [ ] **Step 1: Add a pure proxy-URL helper with a browser assertion**

In `reviews-widget.js`, near `initials()` (line 814), add:

```javascript
  function mediaProxyURL(rawUrl, proxyBase) {
    if (!rawUrl || !proxyBase) {
      return rawUrl || "";
    }
    return proxyBase.replace(/\/$/, "") + "/media?u=" + encodeURIComponent(rawUrl);
  }
```

Expose it for testing at the bottom of the IIFE where `window.ReviewsWidget` is assigned — add `mediaProxyURL` to that exported object.

In `web/reviews-widget/test/loader.test.html`, after the existing assertions, add:

```html
<script>
  eq("proxy wraps url",
     ReviewsWidget.mediaProxyURL("https://cdn.test/a.jpg", "https://rev.test"),
     "https://rev.test/media?u=https%3A%2F%2Fcdn.test%2Fa.jpg");
  eq("proxy passthrough when no base",
     ReviewsWidget.mediaProxyURL("https://cdn.test/a.jpg", ""),
     "https://cdn.test/a.jpg");
</script>
```

(If `ReviewsWidget` is not loaded in `loader.test.html`, add `<script src="../reviews-widget.js"></script>` before the assertions.)

- [ ] **Step 2: Capture the proxy base at mount**

In `mount()` (around line 154-214) where `state` is built, derive the proxy base from the data source origin, falling back to the script origin:

```javascript
    let proxyBase = "";
    try {
      if (options.source) {
        proxyBase = new URL(options.source, window.location.href).origin;
      }
    } catch (e) {
      proxyBase = "";
    }
    state.proxyBase = proxyBase;
```

Pass `state.proxyBase` into the render functions. The simplest path: stash it on the root element alongside the existing config — `root.__reviewsProxyBase = proxyBase;` right after `root.__reviewsWidgetConfig` is set (search for that assignment).

- [ ] **Step 3: Use the proxy in the three image render points**

Media strip (line 544), replace the `src` line:

```javascript
            const rawSrc = item.kind === "video" ? item.previewUrl || "./assets/review-video.svg" : item.url;
            const src = item.kind === "video" ? rawSrc : mediaProxyURL(rawSrc, root.__reviewsProxyBase);
```

(The media strip render needs `root` in scope — it is called from `renderStrip(root, ...)`; confirm and thread `root` if necessary.)

Card media (line 658), in `renderMedia(media, config)` change the signature to `renderMedia(media, config, proxyBase)` and the call site (line 586) to `renderMedia(review.media, root.__reviewsWidgetConfig, root.__reviewsProxyBase)`, then:

```javascript
            const rawSrc = item.kind === "video" ? item.previewUrl || "./assets/review-video.svg" : item.url;
            const src = item.kind === "video" ? rawSrc : mediaProxyURL(rawSrc, proxyBase);
```

Media viewer (~line 757): where the viewer `<img>` src is set, wrap photo URLs:

```javascript
    const viewerSrc = mediaProxyURL(item.url, root.__reviewsProxyBase);
```

(Confirm the viewer has `root` in scope via `openMediaViewer(root, ...)`.)

Leave the anchor `href` pointing at the **original** `item.url` (so "open original" still works) — only the rendered `<img src>` is proxied.

- [ ] **Step 4: Label the seller reply**

Replace `renderAnswer` (lines 792-802):

```javascript
  function renderAnswer(answer, config) {
    if (!config.visibility.sellerAnswers || !answer || !answer.text) {
      return "";
    }
    const title = answer.kind === "seller" ? "Ответ продавца" : "Ответ магазина";
    return `
      <div class="rw-answer" data-answer-kind="${escapeHTML(answer.kind || "")}">
        <div class="rw-answer-title">${title}</div>
        <p>${escapeHTML(answer.text)}</p>
      </div>
    `;
  }
```

- [ ] **Step 5: Verify in the browser**

Open `web/reviews-widget/test/loader.test.html` in a browser — the two new `eq()` assertions must show **PASS** and the totals must report 0 failures.

Open `web/reviews-widget/test/fixture-render.html` — confirm the widget renders. Temporarily add a review with `media: [{kind:"photo", url:"https://cdn.test/x.jpg"}]` and a `source` pointing at an origin, and confirm the rendered `<img>` `src` is rewritten to `/media?u=...` (inspect element). Revert the fixture edit after checking.

- [ ] **Step 6: Commit**

```bash
git add web/reviews-widget/reviews-widget.js web/reviews-widget/test/loader.test.html
git commit -m "feat(widget): proxy review images for face blur and label seller replies"
```

---

### Task 10: Hard-delete for data-subject erasure

**Files:**
- Modify: `internal/store/curation.go` (add `HardDeleteReview`)
- Create/Modify: `internal/store/curation_test.go`
- Modify: `internal/server/admin_reviews.go` (add `handleAdminReviewPurge`)
- Modify: `internal/server/server.go` adminMux (register route after line 139)

**Interfaces:**
- Produces: `func (s *Store) HardDeleteReview(ctx context.Context, id uint) error` (removes the review row and its `ReviewMedia`); admin route `DELETE /admin/api/reviews/{id}/purge`.

- [ ] **Step 1: Write the failing store test**

```go
func TestHardDeleteReview(t *testing.T) {
	s := newTestStore(t)
	res, _ := s.UpsertReview(testCtx(t), marketplace.Review{
		Marketplace: "wb", ExternalReviewID: "r1", AuthorName: "Анна Котова",
		Media: []marketplace.Media{{Kind: "photo", URL: "https://cdn.test/x.jpg"}},
	})
	if err := s.HardDeleteReview(testCtx(t), res.Review.ID); err != nil {
		t.Fatal(err)
	}
	var rc, mc int64
	s.db.Model(&Review{}).Where("id = ?", res.Review.ID).Count(&rc)
	s.db.Model(&ReviewMedia{}).Where("review_id = ?", res.Review.ID).Count(&mc)
	if rc != 0 || mc != 0 {
		t.Errorf("rows left: review=%d media=%d, want 0/0", rc, mc)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/store/ -run TestHardDeleteReview -v`
Expected: FAIL — `undefined: (*Store).HardDeleteReview`

- [ ] **Step 3: Implement store method**

Append to `internal/store/curation.go`:

```go
// HardDeleteReview permanently removes a review and its media. Used to fulfill
// a data-subject erasure request — distinct from SoftDeleteReview.
func (s *Store) HardDeleteReview(ctx context.Context, id uint) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("review_id = ?", id).Delete(&ReviewMedia{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&Review{}).Error
	})
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/store/ -run TestHardDeleteReview -v`
Expected: PASS

- [ ] **Step 5: Add the admin handler + route**

In `internal/server/admin_reviews.go`, after `handleAdminReviewDelete` (line 163):

```go
func (s *Server) handleAdminReviewPurge(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid review id"))
		return
	}
	if err := s.store.HardDeleteReview(r.Context(), uint(id)); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
```

In `internal/server/server.go` adminMux, after line 139:

```go
	protected.Handle("DELETE /admin/api/reviews/{id}/purge", requireCSRF(http.HandlerFunc(s.handleAdminReviewPurge)))
```

- [ ] **Step 6: Build + test**

Run: `go build ./... && go test ./internal/store/ ./internal/server/ -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/store/curation.go internal/store/curation_test.go internal/server/admin_reviews.go internal/server/server.go
git commit -m "feat: hard-delete endpoint for data-subject erasure requests"
```

---

### Task 11: Privacy-policy template + deletion contact config

**Files:**
- Create: `docs/legal/privacy-policy-template.ru.md`
- Modify: `internal/config/config.go` (add `PrivacyContact` to `WebConfig`, parse `REVIEWS_PRIVACY_CONTACT`)
- Modify: `.env.example` (document the new var)

No automated test (static content + a parsed env string); verification is build + presence.

- [ ] **Step 1: Add the config field**

In `WebConfig` (after `SitemapURL`):

```go
	// PrivacyContact is the contact shown for data-subject requests (152-ФЗ).
	PrivacyContact string
```

In `LoadFromEnv` `Web:` block, add:

```go
			PrivacyContact:     envString("REVIEWS_PRIVACY_CONTACT", ""),
```

- [ ] **Step 2: Write the template**

Create `docs/legal/privacy-policy-template.ru.md`:

```markdown
# Политика обработки персональных данных (шаблон)

> Замените плейсхолдеры `{{...}}` и разместите итог на странице сайта,
> ссылку на которую укажите рядом с виджетом отзывов.

Оператор: **{{OPERATOR_NAME}}**, сайт **{{DOMAIN}}**, контакт для запросов: **{{CONTACT_EMAIL}}**.

## Какие данные обрабатываются
Виджет отображает отзывы о товарах, опубликованные покупателями на маркетплейсах
(Wildberries, Яндекс Маркет) и полученные по официальным API этих площадок.

Отображаются: текст отзыва, достоинства/недостатки, оценка, дата, обезличенное имя
автора (имя и первая буква фамилии, например «Анна К.») и приложенные фотографии.

## Что НЕ хранится и минимизация данных
- Полное имя автора **не сохраняется** — обезличивается при получении.
- Медиафайлы **не сохраняются** — изображения транслируются напрямую и проходят
  через прокси, **автоматически размывающий лица** на фотографиях.
- Служебный «сырой» ответ маркетплейса с дополнительными идентификаторами
  **не сохраняется**.

## Источник, цель и основание
Источник — маркетплейсы по официальным API. Цель — информирование покупателей о
товарах. Основание — законный интерес и публичный характер отзывов на площадке.

## Срок и права субъекта
Данные хранятся, пока отзыв актуален. Субъект вправе запросить доступ к данным или
их удаление, написав на **{{CONTACT_EMAIL}}**. Запрос исполняется удалением отзыва
из публичной выдачи и, при необходимости, безвозвратным удалением записи.
```

- [ ] **Step 3: Document the env var**

Add to `.env.example`:

```bash
# Contact shown in the privacy policy for data-subject (152-ФЗ) requests.
REVIEWS_PRIVACY_CONTACT=
```

- [ ] **Step 4: Build**

Run: `go build ./... && go test ./internal/config/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add docs/legal/privacy-policy-template.ru.md internal/config/config.go .env.example
git commit -m "docs(legal): privacy policy template and deletion-contact config"
```

---

### Task 12: Installer consent + README legal checklist + hide warning

**Files:**
- Modify: `internal/installer/` — find the wizard step sequence with `rg -n "func.*[Ss]tep|Update\b|tea.Model|case .*KeyEnter" internal/installer` and insert a consent gate before the final/confirm step
- Modify: `README.md`
- Modify: admin UI hide control — find with `rg -n "hidden|visibility|Скрыть|hide" internal/server/admin_*.go web/` and add the warning copy where the hide action is presented

- [ ] **Step 1: Add installer consent gate**

In the installer wizard, add a step that shows the consent text and requires explicit acceptance (e.g. typing `y`/pressing a confirm key) before proceeding:

```
Подтвердите перед установкой:
[ ] Я имею право переопубликовывать отзывы со своих карточек товаров.
[ ] Я ознакомлен(а) с обязанностями оператора персональных данных (152-ФЗ)
    и размещу политику конфиденциальности (шаблон: docs/legal/privacy-policy-template.ru.md).

Нажмите Y для подтверждения, Q для выхода.
```

Wire it so that without confirmation the wizard does not write config / does not proceed. Follow the existing bubbletea model/update pattern already used by the wizard (mirror an existing yes/no step).

- [ ] **Step 2: Verify installer still builds and runs**

Run: `go build ./... && go test ./internal/installer/ -v`
Expected: PASS (existing installer tests green; if a render snapshot test exists, update it to include the new step)

- [ ] **Step 3: Add README legal checklist**

Add a `## Юридический чек-лист` section to `README.md`:

```markdown
## Юридический чек-лист

Перед публикацией виджета убедитесь, что вы:

- [ ] Имеете право переопубликовывать отзывы со своих карточек (проверьте условия
      API соответствующего маркетплейса).
- [ ] Разместили политику конфиденциальности — шаблон в
      [docs/legal/privacy-policy-template.ru.md](docs/legal/privacy-policy-template.ru.md),
      и указали контакт в `REVIEWS_PRIVACY_CONTACT`.
- [ ] Понимаете, что скрытие отзывов предназначено только для спама/дублей.
      Сокрытие негатива ради рейтинга — риск по ЗоЗПП и закону «О рекламе» (ФАС).

Сервис минимизирует персональные данные автоматически: имя обезличивается
(«Анна К.»), лица на фото размываются в виджете, «сырой» ответ маркетплейса и
медиафайлы не хранятся.
```

- [ ] **Step 4: Add the admin hide warning**

Where the hide/visibility control is rendered in the admin UI, add inline helper copy:

```
Скрытие — только для спама, дублей и мусора. Не используйте его для сокрытия
негативных отзывов: это риск по ЗоЗПП и закону «О рекламе».
```

Place it near the visibility toggle (admin SPA template or the relevant handler-rendered text). If the admin UI is a separate SPA bundle, add it to that component; locate via the `rg` search above.

- [ ] **Step 5: Build + full test**

Run: `go build ./... && go test ./...`
Expected: build OK, all tests PASS

- [ ] **Step 6: Commit**

```bash
git add internal/installer/ README.md internal/server/ web/
git commit -m "docs+ux: installer consent gate, legal checklist, hide-warning copy"
```

---

### Task 13: Final verification

- [ ] **Step 1: Full build + test**

Run: `go build ./... && go test ./...`
Expected: all green

- [ ] **Step 2: Manual smoke of the proxy**

```bash
go run . serve &   # or the project's run command — confirm with rg -n "func main|serve" cmd *.go
curl -s -o /dev/null -w "%{http_code}\n" "http://localhost:8080/media?u=https%3A%2F%2Fevil.com%2Fx.jpg"   # expect 403
```

(Use the project's actual run command and port; confirm via README "Quick Start".)

- [ ] **Step 3: Verify no PD remains after migration**

With a dev DB that had legacy rows, start the server once, then:

```bash
# confirm no non-empty Raw and no multi-word full names remain
# (use the project's sqlite path from REVIEWS_DB_DSN)
echo "SELECT count(*) FROM reviews WHERE raw <> '';" | sqlite3 ./reviews.db   # expect 0
```

- [ ] **Step 4: Widget browser check**

Open `web/reviews-widget/test/loader.test.html` and `fixture-render.html` — assertions PASS, widget renders, image `src` rewritten to `/media?u=`.

- [ ] **Step 5: Final commit (if any docs/cleanup pending)**

```bash
git add -A && git commit -m "chore: legal mitigation final verification"
```

---

## Self-Review

**Spec coverage check:**
- A1 face blur → Tasks 6, 7, 8, 9 ✓
- A2 name anonymization (at ingestion) → Task 1 ✓; static export covered transitively (reads DB) ✓
- A3 deletion channel + hard-delete → Tasks 10, 11 ✓
- A4 privacy template → Task 11 ✓
- B1 hide warning + docs → Task 12 ✓
- B3 seller-reply labeling → Tasks 4 (JSON) + 9 (widget) ✓
- C1 YM attribution → Task 5 ✓
- C2 installer consent + README checklist → Task 12 ✓
- C3 referrer policy on media → Task 7 (`Referrer-Policy: no-referrer`) ✓
- Remove Raw storage + migration → Tasks 1, 2, 3 ✓

**Type consistency:** `AnonymizeAuthorName` (exported, used in Tasks 1+3); `supplierArticleFromRaw` (unexported, Tasks 2+3); `Answer.Kind` (Task 4 → consumed Task 9 as `answer.kind`); `HostAllowed` (Task 6 → Task 7); `NewHandler`/`HTTPGetter` (Task 7 → Task 8); `mediaProxyURL` (Task 9, matches `/media?u=` from Task 8); `HardDeleteReview` (Task 10). Consistent.

**Placeholder scan:** Helper/constructor names that vary by codebase (`newTestStore`, `testCtx`, `newTestServer`, installer wizard step, admin hide control, run command) are explicitly flagged with an `rg` command to resolve the real name at implementation time — these are lookups, not unresolved design gaps.
