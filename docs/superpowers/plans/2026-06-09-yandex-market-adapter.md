# Yandex Market Reviews Adapter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Yandex Market marketplace adapter so the embedded `shegida.ru` widget shows reviews from both Wildberries and Yandex Market.

**Architecture:** A new `internal/marketplace/ym` package implements the existing `marketplace.Adapter` interface, mirroring `internal/marketplace/wb`. `FetchReviews` POSTs to `/v2/businesses/{businessId}/goods-feedback`, paginates via `page_token`/`nextPageToken` (mapped to the collector's opaque cursor), maps `GoodsFeedbackDTO` to `marketplace.Review`, and drops reviews older than `since`. Seller answers and outbound YM links are out of scope for v1. One line in `cmd/reviews/main.go` `buildAdapters` wires it on. Config and credential validation already exist.

**Tech Stack:** Go (stdlib `net/http`, `encoding/json`), Yandex Market Partner API v2.

**Spec:** [docs/superpowers/specs/2026-06-09-yandex-market-adapter-design.md](../specs/2026-06-09-yandex-market-adapter-design.md)

---

## Reference: existing code this plan reuses

- `internal/marketplace/model.go` — `Adapter` interface (`Marketplace() string`, `FetchReviews(ctx, since time.Time, cursor string) ([]Review, string, error)`), and `Review` / `Media` / `Answer` structs. The adapter produces `marketplace.Review` values only — outbound URLs and article normalization happen downstream in `reviewjson`, **not** in the adapter.
- `internal/marketplace/wb/client.go` — the WB adapter this plan mirrors. Note it sets `SellerArticle: f.ProductDetails.SupplierArticle` **raw** (no normalization at adapter level).
- `internal/marketplace/wb/client_test.go` — the test pattern this plan mirrors: a `roundTripFunc` `http.RoundTripper` stub, inline JSON response, assertions on the mapped `Review`.
- `internal/config/config.go` — `YMConfig{ Enabled, APIKey, OAuthToken, BusinessID, CampaignID }` (already defined), `MarketplaceYM = "ym"`, `ValidateMarketplaceCredentials` already requires `(APIKey or OAuthToken)` + `BusinessID` when YM is enabled.
- `internal/collector/collector.go` — drives the adapter: calls `FetchReviews(ctx, since, cursor)` in a loop, advancing `cursor` to the returned next-cursor until it is `""`. Upserts are idempotent by `(marketplace, external_review_id)`. `since` is a lower bound the adapter must honor.
- `cmd/reviews/main.go:284` — `buildAdapters(cfg)`; currently only appends `wb.New(...)`.

Run the package tests with: `go test ./internal/marketplace/ym/`
Run the full suite with: `go test ./...` (baseline: `config`, `wb`, `store`, `reviewjson` pass).

---

## Task 1: Confirm API access and capture a real fixture

De-risk the two unknowns before writing code: (a) is reading reviews gated behind a paid subscription like Ozon, and (b) the exact JSON field names. This task makes one real API call with the seller's key.

**Files:**
- Ensure `.env` contains `YM_API_KEY` and `YM_BUSINESS_ID` (gitignored — do not commit).
- Create: `internal/marketplace/ym/testdata/goods-feedback-sample.json` (sanitized capture, committed as a reference fixture).

- [ ] **Step 1: Confirm credentials are present**

Run:
```bash
grep -E 'YM_API_KEY|YM_BUSINESS_ID|REVIEWS_YM_' .env
```
Expected: both an API key and a business id are set. If absent, add them to `.env` (the seller confirmed a key exists) before continuing.

- [ ] **Step 2: Make one live call**

Run (loads the two vars from `.env` without exporting the whole file):
```bash
YM_API_KEY=$(grep -E '^(REVIEWS_YM_API_KEY|YM_API_KEY)=' .env | head -1 | cut -d= -f2-)
YM_BUSINESS_ID=$(grep -E '^(REVIEWS_YM_BUSINESS_ID|YM_BUSINESS_ID)=' .env | head -1 | cut -d= -f2-)
mkdir -p internal/marketplace/ym/testdata
curl -sS -w '\nHTTP %{http_code}\n' -X POST \
  "https://api.partner.market.yandex.ru/v2/businesses/${YM_BUSINESS_ID}/goods-feedback?limit=5" \
  -H "Api-Key: ${YM_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{}' | tee internal/marketplace/ym/testdata/goods-feedback-sample.json
```

Interpret the result:
- **HTTP 200 with a `feedbacks` array** → access works; proceed.
- **HTTP 403 / 420 / a body mentioning a subscription or access right** → reading reviews is gated (like Ozon). **STOP** and report to the user; the rest of the plan is blocked until access is granted.
- **HTTP 200 with an empty `feedbacks` array** → access works but no reviews yet; proceed (mapping is still implementable and testable via the inline fixtures in later tasks).

- [ ] **Step 3: Reconcile field names with Task 2's DTOs**

Open the captured `goods-feedback-sample.json`. Compare its actual field names against the `json` tags used in Task 2 (`feedbackId`, `createdAt`, `author`, `identifiers.offerId`, `identifiers.marketSku`, `identifiers.modelId`, `description.comment`/`advantages`/`disadvantages`, `media.photos`/`videos`, `statistics.rating`, `result.paging.nextPageToken`). **If any real field name differs, update the `json` tags in Task 2's `client.go` and the inline JSON in Task 2/Task 3's tests to match the real payload.** Note the actual `offerId` values — they feed the Task 5 reconciliation gate.

- [ ] **Step 4: Commit the fixture**

```bash
git add internal/marketplace/ym/testdata/goods-feedback-sample.json
git commit -m "test: capture real Yandex Market goods-feedback fixture"
```

---

## Task 2: YM client — single-page fetch and mapping

Create the adapter with DTOs and the `GoodsFeedbackDTO` → `marketplace.Review` mapping, driven by a one-page test.

**Files:**
- Create: `internal/marketplace/ym/client.go`
- Create: `internal/marketplace/ym/client_test.go`

- [ ] **Step 1: Write the failing test**

`internal/marketplace/ym/client_test.go`:

```go
package ym

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"reviews/internal/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func jsonResponse(status int, payload any) *http.Response {
	var body strings.Builder
	_ = json.NewEncoder(&body).Encode(payload)
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body.String())),
	}
}

func TestFetchReviewsMapsYMResponse(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.Method; got != http.MethodPost {
			t.Fatalf("method = %q", got)
		}
		if got := r.Header.Get("Api-Key"); got != "key" {
			t.Fatalf("api-key header = %q", got)
		}
		if got := r.URL.Path; got != "/v2/businesses/777/goods-feedback" {
			t.Fatalf("path = %q", got)
		}
		if got := r.URL.Query().Get("limit"); got != "50" {
			t.Fatalf("limit = %q", got)
		}
		if got := r.URL.Query().Get("page_token"); got != "" {
			t.Fatalf("page_token should be empty on first page, got %q", got)
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"status": "OK",
			"result": map[string]any{
				"feedbacks": []map[string]any{
					{
						"feedbackId":   1001,
						"createdAt":    "2026-05-28T12:20:00Z",
						"needReaction": true,
						"author":       "Мария",
						"identifiers": map[string]any{
							"offerId":   "1523",
							"shopSku":   "1523",
							"marketSku": 70476012,
							"modelId":   12345,
						},
						"description": map[string]any{
							"advantages":    "Крой",
							"disadvantages": "Нет",
							"comment":       "Отличная ткань",
						},
						"media": map[string]any{
							"photos": []string{"https://cdn.test/p1.jpg"},
							"videos": []string{"https://cdn.test/v1.mp4"},
						},
						"statistics": map[string]any{
							"rating": 5,
						},
					},
				},
				"paging": map[string]any{"nextPageToken": ""},
			},
		}), nil
	})}

	client := NewWithHTTPClient(
		config.YMConfig{APIKey: "key", BusinessID: "777"},
		"https://api.partner.test", httpClient, 50,
	)
	reviews, nextCursor, err := client.FetchReviews(context.Background(), time.Now().Add(-30*24*time.Hour), "")
	if err != nil {
		t.Fatalf("fetch reviews: %v", err)
	}
	if nextCursor != "" {
		t.Fatalf("next cursor = %q", nextCursor)
	}
	if len(reviews) != 1 {
		t.Fatalf("reviews len = %d", len(reviews))
	}

	review := reviews[0]
	if review.Marketplace != "ym" {
		t.Fatalf("marketplace = %q", review.Marketplace)
	}
	if review.ExternalReviewID != "1001" {
		t.Fatalf("external review id = %q", review.ExternalReviewID)
	}
	if review.ExternalProductID != "70476012" {
		t.Fatalf("external product id = %q", review.ExternalProductID)
	}
	if review.SellerArticle != "1523" {
		t.Fatalf("seller article = %q", review.SellerArticle)
	}
	if review.Rating == nil || *review.Rating != 5 {
		t.Fatalf("rating = %v", review.Rating)
	}
	if review.Text != "Отличная ткань" || review.Pros != "Крой" || review.Cons != "Нет" {
		t.Fatalf("text/pros/cons = %q / %q / %q", review.Text, review.Pros, review.Cons)
	}
	if review.AuthorName != "Мария" {
		t.Fatalf("author = %q", review.AuthorName)
	}
	if !review.CreatedAtMP.Equal(time.Date(2026, 5, 28, 12, 20, 0, 0, time.UTC)) {
		t.Fatalf("createdAt = %v", review.CreatedAtMP)
	}
	if review.Answer != nil {
		t.Fatalf("answer should be nil in v1, got %+v", review.Answer)
	}
	if len(review.Media) != 2 {
		t.Fatalf("media len = %d", len(review.Media))
	}
	if review.Media[0].Kind != "photo" || review.Media[0].URL != "https://cdn.test/p1.jpg" {
		t.Fatalf("photo media = %+v", review.Media[0])
	}
	if review.Media[1].Kind != "video" || review.Media[1].URL != "https://cdn.test/v1.mp4" {
		t.Fatalf("video media = %+v", review.Media[1])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/marketplace/ym/`
Expected: FAIL — package does not compile (`NewWithHTTPClient` and types undefined).

- [ ] **Step 3: Write the implementation**

`internal/marketplace/ym/client.go`:

```go
package ym

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"reviews/internal/config"
	"reviews/internal/marketplace"
)

const (
	marketplaceID  = config.MarketplaceYM
	defaultBaseURL = "https://api.partner.market.yandex.ru"
	defaultPageSize = 50
)

type Client struct {
	baseURL    string
	apiKey     string
	businessID string
	httpClient *http.Client
	pageSize   int
}

func New(cfg config.YMConfig) *Client {
	return &Client{
		baseURL:    defaultBaseURL,
		apiKey:     cfg.APIKey,
		businessID: cfg.BusinessID,
		httpClient: http.DefaultClient,
		pageSize:   defaultPageSize,
	}
}

func NewWithHTTPClient(cfg config.YMConfig, baseURL string, httpClient *http.Client, pageSize int) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     cfg.APIKey,
		businessID: cfg.BusinessID,
		httpClient: httpClient,
		pageSize:   pageSize,
	}
}

func (c *Client) Marketplace() string {
	return marketplaceID
}

func (c *Client) FetchReviews(ctx context.Context, since time.Time, cursor string) ([]marketplace.Review, string, error) {
	feedbacks, nextToken, err := c.fetchPage(ctx, cursor)
	if err != nil {
		return nil, "", err
	}

	reviews := make([]marketplace.Review, 0, len(feedbacks))
	for _, feedback := range feedbacks {
		review, err := feedback.toReview()
		if err != nil {
			return nil, "", err
		}
		if !since.IsZero() && review.CreatedAtMP.Before(since) {
			continue
		}
		reviews = append(reviews, review)
	}

	return reviews, nextToken, nil
}

func (c *Client) fetchPage(ctx context.Context, cursor string) ([]goodsFeedback, string, error) {
	endpoint, err := url.Parse(fmt.Sprintf("%s/v2/businesses/%s/goods-feedback", c.baseURL, c.businessID))
	if err != nil {
		return nil, "", err
	}
	query := endpoint.Query()
	query.Set("limit", strconv.Itoa(c.pageSize))
	if cursor != "" {
		query.Set("page_token", cursor)
	}
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), strings.NewReader("{}"))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Api-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	var payload ymFeedbacksResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, "", fmt.Errorf("decode YM goods-feedback response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if len(payload.Errors) > 0 {
			return nil, "", fmt.Errorf("YM goods-feedback: status %d: %s", resp.StatusCode, payload.Errors[0].Message)
		}
		return nil, "", fmt.Errorf("YM goods-feedback: status %d", resp.StatusCode)
	}

	return payload.Result.Feedbacks, payload.Result.Paging.NextPageToken, nil
}

type ymFeedbacksResponse struct {
	Status string                `json:"status"`
	Result ymFeedbacksResult     `json:"result"`
	Errors []ymError             `json:"errors"`
}

type ymError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ymFeedbacksResult struct {
	Feedbacks []goodsFeedback `json:"feedbacks"`
	Paging    ymPaging        `json:"paging"`
}

type ymPaging struct {
	NextPageToken string `json:"nextPageToken"`
}

type goodsFeedback struct {
	FeedbackID   int64                    `json:"feedbackId"`
	CreatedAt    string                   `json:"createdAt"`
	NeedReaction bool                     `json:"needReaction"`
	Author       string                   `json:"author"`
	Identifiers  goodsFeedbackIdentifiers `json:"identifiers"`
	Description  goodsFeedbackDescription `json:"description"`
	Media        goodsFeedbackMedia       `json:"media"`
	Statistics   goodsFeedbackStatistics  `json:"statistics"`
}

type goodsFeedbackIdentifiers struct {
	OfferID   string `json:"offerId"`
	ShopSku   string `json:"shopSku"`
	MarketSku int64  `json:"marketSku"`
	ModelID   int64  `json:"modelId"`
}

type goodsFeedbackDescription struct {
	Advantages    string `json:"advantages"`
	Disadvantages string `json:"disadvantages"`
	Comment       string `json:"comment"`
}

type goodsFeedbackMedia struct {
	Photos []string `json:"photos"`
	Videos []string `json:"videos"`
}

type goodsFeedbackStatistics struct {
	Rating int `json:"rating"`
}

func (f goodsFeedback) toReview() (marketplace.Review, error) {
	createdAt, err := time.Parse(time.RFC3339Nano, f.CreatedAt)
	if err != nil {
		return marketplace.Review{}, fmt.Errorf("parse YM createdAt for %d: %w", f.FeedbackID, err)
	}

	raw, err := json.Marshal(f)
	if err != nil {
		return marketplace.Review{}, err
	}

	var rating *int
	if f.Statistics.Rating > 0 {
		value := f.Statistics.Rating
		rating = &value
	}

	media := make([]marketplace.Media, 0, len(f.Media.Photos)+len(f.Media.Videos))
	for i, photo := range f.Media.Photos {
		if photo == "" {
			continue
		}
		media = append(media, marketplace.Media{
			Kind:     "photo",
			URL:      photo,
			Position: i,
		})
	}
	for _, video := range f.Media.Videos {
		if video == "" {
			continue
		}
		media = append(media, marketplace.Media{
			Kind:     "video",
			URL:      video,
			Position: len(media),
		})
	}

	return marketplace.Review{
		Marketplace:       marketplaceID,
		ExternalReviewID:  strconv.FormatInt(f.FeedbackID, 10),
		ExternalProductID: f.externalProductID(),
		SellerArticle:     f.Identifiers.OfferID,
		Rating:            rating,
		AuthorName:        f.Author,
		Text:              f.Description.Comment,
		Pros:              f.Description.Advantages,
		Cons:              f.Description.Disadvantages,
		CreatedAtMP:       createdAt,
		Media:             media,
		Raw:               raw,
	}, nil
}

func (f goodsFeedback) externalProductID() string {
	if f.Identifiers.MarketSku > 0 {
		return strconv.FormatInt(f.Identifiers.MarketSku, 10)
	}
	if f.Identifiers.ModelID > 0 {
		return strconv.FormatInt(f.Identifiers.ModelID, 10)
	}
	return f.Identifiers.OfferID
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/marketplace/ym/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/marketplace/ym/client.go internal/marketplace/ym/client_test.go
git commit -m "feat: Yandex Market adapter — goods-feedback fetch and mapping"
```

---

## Task 3: Pagination across pages and `since` filtering

Verify the cursor loop (page_token → next-cursor) and that reviews older than `since` are dropped.

**Files:**
- Modify: `internal/marketplace/ym/client_test.go` (add two tests)

- [ ] **Step 1: Write the failing tests**

Append to `internal/marketplace/ym/client_test.go`:

```go
func TestFetchReviewsPaginatesWithPageToken(t *testing.T) {
	// First page returns a nextPageToken; the client must surface it as the cursor.
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		token := r.URL.Query().Get("page_token")
		switch token {
		case "":
			return jsonResponse(http.StatusOK, map[string]any{
				"result": map[string]any{
					"feedbacks": []map[string]any{
						{
							"feedbackId": 1, "createdAt": "2026-06-01T00:00:00Z",
							"identifiers": map[string]any{"offerId": "A"},
							"statistics":  map[string]any{"rating": 4},
						},
					},
					"paging": map[string]any{"nextPageToken": "PAGE2"},
				},
			}), nil
		case "PAGE2":
			return jsonResponse(http.StatusOK, map[string]any{
				"result": map[string]any{
					"feedbacks": []map[string]any{
						{
							"feedbackId": 2, "createdAt": "2026-06-02T00:00:00Z",
							"identifiers": map[string]any{"offerId": "B"},
							"statistics":  map[string]any{"rating": 5},
						},
					},
					"paging": map[string]any{"nextPageToken": ""},
				},
			}), nil
		default:
			t.Fatalf("unexpected page_token %q", token)
			return nil, nil
		}
	})}

	client := NewWithHTTPClient(config.YMConfig{APIKey: "key", BusinessID: "777"}, "https://api.partner.test", httpClient, 50)

	page1, cursor1, err := client.FetchReviews(context.Background(), time.Time{}, "")
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if cursor1 != "PAGE2" {
		t.Fatalf("cursor1 = %q", cursor1)
	}
	if len(page1) != 1 || page1[0].ExternalReviewID != "1" {
		t.Fatalf("page1 = %+v", page1)
	}

	page2, cursor2, err := client.FetchReviews(context.Background(), time.Time{}, cursor1)
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if cursor2 != "" {
		t.Fatalf("cursor2 = %q", cursor2)
	}
	if len(page2) != 1 || page2[0].ExternalReviewID != "2" {
		t.Fatalf("page2 = %+v", page2)
	}
}

func TestFetchReviewsDropsReviewsOlderThanSince(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{
			"result": map[string]any{
				"feedbacks": []map[string]any{
					{
						"feedbackId": 10, "createdAt": "2026-01-01T00:00:00Z", // old → dropped
						"identifiers": map[string]any{"offerId": "OLD"},
						"statistics":  map[string]any{"rating": 3},
					},
					{
						"feedbackId": 11, "createdAt": "2026-06-05T00:00:00Z", // recent → kept
						"identifiers": map[string]any{"offerId": "NEW"},
						"statistics":  map[string]any{"rating": 5},
					},
				},
				"paging": map[string]any{"nextPageToken": ""},
			},
		}), nil
	})}

	client := NewWithHTTPClient(config.YMConfig{APIKey: "key", BusinessID: "777"}, "https://api.partner.test", httpClient, 50)
	since := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	reviews, _, err := client.FetchReviews(context.Background(), since, "")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(reviews) != 1 || reviews[0].ExternalReviewID != "11" {
		t.Fatalf("expected only the recent review, got %+v", reviews)
	}
}

func TestFetchReviewsErrorsOnNon2xx(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusForbidden, map[string]any{
			"errors": []map[string]any{{"code": "FORBIDDEN", "message": "no access"}},
		}), nil
	})}

	client := NewWithHTTPClient(config.YMConfig{APIKey: "key", BusinessID: "777"}, "https://api.partner.test", httpClient, 50)
	_, _, err := client.FetchReviews(context.Background(), time.Time{}, "")
	if err == nil {
		t.Fatal("expected an error on HTTP 403")
	}
	if !strings.Contains(err.Error(), "no access") {
		t.Fatalf("error should surface the API message, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test ./internal/marketplace/ym/ -run TestFetchReviews -v`
Expected: PASS for all four `TestFetchReviews*` tests. (The pagination, since-filter, and error behavior are already implemented in Task 2's `client.go`; these tests lock that behavior in.)

- [ ] **Step 3: Commit**

```bash
git add internal/marketplace/ym/client_test.go
git commit -m "test: Yandex Market adapter pagination, since filter, error handling"
```

---

## Task 4: Wire the adapter into the collector

Register the YM adapter so `reviews sync` actually runs it.

**Files:**
- Modify: `cmd/reviews/main.go` (add the import and the `buildAdapters` branch)

- [ ] **Step 1: Add the import**

In `cmd/reviews/main.go`, find the import block that already imports `"reviews/internal/marketplace/wb"` and add directly below it:

```go
	"reviews/internal/marketplace/ym"
```

- [ ] **Step 2: Add the buildAdapters branch**

In `cmd/reviews/main.go`, change `buildAdapters` from:

```go
func buildAdapters(cfg config.Config) []marketplace.Adapter {
	var adapters []marketplace.Adapter
	if cfg.Marketplaces.WB.Enabled {
		adapters = append(adapters, wb.New(cfg.Marketplaces.WB))
	}
	return adapters
}
```

to:

```go
func buildAdapters(cfg config.Config) []marketplace.Adapter {
	var adapters []marketplace.Adapter
	if cfg.Marketplaces.WB.Enabled {
		adapters = append(adapters, wb.New(cfg.Marketplaces.WB))
	}
	if cfg.Marketplaces.YM.Enabled {
		adapters = append(adapters, ym.New(cfg.Marketplaces.YM))
	}
	return adapters
}
```

- [ ] **Step 3: Build and run the full suite**

Run:
```bash
go build ./... && go test ./...
```
Expected: build succeeds; all packages pass (`config`, `wb`, `ym`, `store`, `reviewjson`, …).

- [ ] **Step 4: Commit**

```bash
git add cmd/reviews/main.go
git commit -m "feat: register Yandex Market adapter in buildAdapters"
```

---

## Task 5: Live sync + export + widget-bundle reconciliation gate

The real verification. Run the adapter against the live API, persist, export, and confirm YM reviews land on the right product — the primary risk from the spec (`offerId` may not match the site SKU).

**Files:**
- No source changes unless reconciliation reveals an article mismatch (see Step 4).

- [ ] **Step 1: Migrate and run a YM-only sync**

Run:
```bash
go run ./cmd/reviews migrate
go run ./cmd/reviews sync --once --marketplace ym
```
Expected: log line `sync marketplace complete marketplace=ym seen=<N> upserted=<M>` with no error. If it errors with an access/subscription message, this is the Ozon-style paywall — stop and report.

- [ ] **Step 2: Inspect what landed**

Run:
```bash
go run ./cmd/reviews serve --addr 127.0.0.1:8080 &
sleep 1
curl -sS 'http://127.0.0.1:8080/api/reviews?marketplace=ym' | head -c 2000
kill %1
```
Expected: JSON reviews with `marketplace":"ym"`, non-empty `text`/`rating`, and a `sellerArticle`. Note the `sellerArticle` values.

- [ ] **Step 3: Export static bundles**

Run:
```bash
go run ./cmd/reviews export --out web/reviews-data
ls web/reviews-data/by-article/ | head
```
Expected: per-article JSON bundles written.

- [ ] **Step 4: Reconcile YM article against site SKUs (the gate)**

Compare the YM `sellerArticle` values from Step 2 against the site SKUs the widget looks up. The site links live in `data/shegida-product-links.json`; the widget keys bundles by the normalized seller article.

Run (pick one YM `sellerArticle` value, e.g. `1523`, and check a bundle exists and contains the YM review):
```bash
SKU=<one-ym-sellerArticle-from-step-2>
cat web/reviews-data/by-article/${SKU}.json | grep -c '"marketplace":"ym"' || echo "NO YM REVIEW IN BUNDLE FOR ${SKU}"
grep -o '"'${SKU}'"' data/shegida-product-links.json | head -1 || echo "SKU ${SKU} not in site product links"
```

Decide:
- **YM review present in a bundle whose SKU the site uses** → success. The end-to-end path works; proceed to Step 5.
- **YM `sellerArticle` does not match any site SKU** → the offer-id scheme differs (the anticipated risk). Do **not** claim success. Capture the mismatch (a few example YM `offerId`s vs the corresponding site SKUs) and report to the user with options: (a) add a YM-specific normalization rule in `reviewjson` analogous to the WB color-suffix handling, or (b) add an explicit YM article→site-SKU map. Implementing the chosen fix is a follow-up task driven by real data, not guessable in advance.

- [ ] **Step 5: Report outcome**

Summarize for the user: number of YM reviews synced, whether they attach to the correct product bundles, and any article-mismatch follow-up. Do not mark the feature complete unless a YM review verifiably renders for the correct product.

---

## Self-review notes

- **Spec coverage:** endpoint + auth (Task 2), pagination via page_token (Task 3), `since` lower bound (Task 3), mapping table incl. media/rating/text (Task 2), no seller answers — `Answer` asserted nil (Task 2), error isolation surfaces API message (Task 3), wiring (Task 4), live smoke + paywall check (Task 1 & 5), article-matching gate (Task 5). All spec sections map to a task.
- **Out of scope (unchanged):** seller answers/comments, outbound `market.yandex.ru` links, Ozon.
- **Field-name risk:** Task 1 captures the real payload and Step 3 there instructs reconciling the `json` tags before relying on Task 2's DTOs.
