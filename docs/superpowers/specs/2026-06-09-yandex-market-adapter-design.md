# Yandex Market Reviews Adapter — Design

**Date:** 2026-06-09
**Status:** Approved (pending spec review)
**Goal:** Collect product reviews from Yandex Market so the embedded widget on
`shegida.ru` shows reviews from both Wildberries and Yandex Market, not WB only.

## Context

The Reviews service already collects, stores, exports, and renders Wildberries
reviews end-to-end. The marketplace layer is a single small interface
(`internal/marketplace/model.go`):

```go
type Adapter interface {
    Marketplace() string
    FetchReviews(ctx context.Context, since time.Time, cursor string) ([]Review, string, error)
}
```

The collector (`internal/collector/collector.go`) drives it: it calls
`FetchReviews(ctx, since, cursor)` in a loop, advancing `cursor` to the returned
next-cursor until that next-cursor is empty. Upserts are idempotent by
`(marketplace, external_review_id)`; `since` is a lower bound the adapter must
honor; the watermark is the newest review time seen.

The config layer is already scaffolded for YM (`internal/config/config.go`):
`YMConfig{ Enabled, APIKey, OAuthToken, BusinessID, CampaignID }`, credential
validation, and `Enabled: true` by default. Only the adapter package and one
wiring line in `cmd/reviews/main.go` `buildAdapters` are missing.

## API feasibility (researched 2026-06-09)

- **Endpoint:** `POST /v2/businesses/{businessId}/goods-feedback` — synchronous,
  paginated JSON list, max 50 feedbacks per page, `nextPageToken` pagination.
  Optional body filters `offerIds`, `reactionStatus`.
- **Auth:** `Api-Key` request header (OAuth is deprecated). `businessId` in the
  path. Both already modeled in `YMConfig`.
- **Model `GoodsFeedbackDTO`:** `feedbackId`, `createdAt`, `author`,
  `needReaction`, nested `identifiers` (offerId / shopSku / marketSku),
  `description` (comment / advantages / disadvantages), `media` (photos,
  videos), `statistics` (rating).
- **Seller answers** live in a *separate* endpoint
  `POST /v2/businesses/{businessId}/goods-feedback/comments` (unlike WB, where
  the answer is inline). **Out of scope for v1.**
- **Paywall:** the docs do not mention a paid-subscription gate for reading
  reviews (unlike Ozon, which is gated behind the paid "Управление отзывами"
  subscription). Not 100% confirmed — an early live smoke test with the real key
  resolves it.

## Decisions (from brainstorming)

1. We have a real YM Partner API `Api-Key` + `businessId` → we can test against
   the live API.
2. **v1 = reviews only**, no seller answers. A review with no answer renders in
   the widget without an answer block (graceful).
3. The YM `offerId` (shop SKU) scheme **may differ** from the site/WB article
   scheme — unknown. The design includes a live reconciliation gate (see Risks).

## Approach

Chosen: **mirror the WB adapter** — a synchronous paginating client over
`goods-feedback`. Consistent with the existing architecture, smallest surface,
TDD on fixtures like `wb/client_test.go`.

Rejected alternatives:
- **Adapter + answers now** — adds per-page `/goods-feedback/comments` calls,
  more code and rate-limit handling; answers were explicitly deferred.
- **Async report `/reports/goods-feedback/generate`** — file-based async report
  fits the incremental hourly cursor model poorly (polling, file parsing).

## Components

New package `internal/marketplace/ym` (shaped like `internal/marketplace/wb`):

- `Client{ baseURL, apiKey, businessID, httpClient, pageSize }`.
- `New(cfg config.YMConfig) *Client` — production defaults
  (`baseURL = https://api.partner.market.yandex.ru`, `pageSize = 50`).
- `NewWithHTTPClient(cfg, baseURL, httpClient, pageSize) *Client` — for tests.
- `Marketplace() string` → `config.MarketplaceYM` (`"ym"`).
- Internal DTOs (`ymFeedbacksResponse`, `goodsFeedbackDTO`, and the nested
  `identifiers` / `description` / `media` / `statistics` types) and a
  `toReview()` mapper.

## Data flow — `FetchReviews(ctx, since, cursor)`

1. Build `POST {baseURL}/v2/businesses/{businessID}/goods-feedback?limit=50`,
   appending `&page_token=<cursor>` when `cursor != ""`. Header
   `Api-Key: <apiKey>`. Minimal body (no filters in v1, to read all reviews).
2. Decode `result.feedbacks[]` and `result.paging.nextPageToken`.
3. Map each feedback to `marketplace.Review` via `toReview()`.
4. Apply `since`: drop reviews with `CreatedAtMP` older than `since`.
5. Return `(reviews, result.paging.nextPageToken, nil)`. Empty token → the
   collector stops looping.
6. **Pagination/since strategy:** v1 paginates fully and relies on idempotent
   upsert (conservative — never misses reviews). If the live smoke test confirms
   `goods-feedback` returns newest-first, add an early-stop once a full page is
   entirely older than `since`, as an optimization.

## Mapping `goodsFeedbackDTO` → `marketplace.Review`

| `marketplace.Review` field | YM source |
| --- | --- |
| `Marketplace` | `"ym"` |
| `ExternalReviewID` | `feedbackId` (stringified) |
| `ExternalProductID` | `identifiers.marketSku` (fallback `identifiers.modelId`) |
| `SellerArticle` | `identifiers.offerId` (same normalization as WB) |
| `Rating` | `&statistics.rating` |
| `Text` | `description.comment` |
| `Pros` | `description.advantages` |
| `Cons` | `description.disadvantages` |
| `AuthorName` | `author` |
| `CreatedAtMP` | parsed `createdAt` |
| `Media` | `media.photos[]` → `{Kind:"photo"}`, `media.videos[]` → `{Kind:"video"}`, positioned in order |
| `Answer` | `nil` (deferred) |
| `Raw` | raw JSON of the DTO |

Article normalization reuses the same rule WB uses (trim, strip color suffix
like `3467/Белый` → `3467`) so the widget groups YM and WB reviews under one
seller article.

## Error handling

Same shape as the WB client: a non-2xx status, or a response body carrying an
error, returns an `error` from `FetchReviews`. The collector already isolates a
failing marketplace from the others and records the failure in `sync_runs`, so a
YM outage never blocks the WB sync.

## Wiring

`cmd/reviews/main.go` `buildAdapters` gains:

```go
if cfg.Marketplaces.YM.Enabled {
    adapters = append(adapters, ym.New(cfg.Marketplaces.YM))
}
```

Config loading, credential validation, and `--marketplace ym` dispatch already
exist.

## Testing

- **Unit (TDD, fixtures):** mapping a saved `GoodsFeedbackDTO` JSON fixture →
  `marketplace.Review`; pagination across pages via `httptest.Server`
  (cursor → `page_token`, empty token ends the loop); `since` filtering drops
  older reviews; non-2xx → error.
- **Live smoke:** `reviews sync --once --marketplace ym` against the real key;
  inspect the DB rows and confirm reviews land with expected fields.
- **End-to-end gate:** `reviews export` then confirm at least one YM review
  appears in a `web/reviews-data/by-article/<sku>.json` bundle.

## Risks and verification gates

1. **(Primary) Article matching.** `offerId` may not equal the site/WB article.
   If so, YM reviews won't attach to product pages. **Gate:** on live data,
   compare `offerId` against the site SKUs (`data/shegida-product-links.json` /
   page SKUs); if they diverge, add a normalization rule or an explicit YM
   article map. Do not declare success until a YM review actually appears in a
   widget bundle for the right product.
2. **Possible undocumented access gate** (like Ozon). An early live smoke test
   with the real key resolves it before more code is written.
3. **Sort order** for the early-stop optimization is unverified — v1 paginates
   fully; optimize only after confirming order live.
4. **Rate limits** on `goods-feedback` — sequential page fetches, 50/page.

## Out of scope (v1)

- Seller answers / comments (`/goods-feedback/comments`).
- Outbound `market.yandex.ru` review/product link in the widget (a YM review
  renders without an external link; can be added later in `reviewjson`).
- Ozon (separate, paywalled).
