# Marketplace Reply Publishing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When an admin saves a reply to a *marketplace* review, also publish it to that marketplace's API (WB/YM/Ozon), tracking per-review publish state with retry on failure.

**Architecture:** A new `marketplace.ReplyPublisher` capability is implemented by the three existing clients. Three new `Review` columns track publish state. The reply handler attempts publishing synchronously after saving; failures persist as `failed` and are retried at the end of each sync run. A per-marketplace "publish replies" toggle lives in `app_settings`. The HTTP server gains a `map[string]marketplace.ReplyPublisher`, injected from `cmd/reviews/main.go` (which already builds the adapters for the collector).

**Tech Stack:** Go (pure-Go, `CGO_ENABLED=0`), GORM/SQLite, `net/http`, `httptest` for client tests. React/TS admin SPA (Vite) for the UI task.

## Global Constraints

- Pure Go only — no CGO, no new heavy deps. Reuse each client's existing auth pattern verbatim:
  - WB: header `Authorization: <token>`, base `https://feedbacks-api.wildberries.ru`.
  - YM: header `Api-Key: <apiKey>`, base `https://api.partner.market.yandex.ru`, path uses `businessID`.
  - Ozon: headers `Client-Id: <clientID>` + `Api-Key: <apiKey>`, base `https://api-seller.ozon.ru`.
- Publish-once: no editing an already-`published` answer in this feature. Retry applies only to `pending`/`failed`.
- Publish state values are exactly: `pending`, `published`, `failed`, `unsupported`.
- Per-marketplace toggle default: **WB on, YM on, Ozon off** when unset.
- Site reviews (`marketplace == "site"`) are never published — state `unsupported`.
- Admin UI copy is Russian.
- Marketplace clients are constructed with `New(cfg)` (live) and `NewWithHTTPClient(cfg, baseURL, httpClient, pageSize)` (tests) — add publish support to both paths without breaking signatures.
- Go commands run from repo root `/home/mama/DEV/Reviews`; admin npm from `web/admin`; rebuild+embed the admin bundle (`rm -rf internal/server/admin_dist && cp -r web/admin/dist internal/server/admin_dist`) before committing UI changes.
- ⚠️ **Endpoint verification:** the WB and Ozon request bodies below are confirmed against current docs. The YM body shape (`goods-feedback/comments/update`) must be confirmed against https://yandex.ru/dev/market/partner-api/doc/ru/reference/goods-feedback/ before finalizing Task 5; keep the test as the contract and adjust the struct if the live shape differs.

## Verified marketplace endpoints

| MP | Method + path | Auth | Body | Success |
|---|---|---|---|---|
| WB | `POST /api/v1/feedbacks/answer` | `Authorization: <token>` | `{"id":"<feedbackId>","text":"<text>"}` | HTTP 204 (no body). NB: WB does **not** validate the id — 204 ≠ guaranteed landed. |
| YM | `POST /v2/businesses/{businessId}/goods-feedback/comments/update` | `Api-Key` | `{"feedbackId":<int64>,"comment":{"text":"<text>"}}` (omit comment.id to create) | HTTP 200 |
| Ozon | `POST /v1/review/comment/create` | `Client-Id`+`Api-Key` | `{"review_id":<int64>,"text":"<text>","mark_review_as_processed":true,"parent_comment_id":0}` | HTTP 200, `{"comment_id":<int64>}` (Premium-Plus, beta) |

`Review.ExternalReviewID` is stored as a string: WB uses it as-is; YM and Ozon need `strconv.ParseInt`.

## File Structure

- Modify: `internal/marketplace/model.go` — add `ReplyPublisher` interface.
- Modify: `internal/marketplace/wb/client.go` (+ `client_test.go`) — `PublishReply`.
- Modify: `internal/marketplace/ym/client.go` (+ `client_test.go`) — `PublishReply`.
- Modify: `internal/marketplace/ozon/client.go` (+ `client_test.go`) — `PublishReply`.
- Modify: `internal/store/models.go` — three publish columns.
- Create: `internal/store/reply_publish.go` (+ `reply_publish_test.go`) — state setters + query + getter.
- Modify: `internal/store/app_settings.go` — toggle keys.
- Create: `internal/server/reply_publish.go` (+ `reply_publish_test.go`) — `publishReply`, `RetryPendingReplies`, `replyPublishEnabled`.
- Modify: `internal/server/server.go` — `Config.ReplyPublishers`, `Server.replyPublishers`, wire in `New`; register retry route.
- Modify: `internal/server/admin_reviews.go` — reply handler triggers publish; expose publish state in JSON; retry handler.
- Modify: `cmd/reviews/main.go` — build publisher map, inject into server; wrap sync to call `RetryPendingReplies`.
- Modify: `web/admin/src/pages/Reviews.tsx`, `web/admin/src/pages/Marketplaces.tsx`, `web/admin/src/types.ts` — badge, retry button, per-MP toggle.
- Regenerate: `internal/server/admin_dist/*`.

---

### Task 1: `ReplyPublisher` capability interface

**Files:**
- Modify: `internal/marketplace/model.go`

**Interfaces:**
- Produces: `marketplace.ReplyPublisher` with `PublishReply(ctx context.Context, externalReviewID, text string) error`.

- [ ] **Step 1: Add the interface**

Append to `internal/marketplace/model.go`:

```go
// ReplyPublisher is implemented by adapters that can publish a seller reply
// back to the marketplace. Adapters that cannot (or for accounts lacking
// access) simply do not implement it; callers treat that as "unsupported".
type ReplyPublisher interface {
	PublishReply(ctx context.Context, externalReviewID, text string) error
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/marketplace/...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/marketplace/model.go
git commit -m "feat(marketplace): add ReplyPublisher capability interface"
```

---

### Task 2: Publish-state columns + store methods

**Files:**
- Modify: `internal/store/models.go` (the `Review` struct, after `AdminReplyAt` at line 47)
- Create: `internal/store/reply_publish.go`
- Test: `internal/store/reply_publish_test.go`

**Interfaces:**
- Produces: `Review.ReplyPublishState *string`, `Review.ReplyPublishError *string`, `Review.ReplyPublishedAt *time.Time`.
- Produces: `func (s *Store) SetReplyPublishState(ctx, id uint, state string, errText *string, publishedAt *time.Time) error`
- Produces: `func (s *Store) ReviewsNeedingReplyPublish(ctx context.Context) ([]Review, error)` — reviews with a non-empty `admin_reply_text`, `marketplace != 'site'`, and `reply_publish_state` IN (`pending`,`failed`) (or NULL).
- Produces: `func (s *Store) ReviewByID(ctx context.Context, id uint) (Review, error)`

- [ ] **Step 1: Add the columns**

In `internal/store/models.go`, after the `AdminReplyAt *time.Time` field, add:

```go
	ReplyPublishState  *string `gorm:"size:16;index"`
	ReplyPublishError  *string
	ReplyPublishedAt   *time.Time
```

- [ ] **Step 2: Write the failing test**

Create `internal/store/reply_publish_test.go`:

```go
package store

import (
	"context"
	"testing"
	"time"

	"reviews/internal/marketplace"
)

func TestReplyPublishStateAndQueue(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	rating := 5
	res, err := s.UpsertReview(ctx, marketplace.Review{
		Marketplace: "wb", ExternalReviewID: "wb-1", ExternalProductID: "p1",
		Rating: &rating, Text: "ok", CreatedAtMP: time.Unix(1700000000, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	id := res.Review.ID

	// A review with a reply but no publish state is queued.
	reply := "Спасибо!"
	if err := s.SetReviewReply(ctx, id, &reply); err != nil {
		t.Fatalf("reply: %v", err)
	}
	if err := s.SetReplyPublishState(ctx, id, "pending", nil, nil); err != nil {
		t.Fatalf("set pending: %v", err)
	}
	queued, err := s.ReviewsNeedingReplyPublish(ctx)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if len(queued) != 1 || queued[0].ID != id {
		t.Fatalf("expected 1 queued, got %+v", queued)
	}

	// Marking published removes it from the queue and records the timestamp.
	now := time.Now().UTC()
	if err := s.SetReplyPublishState(ctx, id, "published", nil, &now); err != nil {
		t.Fatalf("set published: %v", err)
	}
	queued, _ = s.ReviewsNeedingReplyPublish(ctx)
	if len(queued) != 0 {
		t.Fatalf("expected empty queue, got %d", len(queued))
	}
	got, err := s.ReviewByID(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ReplyPublishState == nil || *got.ReplyPublishState != "published" || got.ReplyPublishedAt == nil {
		t.Fatalf("unexpected state: %+v", got)
	}
}
```

- [ ] **Step 2b: Run it to confirm it fails**

Run: `go test ./internal/store/ -run TestReplyPublishStateAndQueue -count=1`
Expected: FAIL (undefined `SetReplyPublishState` / `ReviewsNeedingReplyPublish` / `ReviewByID`).

- [ ] **Step 3: Implement**

Create `internal/store/reply_publish.go`:

```go
package store

import (
	"context"
	"time"
)

// SetReplyPublishState records the outcome of a publish attempt. errText is
// stored only for the "failed" state; publishedAt only for "published".
func (s *Store) SetReplyPublishState(ctx context.Context, id uint, state string, errText *string, publishedAt *time.Time) error {
	updates := map[string]any{
		"reply_publish_state": state,
		"reply_publish_error": errText,
		"reply_published_at":  publishedAt,
	}
	return s.db.WithContext(ctx).Model(&Review{}).Where("id = ?", id).Updates(updates).Error
}

// ReviewsNeedingReplyPublish returns marketplace reviews that carry a seller
// reply but have not been successfully published yet (pending, failed, or
// never attempted).
func (s *Store) ReviewsNeedingReplyPublish(ctx context.Context) ([]Review, error) {
	var rows []Review
	err := s.db.WithContext(ctx).
		Where("marketplace <> ?", MarketplaceSite).
		Where("admin_reply_text IS NOT NULL AND admin_reply_text <> ''").
		Where("reply_publish_state IS NULL OR reply_publish_state IN ?", []string{"pending", "failed"}).
		Order("id asc").
		Find(&rows).Error
	return rows, err
}

func (s *Store) ReviewByID(ctx context.Context, id uint) (Review, error) {
	var review Review
	err := s.db.WithContext(ctx).First(&review, id).Error
	return review, err
}
```

- [ ] **Step 4: Add the column to AutoMigrate (already covered — Review is migrated) and run the test**

Run: `go test ./internal/store/ -run TestReplyPublishStateAndQueue -count=1`
Expected: PASS. (The new columns are migrated automatically because `Review` is already in the `AutoMigrate` list.)

- [ ] **Step 5: Commit**

```bash
git add internal/store/models.go internal/store/reply_publish.go internal/store/reply_publish_test.go
git commit -m "feat(store): reply publish-state columns, setters, and queue query"
```

---

### Task 3: Per-marketplace publish toggle in app_settings

**Files:**
- Modify: `internal/store/app_settings.go` (the const block)

**Interfaces:**
- Produces: `store.SettingPublishRepliesPrefix = "publish_replies_"` and helper `store.PublishRepliesKey(marketplace string) string`.

- [ ] **Step 1: Add the key helper**

In `internal/store/app_settings.go`, add to the const block:

```go
	// SettingPublishRepliesPrefix + marketplace id is the per-marketplace
	// "publish seller replies back to the marketplace" toggle ("true"/"").
	SettingPublishRepliesPrefix = "publish_replies_"
```

And add below the constants:

```go
// PublishRepliesKey is the app_settings key for a marketplace's publish toggle.
func PublishRepliesKey(marketplace string) string {
	return SettingPublishRepliesPrefix + marketplace
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./internal/store/...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/store/app_settings.go
git commit -m "feat(store): per-marketplace publish-replies toggle key"
```

---

### Task 4: WB `PublishReply`

**Files:**
- Modify: `internal/marketplace/wb/client.go`
- Test: `internal/marketplace/wb/client_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `func (c *Client) PublishReply(ctx context.Context, externalReviewID, text string) error` on `*wb.Client`.

- [ ] **Step 1: Write the failing test**

Add to `internal/marketplace/wb/client_test.go`:

```go
func TestPublishReplyPostsAnswer(t *testing.T) {
	var gotBody string
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/feedbacks/answer" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewWithHTTPClient(config.WBConfig{Token: "tok"}, srv.URL, srv.Client(), 0)
	if err := c.PublishReply(context.Background(), "fb-1", "Спасибо!"); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if gotAuth != "tok" {
		t.Fatalf("auth = %q", gotAuth)
	}
	if !strings.Contains(gotBody, `"id":"fb-1"`) || !strings.Contains(gotBody, `"text":"Спасибо!"`) {
		t.Fatalf("body = %s", gotBody)
	}
}

func TestPublishReplyError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errorText":"bad"}`))
	}))
	defer srv.Close()
	c := NewWithHTTPClient(config.WBConfig{Token: "tok"}, srv.URL, srv.Client(), 0)
	if err := c.PublishReply(context.Background(), "fb-1", "x"); err == nil {
		t.Fatal("expected error on non-204")
	}
}
```

Ensure the test file imports `io` and `strings` (add if missing).

- [ ] **Step 2: Run to confirm failure**

Run: `go test ./internal/marketplace/wb/ -run TestPublishReply -count=1`
Expected: FAIL (undefined `PublishReply`).

- [ ] **Step 3: Implement**

Add to `internal/marketplace/wb/client.go`:

```go
func (c *Client) PublishReply(ctx context.Context, externalReviewID, text string) error {
	body, err := json.Marshal(map[string]string{"id": externalReviewID, "text": text})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/feedbacks/answer", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
		return nil
	}
	return fmt.Errorf("WB publish reply: status %d", resp.StatusCode)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/marketplace/wb/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/marketplace/wb/client.go internal/marketplace/wb/client_test.go
git commit -m "feat(wb): publish seller reply via feedbacks/answer"
```

---

### Task 5: YM `PublishReply`

**Files:**
- Modify: `internal/marketplace/ym/client.go`
- Test: `internal/marketplace/ym/client_test.go`

**Interfaces:**
- Produces: `func (c *Client) PublishReply(ctx context.Context, externalReviewID, text string) error` on `*ym.Client`.

⚠️ Confirm the request body shape at https://yandex.ru/dev/market/partner-api/doc/ru/reference/goods-feedback/ before finalizing. The plan assumes `{"feedbackId":<int64>,"comment":{"text":"<text>"}}`.

- [ ] **Step 1: Write the failing test**

Add to `internal/marketplace/ym/client_test.go` (match the existing test's client constructor and `businessID` setup — inspect the file's existing `NewWithHTTPClient` usage and reuse it):

```go
func TestPublishReplyPostsComment(t *testing.T) {
	var gotPath, gotBody, gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("Api-Key")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewWithHTTPClient(config.YMConfig{APIKey: "key", BusinessID: "42"}, srv.URL, srv.Client(), 0)
	if err := c.PublishReply(context.Background(), "1001", "Спасибо!"); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if gotPath != "/v2/businesses/42/goods-feedback/comments/update" {
		t.Fatalf("path = %s", gotPath)
	}
	if gotKey != "key" {
		t.Fatalf("api-key = %q", gotKey)
	}
	if !strings.Contains(gotBody, `"feedbackId":1001`) || !strings.Contains(gotBody, `"text":"Спасибо!"`) {
		t.Fatalf("body = %s", gotBody)
	}
}

func TestPublishReplyBadID(t *testing.T) {
	c := NewWithHTTPClient(config.YMConfig{APIKey: "key", BusinessID: "42"}, "http://unused", nil, 0)
	if err := c.PublishReply(context.Background(), "not-a-number", "x"); err == nil {
		t.Fatal("expected error for non-numeric feedback id")
	}
}
```

Match `NewWithHTTPClient`'s real signature in `ym/client.go` (inspect it; the WB one is `(cfg, baseURL, httpClient, pageSize)` — YM may differ). Adjust the call to the actual signature; if YM has no `NewWithHTTPClient`, add one mirroring WB's.

- [ ] **Step 2: Run to confirm failure**

Run: `go test ./internal/marketplace/ym/ -run TestPublishReply -count=1`
Expected: FAIL.

- [ ] **Step 3: Implement**

Add to `internal/marketplace/ym/client.go` (use the client's existing `baseURL`, `apiKey`, `businessID` fields):

```go
func (c *Client) PublishReply(ctx context.Context, externalReviewID, text string) error {
	feedbackID, err := strconv.ParseInt(externalReviewID, 10, 64)
	if err != nil {
		return fmt.Errorf("YM publish reply: bad feedback id %q: %w", externalReviewID, err)
	}
	payload := map[string]any{
		"feedbackId": feedbackID,
		"comment":    map[string]string{"text": text},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/v2/businesses/%s/goods-feedback/comments/update", c.baseURL, c.businessID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Api-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("YM publish reply: status %d", resp.StatusCode)
	}
	return nil
}
```

Ensure `strconv` is imported.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/marketplace/ym/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/marketplace/ym/client.go internal/marketplace/ym/client_test.go
git commit -m "feat(ym): publish seller reply via goods-feedback comments/update"
```

---

### Task 6: Ozon `PublishReply`

**Files:**
- Modify: `internal/marketplace/ozon/client.go`
- Test: `internal/marketplace/ozon/client_test.go`

**Interfaces:**
- Produces: `func (c *Client) PublishReply(ctx context.Context, externalReviewID, text string) error` on `*ozon.Client`.

- [ ] **Step 1: Write the failing test**

Add to `internal/marketplace/ozon/client_test.go` (reuse the file's existing `NewWithHTTPClient` constructor and header assertions style):

```go
func TestPublishReplyCreatesComment(t *testing.T) {
	var gotPath, gotBody, gotClient, gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotClient = r.Header.Get("Client-Id")
		gotKey = r.Header.Get("Api-Key")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"comment_id":555}`))
	}))
	defer srv.Close()

	c := NewWithHTTPClient(config.OzonConfig{ClientID: "cid", APIKey: "key"}, srv.URL, srv.Client(), 0)
	if err := c.PublishReply(context.Background(), "9001", "Спасибо!"); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if gotPath != "/v1/review/comment/create" {
		t.Fatalf("path = %s", gotPath)
	}
	if gotClient != "cid" || gotKey != "key" {
		t.Fatalf("headers c=%q k=%q", gotClient, gotKey)
	}
	if !strings.Contains(gotBody, `"review_id":9001`) || !strings.Contains(gotBody, `"text":"Спасибо!"`) {
		t.Fatalf("body = %s", gotBody)
	}
}
```

- [ ] **Step 2: Run to confirm failure**

Run: `go test ./internal/marketplace/ozon/ -run TestPublishReply -count=1`
Expected: FAIL.

- [ ] **Step 3: Implement**

Add to `internal/marketplace/ozon/client.go`:

```go
func (c *Client) PublishReply(ctx context.Context, externalReviewID, text string) error {
	reviewID, err := strconv.ParseInt(externalReviewID, 10, 64)
	if err != nil {
		return fmt.Errorf("Ozon publish reply: bad review id %q: %w", externalReviewID, err)
	}
	payload := map[string]any{
		"review_id":                reviewID,
		"text":                     text,
		"mark_review_as_processed": true,
		"parent_comment_id":        0,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/review/comment/create", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Client-Id", c.clientID)
	req.Header.Set("Api-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Ozon publish reply: status %d", resp.StatusCode)
	}
	return nil
}
```

Ensure `strconv` and `bytes` are imported (the fetch path already uses `bytes`).

- [ ] **Step 4: Run tests**

Run: `go test ./internal/marketplace/ozon/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/marketplace/ozon/client.go internal/marketplace/ozon/client_test.go
git commit -m "feat(ozon): publish seller reply via review/comment/create"
```

---

### Task 7: Server publish orchestration + retry

**Files:**
- Modify: `internal/server/server.go` (the `Config` struct, the `Server` struct, and `New`)
- Create: `internal/server/reply_publish.go`
- Test: `internal/server/reply_publish_test.go`

**Interfaces:**
- Consumes: `marketplace.ReplyPublisher`, `store.ReviewByID`, `store.SetReplyPublishState`, `store.ReviewsNeedingReplyPublish`, `store.PublishRepliesKey`.
- Produces: `Config.ReplyPublishers map[string]marketplace.ReplyPublisher`; `Server.replyPublishers`; `func (s *Server) publishReply(ctx context.Context, review store.Review)`; `func (s *Server) RetryPendingReplies(ctx context.Context)`; `func (s *Server) replyPublishEnabled(ctx context.Context, marketplace string) bool`.

- [ ] **Step 1: Add the field to Config and Server, wire in New**

In `internal/server/server.go`: add to the `Config` struct:

```go
	// ReplyPublishers maps marketplace id → publisher for posting seller
	// replies back to the marketplace. Marketplaces absent here are treated
	// as "unsupported".
	ReplyPublishers map[string]marketplace.ReplyPublisher
```

Add to the `Server` struct a field `replyPublishers map[string]marketplace.ReplyPublisher`, and in `New(...)` set `srv.replyPublishers = cfg.ReplyPublishers` (or the appropriate constructor assignment — match how `New` builds the `Server`). Add the `reviews/internal/marketplace` import.

- [ ] **Step 2: Write the failing test**

Create `internal/server/reply_publish_test.go`:

```go
package server

import (
	"context"
	"errors"
	"testing"

	"reviews/internal/marketplace"
	"reviews/internal/store"
)

type fakePublisher struct {
	calls int
	err   error
	last  string
}

func (f *fakePublisher) PublishReply(_ context.Context, _, text string) error {
	f.calls++
	f.last = text
	return f.err
}

func TestPublishReplySuccessAndUnsupported(t *testing.T) {
	s := newAuthTestServer(t)
	ctx := context.Background()
	pub := &fakePublisher{}
	s.replyPublishers = map[string]marketplace.ReplyPublisher{"wb": pub}

	rating := 5
	res, _ := s.store.UpsertReview(ctx, marketplace.Review{
		Marketplace: "wb", ExternalReviewID: "wb-1", ExternalProductID: "p1",
		Rating: &rating, Text: "t", CreatedAtMP: testTime(),
	})
	reply := "Спасибо!"
	_ = s.store.SetReviewReply(ctx, res.Review.ID, &reply)
	rv, _ := s.store.ReviewByID(ctx, res.Review.ID)
	s.publishReply(ctx, rv)

	if pub.calls != 1 || pub.last != "Спасибо!" {
		t.Fatalf("publisher not called correctly: %+v", pub)
	}
	got, _ := s.store.ReviewByID(ctx, res.Review.ID)
	if got.ReplyPublishState == nil || *got.ReplyPublishState != "published" {
		t.Fatalf("state = %v", got.ReplyPublishState)
	}

	// A site review is never published.
	res2, _ := s.store.CreateSiteReview(ctx, store.SiteReviewInput{
		ExternalReviewID: "site-x", SellerArticle: "a", Rating: 5,
		AuthorName: "A", AuthorEmail: "a@b.co", Text: "hi",
	})
	rv2, _ := s.store.ReviewByID(ctx, res2.ID)
	s.publishReply(ctx, rv2)
	got2, _ := s.store.ReviewByID(ctx, res2.ID)
	if got2.ReplyPublishState == nil || *got2.ReplyPublishState != "unsupported" {
		t.Fatalf("site state = %v", got2.ReplyPublishState)
	}
}

func TestPublishReplyFailureRecorded(t *testing.T) {
	s := newAuthTestServer(t)
	ctx := context.Background()
	s.replyPublishers = map[string]marketplace.ReplyPublisher{"wb": &fakePublisher{err: errors.New("boom")}}
	rating := 5
	res, _ := s.store.UpsertReview(ctx, marketplace.Review{
		Marketplace: "wb", ExternalReviewID: "wb-2", ExternalProductID: "p1",
		Rating: &rating, Text: "t", CreatedAtMP: testTime(),
	})
	reply := "x"
	_ = s.store.SetReviewReply(ctx, res.Review.ID, &reply)
	rv, _ := s.store.ReviewByID(ctx, res.Review.ID)
	s.publishReply(ctx, rv)
	got, _ := s.store.ReviewByID(ctx, res.Review.ID)
	if got.ReplyPublishState == nil || *got.ReplyPublishState != "failed" || got.ReplyPublishError == nil {
		t.Fatalf("expected failed+error, got %+v", got)
	}
}
```

Add a `testTime()` helper if none exists (`func testTime() time.Time { return time.Unix(1700000000, 0).UTC() }`) — check the package for an existing time constant first and reuse it; only add if absent (and import `time`).

- [ ] **Step 2b: Run to confirm failure**

Run: `go test ./internal/server/ -run TestPublishReply -count=1`
Expected: FAIL (undefined `publishReply`).

- [ ] **Step 3: Implement**

Create `internal/server/reply_publish.go`:

```go
package server

import (
	"context"
	"time"

	"reviews/internal/store"
)

// replyPublishEnabled reports whether replies should be auto-published for a
// marketplace. Default when unset: WB and YM on, Ozon off.
func (s *Server) replyPublishEnabled(ctx context.Context, marketplace string) bool {
	value, err := s.store.GetAppSetting(ctx, store.PublishRepliesKey(marketplace))
	if err == nil && value != "" {
		return value == "true"
	}
	return marketplace != "ozon"
}

// publishReply attempts to publish a saved reply to the marketplace and records
// the outcome on the review. Site reviews and marketplaces without a publisher
// are marked "unsupported".
func (s *Server) publishReply(ctx context.Context, review store.Review) {
	id := review.ID
	if review.Marketplace == store.MarketplaceSite {
		s.setUnsupported(ctx, id)
		return
	}
	pub, ok := s.replyPublishers[review.Marketplace]
	if !ok || !s.replyPublishEnabled(ctx, review.Marketplace) {
		s.setUnsupported(ctx, id)
		return
	}
	text := ""
	if review.AdminReplyText != nil {
		text = *review.AdminReplyText
	}
	if err := pub.PublishReply(ctx, review.ExternalReviewID, text); err != nil {
		msg := err.Error()
		_ = s.store.SetReplyPublishState(ctx, id, "failed", &msg, nil)
		s.logger.Warn("publish reply failed", "review", id, "marketplace", review.Marketplace, "error", err)
		return
	}
	now := time.Now().UTC()
	_ = s.store.SetReplyPublishState(ctx, id, "published", nil, &now)
}

func (s *Server) setUnsupported(ctx context.Context, id uint) {
	_ = s.store.SetReplyPublishState(ctx, id, "unsupported", nil, nil)
}

// RetryPendingReplies re-attempts every queued (pending/failed) reply. Called
// at the end of each sync run.
func (s *Server) RetryPendingReplies(ctx context.Context) {
	rows, err := s.store.ReviewsNeedingReplyPublish(ctx)
	if err != nil {
		s.logger.Error("load pending replies", "error", err)
		return
	}
	for _, review := range rows {
		s.publishReply(ctx, review)
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/server/ -run TestPublishReply -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/server/server.go internal/server/reply_publish.go internal/server/reply_publish_test.go
git commit -m "feat(server): reply publish orchestration, toggle, and retry"
```

---

### Task 8: Reply handler triggers publish + retry endpoint

**Files:**
- Modify: `internal/server/admin_reviews.go` (`handleAdminReviewReply` at line 245; the `adminReviewItem`/`adminReply` JSON struct around lines 22-95)
- Modify: `internal/server/server.go` (register the retry route)
- Test: `internal/server/reply_publish_test.go` (add handler test)

**Interfaces:**
- Consumes: `s.publishReply`, `s.store.ReviewByID`.
- Produces: `POST /admin/api/reviews/{id}/reply/retry` → `handleAdminReviewReplyRetry`; JSON `adminReviewItem.ReplyPublish *replyPublishStatus`.

- [ ] **Step 1: Trigger publish after saving the reply**

In `handleAdminReviewReply`, after the successful `SetReviewReply` and before writing the response, add a publish attempt for a non-empty reply:

```go
	if strings.TrimSpace(req.Text) != "" {
		if review, err := s.store.ReviewByID(r.Context(), uint(id)); err == nil {
			s.publishReply(r.Context(), review)
		}
	}
```

Ensure `strings` is imported in the file.

- [ ] **Step 2: Expose publish state in the admin reviews JSON**

In the admin review item struct (near `adminReply` at line 25), add:

```go
type replyPublishStatus struct {
	State       string     `json:"state"`
	Error       string     `json:"error,omitempty"`
	PublishedAt *time.Time `json:"publishedAt,omitempty"`
}
```

Add `ReplyPublish *replyPublishStatus` to the admin review item struct (alongside `AdminReply`), and populate it in the mapping loop (near line 94) when `rv.ReplyPublishState != nil`:

```go
		if rv.ReplyPublishState != nil {
			item.ReplyPublish = &replyPublishStatus{State: *rv.ReplyPublishState, PublishedAt: rv.ReplyPublishedAt}
			if rv.ReplyPublishError != nil {
				item.ReplyPublish.Error = *rv.ReplyPublishError
			}
		}
```

- [ ] **Step 3: Add the retry handler**

Add to `internal/server/admin_reviews.go`:

```go
func (s *Server) handleAdminReviewReplyRetry(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid review id"))
		return
	}
	review, err := s.store.ReviewByID(r.Context(), uint(id))
	if err != nil {
		writeError(w, http.StatusNotFound, errors.New("review not found"))
		return
	}
	s.publishReply(r.Context(), review)
	updated, err := s.store.ReviewByID(r.Context(), uint(id))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	state := ""
	if updated.ReplyPublishState != nil {
		state = *updated.ReplyPublishState
	}
	writeJSON(w, http.StatusOK, map[string]string{"state": state})
}
```

- [ ] **Step 4: Register the route**

In `internal/server/server.go`, after the existing reply route:

```go
	protected.Handle("PUT /admin/api/reviews/{id}/reply", requireCSRF(http.HandlerFunc(s.handleAdminReviewReply)))
	protected.Handle("POST /admin/api/reviews/{id}/reply/retry", requireCSRF(http.HandlerFunc(s.handleAdminReviewReplyRetry)))
```

- [ ] **Step 5: Add a handler test**

Add to `internal/server/reply_publish_test.go`:

```go
func TestReplyHandlerPublishesAndRetry(t *testing.T) {
	s := newAuthTestServer(t)
	pub := &fakePublisher{err: errors.New("boom")}
	s.replyPublishers = map[string]marketplace.ReplyPublisher{"wb": pub}
	cookie := loginTestAdmin(t, s)
	csrf := getCSRFToken(t, s, cookie)

	rating := 5
	res, _ := s.store.UpsertReview(context.Background(), marketplace.Review{
		Marketplace: "wb", ExternalReviewID: "wb-9", ExternalProductID: "p1",
		Rating: &rating, Text: "t", CreatedAtMP: testTime(),
	})

	put := func(path string, method string) int {
		req := httptest.NewRequest(method, path, strings.NewReader(`{"text":"Спасибо!"}`))
		req.AddCookie(cookie)
		req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
		req.Header.Set(csrfHeaderName, csrf)
		rec := httptest.NewRecorder()
		s.adminMux().ServeHTTP(rec, req)
		return rec.Code
	}

	if code := put("/admin/api/reviews/"+strconv.FormatUint(uint64(res.Review.ID), 10)+"/reply", http.MethodPut); code != http.StatusOK {
		t.Fatalf("reply status %d", code)
	}
	got, _ := s.store.ReviewByID(context.Background(), res.Review.ID)
	if got.ReplyPublishState == nil || *got.ReplyPublishState != "failed" {
		t.Fatalf("expected failed after publish attempt, got %v", got.ReplyPublishState)
	}

	pub.err = nil // marketplace recovers
	if code := put("/admin/api/reviews/"+strconv.FormatUint(uint64(res.Review.ID), 10)+"/reply/retry", http.MethodPost); code != http.StatusOK {
		t.Fatalf("retry status %d", code)
	}
	got, _ = s.store.ReviewByID(context.Background(), res.Review.ID)
	if got.ReplyPublishState == nil || *got.ReplyPublishState != "published" {
		t.Fatalf("expected published after retry, got %v", got.ReplyPublishState)
	}
}
```

Ensure imports: `net/http`, `net/http/httptest`, `strconv`, `strings`.

- [ ] **Step 6: Run tests**

Run: `go test ./internal/server/ -run 'TestReplyHandler|TestPublishReply' -count=1`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/server/admin_reviews.go internal/server/server.go internal/server/reply_publish_test.go
git commit -m "feat(server): publish on reply save, expose state, add retry endpoint"
```

---

### Task 9: Wire publishers from main + retry after sync

**Files:**
- Modify: `cmd/reviews/main.go` (where adapters are built ~line 465 and where the server `Config` and `TriggerSync` are constructed)

**Interfaces:**
- Consumes: `marketplace.ReplyPublisher`, `server.Config.ReplyPublishers`, `s.RetryPendingReplies`.

- [ ] **Step 1: Build the publisher map from the adapters**

Where `adapters` is assembled (the slice appended with `wb.New(...)`, `ym.New(...)`, `ozon.New(...)`), build a publisher map by type-asserting each adapter:

```go
	publishers := map[string]marketplace.ReplyPublisher{}
	for _, adapter := range adapters {
		if pub, ok := adapter.(marketplace.ReplyPublisher); ok {
			publishers[adapter.Marketplace()] = pub
		}
	}
```

- [ ] **Step 2: Inject into the server Config**

Add `ReplyPublishers: publishers,` to the `server.Config{...}` literal used to construct the server. (Find the existing `server.New(store, server.Config{...}, logger)` call.)

- [ ] **Step 3: Retry pending replies after each sync run**

Find the `TriggerSync` function literal assigned into the collector/config (the `go s.cfg.TriggerSync(...)` path originates here). After the runner completes a run, call `srv.RetryPendingReplies(ctx)`. Concretely, wrap the existing trigger so that after `runner.RunOnce(ctx, marketplaces)` it calls `srv.RetryPendingReplies(ctx)` using a background context. If the server variable is constructed after the trigger closure, restructure so the closure captures the server (declare `var srv *server.Server` first, assign before starting the scheduler).

- [ ] **Step 4: Verify build and full suite**

Run: `go build ./... && go test ./... -count=1`
Expected: PASS (pre-existing `internal/marketplace/ym` time-dependent fixture `TestFetchReviewsMapsYMResponse` may fail — that is unrelated to this work; confirm it is the only failure).

- [ ] **Step 5: Commit**

```bash
git add cmd/reviews/main.go
git commit -m "feat(main): inject reply publishers and retry pending replies after sync"
```

---

### Task 10: Admin UI — publish badge, retry button, per-MP toggle

**Files:**
- Modify: `web/admin/src/types.ts` (review type + marketplace type)
- Modify: `web/admin/src/pages/Reviews.tsx` (badge + retry button near the reply editor at lines ~535-543)
- Modify: `web/admin/src/pages/Marketplaces.tsx` (per-MP publish toggle)
- Regenerate: `internal/server/admin_dist/*`

**Interfaces:**
- Consumes: JSON `review.replyPublish` (`{state, error?, publishedAt?}`); settings endpoint keys `publish_replies_<mp>`.

- [ ] **Step 1: Extend the TS types**

In `web/admin/src/types.ts`, add to the admin review type:

```ts
  replyPublish?: { state: string; error?: string; publishedAt?: string }
```

- [ ] **Step 2: Show publish state + retry in the reply editor**

In `web/admin/src/pages/Reviews.tsx`, near the reply editor (the `reply-editor` block ~line 535), render the status under the "Сохранить ответ" button:

```tsx
{review.replyPublish && (
  <div className="reply-publish">
    {review.replyPublish.state === 'published' && <span className="status-ok">Опубликовано на МП</span>}
    {review.replyPublish.state === 'pending' && <span className="status-muted">Публикация…</span>}
    {review.replyPublish.state === 'unsupported' && <span className="status-muted">Публикация на МП недоступна</span>}
    {review.replyPublish.state === 'failed' && (
      <>
        <span className="status-warn">Ошибка публикации: {review.replyPublish.error}</span>
        <button className="secondary" onClick={() => retryPublish(review.id)}>Повторить</button>
      </>
    )}
  </div>
)}
```

Add the `retryPublish` handler near `saveReply` (line ~217):

```tsx
  async function retryPublish(id: number) {
    try {
      await apiWrite('POST', `/admin/api/reviews/${id}/reply/retry`)
      load()
    } catch (err) {
      setMessage(err instanceof Error ? err.message : 'Запрос не выполнен')
    }
  }
```

(Confirm `load` and `setMessage` exist in this component — they are used by `saveReply`; reuse them.)

- [ ] **Step 3: Per-MP publish toggle in Marketplaces page**

In `web/admin/src/pages/Marketplaces.tsx`, add a toggle that reads/writes the setting via the existing settings endpoint. Load current values on mount:

```tsx
  const [publish, setPublish] = useState<Record<string, boolean>>({})
  useEffect(() => {
    apiGet<Record<string, string>>('/admin/api/settings').then((data) => {
      setPublish({
        wb: data['publish_replies_wb'] === 'true',
        ym: data['publish_replies_ym'] === 'true',
        ozon: data['publish_replies_ozon'] === 'true',
      })
    }).catch(() => {})
  }, [])

  async function togglePublish(mp: string, value: boolean) {
    setPublish((p) => ({ ...p, [mp]: value }))
    await apiWrite('PUT', '/admin/api/settings', { [`publish_replies_${mp}`]: value ? 'true' : '' })
  }
```

Render a checkbox per marketplace row:

```tsx
<label className="inline-check">
  <input type="checkbox" checked={!!publish[item.id]} onChange={(e) => togglePublish(item.id, e.target.checked)} />
  <span>Публиковать ответы на МП</span>
</label>
```

⚠️ The settings `PUT` handler currently whitelists only `agreementUrl`/`shopOrigin`/`sitemapUrl` (see `internal/server/admin_settings.go`). **Before this step, extend `settingsRequest` and `handlePutSettings`/`loadSettings` to accept and persist the three `publish_replies_<mp>` keys** (mirror the existing field handling; values are `"true"`/`""`, no URL validation). Add this as a backend sub-step and a store round-trip assertion in `admin_settings_test.go`.

- [ ] **Step 4: Rebuild the embedded bundle and verify**

```bash
cd /home/mama/DEV/Reviews/web/admin && npm run build
cd /home/mama/DEV/Reviews && rm -rf internal/server/admin_dist && cp -r web/admin/dist internal/server/admin_dist
go build ./... && go test ./internal/server/ -count=1
```
Expected: build PASS; server tests PASS.

- [ ] **Step 5: Commit**

```bash
git add web/admin/src internal/server/admin_settings.go internal/server/admin_settings_test.go internal/server/admin_dist
git commit -m "feat(admin): reply publish status, retry button, per-MP publish toggle"
```

---

## Self-Review

- **Spec coverage:** D1 (all 3 MPs — Tasks 4-6) ✓; D2 (sync attempt + retry — Tasks 7-9) ✓; D3 (publish-once, no edit — no edit path added) ✓; D4 (per-MP toggle, Ozon off default — Tasks 3, 7, 10) ✓; publish-state model (Task 2) ✓; site → unsupported (Task 7) ✓.
- **Placeholder scan:** every code step shows full code; the two ⚠️ items (YM body shape, settings-whitelist extension) are explicit verification/sub-steps with the expected shape given, not hand-waving.
- **Type consistency:** `PublishReply(ctx, externalReviewID, text string) error` identical across Tasks 1/4/5/6; state strings `pending|published|failed|unsupported` consistent across Tasks 2/7/8/10; `ReplyPublishState/Error/PublishedAt` column names match between model (Task 2) and readers (Tasks 7/8); `replyPublish` JSON key matches between Go (Task 8) and TS (Task 10).
- **Risk note:** WB returns 204 without validating the feedback id, so `published` means "accepted by WB", not "verified visible" — acceptable for v1; documented here and in the WB risk row.
- **Pre-existing failure:** `internal/marketplace/ym` `TestFetchReviewsMapsYMResponse` fails independently of this work (time-dependent fixture); do not attempt to fix it here.
