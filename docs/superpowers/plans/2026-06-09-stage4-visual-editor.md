# Stage 4: Visual Editor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the admin customize the widget's theme, typography, layout, and element visibility through a live-preview editor, publish versioned configs, and have the embedded widget fetch and apply the active config for both the product-card and homepage contexts. Plus an embed-snippet generator.

**Architecture:** New `widget_configs` table holds versioned JSON payloads per context (`product`/`homepage`), one marked active. Admin API: get active, publish (new active version), list versions, rollback. Public `GET /api/widget-config?context=` returns the active payload. `loader.js` fetches it and applies CSS variables + behavior flags. The React editor renders controls + a live preview that mounts the real widget against an in-browser draft; "Publish" persists it. The editor also generates the install snippet (FR-7).

**Tech Stack:** Go 1.26, GORM, React + Vite + TS, the existing vanilla widget (`web/reviews-widget`).

**Spec:** `docs/superpowers/specs/2026-06-09-reviews-admin-and-containerization-design.md` (FR-6, FR-7; "Доставка конфига в виджет").

**Depends on:** Stage 2 (auth/SPA) and Stage 3 (admin mux, CSRF, API client).

**Pre-implementation reading:** inspect `web/reviews-widget/loader.js` (the `CFG` object + `window.REVIEWS_EMBED_CONFIG` merge) and `web/reviews-widget/reviews-widget.js` / `.css` (CSS custom properties like `--rw-accent`) to align the payload schema with the actual variables the widget consumes.

---

## Config payload schema (shared contract)

The JSON payload stored per config version and served to the widget:

```jsonc
{
  "theme": {
    "accent": "#2f7a5b",
    "accentInk": "#ffffff",
    "text": "#1f2520",
    "muted": "#687067",
    "panel": "#ffffff",
    "border": "#d9ded7",
    "dark": false
  },
  "typography": {
    "fontFamily": "inherit",   // "inherit" or a CSS font-family string
    "scale": 1.0,              // multiplier for base font size
    "radius": 12,              // px corner radius
    "density": "comfortable"   // "comfortable" | "compact"
  },
  "layout": {
    "mode": "list",            // "list" | "grid" | "carousel"
    "columns": 2,              // used by grid
    "pageSize": 6,
    "pagination": "more"       // "more" | "pages"
  },
  "visibility": {
    "photos": true,
    "sellerAnswers": true,
    "prosCons": true,
    "marketplaceBadges": true,
    "ratingDistribution": true,
    "filters": true
  }
}
```

Theme keys map to the widget's `--rw-*` CSS variables. This schema is the single source of truth referenced by the store, the public endpoint, the widget, and the editor.

---

## File Structure

- Create: `internal/store/widget_config_models.go` — `WidgetConfig` model.
- Create: `internal/store/widget_config.go` — publish/get/list/rollback methods.
- Create: `internal/store/widget_config_test.go`
- Modify: `internal/store/store.go` — add `WidgetConfig` to migrations.
- Create: `internal/server/admin_widget_config.go` — admin endpoints + public `/api/widget-config`.
- Create: `internal/server/admin_widget_config_test.go`
- Modify: `internal/server/server.go` — register routes.
- Modify: `web/reviews-widget/loader.js` — fetch and apply the active config.
- Modify: `web/reviews-widget/reviews-widget.js` — honor visibility/layout flags.
- Create: `web/admin/src/pages/Editor.tsx` — controls + live preview + publish.
- Create: `web/admin/src/pages/Embed.tsx` — snippet generator.
- Modify: `web/admin/src/App.tsx` — add nav entries.

---

## Task 1: WidgetConfig model and store methods

**Files:**
- Create: `internal/store/widget_config_models.go`
- Create: `internal/store/widget_config.go`
- Create: `internal/store/widget_config_test.go`
- Modify: `internal/store/store.go`

- [ ] **Step 1: Create the model**

Create `internal/store/widget_config_models.go`:

```go
package store

import "time"

// WidgetConfig is a versioned widget configuration payload for one context
// ("product" or "homepage"). Exactly one row per (tenant, context) is active.
type WidgetConfig struct {
	ID        uint   `gorm:"primaryKey"`
	TenantID  uint   `gorm:"not null;default:1;index:idx_widget_ctx"`
	Context   string `gorm:"size:16;not null;index:idx_widget_ctx"`
	Version   int    `gorm:"not null"`
	Payload   string `gorm:"not null"` // JSON
	IsActive  bool   `gorm:"not null;default:false;index"`
	CreatedAt time.Time
}
```

Add `&WidgetConfig{}` to `AutoMigrate` in `internal/store/store.go`.

- [ ] **Step 2: Write the failing test**

Create `internal/store/widget_config_test.go`:

```go
package store

import (
	"context"
	"testing"
)

func TestPublishGetListRollback(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	// No config yet → ErrNotFound.
	if _, err := st.GetActiveWidgetConfig(ctx, "product"); err == nil {
		t.Fatal("expected ErrNotFound for missing config")
	}

	v1, err := st.PublishWidgetConfig(ctx, "product", `{"theme":{"accent":"#111"}}`)
	if err != nil {
		t.Fatalf("publish v1: %v", err)
	}
	if v1.Version != 1 || !v1.IsActive {
		t.Fatalf("unexpected v1 %+v", v1)
	}

	v2, err := st.PublishWidgetConfig(ctx, "product", `{"theme":{"accent":"#222"}}`)
	if err != nil {
		t.Fatalf("publish v2: %v", err)
	}
	if v2.Version != 2 {
		t.Fatalf("expected version 2, got %d", v2.Version)
	}

	active, _ := st.GetActiveWidgetConfig(ctx, "product")
	if active.Version != 2 {
		t.Fatalf("expected active v2, got v%d", active.Version)
	}

	versions, _ := st.ListWidgetConfigVersions(ctx, "product")
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}

	// Rollback to v1.
	if err := st.SetActiveWidgetConfigVersion(ctx, "product", 1); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	active, _ = st.GetActiveWidgetConfig(ctx, "product")
	if active.Version != 1 {
		t.Fatalf("expected active v1 after rollback, got v%d", active.Version)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestPublishGetListRollback -v`
Expected: FAIL — undefined methods.

- [ ] **Step 4: Implement the methods**

Create `internal/store/widget_config.go`:

```go
package store

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

func (s *Store) GetActiveWidgetConfig(ctx context.Context, widgetContext string) (WidgetConfig, error) {
	var cfg WidgetConfig
	err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND context = ? AND is_active = ?", DefaultTenantID, widgetContext, true).
		First(&cfg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return WidgetConfig{}, ErrNotFound
	}
	return cfg, err
}

// PublishWidgetConfig inserts a new version, marks it active, and deactivates
// any previously active version for the same context.
func (s *Store) PublishWidgetConfig(ctx context.Context, widgetContext, payload string) (WidgetConfig, error) {
	var created WidgetConfig
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var maxVersion int
		if err := tx.Model(&WidgetConfig{}).
			Where("tenant_id = ? AND context = ?", DefaultTenantID, widgetContext).
			Select("COALESCE(MAX(version), 0)").Scan(&maxVersion).Error; err != nil {
			return err
		}
		if err := tx.Model(&WidgetConfig{}).
			Where("tenant_id = ? AND context = ?", DefaultTenantID, widgetContext).
			Update("is_active", false).Error; err != nil {
			return err
		}
		created = WidgetConfig{
			TenantID: DefaultTenantID, Context: widgetContext,
			Version: maxVersion + 1, Payload: payload, IsActive: true,
		}
		return tx.Create(&created).Error
	})
	return created, err
}

func (s *Store) ListWidgetConfigVersions(ctx context.Context, widgetContext string) ([]WidgetConfig, error) {
	var versions []WidgetConfig
	err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND context = ?", DefaultTenantID, widgetContext).
		Order("version desc").Find(&versions).Error
	return versions, err
}

func (s *Store) SetActiveWidgetConfigVersion(ctx context.Context, widgetContext string, version int) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&WidgetConfig{}).
			Where("tenant_id = ? AND context = ?", DefaultTenantID, widgetContext).
			Update("is_active", false).Error; err != nil {
			return err
		}
		res := tx.Model(&WidgetConfig{}).
			Where("tenant_id = ? AND context = ? AND version = ?", DefaultTenantID, widgetContext, version).
			Update("is_active", true)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestPublishGetListRollback -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/store/widget_config_models.go internal/store/widget_config.go internal/store/widget_config_test.go internal/store/store.go
git commit -m "feat(store): versioned widget config with publish/rollback"
```

---

## Task 2: Widget config endpoints (admin + public)

**Files:**
- Create: `internal/server/admin_widget_config.go`
- Create: `internal/server/admin_widget_config_test.go`
- Modify: `internal/server/server.go`

- [ ] **Step 1: Write the failing test**

Create `internal/server/admin_widget_config_test.go`:

```go
package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWidgetConfigPublishAndPublicFetch(t *testing.T) {
	s, cookie := authedServer(t)
	mux := s.adminMux()

	// Obtain CSRF token.
	csrfRec := httptest.NewRecorder()
	csrfReq := httptest.NewRequest(http.MethodGet, "/admin/api/csrf", nil)
	csrfReq.AddCookie(cookie)
	mux.ServeHTTP(csrfRec, csrfReq)
	var csrfCookie *http.Cookie
	for _, c := range csrfRec.Result().Cookies() {
		if c.Name == csrfCookieName {
			csrfCookie = c
		}
	}
	token := csrfCookie.Value

	// Publish a config.
	pubReq := httptest.NewRequest(http.MethodPost, "/admin/api/widget-config/product",
		strings.NewReader(`{"theme":{"accent":"#abc"}}`))
	pubReq.AddCookie(cookie)
	pubReq.AddCookie(csrfCookie)
	pubReq.Header.Set(csrfHeaderName, token)
	pubRec := httptest.NewRecorder()
	mux.ServeHTTP(pubRec, pubReq)
	if pubRec.Code != http.StatusOK {
		t.Fatalf("publish status = %d, body=%s", pubRec.Code, pubRec.Body.String())
	}

	// Public fetch (no auth) returns the active payload.
	pubGet := httptest.NewRecorder()
	// public endpoint is on the main mux, not adminMux; build it:
	s.publicMux().ServeHTTP(pubGet, httptest.NewRequest(http.MethodGet, "/api/widget-config?context=product", nil))
	if pubGet.Code != http.StatusOK || !strings.Contains(pubGet.Body.String(), "#abc") {
		t.Fatalf("public fetch failed: %d %s", pubGet.Code, pubGet.Body.String())
	}
}
```

Note: this test introduces `s.publicMux()` — refactor `Run` so the public route registration lives in a `publicMux()` method (used by both `Run` and the test). Do this refactor as part of Step 3.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestWidgetConfigPublishAndPublicFetch -v`
Expected: FAIL — undefined handlers / `publicMux`.

- [ ] **Step 3: Implement handlers and refactor public routes**

Create `internal/server/admin_widget_config.go`:

```go
package server

import (
	"errors"
	"io"
	"net/http"
	"strconv"
)

const maxConfigBytes = 64 * 1024

func (s *Server) handleGetWidgetConfig(w http.ResponseWriter, r *http.Request) {
	ctxName := r.PathValue("context")
	cfg, err := s.store.GetActiveWidgetConfig(r.Context(), ctxName)
	if err != nil {
		writeError(w, http.StatusNotFound, errors.New("no active config"))
		return
	}
	writeRawJSON(w, http.StatusOK, cfg.Payload)
}

func (s *Server) handlePublishWidgetConfig(w http.ResponseWriter, r *http.Request) {
	ctxName := r.PathValue("context")
	if ctxName != "product" && ctxName != "homepage" {
		writeError(w, http.StatusBadRequest, errors.New("context must be product or homepage"))
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxConfigBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !json.Valid(body) {
		writeError(w, http.StatusBadRequest, errors.New("payload must be valid JSON"))
		return
	}
	cfg, err := s.store.PublishWidgetConfig(r.Context(), ctxName, string(body))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": cfg.Version})
}

func (s *Server) handleListWidgetConfigVersions(w http.ResponseWriter, r *http.Request) {
	versions, err := s.store.ListWidgetConfigVersions(r.Context(), r.PathValue("context"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": versions})
}

func (s *Server) handleRollbackWidgetConfig(w http.ResponseWriter, r *http.Request) {
	version, err := strconv.Atoi(r.PathValue("version"))
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid version"))
		return
	}
	if err := s.store.SetActiveWidgetConfigVersion(r.Context(), r.PathValue("context"), version); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handlePublicWidgetConfig is the PUBLIC endpoint the widget/loader calls.
func (s *Server) handlePublicWidgetConfig(w http.ResponseWriter, r *http.Request) {
	ctxName := r.URL.Query().Get("context")
	if ctxName == "" {
		ctxName = "product"
	}
	cfg, err := s.store.GetActiveWidgetConfig(r.Context(), ctxName)
	if err != nil {
		// No config published yet → empty object; widget uses built-in defaults.
		writeRawJSON(w, http.StatusOK, "{}")
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	writeRawJSON(w, http.StatusOK, cfg.Payload)
}
```

Add the `"encoding/json"` import (used by `json.Valid`). Add a raw-JSON writer to `internal/server/server.go`:

```go
func writeRawJSON(w http.ResponseWriter, status int, raw string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, raw)
}
```

(Add `"io"` to the `server.go` imports.)

Refactor public route registration in `internal/server/server.go` into a method, and call it from `Run`:

```go
func (s *Server) publicMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/reviews", s.handleReviews)
	mux.HandleFunc("GET /api/showcase", s.handleShowcase)
	mux.HandleFunc("GET /api/widget-config", s.handlePublicWidgetConfig)
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	return mux
}
```

In `Run`, mount it:

```go
	mux := http.NewServeMux()
	publicRoutes := s.publicMux()
	mux.Handle("/api/", publicRoutes)
	mux.Handle("/healthz", publicRoutes)
	mux.Handle("/admin/", s.adminMux())
	mux.Handle("/", http.FileServer(http.Dir(s.cfg.StaticDir)))
```

Add the protected admin routes for config management in `adminMux`'s `protected` sub-mux:

```go
	protected.HandleFunc("GET /admin/api/widget-config/{context}", s.handleGetWidgetConfig)
	protected.Handle("POST /admin/api/widget-config/{context}", requireCSRF(http.HandlerFunc(s.handlePublishWidgetConfig)))
	protected.HandleFunc("GET /admin/api/widget-config/{context}/versions", s.handleListWidgetConfigVersions)
	protected.Handle("POST /admin/api/widget-config/{context}/rollback/{version}", requireCSRF(http.HandlerFunc(s.handleRollbackWidgetConfig)))
```

- [ ] **Step 4: Run the test**

Run: `go test ./internal/server/ -run TestWidgetConfigPublishAndPublicFetch -v`
Expected: PASS.

- [ ] **Step 5: Full suite**

Run: `go test ./...`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/server/admin_widget_config.go internal/server/admin_widget_config_test.go internal/server/server.go
git commit -m "feat(server): widget config publish/rollback and public delivery endpoint"
```

---

## Task 3: Widget consumes the active config

**Files:**
- Modify: `web/reviews-widget/loader.js`
- Modify: `web/reviews-widget/reviews-widget.js`

- [ ] **Step 1: Fetch the config in the loader**

In `web/reviews-widget/loader.js`, locate the `CFG` object and the mount routine (where `dataBase` is read and the widget is mounted into the Shadow DOM). Add a config fetch keyed by context. Add to `CFG` a `configBase` default and a `context`:

```js
// inside the CFG defaults object:
configBase: "https://reviews.shegida.ru",
context: "product", // overridable via window.REVIEWS_EMBED_CONFIG
```

Before mounting the widget, fetch and merge the published config:

```js
async function loadWidgetConfig() {
  try {
    var res = await fetch(CFG.configBase + "/api/widget-config?context=" + encodeURIComponent(CFG.context));
    if (!res.ok) return {};
    return await res.json();
  } catch (e) {
    if (CFG.debug) console.warn("widget-config fetch failed", e);
    return {};
  }
}
```

Apply the theme to the Shadow DOM host as CSS variables, mapping payload `theme.*` to `--rw-*`:

```js
function applyTheme(root, theme) {
  if (!theme) return;
  var map = {
    accent: "--rw-accent", accentInk: "--rw-accent-ink", text: "--rw-text",
    muted: "--rw-muted", panel: "--rw-panel", border: "--rw-border",
  };
  Object.keys(map).forEach(function (k) {
    if (theme[k]) root.style.setProperty(map[k], theme[k]);
  });
}
```

In the mount sequence, `await loadWidgetConfig()`, call `applyTheme(shadowRootHostEl, config.theme)`, and pass `config.layout`/`config.visibility`/`config.typography` into the widget render call.

- [ ] **Step 2: Honor visibility/layout flags in the widget**

In `web/reviews-widget/reviews-widget.js`, locate the render function that builds each review card. Accept an options object (default everything on for backward compatibility) and gate sections:

```js
function renderReview(review, opts) {
  opts = opts || {};
  var showPhotos = opts.visibility ? opts.visibility.photos !== false : true;
  var showAnswers = opts.visibility ? opts.visibility.sellerAnswers !== false : true;
  var showProsCons = opts.visibility ? opts.visibility.prosCons !== false : true;
  var showBadges = opts.visibility ? opts.visibility.marketplaceBadges !== false : true;
  // ...gate the existing media/answer/pros-cons/badge blocks on these flags...
}
```

For layout, set a container class from `opts.layout.mode` (`rw-layout-list|grid|carousel`) and add matching CSS rules to `reviews-widget.css` (grid uses `--rw-columns`). For typography, apply `--rw-radius` and a font scale variable.

Note: the exact function names and DOM structure must be read from the current `reviews-widget.js`; adapt the gating to the real code. Keep all flags defaulting to "on" so an empty config (`{}`) renders identically to today.

- [ ] **Step 3: Manual verification with a published config**

```bash
REVIEWS_INSECURE_COOKIES=1 go run ./cmd/reviews serve --addr 127.0.0.1:8080 &
sleep 1
# Use the admin (or curl with session+csrf) to publish a config with accent "#cc0000".
# Open web/reviews-widget/demo.html pointed at localhost and confirm the accent changes
# and that toggling visibility.photos=false hides photos.
kill %1
```
Expected: theme + visibility changes reflect in the rendered widget; empty/missing config renders the default look.

- [ ] **Step 4: Commit**

```bash
git add web/reviews-widget/loader.js web/reviews-widget/reviews-widget.js web/reviews-widget/reviews-widget.css
git commit -m "feat(widget): fetch and apply published widget config"
```

---

## Task 4: Visual editor page with live preview

**Files:**
- Create: `web/admin/src/pages/Editor.tsx`
- Create: `web/admin/src/widgetConfig.ts` — shared TS type + defaults matching the payload schema.
- Modify: `web/admin/src/App.tsx`

- [ ] **Step 1: Shared config type and defaults**

Create `web/admin/src/widgetConfig.ts`:

```ts
export type WidgetConfig = {
  theme: { accent: string; accentInk: string; text: string; muted: string; panel: string; border: string; dark: boolean }
  typography: { fontFamily: string; scale: number; radius: number; density: 'comfortable' | 'compact' }
  layout: { mode: 'list' | 'grid' | 'carousel'; columns: number; pageSize: number; pagination: 'more' | 'pages' }
  visibility: {
    photos: boolean; sellerAnswers: boolean; prosCons: boolean
    marketplaceBadges: boolean; ratingDistribution: boolean; filters: boolean
  }
}

export const defaultConfig: WidgetConfig = {
  theme: { accent: '#2f7a5b', accentInk: '#ffffff', text: '#1f2520', muted: '#687067', panel: '#ffffff', border: '#d9ded7', dark: false },
  typography: { fontFamily: 'inherit', scale: 1, radius: 12, density: 'comfortable' },
  layout: { mode: 'list', columns: 2, pageSize: 6, pagination: 'more' },
  visibility: { photos: true, sellerAnswers: true, prosCons: true, marketplaceBadges: true, ratingDistribution: true, filters: true },
}
```

- [ ] **Step 2: Editor with controls + live preview**

Create `web/admin/src/pages/Editor.tsx`. It loads the active config, renders controls, and shows a live preview by mounting the real widget in an iframe whose `window.REVIEWS_EMBED_CONFIG` is set to the in-browser draft (no publish needed for preview):

```tsx
import { useEffect, useRef, useState } from 'react'
import { apiGet, apiWrite } from '../api'
import { defaultConfig, WidgetConfig } from '../widgetConfig'

const CONTEXTS = ['product', 'homepage'] as const

export default function Editor() {
  const [context, setContext] = useState<(typeof CONTEXTS)[number]>('product')
  const [cfg, setCfg] = useState<WidgetConfig>(defaultConfig)
  const [status, setStatus] = useState('')
  const previewRef = useRef<HTMLIFrameElement>(null)

  useEffect(() => {
    apiGet<Partial<WidgetConfig>>(`/admin/api/widget-config/${context}`)
      .then((p) => setCfg({ ...defaultConfig, ...p, theme: { ...defaultConfig.theme, ...p.theme } }))
      .catch(() => setCfg(defaultConfig))
  }, [context])

  // Push the draft into the preview iframe whenever it changes.
  useEffect(() => {
    const frame = previewRef.current
    if (!frame?.contentWindow) return
    frame.contentWindow.postMessage({ type: 'reviews-preview-config', config: cfg }, '*')
  }, [cfg])

  async function publish() {
    setStatus('publishing…')
    await apiWrite('POST', `/admin/api/widget-config/${context}`, cfg)
    setStatus('published ✓')
  }

  const t = cfg.theme
  return (
    <section style={{ display: 'flex', gap: 24 }}>
      <div style={{ width: 320 }}>
        <h2>Editor</h2>
        <select value={context} onChange={(e) => setContext(e.target.value as typeof context)}>
          {CONTEXTS.map((c) => <option key={c} value={c}>{c}</option>)}
        </select>
        <h3>Theme</h3>
        {(['accent', 'text', 'panel', 'border'] as const).map((k) => (
          <label key={k} style={{ display: 'block' }}>{k}
            <input type="color" value={t[k]} onChange={(e) => setCfg({ ...cfg, theme: { ...t, [k]: e.target.value } })} />
          </label>
        ))}
        <h3>Layout</h3>
        <select value={cfg.layout.mode}
          onChange={(e) => setCfg({ ...cfg, layout: { ...cfg.layout, mode: e.target.value as WidgetConfig['layout']['mode'] } })}>
          <option value="list">list</option><option value="grid">grid</option><option value="carousel">carousel</option>
        </select>
        <h3>Visibility</h3>
        {(Object.keys(cfg.visibility) as (keyof WidgetConfig['visibility'])[]).map((k) => (
          <label key={k} style={{ display: 'block' }}>
            <input type="checkbox" checked={cfg.visibility[k]}
              onChange={(e) => setCfg({ ...cfg, visibility: { ...cfg.visibility, [k]: e.target.checked } })} /> {k}
          </label>
        ))}
        <h3>Typography</h3>
        <label>radius <input type="range" min={0} max={24} value={cfg.typography.radius}
          onChange={(e) => setCfg({ ...cfg, typography: { ...cfg.typography, radius: Number(e.target.value) } })} /></label>
        <button onClick={publish}>Publish</button> <span>{status}</span>
      </div>
      <iframe ref={previewRef} src="/admin/preview.html" style={{ flex: 1, height: 600, border: '1px solid #ccc' }} title="preview" />
    </section>
  )
}
```

- [ ] **Step 3: Preview host page**

Create `web/admin/public/preview.html` (Vite copies `public/` verbatim into the build):

```html
<!doctype html>
<html><head><meta charset="utf-8"><title>preview</title></head>
<body>
  <div id="reviews-embed-host"></div>
  <script>
    // Receives draft config from the editor and re-mounts the widget.
    window.REVIEWS_EMBED_CONFIG = { configBase: location.origin, context: 'product' };
    window.addEventListener('message', function (e) {
      if (e.data && e.data.type === 'reviews-preview-config') {
        window.__previewConfig = e.data.config;
        // Re-apply theme/visibility using the widget's exported preview hook.
        if (window.ReviewsWidget && window.ReviewsWidget.renderPreview) {
          window.ReviewsWidget.renderPreview(document.getElementById('reviews-embed-host'), e.data.config);
        }
      }
    });
  </script>
  <script src="/reviews-widget.js"></script>
</body></html>
```

This requires `reviews-widget.js` to expose a `window.ReviewsWidget.renderPreview(hostEl, config)` entry point. Add a thin export in `reviews-widget.js` that renders sample/fetched reviews applying the given config (reuse the gating from Task 3). Document this hook near the existing mount code.

- [ ] **Step 4: Add Editor + Embed to the nav**

In `web/admin/src/App.tsx` `AdminShell`, add `<a href="#editor">Editor</a>` and `<a href="#embed">Embed</a>`, import `Editor` and `Embed`, and render them for `#editor` / `#embed`.

- [ ] **Step 5: Build and manually verify live preview**

Run: `cd web/admin && ./build-embed.sh && cd ../..`
Then run the server and open `/admin/#editor`. Expected: changing the accent color updates the preview iframe live; "Publish" persists (reload shows the published values; the public site widget reflects them).

- [ ] **Step 6: Commit**

```bash
git add web/admin internal/server/admin_dist web/reviews-widget/reviews-widget.js
git commit -m "feat(admin): visual editor with live preview"
```

---

## Task 5: Embed snippet generator

**Files:**
- Create: `web/admin/src/pages/Embed.tsx`

- [ ] **Step 1: Snippet generator page**

Create `web/admin/src/pages/Embed.tsx`:

```tsx
import { useState } from 'react'

export default function Embed() {
  const [context, setContext] = useState('product')
  const origin = window.location.origin
  const snippet = `<script>
  window.REVIEWS_EMBED_CONFIG = {
    configBase: "${origin}",
    dataBase: "${origin}/reviews-data",
    context: "${context}"
  };
</script>
<script src="${origin}/loader.js" async></script>
<div id="reviews-embed-host"></div>`

  return (
    <section>
      <h2>Install snippet</h2>
      <select value={context} onChange={(e) => setContext(e.target.value)}>
        <option value="product">product card</option>
        <option value="homepage">homepage</option>
      </select>
      <pre style={{ background: '#f5f7f2', padding: 12, whiteSpace: 'pre-wrap' }}>{snippet}</pre>
      <button onClick={() => navigator.clipboard.writeText(snippet)}>Copy</button>
      <p>Paste before <code>&lt;/body&gt;</code> on the target page (or via your tag manager).</p>
    </section>
  )
}
```

- [ ] **Step 2: Build, verify, commit**

Run: `cd web/admin && ./build-embed.sh && cd ../.. && go build ./...`
Open `/admin/#embed`, confirm the snippet renders for both contexts and Copy works.

```bash
git add web/admin internal/server/admin_dist
git commit -m "feat(admin): embed snippet generator"
```

---

## Self-Review Notes

- **Spec coverage:** FR-6 theme/typography/layout/visibility controls (Task 4), live preview (Task 4), versioned publish + rollback (Tasks 1–2), config delivery to widget (Tasks 2–3, "Доставка конфига в виджет"). FR-7 snippet generator with context + copy (Task 5). Product-card vs homepage contexts handled throughout via the `context` path/query param.
- **Type consistency:** payload schema is fixed once (top of plan) and mirrored in Go (`WidgetConfig.Payload` is opaque JSON, validated only as well-formed) and TS (`widgetConfig.ts`). Store method names (`GetActiveWidgetConfig`, `PublishWidgetConfig`, `ListWidgetConfigVersions`, `SetActiveWidgetConfigVersion`) match between Task 1 definitions and Task 2 call sites. `publicMux()` introduced in Task 2 and used by both `Run` and the test.
- **Backward compatibility:** an empty/absent config returns `{}` and the widget defaults every flag to "on", so existing embeds render unchanged until a config is published.
- **Implementation-time reading required:** exact `loader.js`/`reviews-widget.js` function names and DOM structure (Task 3) and the `renderPreview` hook (Task 4) must be aligned with the current widget code; the plan flags each spot.
