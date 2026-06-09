# Stage 2: Auth + Admin Scaffold + Write-API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add admin authentication (login/password + server-side session), the security middleware layer, a tenant-ready schema, and an embedded React SPA shell that the admin authenticates into — so later stages have a protected surface to build on.

**Architecture:** New `internal/auth` package (argon2id hashing + secure token generation). New `admin_users`/`sessions` GORM models with store methods. The HTTP server gains a session-cookie middleware, security headers, CSRF protection, a setup wizard (only usable while no admin exists), login/logout endpoints, and serves a Vite-built React SPA (embedded via `go:embed`) under `/admin/`. A `tenant_id` column is added to tenant-scoped tables with a single implicit tenant (id 1) now.

**Tech Stack:** Go 1.26, `golang.org/x/crypto/argon2`, GORM, Go 1.22+ `net/http` method routing, React 18 + Vite + TypeScript, `go:embed`.

**Spec:** `docs/superpowers/specs/2026-06-09-reviews-admin-and-containerization-design.md` (Parts "Архитектура", "Модель данных", "Безопасность", FR-1).

**Depends on:** Stage 1 (containerization) — not strictly required to build, but the Dockerfile gains an SPA build stage here (Task 8).

---

## File Structure

- Create: `internal/auth/password.go` — argon2id hash/verify.
- Create: `internal/auth/password_test.go`
- Create: `internal/auth/token.go` — secure random session tokens.
- Create: `internal/auth/token_test.go`
- Create: `internal/store/auth_models.go` — `AdminUser`, `Session` models + `TenantID` additions.
- Create: `internal/store/auth.go` — store methods for users/sessions.
- Create: `internal/store/auth_test.go`
- Modify: `internal/store/store.go` — add new models to `Migrate`.
- Modify: `internal/store/models.go` — add `TenantID` to `Product`, `Review`, `ProductMarketplaceLink`.
- Create: `internal/server/middleware.go` — security headers, session auth, CSRF.
- Create: `internal/server/admin_auth.go` — setup/login/logout/me handlers.
- Create: `internal/server/admin_auth_test.go`
- Create: `internal/server/spa.go` — embedded SPA file server.
- Modify: `internal/server/server.go` — register admin routes, accept session TTL config.
- Create: `web/admin/` — Vite + React + TS project (its own `package.json`).
- Create: `internal/server/admin_dist/.gitkeep` — placeholder; Vite build output embedded from here.
- Modify: `Dockerfile` — add a Node build stage for the SPA.
- Modify: `cmd/reviews/main.go` — pass session config into `server.New`.

---

## Task 1: argon2id password hashing

**Files:**
- Create: `internal/auth/password.go`
- Create: `internal/auth/password_test.go`

- [ ] **Step 1: Add the dependency**

Run: `go get golang.org/x/crypto/argon2@latest`
Expected: `go.mod`/`go.sum` updated with `golang.org/x/crypto`.

- [ ] **Step 2: Write the failing test**

Create `internal/auth/password_test.go`:

```go
package auth

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if hash == "" {
		t.Fatal("empty hash")
	}

	ok, err := VerifyPassword(hash, "correct horse battery staple")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !ok {
		t.Fatal("expected password to verify")
	}

	bad, err := VerifyPassword(hash, "wrong password")
	if err != nil {
		t.Fatalf("verify wrong: %v", err)
	}
	if bad {
		t.Fatal("expected wrong password to fail")
	}
}

func TestHashIsSaltedPerCall(t *testing.T) {
	a, _ := HashPassword("same")
	b, _ := HashPassword("same")
	if a == b {
		t.Fatal("expected distinct salts to produce distinct hashes")
	}
}

func TestVerifyRejectsMalformedHash(t *testing.T) {
	if _, err := VerifyPassword("not-a-phc-string", "x"); err == nil {
		t.Fatal("expected error for malformed hash")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/auth/ -run TestHash -v`
Expected: FAIL — `undefined: HashPassword`.

- [ ] **Step 4: Write minimal implementation**

Create `internal/auth/password.go`:

```go
// Package auth provides password hashing and session token primitives for the
// admin panel.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonTime    = 1
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16
)

var errMalformedHash = errors.New("auth: malformed password hash")

// HashPassword returns an argon2id PHC-format hash string.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword reports whether password matches the encoded argon2id hash.
func VerifyPassword(encoded, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errMalformedHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, errMalformedHash
	}
	var memory uint32
	var time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return false, errMalformedHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, errMalformedHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, errMalformedHash
	}
	got := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/auth/ -run TestHash -v` then `go test ./internal/auth/ -run TestVerify -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/auth/password.go internal/auth/password_test.go go.mod go.sum
git commit -m "feat(auth): argon2id password hashing"
```

---

## Task 2: Secure session tokens

**Files:**
- Create: `internal/auth/token.go`
- Create: `internal/auth/token_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/auth/token_test.go`:

```go
package auth

import "testing"

func TestNewSessionTokenIsUniqueAndLong(t *testing.T) {
	a, err := NewSessionToken()
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	b, _ := NewSessionToken()
	if a == b {
		t.Fatal("expected unique tokens")
	}
	if len(a) < 32 {
		t.Fatalf("token too short: %d", len(a))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/auth/ -run TestNewSessionToken -v`
Expected: FAIL — `undefined: NewSessionToken`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/auth/token.go`:

```go
package auth

import (
	"crypto/rand"
	"encoding/base64"
)

// NewSessionToken returns a URL-safe, unpredictable 256-bit token.
func NewSessionToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/auth/ -run TestNewSessionToken -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/token.go internal/auth/token_test.go
git commit -m "feat(auth): secure session token generation"
```

---

## Task 3: Admin user & session models + tenant column

**Files:**
- Create: `internal/store/auth_models.go`
- Modify: `internal/store/models.go`
- Modify: `internal/store/store.go`

- [ ] **Step 1: Create the new models**

Create `internal/store/auth_models.go`:

```go
package store

import "time"

// DefaultTenantID is the single implicit tenant used until multi-tenancy is
// enabled. All tenant-scoped rows use this value for now.
const DefaultTenantID uint = 1

type AdminUser struct {
	ID           uint   `gorm:"primaryKey"`
	TenantID     uint   `gorm:"not null;default:1;index"`
	Login        string `gorm:"size:128;not null;uniqueIndex"`
	PasswordHash string `gorm:"not null"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Session struct {
	Token     string `gorm:"primaryKey;size:64"`
	UserID    uint   `gorm:"not null;index"`
	ExpiresAt time.Time `gorm:"not null;index"`
	CreatedAt time.Time
}
```

- [ ] **Step 2: Add `TenantID` to existing tenant-scoped models**

In `internal/store/models.go`, add `TenantID uint` as the first field after `ID` in `Product`, `Review`, and `ProductMarketplaceLink`. For `Product`:

```go
type Product struct {
	ID             uint `gorm:"primaryKey"`
	TenantID       uint `gorm:"not null;default:1;index"`
	Title          *string
	SiteProductKey *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
```

For `Review`, add after `ID`:

```go
	TenantID          uint   `gorm:"not null;default:1;index"`
```

For `ProductMarketplaceLink`, add after `ID`:

```go
	TenantID          uint `gorm:"not null;default:1;index"`
```

Note: queries are NOT changed in this stage — the default keeps all existing behavior identical. Multi-tenant filtering is deferred to the SaaS phase.

- [ ] **Step 3: Register new models in migrations**

In `internal/store/store.go`, extend the `AutoMigrate` call:

```go
func (s *Store) Migrate(ctx context.Context) error {
	return s.db.WithContext(ctx).AutoMigrate(
		&Product{},
		&ProductMarketplaceLink{},
		&Review{},
		&ReviewMedia{},
		&SyncState{},
		&SyncRun{},
		&AdminUser{},
		&Session{},
	)
}
```

- [ ] **Step 4: Verify build and existing tests still pass**

Run: `go build ./... && go test ./internal/store/ -v`
Expected: build OK; existing store tests PASS (the added columns default to 1 and break nothing).

- [ ] **Step 5: Commit**

```bash
git add internal/store/auth_models.go internal/store/models.go internal/store/store.go
git commit -m "feat(store): admin_users/sessions models and tenant_id columns"
```

---

## Task 4: Store methods for users and sessions

**Files:**
- Create: `internal/store/auth.go`
- Create: `internal/store/auth_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/store/auth_test.go`:

```go
package store

import (
	"context"
	"testing"
	"time"

	"reviews/internal/config"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(config.DBConfig{Driver: "sqlite", DSN: "file::memory:?cache=shared"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return st
}

func TestAdminUserLifecycle(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	if n, _ := st.CountAdminUsers(ctx); n != 0 {
		t.Fatalf("expected 0 users, got %d", n)
	}

	user, err := st.CreateAdminUser(ctx, "admin", "hash-1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if user.ID == 0 || user.TenantID != DefaultTenantID {
		t.Fatalf("unexpected user %+v", user)
	}

	got, err := st.GetAdminUserByLogin(ctx, "admin")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.PasswordHash != "hash-1" {
		t.Fatalf("unexpected hash %q", got.PasswordHash)
	}

	if err := st.UpdateAdminPassword(ctx, user.ID, "hash-2"); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ = st.GetAdminUserByLogin(ctx, "admin")
	if got.PasswordHash != "hash-2" {
		t.Fatalf("password not updated: %q", got.PasswordHash)
	}

	if n, _ := st.CountAdminUsers(ctx); n != 1 {
		t.Fatalf("expected 1 user, got %d", n)
	}
}

func TestSessionLifecycle(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	user, _ := st.CreateAdminUser(ctx, "admin", "h")

	expires := time.Now().Add(time.Hour)
	if err := st.CreateSession(ctx, "tok-1", user.ID, expires); err != nil {
		t.Fatalf("create session: %v", err)
	}

	sess, err := st.GetValidSession(ctx, "tok-1", time.Now())
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if sess.UserID != user.ID {
		t.Fatalf("unexpected session user %d", sess.UserID)
	}

	// Expired session is not returned.
	if _, err := st.GetValidSession(ctx, "tok-1", time.Now().Add(2*time.Hour)); err == nil {
		t.Fatal("expected expired session to be rejected")
	}

	if err := st.DeleteSession(ctx, "tok-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := st.GetValidSession(ctx, "tok-1", time.Now()); err == nil {
		t.Fatal("expected deleted session to be gone")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run 'TestAdminUserLifecycle|TestSessionLifecycle' -v`
Expected: FAIL — `undefined: (*Store).CountAdminUsers` etc.

- [ ] **Step 3: Write minimal implementation**

Create `internal/store/auth.go`:

```go
package store

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// ErrNotFound is returned when a lookup yields no row.
var ErrNotFound = errors.New("store: not found")

func (s *Store) CountAdminUsers(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.WithContext(ctx).Model(&AdminUser{}).Count(&n).Error
	return n, err
}

func (s *Store) CreateAdminUser(ctx context.Context, login, passwordHash string) (AdminUser, error) {
	user := AdminUser{TenantID: DefaultTenantID, Login: login, PasswordHash: passwordHash}
	if err := s.db.WithContext(ctx).Create(&user).Error; err != nil {
		return AdminUser{}, err
	}
	return user, nil
}

func (s *Store) GetAdminUserByLogin(ctx context.Context, login string) (AdminUser, error) {
	var user AdminUser
	err := s.db.WithContext(ctx).Where("login = ?", login).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return AdminUser{}, ErrNotFound
	}
	return user, err
}

func (s *Store) UpdateAdminPassword(ctx context.Context, userID uint, passwordHash string) error {
	return s.db.WithContext(ctx).Model(&AdminUser{}).
		Where("id = ?", userID).
		Update("password_hash", passwordHash).Error
}

func (s *Store) CreateSession(ctx context.Context, token string, userID uint, expiresAt time.Time) error {
	return s.db.WithContext(ctx).Create(&Session{Token: token, UserID: userID, ExpiresAt: expiresAt}).Error
}

func (s *Store) GetValidSession(ctx context.Context, token string, now time.Time) (Session, error) {
	var sess Session
	err := s.db.WithContext(ctx).
		Where("token = ? AND expires_at > ?", token, now).
		First(&sess).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Session{}, ErrNotFound
	}
	return sess, err
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	return s.db.WithContext(ctx).Where("token = ?", token).Delete(&Session{}).Error
}

func (s *Store) DeleteExpiredSessions(ctx context.Context, now time.Time) error {
	return s.db.WithContext(ctx).Where("expires_at <= ?", now).Delete(&Session{}).Error
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run 'TestAdminUserLifecycle|TestSessionLifecycle' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/auth.go internal/store/auth_test.go
git commit -m "feat(store): admin user and session persistence methods"
```

---

## Task 5: Security headers and session middleware

**Files:**
- Create: `internal/server/middleware.go`

- [ ] **Step 1: Write the middleware**

Create `internal/server/middleware.go`:

```go
package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"reviews/internal/store"
)

type ctxKey string

const userIDKey ctxKey = "adminUserID"

const sessionCookieName = "reviews_session"

// securityHeaders sets conservative defaults for all responses.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

// requireSession rejects requests without a valid session cookie.
func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, errors.New("authentication required"))
			return
		}
		sess, err := s.store.GetValidSession(r.Context(), cookie.Value, time.Now())
		if err != nil {
			writeError(w, http.StatusUnauthorized, errors.New("authentication required"))
			return
		}
		ctx := context.WithValue(r.Context(), userIDKey, sess.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func userIDFromContext(ctx context.Context) (uint, bool) {
	id, ok := ctx.Value(userIDKey).(uint)
	return id, ok
}

// setSessionCookie writes the session cookie with hardened attributes.
func setSessionCookie(w http.ResponseWriter, token string, expires time.Time, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

var _ = store.ErrNotFound // ensure store import is used across the package
```

Note: remove the trailing `var _ = store.ErrNotFound` line once `admin_auth.go` (Task 6) references the `store` package; it is only there to keep this file compiling in isolation if implemented first. If `store` is already used, omit it.

- [ ] **Step 2: Verify build**

Run: `go build ./internal/server/`
Expected: compiles.

- [ ] **Step 3: Commit**

```bash
git add internal/server/middleware.go
git commit -m "feat(server): security headers and session middleware"
```

---

## Task 6: Setup, login, logout, and CSRF

**Files:**
- Create: `internal/server/admin_auth.go`
- Create: `internal/server/admin_auth_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/server/admin_auth_test.go`:

```go
package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"log/slog"
	"io"

	"reviews/internal/config"
	"reviews/internal/store"
)

func newAuthTestServer(t *testing.T) *Server {
	t.Helper()
	st, err := store.Open(config.DBConfig{Driver: "sqlite", DSN: "file::memory:?cache=shared"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(st, Config{SessionTTL: time.Hour}, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestSetupThenLoginFlow(t *testing.T) {
	s := newAuthTestServer(t)
	mux := s.adminMux()

	// Setup the first admin.
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"login":"admin","password":"s3cret-pass"}`)
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/api/setup", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup status = %d, body=%s", rec.Code, rec.Body.String())
	}

	// Second setup attempt is rejected (admin already exists).
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/api/setup",
		strings.NewReader(`{"login":"x","password":"yyyyyyyy"}`)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("second setup status = %d", rec.Code)
	}

	// Login with correct credentials sets a session cookie.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/api/login",
		strings.NewReader(`{"login":"admin","password":"s3cret-pass"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Set-Cookie"), sessionCookieName) {
		t.Fatalf("expected session cookie, got %q", rec.Header().Get("Set-Cookie"))
	}

	// Wrong password is rejected.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/api/login",
		strings.NewReader(`{"login":"admin","password":"wrong"}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad login status = %d", rec.Code)
	}
}
```

Add `"time"` to the test imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestSetupThenLoginFlow -v`
Expected: FAIL — `undefined: (*Server).adminMux`, `Config.SessionTTL`.

- [ ] **Step 3: Write the implementation**

Create `internal/server/admin_auth.go`:

```go
package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"reviews/internal/auth"
	"reviews/internal/store"
)

type credentials struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

func (c credentials) validate() error {
	if strings.TrimSpace(c.Login) == "" {
		return errors.New("login is required")
	}
	if len(c.Password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	return nil
}

// handleSetup creates the first admin user. It only works while no admin exists.
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.CountAdminUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if n > 0 {
		writeError(w, http.StatusConflict, errors.New("admin already configured"))
		return
	}

	var creds credentials
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
		return
	}
	if err := creds.validate(); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	hash, err := auth.HashPassword(creds.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if _, err := s.store.CreateAdminUser(r.Context(), creds.Login, hash); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "ok"})
}

// handleLogin verifies credentials and starts a session.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var creds credentials
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
		return
	}

	user, err := s.store.GetAdminUserByLogin(r.Context(), creds.Login)
	if err != nil {
		// Generic message: do not reveal whether the login exists.
		writeError(w, http.StatusUnauthorized, errors.New("invalid login or password"))
		return
	}
	ok, err := auth.VerifyPassword(user.PasswordHash, creds.Password)
	if err != nil || !ok {
		writeError(w, http.StatusUnauthorized, errors.New("invalid login or password"))
		return
	}

	token, err := auth.NewSessionToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	expires := time.Now().Add(s.cfg.SessionTTL)
	if err := s.store.CreateSession(r.Context(), token, user.ID, expires); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	setSessionCookie(w, token, expires, s.cfg.SecureCookies)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleLogout deletes the current session.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		_ = s.store.DeleteSession(r.Context(), cookie.Value)
	}
	clearSessionCookie(w, s.cfg.SecureCookies)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleMe reports the authenticated admin (used by the SPA on load).
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	id, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, errors.New("authentication required"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user_id": id})
}

// handleSetupStatus tells the SPA whether to show the setup wizard or login.
func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.CountAdminUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"needs_setup": n == 0})
}

var _ = store.ErrNotFound
```

- [ ] **Step 4: Add `adminMux` and config fields (Task 7 completes wiring)**

This test depends on `adminMux` and `Config.SessionTTL`/`Config.SecureCookies`, defined in Task 7. Implement Task 7 before re-running. (Tasks 6 and 7 form one commit boundary; commit after Task 7's build passes.)

---

## Task 7: Wire admin routes into the server

**Files:**
- Modify: `internal/server/server.go`
- Modify: `cmd/reviews/main.go`

- [ ] **Step 1: Extend `server.Config` and `New`**

In `internal/server/server.go`, add fields to `Config`:

```go
type Config struct {
	Addr               string
	StaticDir          string
	ProductURLTemplate string
	ProductLinks       map[string]string
	SessionTTL         time.Duration
	SecureCookies      bool
}
```

In `New`, after the existing defaults, add:

```go
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = 24 * time.Hour
	}
```

- [ ] **Step 2: Add `adminMux` and register it in `Run`**

Add to `internal/server/server.go`:

```go
// adminMux builds the admin API + SPA routes. Public auth endpoints (setup,
// login, status) are unauthenticated; everything else requires a session.
func (s *Server) adminMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/api/setup-status", s.handleSetupStatus)
	mux.HandleFunc("POST /admin/api/setup", s.handleSetup)
	mux.HandleFunc("POST /admin/api/login", s.handleLogin)
	mux.HandleFunc("POST /admin/api/logout", s.handleLogout)

	protected := http.NewServeMux()
	protected.HandleFunc("GET /admin/api/me", s.handleMe)
	mux.Handle("/admin/api/me", s.requireSession(protected))

	// SPA assets and client-side routes (Task 8 provides adminSPAHandler).
	mux.Handle("/admin/", s.adminSPAHandler())
	return mux
}
```

In `Run`, register the admin mux on the main mux before the catch-all static handler:

```go
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/reviews", s.handleReviews)
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.Handle("/admin/", s.adminMux())
	mux.Handle("/", http.FileServer(http.Dir(s.cfg.StaticDir)))
```

Wrap the handler with `securityHeaders`:

```go
	s.server = &http.Server{
		Addr:              s.cfg.Addr,
		Handler:           securityHeaders(s.logRequests(mux)),
		ReadHeaderTimeout: 5 * time.Second,
	}
```

- [ ] **Step 3: Pass session config from `serve`**

In `cmd/reviews/main.go` `runServe`, add to the `server.Config` literal:

```go
		SessionTTL:    24 * time.Hour,
		SecureCookies: os.Getenv("REVIEWS_INSECURE_COOKIES") == "",
```

(Allows local HTTP testing by setting `REVIEWS_INSECURE_COOKIES=1`; production over HTTPS keeps `Secure` on by default.)

- [ ] **Step 4: Run the auth flow test**

Run: `go test ./internal/server/ -run TestSetupThenLoginFlow -v`
Expected: PASS. (Requires Task 8's `adminSPAHandler` to compile — implement Task 8's `spa.go` first, or temporarily stub `adminSPAHandler` returning `http.NotFoundHandler()`; the test does not hit SPA routes.)

- [ ] **Step 5: Commit (Tasks 6+7+stub)**

```bash
git add internal/server/admin_auth.go internal/server/admin_auth_test.go internal/server/server.go cmd/reviews/main.go
git commit -m "feat(server): admin setup/login/logout endpoints and routing"
```

---

## Task 8: Embedded React SPA shell

**Files:**
- Create: `web/admin/` (Vite React TS project)
- Create: `internal/server/spa.go`
- Create: `internal/server/admin_dist/.gitkeep`
- Modify: `Dockerfile`

- [ ] **Step 1: Scaffold the SPA**

Run:
```bash
cd web && npm create vite@latest admin -- --template react-ts && cd admin && npm install
```
Expected: `web/admin/` created with `package.json`, `src/`, `index.html`.

- [ ] **Step 2: Configure Vite base path and build output**

Edit `web/admin/vite.config.ts` to serve under `/admin/` and build into the Go embed dir:

```ts
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'node:path'

export default defineConfig({
  plugins: [react()],
  base: '/admin/',
  build: {
    outDir: path.resolve(__dirname, '../../internal/server/admin_dist'),
    emptyOutDir: true,
  },
})
```

- [ ] **Step 3: Minimal app that exercises the auth API**

Replace `web/admin/src/App.tsx`:

```tsx
import { useEffect, useState } from 'react'

type Mode = 'loading' | 'setup' | 'login' | 'authed'

async function post(path: string, body: unknown) {
  const res = await fetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error((await res.json()).error ?? 'request failed')
}

export default function App() {
  const [mode, setMode] = useState<Mode>('loading')
  const [login, setLogin] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')

  useEffect(() => {
    fetch('/admin/api/me').then((r) => {
      if (r.ok) return setMode('authed')
      fetch('/admin/api/setup-status')
        .then((s) => s.json())
        .then((d: { needs_setup: boolean }) => setMode(d.needs_setup ? 'setup' : 'login'))
    })
  }, [])

  async function submit() {
    setError('')
    try {
      await post(mode === 'setup' ? '/admin/api/setup' : '/admin/api/login', { login, password })
      setMode('authed')
    } catch (e) {
      setError((e as Error).message)
    }
  }

  if (mode === 'loading') return <p>Loading…</p>
  if (mode === 'authed') return <h1>Reviews admin</h1>

  return (
    <main style={{ maxWidth: 360, margin: '4rem auto', fontFamily: 'system-ui' }}>
      <h1>{mode === 'setup' ? 'Create admin' : 'Sign in'}</h1>
      <input placeholder="login" value={login} onChange={(e) => setLogin(e.target.value)} />
      <input placeholder="password" type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
      <button onClick={submit}>{mode === 'setup' ? 'Create' : 'Sign in'}</button>
      {error && <p style={{ color: 'crimson' }}>{error}</p>}
    </main>
  )
}
```

- [ ] **Step 4: Build the SPA**

Run: `cd web/admin && npm run build`
Expected: assets written to `internal/server/admin_dist/` (`index.html`, `assets/…`).

- [ ] **Step 5: Create the embed + handler**

Create `internal/server/admin_dist/.gitkeep` (empty), then create `internal/server/spa.go`:

```go
package server

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:admin_dist
var adminDistFS embed.FS

// adminSPAHandler serves the embedded SPA, falling back to index.html for
// client-side routes (anything not matching a built asset).
func (s *Server) adminSPAHandler() http.Handler {
	sub, err := fs.Sub(adminDistFS, "admin_dist")
	if err != nil {
		panic(err) // build-time embed guarantees this never fails
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// API routes are handled elsewhere; this only serves /admin/* assets.
		trimmed := strings.TrimPrefix(r.URL.Path, "/admin/")
		if trimmed == "" {
			trimmed = "index.html"
		}
		if _, err := fs.Stat(sub, trimmed); err != nil {
			// Unknown path → SPA entry point for client-side routing.
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/admin/index.html"
			http.StripPrefix("/admin/", fileServer).ServeHTTP(w, r2)
			return
		}
		http.StripPrefix("/admin/", fileServer).ServeHTTP(w, r)
	})
}
```

Note: if Task 7 used a stub `adminSPAHandler`, remove the stub now.

- [ ] **Step 6: Verify backend build + tests with embedded assets**

Run: `go build ./... && go test ./internal/server/ -v`
Expected: build OK (embed dir now non-empty); auth tests PASS.

- [ ] **Step 7: Add the SPA build stage to the Dockerfile**

In `Dockerfile`, add a node stage before the Go builder and copy its output in so `go:embed` finds populated assets:

```dockerfile
# ---- web builder ----
FROM node:22-bookworm AS web
WORKDIR /web/admin
COPY web/admin/package.json web/admin/package-lock.json ./
RUN npm ci
COPY web/admin ./
RUN npm run build   # writes to /internal/server/admin_dist via vite outDir
```

Adjust the Vite `outDir` for the container path or copy explicitly. Simplest: in the Go builder stage, copy the built assets into the source tree before `go build`:

```dockerfile
# ---- builder ----
FROM golang:1.26-bookworm AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /web/admin/dist ./internal/server/admin_dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" -o /out/reviews ./cmd/reviews
```

For this to work, set Vite `outDir` to `./dist` (local default) for the container and keep the host build pointing at `internal/server/admin_dist`. To avoid two configs, standardize on Vite `outDir: 'dist'` and add a copy step in BOTH the Dockerfile (above) and a host build script. Update `web/admin/vite.config.ts` `outDir` to `path.resolve(__dirname, 'dist')` and create `web/admin/build-embed.sh`:

```sh
#!/usr/bin/env sh
set -eu
npm run build
rm -rf ../../internal/server/admin_dist
cp -r dist ../../internal/server/admin_dist
echo "embedded admin SPA into internal/server/admin_dist"
```

Make it executable: `chmod +x web/admin/build-embed.sh`. Host workflow becomes `cd web/admin && ./build-embed.sh`.

- [ ] **Step 8: Update `.gitignore` and `.dockerignore`**

Add to `.gitignore`: `web/admin/node_modules/` and `web/admin/dist/`. Keep `internal/server/admin_dist/` committed (it is the embed source; commit a built copy or the `.gitkeep` + CI build — for this project, commit the built `index.html` so `go build` works without Node). Remove `internal/server/admin_dist` from `.dockerignore` if present.

- [ ] **Step 9: End-to-end manual verification**

```bash
cd web/admin && ./build-embed.sh && cd ../..
go run ./cmd/reviews serve --addr 127.0.0.1:8080 &
sleep 1
curl -fsS http://127.0.0.1:8080/admin/api/setup-status   # {"needs_setup":true}
curl -fsS -X POST http://127.0.0.1:8080/admin/api/setup -d '{"login":"admin","password":"s3cret-pass"}'
# open http://127.0.0.1:8080/admin/ in a browser → login screen
kill %1
```
Expected: setup-status returns `needs_setup:true`, setup returns `{"status":"ok"}`, `/admin/` serves the SPA.

- [ ] **Step 10: Commit**

```bash
git add web/admin internal/server/spa.go internal/server/admin_dist .gitignore .dockerignore Dockerfile
git commit -m "feat(admin): embedded React SPA shell with auth screens"
```

---

## Self-Review Notes

- **Spec coverage:** argon2id + session cookie (Tasks 1–7), CSRF — see note below, security headers (Task 5), generic login error (Task 6), setup wizard guarded by `CountAdminUsers` (Task 6, FR-1), `tenant_id` columns with default tenant (Task 3), embedded React SPA (Task 8).
- **CSRF gap:** This plan ships SameSite=Lax cookies (primary CSRF mitigation for a same-origin SPA). A double-submit CSRF token, mentioned in the spec, is deferred to the first task of Stage 3 (where state-changing write endpoints land) and should be added as a `requireCSRF` middleware alongside them. Flagged here so it is not silently dropped.
- **Type consistency:** `server.Config` gains `SessionTTL`/`SecureCookies` (Task 7), used by `handleLogin` (Task 6). `adminSPAHandler` defined in Task 8, referenced in Task 7 (stub until then). Store methods (`CountAdminUsers`, `CreateAdminUser`, `GetAdminUserByLogin`, `CreateSession`, `GetValidSession`, `DeleteSession`) match between Task 4 definitions and Task 6 call sites.
- **Build-without-Node:** committing a built `admin_dist` keeps `go build` working in CI/local without Node; the Dockerfile still rebuilds it fresh.
