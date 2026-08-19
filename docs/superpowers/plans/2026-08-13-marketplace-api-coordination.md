# Marketplace API Coordination Implementation Plan

> **STATUS: READY — критический путь к SaaS, не начат. 11 tasks, prerequisite для мультитенанта.**
>
> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Route every WB, Yandex Market, and Ozon API request through shared rate-limit-aware execution, reject unsupported WB tokens, prevent duplicate marketplace syncs, and apply credential changes without restarting the service.

**Architecture:** A single process-wide `apihttp.Executor` serializes and paces external requests per marketplace; three explicit policies recognize only documented throttle signals, retry reads once, and never retry writes. A process-local sync coordinator acquires non-blocking per-marketplace leases, while a composition-root operation service reloads DB-overlaid credentials and constructs fresh adapters/publishers for every sync or publication. Existing marketplace clients retain endpoint/auth/decoding responsibility and receive the shared executor by constructor injection.

**Tech Stack:** Go 1.26.3 standard library (`net/http`, `context`, `sync`, `encoding/base64`, `encoding/json`), GORM/SQLite, existing React 19 + TypeScript 5.9 + Vite 7 admin SPA. No new dependencies.

**Approved spec:** `docs/superpowers/specs/2026-08-13-marketplace-api-coordination-design.md` (SHA-256 `d50630e4841f493f6889f7ae658465161c848ca8dfa443c1f82c2d82042cef53`).

## Global Constraints

- WB accepts only an unexpired personal JWT whose decoded payload has numeric `acc == 3`; token payload decoding is metadata validation, not signature verification.
- Existing invalid/base/test/service WB tokens must disable only WB operations; service startup, local data, YM, and Ozon remain available.
- Every direct `httpClient.Do` in `internal/marketplace/{wb,ym,ozon}/client.go` must be removed; all current read, probe, mapping, reply, and Q&A calls use one process-wide executor.
- Operation class is semantic: Ozon/YM read-only `POST` endpoints are `apihttp.Read`; reply and question-answer publication are `apihttp.Write`.
- Reads retry at most once and only after a valid marketplace throttle header. Writes, transport errors, and generic `5xx` responses are never retried.
- WB policy: status `429`, header `X-Ratelimit-Retry` as integer seconds, minimum interval `350ms`.
- YM policy: statuses `420` and `429`, standard `Retry-After`; no pre-emptive minimum interval.
- Ozon policy: status `429`, standard `Retry-After`; no pre-emptive minimum interval.
- `Retry-After` supports integer seconds and HTTP-date via `http.ParseTime`. Delay must be `> 0` and `<= 15m`; otherwise return the throttle error immediately.
- Errors and logs may contain marketplace ID, URL path without query, status, and retry delay; they must not contain credentials, authorization headers, request bodies, query strings, or marketplace response bodies.
- The same marketplace cannot run two syncs concurrently; different marketplaces remain independent.
- Explicit unknown, disabled, or invalidly configured marketplaces produce HTTP `400` and start nothing. All-busy produces `409`; at least one started produces `202` with `started` and `busy` arrays.
- Admin UI copy is Russian. WB credential field label is exactly `Персональный WB API-токен`.
- Preserve existing marketplace payload mapping, pagination, page sizes, public APIs, and reply-publish state semantics.
- Standard library only for executor, coordinator, and token metadata parsing. Do not add a retry library, JWT library, generic job framework, persistent job table, or distributed lock.
- Go commands run from `/home/mama/DEV/Reviews`; admin commands run from `web/admin`.
- UI delivery requires `npm run build`, then replacing `internal/server/admin_dist` with `web/admin/dist` before the Go build/smoke check.

## Documentation Discovery / Allowed APIs

- WB developer portal: <https://dev.wildberries.ru/openapi>
  - Feedbacks API returns `429` and `X-Ratelimit-Retry`; the live client probe on 2026-08-13 confirmed the header is integer seconds.
  - Token creation uses the `Отзывы и вопросы` category. Do not infer that category from JWT metadata; WB `401/403` remains authoritative.
- Yandex Market Partner API: <https://yandex.ru/dev/market/partner-api/>
  - Partner API uses `420` for request-limit exhaustion. Retry only if a valid `Retry-After` is actually present; do not invent a delay.
- Ozon Seller API: <https://docs.ozon.ru/api/seller/>
  - Treat only `429` plus an actual valid `Retry-After` as retryable in this change.
- RFC 9110 §10.2.3: <https://www.rfc-editor.org/rfc/rfc9110#section-10.2.3>
  - `Retry-After = HTTP-date / delay-seconds`; parse dates with `http.ParseTime`.
- RFC 6585 §4: <https://www.rfc-editor.org/rfc/rfc6585#section-4>
  - `429` denotes rate limiting and may carry `Retry-After`; it does not make unsafe requests idempotent.
- Go `net/http` docs: <https://pkg.go.dev/net/http>
  - `Request.Clone` shallow-copies `Body`; do not clone and reuse a consumed body. Use a request factory that constructs a fresh request/body for every attempt.
  - `http.ParseTime` parses the HTTP date formats accepted by HTTP/1.1.

**Documentation gaps and conservative defaults:** Official YM/Ozon pages did not yield source-backed method-specific retry intervals during planning. Therefore their policies have no proactive delay and no retry without a valid response header. This is intentional; do not add exponential backoff or guessed quotas.

## File Structure

### Create

- `internal/marketplace/apihttp/executor.go` — `Executor`, `Operation`, `Policy`, `ThrottleError`, per-marketplace serialization/pacing, request execution.
- `internal/marketplace/apihttp/policy.go` — concrete WB/YM/Ozon policies and retry-header parsing.
- `internal/marketplace/apihttp/executor_test.go` — executor behavior, cancellation, serialization, body recreation, sanitization.
- `internal/marketplace/apihttp/policy_test.go` — policy status/header parsing and delay bounds.
- `internal/marketplace/wbtoken/token.go` — local personal-token metadata validation without a `config` import cycle.
- `internal/marketplace/wbtoken/token_test.go` — personal/base/test/service/expired/malformed cases.
- `internal/syncer/coordinator.go` — non-blocking per-marketplace sync leases.
- `cmd/reviews/marketplace_operations.go` — dynamic effective-config loader, fresh adapter/publisher resolver, coordinated sync dispatch.
- `cmd/reviews/marketplace_operations_test.go` — post-start enable/token replacement/invalid WB behavior.

### Modify

- `internal/marketplace/wb/client.go`, `client_test.go` — inject executor; route four WB calls through `Do` with semantic operation class.
- `internal/marketplace/ym/client.go`, `client_test.go` — inject executor; route five YM calls through `Do`.
- `internal/marketplace/ozon/client.go`, `client_test.go` — inject executor; route six Ozon calls through `Do`.
- `internal/config/config.go`, `config_test.go` — include personal-token validation in WB credential validation used by CLI/runtime.
- `internal/server/server.go` — replace startup publisher maps with resolver functions; change sync trigger contract to return dispatch result/error.
- `internal/server/admin_dashboard.go`, `admin_dashboard_test.go` — validate WB before save, expose unsupported-token warning, return `202/409/400` sync responses.
- `internal/server/reply_publish.go`, `reply_publish_test.go` — resolve current publisher per operation; invalid credentials record failed, not unsupported.
- `internal/server/question_publish.go`, `question_publish_test.go` — same for question answers.
- `internal/scheduler/scheduler.go`, `scheduler_test.go` — runner resolves current enabled marketplaces per tick instead of capturing startup IDs.
- `cmd/reviews/main.go`, `main_test.go` — create one executor/coordinator/operation service; use dynamic sync and publisher resolvers in CLI, scheduler, server, and Ozon probe.
- `web/admin/src/pages/Marketplaces.tsx`, `web/admin/src/types.ts` — personal-token copy, help/warning, structured started/busy sync response, disable unconfigured sync.
- `README.md`, `.env.example` — require personal WB token and describe rate-limit-safe behavior.

### Regenerate

- `internal/server/admin_dist/**` — built admin SPA embedded by `internal/server/spa.go`.

---

### Task 1: Shared marketplace API executor and policies

**Files:**
- Create: `internal/marketplace/apihttp/executor.go`
- Create: `internal/marketplace/apihttp/policy.go`
- Test: `internal/marketplace/apihttp/executor_test.go`
- Test: `internal/marketplace/apihttp/policy_test.go`

**Interfaces:**
- Produces:

```go
type Operation uint8
const (
    Read Operation = iota
    Write
)

type Policy struct { /* immutable private fields; returned by WBPolicy/YMPolicy/OzonPolicy */ }

func WBPolicy() Policy
func YMPolicy() Policy
func OzonPolicy() Policy

type RequestFactory func(context.Context) (*http.Request, error)

type Executor struct { /* private keyed state */ }
func NewExecutor() *Executor
func (e *Executor) Do(ctx context.Context, client *http.Client, policy Policy, operation Operation, makeRequest RequestFactory) (*http.Response, error)

type ThrottleError struct {
    Marketplace string
    Path string
    Status int
    RetryAfter time.Duration
}
func (e *ThrottleError) Error() string
```

- Consumers: all three marketplace clients in Tasks 2–4.
- Invariant: one `Executor` instance must be shared across every adapter in the process.

- [ ] **Step 1: Write failing policy tests**

Create table-driven tests that assert:

```go
func TestPolicies(t *testing.T) {
    tests := []struct {
        name string
        policy Policy
        status int
        header http.Header
        wantThrottle bool
        wantDelay time.Duration
    }{
        {"wb retry seconds", WBPolicy(), 429, http.Header{"X-Ratelimit-Retry": {"696"}}, true, 696 * time.Second},
        {"wb other status", WBPolicy(), 500, nil, false, 0},
        {"ym 420 retry-after", YMPolicy(), 420, http.Header{"Retry-After": {"3"}}, true, 3 * time.Second},
        {"ym 429 http date", YMPolicy(), 429, http.Header{"Retry-After": {time.Now().Add(time.Minute).UTC().Format(http.TimeFormat)}}, true, time.Minute},
        {"ozon 429 retry-after", OzonPolicy(), 429, http.Header{"Retry-After": {"2"}}, true, 2 * time.Second},
        {"missing header", OzonPolicy(), 429, nil, true, 0},
        {"over max", WBPolicy(), 429, http.Header{"X-Ratelimit-Retry": {"901"}}, true, 0},
    }
    // Compare date-derived durations with <=1s tolerance.
}
```

Also assert `WBPolicy().minInterval == 350*time.Millisecond`, YM/Ozon intervals are zero, and delay `0`, negative, malformed, or above `15m` is rejected. Tests use package `apihttp` (not `apihttp_test`) so private immutable policy fields remain testable without exporting mutable configuration.

- [ ] **Step 2: Run policy tests to verify RED**

Run: `go test ./internal/marketplace/apihttp -run TestPolicies -count=1`

Expected: FAIL because the package/types do not exist.

- [ ] **Step 3: Implement policy parsing minimally**

Implement private helpers:

```go
const maxRetryAfter = 15 * time.Minute

func (p Policy) throttle(status int, headers http.Header, now time.Time) (bool, time.Duration) {
    if !p.isThrottle(status) { return false, 0 }
    raw := strings.TrimSpace(headers.Get(p.retryHeader))
    if raw == "" { return true, 0 }
    var delay time.Duration
    if p.standardRetryAfter {
        if seconds, err := strconv.Atoi(raw); err == nil {
            delay = time.Duration(seconds) * time.Second
        } else if at, err := http.ParseTime(raw); err == nil {
            delay = at.Sub(now)
        }
    } else if seconds, err := strconv.Atoi(raw); err == nil {
        delay = time.Duration(seconds) * time.Second
    }
    if delay <= 0 || delay > maxRetryAfter { return true, 0 }
    return true, delay
}

Use two fixed integer status slots plus a count in private policy fields; do not allocate a map for every request. The three constructor functions return package-level immutable policy values.
```

Define concrete policies exactly as listed in Global Constraints.

- [ ] **Step 4: Run policy tests to verify GREEN**

Run: `go test ./internal/marketplace/apihttp -run TestPolicies -count=1`

Expected: PASS.

- [ ] **Step 5: Write failing executor retry/sanitization tests**

Use a custom `RoundTripper` rather than sleeps. Cover:

1. `Read` request returns `429 + Retry-After: 1`, then `200`; factory and transport called twice.
2. `Write` request returns `429`; factory/transport called once and error is `*ThrottleError` with delay.
3. Missing/malformed retry header returns after one attempt.
4. A POST-shaped `Read` factory emits body `{"page":1}` on both attempts, proving a fresh body is built.
5. `ThrottleError.Error()` includes `wb`, `/api/v1/feedbacks`, `429`, and duration but excludes `?token=secret`, `Authorization`, and response body.

Use a policy with `Retry-After: 1` only where a real timer is acceptable; for fast retry tests inject private `now`/`wait` hooks into `Executor` from the same package test. Keep those hooks unexported:

```go
type Executor struct {
    mu sync.Mutex
    states map[string]*marketplaceState
    now func() time.Time
    wait func(context.Context, time.Duration) error
}
```

Production `wait` uses a stopped/drained `time.Timer`, not `time.Sleep`.

- [ ] **Step 6: Write failing executor concurrency/cancellation tests**

Prove:

- two WB calls never overlap (`maxActive == 1`);
- WB and YM calls can overlap (`maxActive == 2` with a barrier);
- second WB call begins no earlier than fake-clock `350ms` after the first start;
- cancellation while waiting returns `context.Canceled`, does not invoke transport, and does not retain the marketplace lock.

- [ ] **Step 7: Run executor tests to verify RED**

Run: `go test ./internal/marketplace/apihttp -run 'TestExecutor' -count=1`

Expected: FAIL because `Executor.Do` is absent.

- [ ] **Step 8: Implement the executor**

Implementation rules:

```go
type marketplaceState struct {
    mu sync.Mutex
    lastAttempt time.Time
}
```

- Resolve/create state under the executor map mutex, then hold only `state.mu` across pacing, first attempt, retry wait, and retry. This enforces one external request flow per marketplace without blocking other marketplaces.
- Normalize nil clients to `http.DefaultClient`.
- Call `makeRequest(ctx)` for every attempt.
- Store only `req.URL.Path` in errors; never `RawQuery` or `req.URL.String()`.
- For every throttled response that will not be returned, drain at most 4 KiB with `io.CopyN(io.Discard, resp.Body, 4096)` and close it. This includes intermediate retries, writes, invalid-delay throttles, and the second throttled read.
- Retry exactly once only when `operation == Read` and parsed delay is valid.
- For `Write`, missing delay, invalid delay, or second throttled read, return `ThrottleError` immediately with parsed delay if available.
- `ThrottleError.Error()` maps IDs to stable user-facing marketplace names and Russian text (`Wildberries временно ограничил запросы. Повторите через N.`); unknown IDs use the raw non-secret ID. Keep typed fields for logs/tests.
- Return non-throttle responses untouched so existing clients remain responsible for body decoding and status-specific messages.
- On request-factory, transport, or context error, return it without retry.

- [ ] **Step 9: Run executor tests and race detector**

Run: `go test -race ./internal/marketplace/apihttp -count=1`

Expected: PASS, zero races.

- [ ] **Step 10: Commit Task 1**

```bash
git add internal/marketplace/apihttp
git commit -m "feat(marketplace): add shared rate-limit executor"
```

---

### Task 2: Migrate the Wildberries client

**Files:**
- Modify: `internal/marketplace/wb/client.go`
- Modify: `internal/marketplace/wb/client_test.go`

**Interfaces:**
- Consumes: `*apihttp.Executor`, `apihttp.WBPolicy()`, `apihttp.Read`, `apihttp.Write`.
- Produces:

```go
func New(cfg config.WBConfig, executor *apihttp.Executor) *Client
func NewWithHTTPClient(cfg config.WBConfig, baseURL string, httpClient *http.Client, pageSize int, executor *apihttp.Executor) *Client
```

- Constructor rule: nil executor creates a private executor only for isolated tests/CLI compatibility; production composition must pass the shared non-nil instance.

- [ ] **Step 1: Add failing client-level throttle tests**

Add `TestFetchReviewsRetriesWBThrottle` using a transport that returns `429` with `X-Ratelimit-Retry: 1` then a valid feedback page. Inject an executor whose package-private wait hook is not accessible here by instead use an `httptest.Server` response with `X-Ratelimit-Retry: 1` and assert two attempts; keep this single 1-second integration check.

Add `TestPublishReplyDoesNotRetryWBThrottle`: server always returns `429`; assert one request and an error containing `Wildberries`, `429`, and retry duration.

- [ ] **Step 2: Run WB throttle tests to verify RED**

Run: `go test ./internal/marketplace/wb -run 'Test(FetchReviewsRetriesWBThrottle|PublishReplyDoesNotRetryWBThrottle)' -count=1`

Expected: FAIL because the client still calls `httpClient.Do` directly.

- [ ] **Step 3: Inject and use the executor**

Add `executor *apihttp.Executor` to `Client`. Update both constructors and every test callsite in this package. For each API method replace a prebuilt `req` with a closure that creates the request and headers fresh:

```go
resp, err := c.executor.Do(ctx, c.httpClient, apihttp.WBPolicy(), apihttp.Read, func(attemptCtx context.Context) (*http.Request, error) {
    req, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, endpoint.String(), nil)
    if err != nil { return nil, err }
    req.Header.Set("Authorization", c.token)
    req.Header.Set("Accept", "application/json")
    return req, nil
})
```

Classify:

- `fetchFeedbacks`: `Read`;
- `FetchQuestions`: `Read`;
- `PublishQuestionAnswer`: `Write`;
- `PublishReply`: `Write`.

Leave successful response decoding and marketplace-specific error messages in place.

- [ ] **Step 4: Run WB package tests**

Run: `go test ./internal/marketplace/wb -count=1`

Expected: PASS.

- [ ] **Step 5: Verify no direct Do remains**

Run: `grep -R "httpClient.Do" internal/marketplace/wb/client.go`

Expected: no output. Use repository grep tooling if executing in the harness rather than shell grep.

- [ ] **Step 6: Commit Task 2**

```bash
git add internal/marketplace/wb/client.go internal/marketplace/wb/client_test.go
git commit -m "refactor(wb): use shared API executor"
```

---

### Task 3: Migrate the Yandex Market client

**Files:**
- Modify: `internal/marketplace/ym/client.go`
- Modify: `internal/marketplace/ym/client_test.go`

**Interfaces:**
- Consumes: `*apihttp.Executor`, `apihttp.YMPolicy()`, semantic `Read`/`Write`.
- Produces:

```go
func New(cfg config.YMConfig, executor *apihttp.Executor) *Client
func NewWithHTTPClient(cfg config.YMConfig, baseURL string, httpClient *http.Client, pageSize int, executor *apihttp.Executor) *Client
```

- [ ] **Step 1: Add failing semantic-operation tests**

Add a transport sequence test for `FetchReviews`: first response `420` with `Retry-After: 1`, second a valid goods-feedback payload; assert two POST requests and identical bodies. Add `TestPublishReplyDoesNotRetryYMThrottle`: `429 + Retry-After: 1`, assert one POST.

- [ ] **Step 2: Run new YM tests to verify RED**

Run: `go test ./internal/marketplace/ym -run 'Test(FetchReviewsRetriesYMThrottle|PublishReplyDoesNotRetryYMThrottle)' -count=1`

Expected: FAIL under direct HTTP execution.

- [ ] **Step 3: Inject executor and migrate all five calls**

Classify:

- `fetchPage` (`POST`): `Read`;
- `FetchQuestions` (`POST`): `Read`;
- `fetchQuestionAnswer` (`POST`): `Read`;
- `PublishQuestionAnswer` (`POST`): `Write`;
- `PublishReply` (`POST`): `Write`.

Every closure must rebuild `strings.NewReader(string(body))` or `strings.NewReader("{}")`, reapply `Api-Key`, `Content-Type`, and `Accept`, and use `attemptCtx`.

- [ ] **Step 4: Update all constructor callsites in YM tests**

Pass a fresh `apihttp.NewExecutor()` unless a test explicitly needs a shared instance. Preserve base URLs, page sizes, and fixtures unchanged.

- [ ] **Step 5: Run YM package tests**

Run: `go test ./internal/marketplace/ym -count=1`

Expected: PASS. If the historical wall-clock fixture failure reappears, use the repository's already-established fixed-since test behavior; do not change marketplace logic as part of this task.

- [ ] **Step 6: Verify direct calls are gone and commit**

Repository search: `httpClient.Do` in `internal/marketplace/ym/client.go`.

Expected: no matches.

```bash
git add internal/marketplace/ym/client.go internal/marketplace/ym/client_test.go
git commit -m "refactor(ym): use shared API executor"
```

---

### Task 4: Migrate the Ozon client

**Files:**
- Modify: `internal/marketplace/ozon/client.go`
- Modify: `internal/marketplace/ozon/client_test.go`

**Interfaces:**
- Consumes: `*apihttp.Executor`, `apihttp.OzonPolicy()`, semantic `Read`/`Write`.
- Produces:

```go
func New(cfg config.OzonConfig, executor *apihttp.Executor) *Client
func NewWithHTTPClient(cfg config.OzonConfig, baseURL string, httpClient *http.Client, pageSize int, executor *apihttp.Executor) *Client
```

- [ ] **Step 1: Add failing read-vs-write throttle tests**

For `FetchReviews`, return `429 + Retry-After: 1`, then valid review/product responses; assert the `/v1/review/list` read-only POST is retried with identical body. For `PublishQuestionAnswer`, return `429 + Retry-After: 1`; assert one request.

- [ ] **Step 2: Run new Ozon tests to verify RED**

Run: `go test ./internal/marketplace/ozon -run 'Test(FetchReviewsRetriesOzonThrottle|PublishQuestionAnswerDoesNotRetryOzonThrottle)' -count=1`

Expected: FAIL.

- [ ] **Step 3: Inject executor and migrate all six calls**

Classify:

- `fetchPage`: `Read`;
- `FetchQuestions`: `Read`;
- `fetchQuestionAnswer`: `Read`;
- `fetchProductPage` / `CheckProductsAccess`: `Read`;
- `PublishQuestionAnswer`: `Write`;
- `PublishReply`: `Write`.

Every request factory recreates `bytes.NewReader(body)` and both auth headers. Do not alter the product-map cache or error-payload decoding.

- [ ] **Step 4: Update constructor callsites and run package tests**

Run: `go test ./internal/marketplace/ozon -count=1`

Expected: PASS.

- [ ] **Step 5: Verify direct calls are gone and commit**

Repository search: `httpClient.Do` in `internal/marketplace/ozon/client.go`.

Expected: no matches.

```bash
git add internal/marketplace/ozon/client.go internal/marketplace/ozon/client_test.go
git commit -m "refactor(ozon): use shared API executor"
```

---

### Task 5: WB personal-token metadata policy

**Files:**
- Create: `internal/marketplace/wbtoken/token.go`
- Test: `internal/marketplace/wbtoken/token_test.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/server/admin_dashboard.go`
- Modify: `internal/server/admin_dashboard_test.go`
- Modify: `cmd/reviews/main.go`

**Interfaces:**
- Produces:

```go
var ErrPersonalTokenRequired = errors.New("нужен персональный WB API-токен с категорией «Отзывы и вопросы»")

func ValidatePersonalTokenMetadata(token string, now time.Time) error
```

- Server behavior: validate a non-empty newly submitted WB token before calling `SaveMarketplaceCredential`; failed validation returns `400` and preserves the old DB row.
- Status behavior: an existing invalid WB token yields `Configured=false` and a warning; token presence remains masked as `fields.token=true`.

- [ ] **Step 1: Write JWT fixture helper and failing validator tests**

Use stdlib only; tests can build unsigned metadata strings because validation intentionally does not verify signatures:

```go
func testToken(t *testing.T, claims map[string]any) string {
    encode := func(v any) string {
        raw, _ := json.Marshal(v)
        return base64.RawURLEncoding.EncodeToString(raw)
    }
    return encode(map[string]string{"alg":"none"}) + "." + encode(claims) + ".sig"
}
```

Table cases:

- `acc:3`, future numeric `exp` → nil;
- `acc:1`, `2`, `4`, unknown → `ErrPersonalTokenRequired`;
- missing/non-numeric `acc`;
- missing/non-numeric/expired `exp`;
- one/two/four segments;
- invalid base64url;
- invalid JSON.

Assert errors never contain the source token or decoded claim values.

- [ ] **Step 2: Run validator tests to verify RED**

Run: `go test ./internal/marketplace/wbtoken -run TestValidatePersonalTokenMetadata -count=1`

Expected: FAIL.

- [ ] **Step 3: Implement minimal token metadata validation**

Implementation:

```go
type tokenClaims struct {
    Account json.Number `json:"acc"`
    Expires json.Number `json:"exp"`
}
```

Use `json.Decoder.UseNumber`, `base64.RawURLEncoding.DecodeString`, and `strings.Split`. Require exactly three non-empty segments, `acc == 3`, and `time.Unix(exp, 0).After(now)`. Wrap parse failures in stable non-secret errors; do not return raw decoder errors containing token bytes.

- [ ] **Step 4: Extend config validation**

In `ValidateMarketplaceCredentials`, after the non-empty WB token check, call the neutral `wbtoken` package (the `wb` client already imports `config`, so putting token validation there would create an import cycle):

```go
if err := wbtoken.ValidatePersonalTokenMetadata(cfg.Marketplaces.WB.Token, time.Now()); err != nil {
    return err
}
```

Add config tests for enabled personal vs base WB token. Disabled WB ignores token validity.

- [ ] **Step 5: Add failing credentials API preservation test**

Seed a valid personal token in `marketplace_credentials`, then PUT a base token. Assert:

- HTTP `400`;
- response has the Russian personal-token message and does not contain either token;
- stored payload still contains the original valid token.

Also seed an existing base token directly through the store and GET marketplaces. Assert WB has `configured:false`, `fields.token:true`, and non-empty `warning`; YM status remains unchanged.

- [ ] **Step 6: Implement save/status behavior**

Before saving WB values:

- validate only when `values.token` is non-empty;
- reject before `SaveMarketplaceCredential` so transaction/payload remain unchanged;
- when toggling existing credentials with an empty token field, read the existing credential, validate its retained token if enabling, and reject enabling an invalid token;
- status calculation validates current payload token at `time.Now()`; invalid means unconfigured + warning, not an API panic.

Keep the token masked: `credentialFields` reports presence, while `credentialConfigured` for WB receives/uses validation result rather than presence alone.

- [ ] **Step 7: Run focused tests**

Update `marketplaceStatuses(cfg)` in `cmd/reviews/main.go` so an environment-supplied invalid WB token is also reported as unconfigured with the same warning; do not treat token presence as validity in the startup fallback status.

Run:

```bash
go test ./internal/marketplace/wbtoken ./internal/config ./internal/server -run 'Test(ValidatePersonalTokenMetadata|ValidateMarketplaceCredentials|SaveWB|MarketplaceStatus)' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit Task 5**

```bash
git add internal/marketplace/wbtoken internal/config/config.go internal/config/config_test.go internal/server/admin_dashboard.go internal/server/admin_dashboard_test.go
git commit -m "feat(wb): require personal API tokens"
```

---

### Task 6: Per-marketplace sync coordinator

**Files:**
- Create: `internal/syncer/coordinator.go`
- Test: `internal/syncer/coordinator_test.go`

**Interfaces:**
- Produces:

```go
type Coordinator struct { /* private */ }
func NewCoordinator() *Coordinator
func (c *Coordinator) TryAcquire(marketplace string) (release func(), ok bool)
func (c *Coordinator) Busy(marketplace string) bool
```

- Consumers: `marketplaceOperations` in Task 7 and admin sync handler in Task 8.

- [ ] **Step 1: Write failing coordinator tests**

Cover:

```go
release, ok := c.TryAcquire("wb")     // true
_, second := c.TryAcquire("wb")       // false
release()
_, third := c.TryAcquire("wb")        // true

_, wbOK := c.TryAcquire("wb")
_, ymOK := c.TryAcquire("ym")          // both true
```

Also race 100 goroutines for `wb`; exactly one acquisition succeeds before release. Assert double-calling release is safe by wrapping release with `sync.Once`.

- [ ] **Step 2: Run tests to verify RED**

Run: `go test ./internal/syncer -count=1`

Expected: FAIL.

- [ ] **Step 3: Implement minimal coordinator**

Use `sync.Mutex` plus `map[string]bool`. `TryAcquire` sets the key atomically and returns an idempotent release closure. `Busy` reads under the same mutex. No channels, goroutine ownership, persistence, or timeouts.

- [ ] **Step 4: Run race detector and commit**

Run: `go test -race ./internal/syncer -count=1`

Expected: PASS.

```bash
git add internal/syncer
git commit -m "feat(sync): add marketplace coordinator"
```

---

### Task 7: Dynamic marketplace operation service

**Files:**
- Create: `cmd/reviews/marketplace_operations.go`
- Test: `cmd/reviews/marketplace_operations_test.go`
- Modify: `cmd/reviews/main.go`
- Modify: `cmd/reviews/main_test.go`

**Interfaces:**
- Consumes: store, base config, logger, one shared executor, coordinator, collector.
- Produces:

```go
type marketplaceOperations struct { /* private dependencies, including server-lifetime context */ }

func newMarketplaceOperations(ctx context.Context, db *store.Store, base config.Config, logger *slog.Logger, executor *apihttp.Executor, coordinator *syncer.Coordinator) *marketplaceOperations
func (o *marketplaceOperations) EffectiveConfig(ctx context.Context) config.Config
func (o *marketplaceOperations) Runnable(ctx context.Context) []string
func (o *marketplaceOperations) DispatchSync(requested []string, after func()) (server.SyncDispatch, error)
func (o *marketplaceOperations) RunSync(ctx context.Context, requested []string, after func()) ([]collector.Result, error)
func (o *marketplaceOperations) ResolveReplyPublisher(ctx context.Context, marketplaceID string) (marketplace.ReplyPublisher, error)
func (o *marketplaceOperations) ResolveQuestionPublisher(ctx context.Context, marketplaceID string) (marketplace.QuestionAnswerPublisher, error)
func (o *marketplaceOperations) CheckOzonProducts(ctx context.Context) error
```

- Private helper: `adapter(ctx, marketplaceID) (marketplace.Adapter, error)` validates one enabled marketplace and constructs `wb.New`, `ym.New`, or `ozon.New` with the shared executor.

- [ ] **Step 1: Write failing dynamic-config tests**

Test with a temp SQLite store:

1. Base config has WB disabled; save a valid personal WB credential enabled after `marketplaceOperations` construction; `Runnable(ctx)` returns `[]string{"wb"}` and `adapter` succeeds without reconstructing `marketplaceOperations`.
2. Replace the stored personal token with a second personal token; inject a recording adapter factory and prove the next operation receives token 2, not token 1.
3. Seed an existing base WB token alongside valid enabled YM credentials; `Runnable(ctx)` excludes WB and includes YM, explicit `adapter(ctx,"wb")` returns the personal-token error, and YM adapter construction still succeeds.
4. Disable WB after construction; new sync/publisher resolution is rejected.

To keep production simple, the test seam is an unexported constructor function field initialized to real constructors:

```go
type adapterFactory func(config.Config, string, *apihttp.Executor) (marketplace.Adapter, error)
```

- [ ] **Step 2: Run operation tests to verify RED**

Run: `go test ./cmd/reviews -run TestMarketplaceOperations -count=1`

Expected: FAIL.

- [ ] **Step 3: Implement effective config and fresh adapter construction**

Move/reuse `applyStoredMarketplaceCredentials` rather than introducing a second overlay convention. Validate explicit marketplace with `ValidateMarketplaceCredentials(id)` before construction. Build exactly one selected adapter instead of constructing all adapters.

- [ ] **Step 4: Write failing coordinated dispatch tests**

Use blocking fake adapters. Assert:

- first WB dispatch returns `Started:["wb"]` before background work completes;
- second WB dispatch returns `Busy:["wb"]` and makes no collector call;
- YM can start while WB is blocked;
- release occurs after collector error/cancellation;
- explicit unknown/disabled/invalid marketplace returns an error and launches no goroutine;
- no requested IDs means current `Runnable(ctx)` is evaluated at dispatch time and invalid enabled marketplaces are skipped;
- the `after` callback runs once after all marketplaces started by that dispatch finish, not once per marketplace.

`marketplaceOperations` stores the server-lifetime context passed at construction. `DispatchSync` uses that context after returning to the HTTP handler. `RunSync` is the synchronous CLI path and uses its explicit context.

- [ ] **Step 5: Implement dispatch and synchronous sync minimally**

Validation/acquisition order:

1. Resolve requested IDs (`Runnable(ctx)` when empty).
2. Validate every explicitly requested ID before acquiring or launching anything.
3. For each valid ID call `TryAcquire`.
4. Return `Started`/`Busy` immediately.
5. Launch one goroutine per started marketplace; inside it `defer release()`, construct a fresh adapter, create a one-adapter collector runner, and run once.
6. Use a `sync.WaitGroup` in one batch goroutine to invoke `after` exactly once after every started marketplace finishes. A nil callback is allowed.

`RunSync` uses the same validation, fresh-adapter, and coordinator rules synchronously, waits for every requested marketplace, then calls `after` once. `reviews sync --once` must not exit before work completes. CLI “all” skips invalid enabled marketplaces so a bad WB token does not prevent valid YM/Ozon synchronization; an explicitly selected invalid WB returns `exitConfigError`.

- [ ] **Step 6: Resolve publishers dynamically**

`ResolveReplyPublisher` and `ResolveQuestionPublisher` call `adapter` immediately and type-assert the existing capability interfaces. Return a stable unsupported error when the adapter lacks the capability. `CheckOzonProducts` resolves a fresh Ozon adapter and calls `CheckProductsAccess`.

- [ ] **Step 7: Wire one shared executor/coordinator in `runServe` and CLI sync**

In `runServe` create exactly once:

```go
executor := apihttp.NewExecutor()
coordinator := syncer.NewCoordinator()
operations := newMarketplaceOperations(ctx, db, cfg, logger, executor, coordinator)
```

Remove `initialAdapters`, startup publisher maps, startup `runner`, and the all-marketplace startup credential validation that currently exits the process. Invalid enabled credentials are filtered by `Runnable`; an explicitly requested invalid marketplace still returns a validation error. Use operations for manual trigger, scheduler runner, publisher resolvers, and Ozon probe. Update CLI `runSync` to call synchronous `RunSync` while preserving exit codes. Construct `httpServer` before starting the immediate scheduler goroutine so the post-sync publication callback is available on the first tick.

- [ ] **Step 8: Update all live constructors and verify shared executor**

Repository search for `wb.New(`, `ym.New(`, `ozon.New(` must show each live call receives the same executor owned by operations. Test constructors continue to use explicit executor values.

- [ ] **Step 9: Run command tests and race detector**

Run:

```bash
go test -race ./cmd/reviews ./internal/collector -count=1
```

Expected: PASS.

- [ ] **Step 10: Commit Task 7**

```bash
git add cmd/reviews/main.go cmd/reviews/main_test.go cmd/reviews/marketplace_operations.go cmd/reviews/marketplace_operations_test.go
git commit -m "refactor(sync): resolve marketplace operations dynamically"
```

---

### Task 8: Dynamic scheduler and structured admin sync responses

**Files:**
- Modify: `internal/scheduler/scheduler.go`
- Modify: `internal/scheduler/scheduler_test.go`
- Modify: `internal/server/server.go`
- Modify: `internal/server/admin_dashboard.go`
- Modify: `internal/server/admin_dashboard_test.go`

**Interfaces:**
- Scheduler produces:

```go
type Runner interface {
    RunOnce(context.Context)
}
func New(runner Runner, interval time.Duration, logger *slog.Logger) *Scheduler
```

The runner itself resolves current enabled marketplaces through `marketplaceOperations`; scheduler holds no IDs.

- Server consumes:

```go
type SyncDispatch struct {
    Started []string `json:"started"`
    Busy []string `json:"busy"`
}

type TriggerSyncFunc func([]string) (SyncDispatch, error)
```

Define public server DTO/function types in `internal/server` so `cmd/reviews` can populate them without importing command-local types.

- [ ] **Step 1: Rewrite scheduler tests first**

Update fake runner to `RunOnce(ctx)` and make it return/record IDs from a mutable source itself. Test immediate run + interval as before, then mutate enabled IDs between ticks and assert the second invocation sees the new value. This proves scheduler does not capture IDs.

- [ ] **Step 2: Run scheduler tests to verify RED**

Run: `go test ./internal/scheduler -count=1`

Expected: compile/test failure until scheduler API changes.

- [ ] **Step 3: Simplify scheduler**

Remove `marketplaces` field and constructor argument. Log only `scheduled sync starting`; call `runner.RunOnce(ctx)`. Keep immediate first run, ticker behavior, and cancellation unchanged.

- [ ] **Step 4: Add failing admin sync response tests**

Replace channel-only fake trigger with table cases:

- `Started:["wb"]` → HTTP `202`, JSON arrays;
- `Started:["ym"], Busy:["wb"]` → HTTP `202`;
- `Busy:["wb"]`, none started → HTTP `409`;
- trigger returns unknown/disabled/invalid error → HTTP `400`, no success body;
- nil trigger → existing `503`.

The handler no longer starts its own goroutine; `TriggerSync` already dispatches safe background work.

- [ ] **Step 5: Implement sync endpoint contract**

Call `result, err := s.cfg.TriggerSync(marketplaces)` synchronously for validation/acquisition only. Map errors to `400`. Return `409` iff `len(result.Started)==0 && len(result.Busy)>0`; otherwise `202`. Return JSON:

```json
{"started":["ym"],"busy":["wb"]}
```

- [ ] **Step 6: Wire scheduler adapter to operations**

Replace `syncRunnerAdapter` with a small adapter holding `operations`. Its `RunOnce(ctx)` calls `DispatchSync(nil, afterPublish)` and logs dispatch errors. Scheduler ticks now evaluate DB credentials dynamically; `marketplaceOperations` already owns the same server-lifetime context passed by `runServe`.

- [ ] **Step 7: Run focused tests and commit**

Run:

```bash
go test -race ./internal/scheduler ./internal/server ./cmd/reviews -run 'Test(Scheduler|AdminTriggerSync|MarketplaceOperations)' -count=1
```

Expected: PASS.

```bash
git add internal/scheduler internal/server/server.go internal/server/admin_dashboard.go internal/server/admin_dashboard_test.go cmd/reviews/main.go cmd/reviews/main_test.go
git commit -m "feat(sync): coordinate dynamic marketplace runs"
```

---

### Task 9: Dynamic reply and question-answer publishers

**Files:**
- Modify: `internal/server/server.go`
- Modify: `internal/server/reply_publish.go`
- Modify: `internal/server/reply_publish_test.go`
- Modify: `internal/server/question_publish.go`
- Modify: `internal/server/question_publish_test.go`
- Modify: `cmd/reviews/main.go`

**Interfaces:**
- Replace maps with resolver functions:

```go
type ReplyPublisherResolver func(context.Context, string) (marketplace.ReplyPublisher, error)
type QuestionPublisherResolver func(context.Context, string) (marketplace.QuestionAnswerPublisher, error)

// Config fields
ResolveReplyPublisher ReplyPublisherResolver
ResolveQuestionPublisher QuestionPublisherResolver
```

- [ ] **Step 1: Update publisher tests to resolvers and add freshness cases**

Convert existing fake maps to resolver closures. Add tests where resolver returns publisher A on first call and publisher B on second; two publications must call A then B. Add resolver-error cases asserting:

- state becomes `failed`, not `unsupported`;
- persisted error is actionable;
- token/error secret sentinel does not appear if resolver returns the stable credential validation error.

Site content and disabled publish toggle remain `unsupported` without invoking resolver.

- [ ] **Step 2: Run publisher tests to verify RED**

Run: `go test ./internal/server -run 'Test(PublishReply|PublishQuestionAnswer|ReplyHandler)' -count=1`

Expected: compile/test failure until maps are replaced.

- [ ] **Step 3: Replace startup maps with resolvers**

Remove `ReplyPublishers`, `QuestionAnswerPublishers`, `replyPublishers`, and `questionAnswerPublishers`. In each publication method:

1. short-circuit site/disabled toggle as unsupported;
2. call resolver immediately;
3. resolver error → persist `failed` and log sanitized stable error;
4. publisher error → existing failed path;
5. success → existing published path.

If resolver is nil, preserve unsupported behavior for unit tests/configurations without marketplace publishing.

- [ ] **Step 4: Wire operation service resolvers**

`runServe` passes `operations.ResolveReplyPublisher` and `operations.ResolveQuestionPublisher`. Ensure `RetryPendingReplies` and `RetryPendingQuestionAnswers` resolve afresh for every queued row, so a token replacement during a long queue applies at the next item.

- [ ] **Step 5: Run server tests and commit**

Run: `go test -race ./internal/server ./cmd/reviews -count=1`

Expected: PASS.

```bash
git add internal/server/server.go internal/server/reply_publish.go internal/server/reply_publish_test.go internal/server/question_publish.go internal/server/question_publish_test.go cmd/reviews/main.go
git commit -m "refactor(publish): resolve current marketplace credentials"
```

---

### Task 10: Admin marketplace UX for token warnings and sync conflicts

**Files:**
- Modify: `web/admin/src/pages/Marketplaces.tsx`
- Modify: `web/admin/src/types.ts`
- Regenerate: `internal/server/admin_dist/**`

**Interfaces:**
- Consumes marketplace status `{id, enabled, configured, fields, warning}`.
- Consumes sync response:

```ts
type SyncDispatch = { started: string[]; busy: string[] }
```

- [ ] **Step 1: Update TypeScript contracts**

Add `SyncDispatch` to `types.ts` or locally in `Marketplaces.tsx`. Preserve `warning?: string`.

Change WB label:

```ts
wb: { token: 'Персональный WB API-токен' }
```

- [ ] **Step 2: Render WB token help and warning**

Under the WB token field render concise copy:

`Создайте персональный токен WB с категорией «Отзывы и вопросы». Базовый, тестовый и сервисный токены не поддерживаются.`

Keep backend `item.warning` visible. Do not decode JWT in the browser; backend validation is authoritative for product behavior.

- [ ] **Step 3: Handle structured sync dispatch**

Change `sync` to:

```ts
const result = await apiWrite<SyncDispatch>('POST', path)
if (result.started.length) toast.success(`Синхронизация запущена: ${result.started.join(', ')}`)
if (result.busy.length) toast.info(`Уже выполняется: ${result.busy.join(', ')}`)
```

A `409` still flows through `apiWrite` as the server error; ensure its Russian error is `Синхронизация уже выполняется`. The UI must not claim success when nothing started.

- [ ] **Step 4: Disable invalid marketplace launches**

Per-marketplace `Запуск` button: `disabled={busy !== '' || !item.enabled || !item.configured}`. For “sync all”, leave enabled when at least one marketplace is configured; backend remains authoritative.

- [ ] **Step 5: Build admin SPA**

Run from `web/admin`:

```bash
npm run build
```

Expected: Vite build succeeds with zero TypeScript errors and creates `web/admin/dist`.

- [ ] **Step 6: Replace embedded assets**

Use filesystem tools to remove the old `internal/server/admin_dist` contents and copy `web/admin/dist` into `internal/server/admin_dist`. Verify `internal/server/admin_dist/index.html` references existing hashed assets.

- [ ] **Step 7: Browser smoke test**

Run the local server with a temporary SQLite DB and use Chromium:

1. set up/login admin and open `#/settings/marketplaces`;
2. confirm the personal-token label/help text;
3. submit a generated base-token fixture and observe the Russian validation error;
4. submit a generated personal-token fixture and observe configured status without any WB API call;
5. use run-scoped browser request interception to return `202 {"started":["wb"],"busy":[]}` and then `409 {"error":"Синхронизация уже выполняется"}` for `/admin/api/sync`; verify success then busy feedback;
6. remove interception at the end of the browser run and verify YM/Ozon controls remain usable while WB is unconfigured.

Use only locally generated fixture tokens. Never use client credentials or allow fixture tokens to reach marketplace hosts.

- [ ] **Step 8: Commit Task 10**

```bash
git add web/admin/src/pages/Marketplaces.tsx web/admin/src/types.ts internal/server/admin_dist
git commit -m "feat(admin): explain WB token and sync status"
```

---

### Task 11: Operator documentation and final verification

**Files:**
- Modify: `README.md`
- Modify: `.env.example`
- Verify: all files from Tasks 1–10

**Interfaces:**
- Documentation contract: self-hosted WB setup requires a personal token with `Отзывы и вопросы`; existing base tokens block only WB until replaced.

- [ ] **Step 1: Update operator documentation**

In README credential sections (current lines 172–176 and 287–290):

- replace “API-токен” with “персональный API-токен”;
- state required category `Отзывы и вопросы`;
- say base/test/service tokens are rejected by the self-hosted service;
- state token changes apply without container restart;
- explain `429` is retried once for reads only and publication requests are not automatically retried.

In `.env.example` above `REVIEWS_WB_TOKEN`, add the same personal-token requirement in one concise comment.

- [ ] **Step 2: Format Go code**

Run the project formatter on changed Go files (`gofmt` via the repository formatter workflow). Do not reformat unrelated files.

- [ ] **Step 3: Run targeted package tests**

Run:

```bash
go test -race ./internal/marketplace/apihttp ./internal/marketplace/wbtoken ./internal/marketplace/wb ./internal/marketplace/ym ./internal/marketplace/ozon ./internal/syncer ./internal/scheduler ./internal/server ./cmd/reviews -count=1
```

Expected: PASS, zero races.

- [ ] **Step 4: Run full Go verification**

Run:

```bash
go test ./... -count=1
go vet ./...
go build ./cmd/reviews
```

Expected: all commands exit 0.

- [ ] **Step 5: Verify API migration and security guards**

Repository searches:

- `httpClient.Do` under `internal/marketplace/{wb,ym,ozon}` → no matches;
- old `ReplyPublishers` / `QuestionAnswerPublishers` maps → no matches;
- scheduler `marketplaces []string` field → no match;
- `Authorization`, `Api-Key`, `Client-Id` must occur only in client request factories/tests, never in executor errors/logging;
- no `time.Sleep` in `internal/marketplace/apihttp`;
- no added JWT/retry/job dependency in `go.mod`.

- [ ] **Step 6: Smoke the built service**

Start the built binary with a temporary SQLite DB and all marketplaces disabled. Exercise:

```text
GET /healthz -> 200
GET /admin/ -> embedded SPA loads
```

Then enable a fixture personal WB token through authenticated admin API and verify no restart is needed for status/sync dispatch. Use a local fake WB endpoint or injected test seam—never send fixture tokens to production WB.

- [ ] **Step 7: Review against approved spec**

Check every section of `docs/superpowers/specs/2026-08-13-marketplace-api-coordination-design.md`:

- shared process-wide executor;
- all API calls migrated;
- explicit policies and retry bounds;
- no write retries;
- personal WB token migration behavior;
- per-marketplace coordinator;
- dynamic scheduler/adapters/publishers/probe;
- `202/409/400` API behavior;
- UI warning/help and browser smoke;
- no non-goals accidentally implemented.

Expected: no uncovered requirement.

- [ ] **Step 8: Commit documentation/final cleanup**

```bash
git add README.md .env.example
git commit -m "docs: require personal WB API token"
```

## Execution Order and Review Gates

1. Task 1 is foundational and must complete first.
2. Tasks 2, 3, and 4 are independent after Task 1 and may run in parallel; they must agree on the constructor signatures above.
3. Tasks 5 and 6 are independent and may run in parallel with client migrations.
4. Task 7 requires Tasks 2–6.
5. Task 8 requires Tasks 6–7.
6. Task 9 requires Task 7.
7. Task 10 requires Tasks 5 and 8–9 backend contracts.
8. Task 11 runs only after the complete behavior works and the browser smoke passes.

At every task gate, review only that task's diff for spec compliance before moving on. Do not run project-wide test/build commands mid-flight when parallel workers are still editing; run them once in Task 11.
