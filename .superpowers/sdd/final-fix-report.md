# Final Fix Report — feat/legal-mitigation

Date: 2026-06-25

## Fix 1 — Reject oversized images (privacy, most important)

**File:** `internal/mediaproxy/proxy.go`

**Before (line 106):**
```go
body, err := io.ReadAll(io.LimitReader(resp.Body, h.cfg.MaxBytes))
if err != nil {
    http.Error(w, "read error", http.StatusBadGateway)
    return
}
```

**After (lines 106–114):**
```go
body, err := io.ReadAll(io.LimitReader(resp.Body, h.cfg.MaxBytes+1))
if err != nil {
    http.Error(w, "read error", http.StatusBadGateway)
    return
}
if int64(len(body)) > h.cfg.MaxBytes {
    http.Error(w, "image too large", http.StatusBadGateway)
    return
}
```

**Why:** The old code silently truncated images larger than `MaxBytes`. If the truncated bytes still decoded, the image was served—possibly with an un-blurred face. The fix reads one extra byte to distinguish "fits" from "oversized". Oversized images get a 502; the decode-failure fallback (return original bytes) still applies for images that fit.

---

## Fix 2 — SSRF allowlist suffix-spoof test

**File:** `internal/mediaproxy/allowlist_test.go`

**Before (line 20):**
```go
"http://wbbasket.ru.evil.com/x", // suffix-spoof
```

**After (lines 20–21):**
```go
"http://wbbasket.ru.evil.com/x",  // suffix-spoof (rejected by scheme check)
"https://wbbasket.ru.evil.com/x", // suffix-spoof via https (exercises host-suffix boundary)
```

**Why:** The original http case was rejected by the scheme validator before reaching the `strings.HasSuffix(host, "."+s)` logic. The new https case exercises that host-suffix boundary directly.

---

## Fix 3 — Harden migration test error handling

**File:** `internal/store/migrate_pd_test.go`

### (a) Seed create — check error

**Before (line 16):**
```go
s.db.Create(&Review{
    Marketplace: "wb", ExternalReviewID: "legacy1",
    ...
})
```

**After:**
```go
if err := s.db.Create(&Review{
    Marketplace: "wb", ExternalReviewID: "legacy1",
    ...
}).Error; err != nil {
    t.Fatal(err)
}
```

### (b) Second-run idempotency — capture and assert error

**Before (line 40):**
```go
n2, _ := s.ScrubPersonalData(testCtx(t))
```

**After:**
```go
n2, err2 := s.ScrubPersonalData(testCtx(t))
if err2 != nil {
    t.Fatal(err2)
}
```

---

## Fix 4 — New proxy tests: 502 path and cache-hit path

**File:** `internal/mediaproxy/proxy_test.go`

Added two tests before `TestRedirectGuard`:

### TestProxyUpstreamErrorReturns502
Injects a fetch returning HTTP 503; asserts the handler responds 502.

### TestProxyCacheHitReusesResult
Injects a counting fetch; requests the same URL twice; asserts:
- fetch was called exactly once (cache hit on second request)
- both responses are 200

---

## Test Results

### Focused (`./internal/mediaproxy/ ./internal/store/`)
```
--- PASS: TestHostAllowed
--- PASS: TestBlurRegionChangesPixels
--- PASS: TestProxyRejectsDisallowedHost
--- PASS: TestProxyServesAllowedImage
--- PASS: TestProxyMissingURL
--- PASS: TestProxyUpstreamErrorReturns502   [NEW]
--- PASS: TestProxyCacheHitReusesResult      [NEW]
--- PASS: TestRedirectGuard
ok  reviews/internal/mediaproxy  0.004s

--- PASS: TestScrubPersonalData              [HARDENED]
--- PASS: TestSupplierArticleFromRaw
... (all store tests pass)
ok  reviews/internal/store  0.146s
```

### Full suite (`go build ./... && go test ./...`)
All 14 packages: GREEN. No failures, no skips, no build errors.

---

## Concerns

None. All four fixes are surgical; no unrelated code was touched.
