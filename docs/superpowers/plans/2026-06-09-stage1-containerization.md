# Stage 1: Containerization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a self-contained Docker image and `docker-compose` setup so a third party can run the reviews service with one command, with review sync running inside the server process on an interval.

**Architecture:** Add an in-process scheduler (`internal/scheduler`) that calls the existing `collector.Runner.RunOnce` on `Sync.Interval`. Wire it into the `serve` command behind a `--with-sync` flag so the container needs no systemd timer. Package everything in a multi-stage Dockerfile (Go builder → distroless static final), with SQLite living on a named volume. Provide `docker-compose.yml` and `.env.example` for easy configuration.

**Tech Stack:** Go 1.26, `CGO_ENABLED=0` static build, Docker multi-stage, `gcr.io/distroless/static`, docker-compose v2.

**Spec:** `docs/superpowers/specs/2026-06-09-reviews-admin-and-containerization-design.md` (Part "Контейнеризация").

---

## File Structure

- Create: `internal/scheduler/scheduler.go` — in-process periodic sync runner (replaces the `doc.go` stub's promise).
- Modify: `cmd/reviews/main.go` — add `--with-sync` flag to `serve`, start the scheduler goroutine.
- Create: `internal/scheduler/scheduler_test.go` — unit tests for the scheduler.
- Delete: `internal/scheduler/doc.go` — superseded by `scheduler.go` (keeps the package single-purpose).
- Create: `Dockerfile` — multi-stage build.
- Create: `.dockerignore` — keep build context small and secrets out.
- Create: `docker-compose.yml` — service + named volume + env_file.
- Create: `.env.example` — documented configuration template.
- Modify: `deploy/README.md` — document the container workflow.

---

## Task 1: In-process scheduler package

**Files:**
- Create: `internal/scheduler/scheduler.go`
- Create: `internal/scheduler/scheduler_test.go`
- Delete: `internal/scheduler/doc.go`

- [ ] **Step 1: Write the failing test**

Create `internal/scheduler/scheduler_test.go`:

```go
package scheduler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type fakeRunner struct {
	calls   atomic.Int32
	called  chan struct{}
	lastIDs []string
}

func (f *fakeRunner) RunOnce(ctx context.Context, marketplaces []string) {
	f.lastIDs = marketplaces
	f.calls.Add(1)
	select {
	case f.called <- struct{}{}:
	default:
	}
}

func TestSchedulerRunsImmediatelyThenOnInterval(t *testing.T) {
	runner := &fakeRunner{called: make(chan struct{}, 8)}
	s := New(runner, 5*time.Millisecond, []string{"wb"}, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.Run(ctx)
		close(done)
	}()

	// Immediate first run.
	select {
	case <-runner.called:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not run immediately")
	}
	// At least one interval-triggered run.
	select {
	case <-runner.called:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not run on interval")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not stop on context cancel")
	}

	if runner.calls.Load() < 2 {
		t.Fatalf("expected at least 2 runs, got %d", runner.calls.Load())
	}
	if len(runner.lastIDs) != 1 || runner.lastIDs[0] != "wb" {
		t.Fatalf("expected marketplaces [wb], got %v", runner.lastIDs)
	}
}

func TestSchedulerStopsBeforeFirstRunIfCancelled(t *testing.T) {
	runner := &fakeRunner{called: make(chan struct{}, 1)}
	s := New(runner, time.Hour, []string{"wb"}, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before Run.

	done := make(chan struct{})
	go func() {
		s.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not return on pre-cancelled context")
	}
}
```

Add a small logger helper at the bottom of the same file:

```go
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
```

And imports for the helper: add `"io"` and `"log/slog"` to the test's import block.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/scheduler/ -run TestScheduler -v`
Expected: FAIL — `undefined: New` (and `undefined: testLogger` until helper compiles).

- [ ] **Step 3: Write minimal implementation**

Delete the stub: `rm internal/scheduler/doc.go`

Create `internal/scheduler/scheduler.go`:

```go
// Package scheduler runs collector jobs on a configured interval, in-process,
// so the containerized server needs no external cron or systemd timer.
package scheduler

import (
	"context"
	"log/slog"
	"time"
)

// Runner is the subset of collector.Runner the scheduler depends on.
type Runner interface {
	RunOnce(ctx context.Context, marketplaces []string)
}

type Scheduler struct {
	runner       Runner
	interval     time.Duration
	marketplaces []string
	logger       *slog.Logger
}

func New(runner Runner, interval time.Duration, marketplaces []string, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		runner:       runner,
		interval:     interval,
		marketplaces: marketplaces,
		logger:       logger,
	}
}

// Run blocks until ctx is cancelled. It runs one sync immediately, then once
// per interval.
func (s *Scheduler) Run(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	s.runOnce(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runOnce(ctx)
		}
	}
}

func (s *Scheduler) runOnce(ctx context.Context) {
	s.logger.Info("scheduled sync starting", "marketplaces", s.marketplaces)
	s.runner.RunOnce(ctx, s.marketplaces)
}
```

Note: `collector.Runner.RunOnce` returns `[]collector.Result`. The `scheduler.Runner` interface intentionally ignores the return value. To bridge the type mismatch, Task 2 wraps the collector runner in a thin adapter that logs results and discards them.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/scheduler/ -run TestScheduler -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/scheduler/scheduler.go internal/scheduler/scheduler_test.go
git rm internal/scheduler/doc.go
git commit -m "feat(scheduler): in-process periodic sync runner"
```

---

## Task 2: Wire scheduler into the `serve` command

**Files:**
- Modify: `cmd/reviews/main.go` (the `runServe` function, lines ~145-183, and add a helper)

- [ ] **Step 1: Add the `--with-sync` flag and scheduler wiring**

In `cmd/reviews/main.go`, add `"reviews/internal/collector"` (already imported) and `"reviews/internal/scheduler"` to the import block (collector is already imported; add only scheduler).

Replace the body of `runServe` so it starts the scheduler before serving. The new `runServe`:

```go
func runServe(ctx context.Context, args []string, cfg config.Config, logger *slog.Logger) int {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	addr := flags.String("addr", "127.0.0.1:8080", "HTTP listen address")
	staticDir := flags.String("static-dir", "web/reviews-widget", "static widget directory")
	productURLTemplate := flags.String("product-url-template", cfg.Web.ProductURLTemplate, "seller product URL template")
	withSync := flags.Bool("with-sync", false, "run periodic review sync inside the server process")
	if err := flags.Parse(args); err != nil {
		return exitConfigError
	}

	if err := server.StaticDirExists(*staticDir); err != nil {
		logger.Error("static directory", "path", *staticDir, "error", err)
		return exitConfigError
	}

	db, err := store.Open(cfg.DB)
	if err != nil {
		logger.Error("open database", "error", err)
		return exitConfigError
	}
	if err := db.Migrate(ctx); err != nil {
		logger.Error("migrate database", "error", err)
		return exitRunError
	}

	if *withSync {
		if err := cfg.ValidateMarketplaceCredentials(""); err != nil {
			logger.Error("sync credentials invalid", "error", err)
			return exitConfigError
		}
		runner := collector.NewRunner(db, cfg.Sync, logger, buildAdapters(cfg))
		sched := scheduler.New(
			syncRunnerAdapter{runner: runner, logger: logger},
			cfg.Sync.Interval,
			cfg.EnabledMarketplaces(),
			logger,
		)
		go sched.Run(ctx)
		logger.Info("in-process sync scheduler started", "interval", cfg.Sync.Interval.String())
	}

	httpServer := server.New(db, server.Config{
		Addr:               *addr,
		StaticDir:          *staticDir,
		ProductURLTemplate: *productURLTemplate,
		ProductLinks:       loadProductLinks(cfg.Web.ProductLinksPath, logger),
	}, logger)
	if err := httpServer.Run(ctx); err != nil {
		logger.Error("server stopped with error", "error", err)
		return exitRunError
	}

	logger.Info("shutdown complete")
	return exitOK
}
```

Add the adapter that reconciles the return-value mismatch. Put it directly below `runServe`:

```go
// syncRunnerAdapter adapts collector.Runner (which returns results) to the
// scheduler.Runner interface (which returns nothing), logging each result.
type syncRunnerAdapter struct {
	runner *collector.Runner
	logger *slog.Logger
}

func (a syncRunnerAdapter) RunOnce(ctx context.Context, marketplaces []string) {
	for _, result := range a.runner.RunOnce(ctx, marketplaces) {
		if result.Error != nil {
			a.logger.Error("scheduled sync marketplace failed", "marketplace", result.Marketplace, "error", result.Error)
			continue
		}
		a.logger.Info("scheduled sync marketplace ok", "marketplace", result.Marketplace, "seen", result.Seen, "upserted", result.Upserted)
	}
}
```

Add to the import block in `cmd/reviews/main.go`: `"reviews/internal/scheduler"`.

- [ ] **Step 2: Verify the build compiles**

Run: `go build ./...`
Expected: no output, exit 0.

- [ ] **Step 3: Verify the full test suite still passes**

Run: `go test ./...`
Expected: all packages PASS (`ok` lines), no FAIL.

- [ ] **Step 4: Update the usage text**

In `cmd/reviews/main.go`, in `usage()`, change the serve line to:

```go
  reviews serve [--addr 127.0.0.1:8080] [--with-sync]
```

- [ ] **Step 5: Commit**

```bash
git add cmd/reviews/main.go
git commit -m "feat(serve): optional in-process sync via --with-sync"
```

---

## Task 3: Dockerfile (multi-stage)

**Files:**
- Create: `Dockerfile`
- Create: `.dockerignore`

- [ ] **Step 1: Create `.dockerignore`**

```
.git
.gitignore
dist
*.db
*.db-shm
*.db-wal
.env
.env.*
!.env.example
web/reviews-data
docs
*.md
deploy/systemd
testdata
```

- [ ] **Step 2: Create `Dockerfile`**

```dockerfile
# syntax=docker/dockerfile:1

# ---- builder ----
FROM golang:1.26-bookworm AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath -ldflags "-s -w" \
    -o /out/reviews ./cmd/reviews

# ---- final ----
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app

# Static widget assets served by `serve --static-dir`.
COPY --from=builder /src/web/reviews-widget ./web/reviews-widget
COPY --from=builder /out/reviews /usr/local/bin/reviews

# SQLite database lives on a mounted volume.
ENV REVIEWS_DB_DSN=/data/reviews.db
EXPOSE 8080
USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/reviews"]
CMD ["serve", "--addr", "0.0.0.0:8080", "--with-sync"]
```

Note: `serve` runs `db.Migrate` on startup, so no separate migrate step is needed in the container. The distroless `nonroot` image has no shell — keep `CMD` in exec (JSON array) form.

- [ ] **Step 3: Verify the image builds**

Run: `docker build -t reviews:dev .`
Expected: build completes, final line `naming to docker.io/library/reviews:dev`.

- [ ] **Step 4: Smoke-test the binary in the image**

Run: `docker run --rm reviews:dev --help`
Expected: usage text printed, including `reviews serve [--addr 127.0.0.1:8080] [--with-sync]`.

- [ ] **Step 5: Commit**

```bash
git add Dockerfile .dockerignore
git commit -m "build: multi-stage Dockerfile with distroless final image"
```

---

## Task 4: docker-compose and configuration template

**Files:**
- Create: `docker-compose.yml`
- Create: `.env.example`

- [ ] **Step 1: Create `.env.example`**

```dotenv
# ---- Database ----
# sqlite (default) or postgres
REVIEWS_DB_DRIVER=sqlite
# For sqlite in Docker this is set to /data/reviews.db by the image.
# For postgres: host=... user=... password=... dbname=... port=5432 sslmode=disable
# REVIEWS_DB_DSN=/data/reviews.db

# ---- Sync schedule (used by `serve --with-sync`) ----
REVIEWS_SYNC_INTERVAL=1h
REVIEWS_SYNC_BACKFILL_MONTHS=12
REVIEWS_SYNC_OVERLAP=1h

# ---- Logging ----
REVIEWS_LOG_LEVEL=info
REVIEWS_LOG_FORMAT=json

# ---- Site / widget ----
REVIEWS_SITE_PRODUCT_URL_TEMPLATE=https://example.com/search?query={seller_article_url}
REVIEWS_SITE_PRODUCT_LINKS=data/product-links.json

# ---- Wildberries ----
REVIEWS_WB_ENABLED=true
REVIEWS_WB_TOKEN=

# ---- Yandex Market ----
REVIEWS_YM_ENABLED=true
REVIEWS_YM_API_KEY=
REVIEWS_YM_BUSINESS_ID=
# REVIEWS_YM_CAMPAIGN_ID=

# ---- Ozon (disabled by default) ----
REVIEWS_OZON_ENABLED=false
REVIEWS_OZON_CLIENT_ID=
REVIEWS_OZON_API_KEY=
```

- [ ] **Step 2: Create `docker-compose.yml`**

```yaml
services:
  reviews:
    build: .
    image: reviews:latest
    restart: unless-stopped
    ports:
      - "8080:8080"
    env_file:
      - .env
    environment:
      # Keep the SQLite file on the named volume regardless of .env.
      REVIEWS_DB_DSN: /data/reviews.db
    volumes:
      - reviews-data:/data

volumes:
  reviews-data:
```

- [ ] **Step 3: Verify compose config is valid**

Run: `docker compose config`
Expected: prints the normalized config with no error (a missing `.env` is fine for this check; if it errors on `.env`, run `cp .env.example .env` first).

- [ ] **Step 4: End-to-end smoke test**

```bash
cp .env.example .env
docker compose up -d --build
sleep 3
curl -fsS http://localhost:8080/healthz
```
Expected: `/healthz` returns HTTP 200. Then tear down:

```bash
docker compose down
```
Expected: container stops, `reviews-data` volume persists.

- [ ] **Step 5: Commit**

```bash
git add docker-compose.yml .env.example
git commit -m "build: docker-compose stack and .env.example template"
```

---

## Task 5: Document the container workflow

**Files:**
- Modify: `deploy/README.md`

- [ ] **Step 1: Append a "Run with Docker" section**

Add to `deploy/README.md`:

```markdown
## Run with Docker

Prerequisites: Docker with the Compose plugin.

1. Copy and edit configuration:
   ```sh
   cp .env.example .env
   # fill in marketplace tokens (REVIEWS_WB_TOKEN, REVIEWS_YM_API_KEY, REVIEWS_YM_BUSINESS_ID)
   ```
2. Start the service (builds the image on first run):
   ```sh
   docker compose up -d --build
   ```
3. Verify it is healthy:
   ```sh
   curl -fsS http://localhost:8080/healthz
   ```

The server runs database migrations on startup and, because the image's default
command is `serve --with-sync`, it also runs review sync on `REVIEWS_SYNC_INTERVAL`
inside the same process — no systemd timer required. The SQLite database persists
in the `reviews-data` named volume.

To run a one-off sync manually:
```sh
docker compose run --rm reviews sync --once
```
```

- [ ] **Step 2: Commit**

```bash
git add deploy/README.md
git commit -m "docs: document Docker / docker-compose workflow"
```

---

## Self-Review Notes

- **Spec coverage:** Dockerfile (Task 3), docker-compose + volume (Task 4), `.env.example` (Task 4), in-process scheduler using `REVIEWS_SYNC_INTERVAL` (Tasks 1–2), persistence via named volume (Task 4), secrets via env (Task 4). All "Контейнеризация" spec bullets covered. Encrypted-secrets-in-DB is explicitly out of scope per spec.
- **Type consistency:** `scheduler.Runner.RunOnce(ctx, []string)` returns nothing; `collector.Runner.RunOnce(ctx, []string) []collector.Result` returns results; bridged by `syncRunnerAdapter` (Task 2). `scheduler.New(runner, interval, marketplaces, logger)` signature matches both test (Task 1) and call site (Task 2).
- **No placeholders:** every step has concrete code or an exact command with expected output.
