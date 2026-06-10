# Stage 3: Review Management & Curation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the admin a dashboard, a filterable review list with moderation (hide/show, pin), marketplace connection status with manual sync, and configurable homepage showcase rules (auto-rules + manual pin/hide) served to the widget.

**Architecture:** Add `visibility`/`pinned` columns to `Review` and a `ShowcaseRule` model. Extend `ReviewListFilter` with the new admin filters. Apply the Stage 2 `requireCSRF` middleware to all state-changing admin endpoints. New authenticated admin API: dashboard stats, admin review list (with total count), review moderation, marketplace status + manual sync trigger, showcase-rule CRUD. New public endpoint `GET /api/showcase` applies rules + pins/hides. React pages: Dashboard, Reviews, Marketplaces, Showcase.

**Tech Stack:** Go 1.26, GORM, `net/http`, React + Vite + TS.

**Spec:** `docs/superpowers/specs/2026-06-09-reviews-admin-and-containerization-design.md` (FR-2, FR-3, FR-4, FR-5; uses Stage 2 CSRF security layer).

**Depends on:** Stage 2 (auth, `requireSession`, admin mux, store test helpers).

---

## File Structure

- Modify: `internal/store/models.go` — add `Visibility`, `Pinned` to `Review`.
- Modify: `internal/store/store.go` — add `ShowcaseRule` to migrations.
- Create: `internal/store/showcase_models.go` — `ShowcaseRule` model.
- Modify: `internal/store/list.go` — extend `ReviewListFilter`, add `ListReviewsWithCount`.
- Create: `internal/store/curation.go` — `SetReviewVisibility`, `SetReviewPinned`, `DashboardStats`, showcase queries.
- Create: `internal/store/curation_test.go`
- Create: `internal/server/admin_reviews.go` — review list + moderation handlers.
- Create: `internal/server/admin_dashboard.go` — stats + marketplace status + sync trigger.
- Create: `internal/server/admin_showcase.go` — showcase-rule handlers + public `/api/showcase`.
- Create: `internal/server/admin_reviews_test.go`
- Modify: `internal/server/server.go` — register the new routes.
- Modify: `cmd/reviews/main.go` — give the server a sync trigger function.
- Create: `web/admin/src/pages/{Dashboard,Reviews,Marketplaces,Showcase}.tsx` and a simple router.

---

## Task 1: Apply Stage 2 CSRF to write routes

**Files:**
- Modify: `internal/server/server.go`

- [ ] **Step 1: Confirm Stage 2 CSRF is present**

Run: `go test ./internal/server/ -run TestRequireCSRF -v`
Expected: PASS.

- [ ] **Step 2: Use `requireCSRF` for every state-changing admin route**

As each Stage 3 write endpoint is registered, wrap it with `requireCSRF(...)`.
The route registration examples later in this plan already do this for review
moderation, manual sync, and showcase-rule updates:

```go
protected.Handle("PATCH /admin/api/reviews/{id}", requireCSRF(http.HandlerFunc(s.handleAdminReviewModerate)))
protected.Handle("POST /admin/api/sync", requireCSRF(http.HandlerFunc(s.handleTriggerSync)))
protected.Handle("PUT /admin/api/showcase-rule", requireCSRF(http.HandlerFunc(s.handlePutShowcaseRule)))
```

- [ ] **Step 3: Keep reads CSRF-free**

`GET` endpoints remain protected by session auth where appropriate, but do not
require the CSRF header. The SPA obtains the token from `GET /admin/api/csrf`,
introduced in Stage 2, before sending write requests.

---

## Task 2: Review curation columns and filters

**Files:**
- Modify: `internal/store/models.go`
- Modify: `internal/store/list.go`
- Create: `internal/store/curation.go`
- Create: `internal/store/curation_test.go`
- Create: `internal/store/showcase_models.go`
- Modify: `internal/store/store.go`

- [ ] **Step 1: Add columns and model**

In `internal/store/models.go`, add to `Review` (after `Status`):

```go
	Visibility        string `gorm:"size:16;not null;default:visible;index"`
	Pinned            bool   `gorm:"not null;default:false;index"`
```

Create `internal/store/showcase_models.go`:

```go
package store

import "time"

// ShowcaseRule holds the auto-selection rules for the homepage showcase.
// Exactly one row per tenant.
type ShowcaseRule struct {
	ID           uint `gorm:"primaryKey"`
	TenantID     uint `gorm:"not null;default:1;uniqueIndex"`
	MinRating    int  `gorm:"not null;default:4"`
	RequirePhoto bool `gorm:"not null;default:false"`
	MinTextLen   int  `gorm:"not null;default:0"`
	MaxAgeDays   int  `gorm:"not null;default:0"` // 0 = no age limit
	SortBy       string `gorm:"size:16;not null;default:recent"` // recent|rating
	Limit        int  `gorm:"not null;default:12"`
	UpdatedAt    time.Time
}
```

In `internal/store/store.go`, add `&ShowcaseRule{}` to `AutoMigrate`.

- [ ] **Step 2: Extend the filter**

In `internal/store/list.go`, extend `ReviewListFilter`:

```go
type ReviewListFilter struct {
	Marketplace   string
	Rating        int
	Limit         int
	Offset        int
	Visibility    string // "", "visible", "hidden"
	SellerArticle string
	HasPhoto      bool
	Search        string // matches text/author/pros/cons
	PinnedFirst   bool
}
```

In `ListReviews`, after the existing `Rating` filter block, add:

```go
	if filter.Visibility != "" {
		query = query.Where("visibility = ?", filter.Visibility)
	}
	if filter.SellerArticle != "" {
		query = query.Where("seller_article = ?", filter.SellerArticle)
	}
	if filter.HasPhoto {
		query = query.Where("id IN (?)",
			s.db.Model(&ReviewMedia{}).Select("review_id").Where("kind = ?", "photo"))
	}
	if filter.Search != "" {
		like := "%" + filter.Search + "%"
		query = query.Where(
			"text LIKE ? OR author_name LIKE ? OR pros LIKE ? OR cons LIKE ?",
			like, like, like, like)
	}
```

Change the `Order` so pinned can float to the top when requested. Replace the static `Order("created_at_mp desc")` with conditional ordering built before `Find`:

```go
	if filter.PinnedFirst {
		query = query.Order("pinned desc")
	}
	query = query.Order("created_at_mp desc")
```

(Remove the `.Order("created_at_mp desc")` from the initial chain and apply it here instead.)

- [ ] **Step 3: Write the failing test for curation + count + stats**

Create `internal/store/curation_test.go`:

```go
package store

import (
	"context"
	"testing"
	"time"
)

func seedReview(t *testing.T, st *Store, mp, extID string, rating int) Review {
	t.Helper()
	r := Review{
		Marketplace: mp, ExternalReviewID: extID, ExternalProductID: "p1",
		Rating: &rating, Text: "good", CreatedAtMP: time.Now(), FetchedAt: time.Now(),
		Visibility: "visible",
	}
	if err := st.db.WithContext(context.Background()).Create(&r).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	return r
}

func TestSetVisibilityAndPinned(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	r := seedReview(t, st, "wb", "w1", 5)

	if err := st.SetReviewVisibility(ctx, r.ID, "hidden"); err != nil {
		t.Fatalf("hide: %v", err)
	}
	if err := st.SetReviewPinned(ctx, r.ID, true); err != nil {
		t.Fatalf("pin: %v", err)
	}

	hidden, _ := st.ListReviews(ctx, ReviewListFilter{Visibility: "hidden"})
	if len(hidden) != 1 || !hidden[0].Pinned {
		t.Fatalf("expected 1 hidden pinned review, got %+v", hidden)
	}
}

func TestListReviewsWithCount(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedReview(t, st, "wb", "w1", 5)
	seedReview(t, st, "ym", "y1", 3)

	items, total, err := st.ListReviewsWithCount(ctx, ReviewListFilter{Limit: 1})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected total 2, got %d", total)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item (limit), got %d", len(items))
	}
}

func TestDashboardStats(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedReview(t, st, "wb", "w1", 5)
	seedReview(t, st, "wb", "w2", 3)
	seedReview(t, st, "ym", "y1", 4)

	stats, err := st.DashboardStats(ctx)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.TotalReviews != 3 {
		t.Fatalf("expected 3 total, got %d", stats.TotalReviews)
	}
	if stats.ByMarketplace["wb"] != 2 || stats.ByMarketplace["ym"] != 1 {
		t.Fatalf("unexpected per-marketplace counts: %+v", stats.ByMarketplace)
	}
	if stats.AverageRating < 3.9 || stats.AverageRating > 4.1 {
		t.Fatalf("expected avg ~4.0, got %v", stats.AverageRating)
	}
}
```

(`newTestStore` is defined in Stage 2's `internal/store/auth_test.go`.)

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./internal/store/ -run 'TestSetVisibility|TestListReviewsWithCount|TestDashboardStats' -v`
Expected: FAIL — undefined methods.

- [ ] **Step 5: Implement curation + stats**

Create `internal/store/curation.go`:

```go
package store

import "context"

func (s *Store) SetReviewVisibility(ctx context.Context, id uint, visibility string) error {
	return s.db.WithContext(ctx).Model(&Review{}).
		Where("id = ?", id).Update("visibility", visibility).Error
}

func (s *Store) SetReviewPinned(ctx context.Context, id uint, pinned bool) error {
	return s.db.WithContext(ctx).Model(&Review{}).
		Where("id = ?", id).Update("pinned", pinned).Error
}

// ListReviewsWithCount returns a page of reviews plus the total matching count
// (ignoring limit/offset) for pagination.
func (s *Store) ListReviewsWithCount(ctx context.Context, filter ReviewListFilter) ([]Review, int64, error) {
	items, err := s.ListReviews(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	countFilter := filter
	countFilter.Limit, countFilter.Offset = 0, 0
	var total int64
	q := s.db.WithContext(ctx).Model(&Review{})
	if filter.Marketplace != "" && filter.Marketplace != "all" {
		q = q.Where("marketplace = ?", filter.Marketplace)
	}
	if filter.Visibility != "" {
		q = q.Where("visibility = ?", filter.Visibility)
	}
	if filter.SellerArticle != "" {
		q = q.Where("seller_article = ?", filter.SellerArticle)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

type Stats struct {
	TotalReviews  int64
	AverageRating float64
	ByMarketplace map[string]int64
}

func (s *Store) DashboardStats(ctx context.Context) (Stats, error) {
	stats := Stats{ByMarketplace: map[string]int64{}}
	db := s.db.WithContext(ctx)
	if err := db.Model(&Review{}).Count(&stats.TotalReviews).Error; err != nil {
		return Stats{}, err
	}
	var avg *float64
	if err := db.Model(&Review{}).Select("AVG(rating)").Scan(&avg).Error; err != nil {
		return Stats{}, err
	}
	if avg != nil {
		stats.AverageRating = *avg
	}
	rows := []struct {
		Marketplace string
		N           int64
	}{}
	if err := db.Model(&Review{}).
		Select("marketplace, COUNT(*) as n").Group("marketplace").Scan(&rows).Error; err != nil {
		return Stats{}, err
	}
	for _, row := range rows {
		stats.ByMarketplace[row.Marketplace] = row.N
	}
	return stats, nil
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/store/ -v`
Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/store/models.go internal/store/list.go internal/store/curation.go internal/store/curation_test.go internal/store/showcase_models.go internal/store/store.go
git commit -m "feat(store): review curation, paginated list, dashboard stats, showcase rule"
```

---

## Task 3: Admin review list and moderation endpoints

**Files:**
- Create: `internal/server/admin_reviews.go`
- Create: `internal/server/admin_reviews_test.go`
- Modify: `internal/server/server.go`

- [ ] **Step 1: Write the failing test**

Create `internal/server/admin_reviews_test.go`. It logs in (reusing Stage 2 flow helpers), grabs the session cookie, then lists and moderates:

```go
package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"reviews/internal/config"
	"reviews/internal/store"
)

func authedServer(t *testing.T) (*Server, *http.Cookie) {
	t.Helper()
	st, _ := store.Open(config.DBConfig{Driver: "sqlite", DSN: "file::memory:?cache=shared"})
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	s := New(st, Config{SessionTTL: time.Hour}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	mux := s.adminMux()

	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/admin/api/setup",
		strings.NewReader(`{"login":"a","password":"password1"}`)))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/api/login",
		strings.NewReader(`{"login":"a","password":"password1"}`)))
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName {
			return s, c
		}
	}
	t.Fatal("no session cookie")
	return nil, nil
}

func TestAdminReviewsRequiresAuth(t *testing.T) {
	s, _ := authedServer(t)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/api/reviews", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without session, got %d", rec.Code)
	}
}

func TestAdminReviewsListAndHide(t *testing.T) {
	s, cookie := authedServer(t)
	seedReview(t, s.store, "wb", "w1", 5) // helper from store package? define local

	req := httptest.NewRequest(http.MethodGet, "/admin/api/reviews", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"total"`) {
		t.Fatalf("expected total in body: %s", rec.Body.String())
	}
}
```

Note: `seedReview` lives in the `store` package test file and is not importable here. Add a local helper in `admin_reviews_test.go` that inserts a review via a new exported store method, OR call `s.store` through a small exported `InsertReviewForTest`. Simplest: add an exported test seed via the public upsert path — call `s.store.UpsertReview(ctx, marketplace.Review{...})`. Replace the `seedReview` line with:

```go
	_, _ = s.store.UpsertReview(context.Background(), marketplace.Review{
		Marketplace: "wb", ExternalReviewID: "w1", ExternalProductID: "p1",
		Rating: 5, Text: "good", CreatedAtMP: time.Now(),
	})
```

and import `"reviews/internal/marketplace"`. (Check `marketplace.Review` field names against `internal/marketplace/model.go` before finalizing; adjust the literal to match.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestAdminReviews -v`
Expected: FAIL — routes not registered.

- [ ] **Step 3: Implement the handlers**

Create `internal/server/admin_reviews.go`:

```go
package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"reviews/internal/reviewjson"
	"reviews/internal/store"
)

type adminReviewsResponse struct {
	Reviews []reviewjson.Review `json:"reviews"`
	Total   int64               `json:"total"`
}

func (s *Server) handleAdminReviews(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := store.ReviewListFilter{
		Marketplace:   q.Get("marketplace"),
		Visibility:    q.Get("visibility"),
		SellerArticle: q.Get("article"),
		Search:        q.Get("search"),
		HasPhoto:      q.Get("has_photo") == "true",
		Limit:         parseInt(q.Get("limit"), 50),
		Offset:        parseInt(q.Get("offset"), 0),
		PinnedFirst:   true,
	}
	if rating := q.Get("rating"); rating != "" && rating != "all" {
		filter.Rating = parseInt(rating, 0)
	}

	reviews, total, err := s.store.ListReviewsWithCount(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	mapper := reviewjson.Mapper{ProductURLTemplate: s.cfg.ProductURLTemplate, ProductLinks: s.cfg.ProductLinks}
	items := make([]reviewjson.Review, 0, len(reviews))
	for _, rv := range reviews {
		items = append(items, mapper.ToReview(rv))
	}
	writeJSON(w, http.StatusOK, adminReviewsResponse{Reviews: items, Total: total})
}

type moderationRequest struct {
	Visibility *string `json:"visibility"`
	Pinned     *bool   `json:"pinned"`
}

func (s *Server) handleAdminReviewModerate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid review id"))
		return
	}
	var req moderationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid body"))
		return
	}
	if req.Visibility != nil {
		if *req.Visibility != "visible" && *req.Visibility != "hidden" {
			writeError(w, http.StatusBadRequest, errors.New("visibility must be visible or hidden"))
			return
		}
		if err := s.store.SetReviewVisibility(r.Context(), uint(id), *req.Visibility); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	if req.Pinned != nil {
		if err := s.store.SetReviewPinned(r.Context(), uint(id), *req.Pinned); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
```

- [ ] **Step 4: Register protected routes**

In `internal/server/server.go` `adminMux`, replace the single protected route with a protected sub-mux carrying all authenticated endpoints, and wrap writes with CSRF:

```go
	protected := http.NewServeMux()
	protected.HandleFunc("GET /admin/api/me", s.handleMe)
	protected.HandleFunc("GET /admin/api/csrf", s.handleCSRFToken)
	protected.HandleFunc("GET /admin/api/reviews", s.handleAdminReviews)
	protected.Handle("PATCH /admin/api/reviews/{id}", requireCSRF(http.HandlerFunc(s.handleAdminReviewModerate)))
	protected.HandleFunc("GET /admin/api/dashboard", s.handleDashboard)
	protected.HandleFunc("GET /admin/api/marketplaces", s.handleMarketplaces)
	protected.Handle("POST /admin/api/sync", requireCSRF(http.HandlerFunc(s.handleTriggerSync)))
	protected.HandleFunc("GET /admin/api/showcase-rule", s.handleGetShowcaseRule)
	protected.Handle("PUT /admin/api/showcase-rule", requireCSRF(http.HandlerFunc(s.handlePutShowcaseRule)))

	mux.Handle("/admin/api/", s.requireSession(protected))
```

Keep the public `setup-status`/`setup`/`login`/`logout` handlers registered on `mux` BEFORE the `/admin/api/` catch-all so they stay unauthenticated. With Go 1.22 routing, more specific patterns win, so explicit `POST /admin/api/login` etc. take precedence over `/admin/api/`.

Also register the public showcase endpoint in `Run` alongside `/api/reviews`:

```go
	mux.HandleFunc("GET /api/showcase", s.handleShowcase)
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/server/ -run TestAdminReviews -v`
Expected: PASS. (Dashboard/marketplace/showcase handlers come in Tasks 4–5; if compiling before them, temporarily comment their route lines or implement Tasks 4–5 first. Recommended: implement Tasks 4–5, then run the full `go test ./internal/server/`.)

- [ ] **Step 6: Commit (after Tasks 4–5 compile)**

```bash
git add internal/server/admin_reviews.go internal/server/admin_reviews_test.go internal/server/server.go
git commit -m "feat(admin): review list and moderation endpoints"
```

---

## Task 4: Dashboard, marketplace status, manual sync

**Files:**
- Create: `internal/server/admin_dashboard.go`
- Modify: `internal/server/server.go` (Config gains a sync trigger)
- Modify: `cmd/reviews/main.go`

- [ ] **Step 1: Add a sync trigger hook to the server**

In `internal/server/server.go`, add to `Config`:

```go
	// TriggerSync runs a one-off sync for the given marketplaces (empty = all
	// enabled). Injected by the serve command. May be nil (sync disabled).
	TriggerSync func(marketplaces []string)
	// Marketplaces lists configured marketplaces and whether creds are present.
	Marketplaces []MarketplaceStatus
```

Add the status type to `internal/server/admin_dashboard.go`:

```go
package server

import (
	"net/http"
)

type MarketplaceStatus struct {
	ID         string `json:"id"`
	Enabled    bool   `json:"enabled"`
	Configured bool   `json:"configured"`
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.DashboardStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	runs, err := s.store.RecentSyncRuns(r.Context(), 10)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total_reviews":  stats.TotalReviews,
		"average_rating": stats.AverageRating,
		"by_marketplace": stats.ByMarketplace,
		"recent_syncs":   runs,
	})
}

func (s *Server) handleMarketplaces(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"marketplaces": s.cfg.Marketplaces})
}

func (s *Server) handleTriggerSync(w http.ResponseWriter, r *http.Request) {
	if s.cfg.TriggerSync == nil {
		writeError(w, http.StatusServiceUnavailable, errSyncDisabled)
		return
	}
	mp := r.URL.Query().Get("marketplace")
	var list []string
	if mp != "" {
		list = []string{mp}
	}
	go s.cfg.TriggerSync(list)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}
```

Add `var errSyncDisabled = errors.New("sync is disabled")` and the `"errors"` import.

- [ ] **Step 2: Add `RecentSyncRuns` to the store**

In `internal/store/curation.go` (or a new `sync_runs.go`), add:

```go
func (s *Store) RecentSyncRuns(ctx context.Context, limit int) ([]SyncRun, error) {
	var runs []SyncRun
	err := s.db.WithContext(ctx).Order("started_at desc").Limit(limit).Find(&runs).Error
	return runs, err
}
```

- [ ] **Step 3: Wire the trigger and statuses from `serve`**

In `cmd/reviews/main.go` `runServe`, before constructing `server.New`, build the runner and a trigger closure, and compute statuses:

```go
	runner := collector.NewRunner(db, cfg.Sync, logger, buildAdapters(cfg))
	trigger := func(marketplaces []string) {
		if len(marketplaces) == 0 {
			marketplaces = cfg.EnabledMarketplaces()
		}
		for _, res := range runner.RunOnce(ctx, marketplaces) {
			if res.Error != nil {
				logger.Error("manual sync failed", "marketplace", res.Marketplace, "error", res.Error)
			}
		}
	}
	statuses := marketplaceStatuses(cfg)
```

Add to the `server.Config` literal: `TriggerSync: trigger, Marketplaces: statuses,`. When `--with-sync` is set, reuse the same `runner` for the scheduler instead of building a second one.

Add the helper at the bottom of `main.go`:

```go
func marketplaceStatuses(cfg config.Config) []server.MarketplaceStatus {
	return []server.MarketplaceStatus{
		{ID: config.MarketplaceWB, Enabled: cfg.Marketplaces.WB.Enabled, Configured: cfg.Marketplaces.WB.Token != ""},
		{ID: config.MarketplaceYM, Enabled: cfg.Marketplaces.YM.Enabled, Configured: cfg.Marketplaces.YM.BusinessID != "" && (cfg.Marketplaces.YM.APIKey != "" || cfg.Marketplaces.YM.OAuthToken != "")},
		{ID: config.MarketplaceOzon, Enabled: cfg.Marketplaces.Ozon.Enabled, Configured: cfg.Marketplaces.Ozon.ClientID != "" && cfg.Marketplaces.Ozon.APIKey != ""},
	}
}
```

Add `"reviews/internal/server"` import if not already present (it is).

- [ ] **Step 4: Build and test**

Run: `go build ./... && go test ./internal/server/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/server/admin_dashboard.go internal/store/curation.go cmd/reviews/main.go internal/server/server.go
git commit -m "feat(admin): dashboard stats, marketplace status, manual sync trigger"
```

---

## Task 5: Showcase rules + public showcase endpoint

**Files:**
- Create: `internal/server/admin_showcase.go`
- Modify: `internal/store/curation.go`
- Create: store test for showcase query

- [ ] **Step 1: Add store methods for showcase rule + selection**

In `internal/store/curation.go`, add:

```go
func (s *Store) GetShowcaseRule(ctx context.Context) (ShowcaseRule, error) {
	var rule ShowcaseRule
	err := s.db.WithContext(ctx).Where("tenant_id = ?", DefaultTenantID).First(&rule).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ShowcaseRule{TenantID: DefaultTenantID, MinRating: 4, SortBy: "recent", Limit: 12}, nil
	}
	return rule, err
}

func (s *Store) SaveShowcaseRule(ctx context.Context, rule ShowcaseRule) error {
	rule.TenantID = DefaultTenantID
	return s.db.WithContext(ctx).
		Where("tenant_id = ?", DefaultTenantID).
		Assign(rule).
		FirstOrCreate(&rule).Error
}

// ShowcaseReviews applies the rule plus manual pins/hides: pinned visible
// reviews first, then auto-selected visible reviews matching the rule.
func (s *Store) ShowcaseReviews(ctx context.Context, rule ShowcaseRule) ([]Review, error) {
	q := s.db.WithContext(ctx).
		Preload("Media", func(db *gorm.DB) *gorm.DB { return db.Order("position asc") }).
		Where("visibility = ?", "visible")
	if rule.MinRating > 0 {
		q = q.Where("rating >= ?", rule.MinRating)
	}
	if rule.RequirePhoto {
		q = q.Where("id IN (?)", s.db.Model(&ReviewMedia{}).Select("review_id").Where("kind = ?", "photo"))
	}
	if rule.MinTextLen > 0 {
		q = q.Where("LENGTH(text) >= ?", rule.MinTextLen)
	}
	if rule.MaxAgeDays > 0 {
		cutoff := timeNowUTC().AddDate(0, 0, -rule.MaxAgeDays)
		q = q.Where("created_at_mp >= ?", cutoff)
	}
	q = q.Order("pinned desc")
	if rule.SortBy == "rating" {
		q = q.Order("rating desc")
	}
	q = q.Order("created_at_mp desc")
	limit := rule.Limit
	if limit <= 0 || limit > 100 {
		limit = 12
	}
	var reviews []Review
	err := q.Limit(limit).Find(&reviews).Error
	return reviews, err
}
```

Add a small indirection for the cutoff so it is testable, at the bottom of `curation.go`:

```go
import "time" // add to existing import block
var timeNowUTC = func() time.Time { return time.Now().UTC() }
```

(Merge the `time`, `errors`, and `gorm` imports into the existing import block of `curation.go`.)

- [ ] **Step 2: Implement handlers**

Create `internal/server/admin_showcase.go`:

```go
package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"reviews/internal/reviewjson"
	"reviews/internal/store"
)

func (s *Server) handleGetShowcaseRule(w http.ResponseWriter, r *http.Request) {
	rule, err := s.store.GetShowcaseRule(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (s *Server) handlePutShowcaseRule(w http.ResponseWriter, r *http.Request) {
	var rule store.ShowcaseRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid body"))
		return
	}
	if err := s.store.SaveShowcaseRule(r.Context(), rule); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleShowcase is the PUBLIC endpoint the homepage widget calls.
func (s *Server) handleShowcase(w http.ResponseWriter, r *http.Request) {
	rule, err := s.store.GetShowcaseRule(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	reviews, err := s.store.ShowcaseReviews(r.Context(), rule)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	mapper := reviewjson.Mapper{ProductURLTemplate: s.cfg.ProductURLTemplate, ProductLinks: s.cfg.ProductLinks}
	items := make([]reviewjson.Review, 0, len(reviews))
	for _, rv := range reviews {
		items = append(items, mapper.ToReview(rv))
	}
	writeJSON(w, http.StatusOK, reviewsResponse{Reviews: items, Count: len(items)})
}
```

- [ ] **Step 3: Build and run the full suite**

Run: `go build ./... && go test ./...`
Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/server/admin_showcase.go internal/store/curation.go
git commit -m "feat(admin): showcase rules and public showcase endpoint"
```

---

## Task 6: Admin SPA pages

**Files:**
- Create: `web/admin/src/api.ts` — typed fetch wrapper with CSRF.
- Create: `web/admin/src/pages/Dashboard.tsx`, `Reviews.tsx`, `Marketplaces.tsx`, `Showcase.tsx`
- Modify: `web/admin/src/App.tsx` — add a minimal hash router and nav.

- [ ] **Step 1: Create the API client with CSRF handling**

Create `web/admin/src/api.ts`:

```ts
let csrf = ''

async function ensureCSRF() {
  if (csrf) return csrf
  const res = await fetch('/admin/api/csrf')
  csrf = (await res.json()).csrf_token
  return csrf
}

export async function apiGet<T>(path: string): Promise<T> {
  const res = await fetch(path)
  if (!res.ok) throw new Error((await res.json()).error ?? 'request failed')
  return res.json()
}

export async function apiWrite<T>(method: string, path: string, body?: unknown): Promise<T> {
  const token = await ensureCSRF()
  const res = await fetch(path, {
    method,
    headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': token },
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) throw new Error((await res.json()).error ?? 'request failed')
  return res.json()
}
```

- [ ] **Step 2: Dashboard page**

Create `web/admin/src/pages/Dashboard.tsx`:

```tsx
import { useEffect, useState } from 'react'
import { apiGet } from '../api'

type Dash = {
  total_reviews: number
  average_rating: number
  by_marketplace: Record<string, number>
  recent_syncs: { Marketplace: string; Status: string; StartedAt: string }[]
}

export default function Dashboard() {
  const [d, setD] = useState<Dash | null>(null)
  useEffect(() => {
    apiGet<Dash>('/admin/api/dashboard').then(setD).catch(console.error)
  }, [])
  if (!d) return <p>Loading…</p>
  return (
    <section>
      <h2>Dashboard</h2>
      <p>Total reviews: {d.total_reviews}</p>
      <p>Average rating: {d.average_rating.toFixed(2)}</p>
      <ul>{Object.entries(d.by_marketplace).map(([m, n]) => <li key={m}>{m}: {n}</li>)}</ul>
      <h3>Recent syncs</h3>
      <ul>{d.recent_syncs.map((s, i) => <li key={i}>{s.Marketplace} — {s.Status}</li>)}</ul>
    </section>
  )
}
```

- [ ] **Step 3: Reviews page (list + hide/pin)**

Create `web/admin/src/pages/Reviews.tsx`:

```tsx
import { useEffect, useState } from 'react'
import { apiGet, apiWrite } from '../api'

type Review = { id: number; marketplace: string; rating: number; text: string; author: string }
type ListResp = { reviews: Review[]; total: number }

export default function Reviews() {
  const [data, setData] = useState<ListResp>({ reviews: [], total: 0 })
  const [marketplace, setMarketplace] = useState('')

  function load() {
    const q = new URLSearchParams()
    if (marketplace) q.set('marketplace', marketplace)
    apiGet<ListResp>('/admin/api/reviews?' + q.toString()).then(setData).catch(console.error)
  }
  useEffect(load, [marketplace])

  async function moderate(id: number, body: { visibility?: string; pinned?: boolean }) {
    await apiWrite('PATCH', `/admin/api/reviews/${id}`, body)
    load()
  }

  return (
    <section>
      <h2>Reviews ({data.total})</h2>
      <select value={marketplace} onChange={(e) => setMarketplace(e.target.value)}>
        <option value="">all</option><option value="wb">wb</option><option value="ym">ym</option>
      </select>
      <table>
        <tbody>
          {data.reviews.map((r) => (
            <tr key={r.id}>
              <td>{r.marketplace}</td><td>{r.rating}★</td><td>{r.text}</td>
              <td>
                <button onClick={() => moderate(r.id, { visibility: 'hidden' })}>Hide</button>
                <button onClick={() => moderate(r.id, { pinned: true })}>Pin</button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  )
}
```

Note: the field names (`id`, `marketplace`, `rating`, `text`, `author`) must match the JSON produced by `reviewjson.Mapper.ToReview`. Inspect `internal/reviewjson/reviewjson.go` and align the TS type to the actual JSON tags before finalizing.

- [ ] **Step 4: Marketplaces page (status + trigger sync)**

Create `web/admin/src/pages/Marketplaces.tsx`:

```tsx
import { useEffect, useState } from 'react'
import { apiGet, apiWrite } from '../api'

type MP = { id: string; enabled: boolean; configured: boolean }

export default function Marketplaces() {
  const [list, setList] = useState<MP[]>([])
  useEffect(() => {
    apiGet<{ marketplaces: MP[] }>('/admin/api/marketplaces').then((d) => setList(d.marketplaces))
  }, [])
  return (
    <section>
      <h2>Marketplaces</h2>
      <ul>
        {list.map((m) => (
          <li key={m.id}>
            {m.id}: {m.enabled ? 'enabled' : 'disabled'}, {m.configured ? 'configured' : 'no credentials'}
          </li>
        ))}
      </ul>
      <button onClick={() => apiWrite('POST', '/admin/api/sync')}>Sync now</button>
    </section>
  )
}
```

- [ ] **Step 5: Showcase page (rule form)**

Create `web/admin/src/pages/Showcase.tsx`:

```tsx
import { useEffect, useState } from 'react'
import { apiGet, apiWrite } from '../api'

type Rule = {
  MinRating: number; RequirePhoto: boolean; MinTextLen: number
  MaxAgeDays: number; SortBy: string; Limit: number
}

export default function Showcase() {
  const [rule, setRule] = useState<Rule | null>(null)
  useEffect(() => { apiGet<Rule>('/admin/api/showcase-rule').then(setRule) }, [])
  if (!rule) return <p>Loading…</p>
  const set = (k: keyof Rule, v: number | boolean | string) => setRule({ ...rule, [k]: v })
  return (
    <section>
      <h2>Homepage showcase</h2>
      <label>Min rating <input type="number" min={1} max={5} value={rule.MinRating}
        onChange={(e) => set('MinRating', Number(e.target.value))} /></label>
      <label><input type="checkbox" checked={rule.RequirePhoto}
        onChange={(e) => set('RequirePhoto', e.target.checked)} /> require photo</label>
      <label>Limit <input type="number" value={rule.Limit}
        onChange={(e) => set('Limit', Number(e.target.value))} /></label>
      <button onClick={() => apiWrite('PUT', '/admin/api/showcase-rule', rule)}>Save</button>
    </section>
  )
}
```

- [ ] **Step 6: Router + nav in App.tsx**

Update the `authed` branch of `web/admin/src/App.tsx` to render a hash-based nav:

```tsx
import Dashboard from './pages/Dashboard'
import Reviews from './pages/Reviews'
import Marketplaces from './pages/Marketplaces'
import Showcase from './pages/Showcase'

function AdminShell() {
  const [route, setRoute] = useState(window.location.hash || '#dashboard')
  useEffect(() => {
    const on = () => setRoute(window.location.hash || '#dashboard')
    window.addEventListener('hashchange', on)
    return () => window.removeEventListener('hashchange', on)
  }, [])
  return (
    <div style={{ display: 'flex', gap: 24, fontFamily: 'system-ui', padding: 24 }}>
      <nav style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
        <a href="#dashboard">Dashboard</a>
        <a href="#reviews">Reviews</a>
        <a href="#marketplaces">Marketplaces</a>
        <a href="#showcase">Showcase</a>
        <button onClick={() => fetch('/admin/api/logout', { method: 'POST' }).then(() => location.reload())}>Logout</button>
      </nav>
      <main style={{ flex: 1 }}>
        {route === '#reviews' && <Reviews />}
        {route === '#marketplaces' && <Marketplaces />}
        {route === '#showcase' && <Showcase />}
        {(route === '#dashboard' || route === '') && <Dashboard />}
      </main>
    </div>
  )
}
```

Render `<AdminShell />` instead of `<h1>Reviews admin</h1>` when `mode === 'authed'`.

- [ ] **Step 7: Build and verify**

Run: `cd web/admin && ./build-embed.sh && cd ../.. && go build ./...`
Expected: SPA builds, embed succeeds, Go builds.

- [ ] **Step 8: Manual end-to-end check**

```bash
REVIEWS_INSECURE_COOKIES=1 go run ./cmd/reviews serve --addr 127.0.0.1:8080 --with-sync &
sleep 1
# browser: http://127.0.0.1:8080/admin/ → setup → dashboard, reviews list, hide/pin, sync now
kill %1
```
Expected: all pages load, moderation buttons mutate state (verify a hidden review disappears from a `visibility=visible` filter), `/api/showcase` returns curated JSON.

- [ ] **Step 9: Commit**

```bash
git add web/admin internal/server/admin_dist
git commit -m "feat(admin): dashboard, reviews, marketplaces, and showcase SPA pages"
```

---

## Self-Review Notes

- **Spec coverage:** FR-2 dashboard (Task 4 + Task 6), FR-3 marketplace status + manual sync + sync history (Task 4); token EDITING is intentionally deferred (needs encrypted-secrets work, out of scope per spec). FR-4 list/filter/moderation (Tasks 2,3,6). FR-5 homepage auto-rules + manual pin/hide + public delivery (Tasks 2,5); product-card per-article settings are part of the widget config in Stage 4 (flagged). CSRF (Task 1).
- **Type consistency:** `ReviewListFilter` extended once (Task 2) and used in Tasks 3/5. `ShowcaseRule` fields match between model (Task 2), store methods (Task 5), handlers (Task 5), and the SPA form (Task 6). `Stats`/`DashboardStats` consistent between Task 2 and Task 4.
- **Known follow-ups to verify during implementation:** align TS review type with `reviewjson` JSON tags (Task 3/6 notes); confirm `marketplace.Review` literal in the test (Task 3 note).
