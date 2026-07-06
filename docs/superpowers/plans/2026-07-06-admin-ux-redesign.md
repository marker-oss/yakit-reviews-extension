# Admin UX Redesign — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rework the Reviews admin panel with a neutral-SaaS design system, a tabbed Widget editor, prominent save/apply feedback, and a "Состояние" diagnostics page — without touching the widget, public routes, workers, or DB schema.

**Architecture:** Pure frontend for design tokens, toasts, dirty-state, tabs, and nav badges (React 19 + Vite, hand-rolled hash routing, single `styles.css`). New read-only Go handlers in `internal/server` aggregate diagnostics + counts from existing store methods and static-export state; active probes make outbound HTTP with timeouts and degrade softly. No new DB tables.

**Tech Stack:** React 19, Vite 7, TypeScript (frontend, no test runner — gate on `npm run build`); Go net/http `ServeMux` with `writeJSON`/`writeError`/`requireCSRF` (backend, table tests via `go test`).

## Global Constraints

- **Scope:** only `web/admin/**` and `internal/server/**`. Do NOT modify the widget (`web/reviews-widget/**`), public `/api/**` routes, sync workers, or `internal/store` schema/migrations. Diagnostics is read-only.
- **Fonts:** Inter via local `@fontsource/inter` npm package. Do NOT use Google Fonts CDN. Remove the Prata serif and the `@import url("https://fonts.googleapis.com/...")` line.
- **Save mechanics:** explicit "Сохранить/Опубликовать" button everywhere. No autosave.
- **Widget config model is unchanged:** one versioned document per context (`product` | `homepage`), published whole by one button. Tabs are visual grouping only — never split the config across routes or add a second publish path.
- **Backend endpoints are read-only aggregators.** Active probes must have per-request timeouts and must not fail the whole response when one probe is unreachable (return a `fail`/`warn` item, not HTTP 500).
- **UI copy is Russian**, matching existing tone (e.g. «Сохранено», «Опубликована версия N»).
- **New protected admin routes** must be registered on the `protected` mux in `internal/server/server.go` (GET without CSRF wrapper; POST wrapped in `requireCSRF`).
- **Branch:** `feat/admin-ux-redesign` (already created; spec committed at `6223cb9`).

---

## Phase A — Design system

### Task A1: Bundle Inter locally, drop Prata + CDN

**Files:**
- Modify: `web/admin/package.json` (add dependency)
- Modify: `web/admin/src/main.tsx` (import font CSS)
- Modify: `web/admin/src/styles.css:1` (remove `@import` line; update font-family)

**Interfaces:**
- Produces: Inter available app-wide as the default `font-family`; no external font requests.

- [ ] **Step 1: Add the dependency**

Run:
```bash
cd web/admin && npm install @fontsource/inter@^5
```
Expected: `@fontsource/inter` appears under `dependencies` in `package.json`, `package-lock.json` updated.

- [ ] **Step 2: Import the weights in `main.tsx`**

`web/admin/src/main.tsx` currently:
```tsx
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import App from './App'
import './styles.css'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
```
Add these imports above `import './styles.css'`:
```tsx
import '@fontsource/inter/400.css'
import '@fontsource/inter/500.css'
import '@fontsource/inter/600.css'
import '@fontsource/inter/700.css'
```

- [ ] **Step 3: Remove the CDN import and Prata from `styles.css`**

Delete line 1 entirely (the `@import url("https://fonts.googleapis.com/...")`).
In the `:root` block, change the `font-family` declaration to:
```css
  font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
```
Change the `h1` and `h2` rules so headings use the body font (remove the `font-family: Prata, ...` lines from both). Set:
```css
h1 { margin-bottom: 0; font-size: 28px; font-weight: 700; line-height: 1.2; letter-spacing: -0.01em; }
h2 { margin-bottom: 0; font-size: 22px; font-weight: 700; letter-spacing: -0.01em; }
```

- [ ] **Step 4: Build to verify no TS/asset errors**

Run:
```bash
cd web/admin && npm run build
```
Expected: build succeeds; `dist/` produced with no unresolved font import.

- [ ] **Step 5: Grep to prove the CDN and Prata are gone**

Run:
```bash
cd web/admin && grep -rn "googleapis\|Prata" src/ ; echo "exit=$?"
```
Expected: no matches (grep exit=1).

- [ ] **Step 6: Commit**

```bash
git add web/admin/package.json web/admin/package-lock.json web/admin/src/main.tsx web/admin/src/styles.css
git commit -m "feat(admin): bundle Inter locally, drop Prata + Google Fonts CDN"
```

### Task A2: Rewrite design tokens (neutral SaaS)

**Files:**
- Modify: `web/admin/src/styles.css` (`:root` token block + button/input/card/sidebar values)

**Interfaces:**
- Produces: a token layer that all later UI (toasts, badges, tabs, checklist) references. New tokens: `--accent`, `--accent-hover`, `--accent-tint`, `--ok`, `--warn`, `--danger`, plus the existing `--admin-*` names kept as aliases so unported CSS still resolves.

- [ ] **Step 1: Replace the `:root` variable set**

In `web/admin/src/styles.css`, replace the current `--admin-*` custom-property block (lines ~3–17, ending before `font-family:`) with:
```css
:root {
  --admin-bg: #f7f8fa;
  --admin-surface: #ffffff;
  --admin-warm: #f3f4f6;
  --admin-border: #e5e7eb;
  --admin-text: #111827;
  --admin-muted: #6b7280;
  --admin-soft-muted: #9ca3af;
  --admin-iris: #4f46e5;
  --admin-iris-hover: #4338ca;
  --admin-iris-tint: #eef2ff;
  --admin-trust: #16a34a;
  --admin-sale: #dc2626;
  --admin-warn: #d97706;
  --admin-shadow-sm: 0 1px 2px rgba(17, 24, 39, 0.06);
  --admin-shadow-md: 0 4px 12px rgba(17, 24, 39, 0.08);
  /* Semantic aliases used by new components (toasts, badges, tabs). */
  --accent: var(--admin-iris);
  --accent-hover: var(--admin-iris-hover);
  --accent-tint: var(--admin-iris-tint);
  --ok: var(--admin-trust);
  --warn: var(--admin-warn);
  --danger: var(--admin-sale);
```
(Keep the existing `font-family:`, `color:`, `background:` lines that follow, but with A1's Inter font-family.)

- [ ] **Step 2: Flatten radii and shadows**

Change the shared control radii from `12px`/`16px` to the SaaS scale:
- `button` `border-radius: 12px` → `8px`
- `input, select, textarea` `border-radius: 12px` → `8px`
- `.panel`, `.metric`, `.auth-panel`, `.preview-pane`, `.snippet` `border-radius: 16px` → `12px`
- `.sidebar a` `border-radius: 12px` → `8px`

Leave the `.status-warn` colour literal `#9a5b13` replaced with `var(--admin-warn)` for consistency.

- [ ] **Step 3: Build**

Run:
```bash
cd web/admin && npm run build
```
Expected: success.

- [ ] **Step 4: Manual visual check**

Use the `run` skill (or `cd web/admin && npm run dev`) and load the admin. Confirm: cool-grey background, indigo primary buttons, flatter cards, Inter throughout, no purple/cream, headings sans-serif. Log what you saw.

- [ ] **Step 5: Commit**

```bash
git add web/admin/src/styles.css
git commit -m "feat(admin): neutral-SaaS design tokens (indigo accent, flatter radii)"
```

---

## Phase B — Feedback infrastructure

### Task B1: Toast pub/sub module

**Files:**
- Create: `web/admin/src/toast.ts`

**Interfaces:**
- Produces:
  - `type ToastKind = 'success' | 'error' | 'info'`
  - `type ToastAction = { label: string; onClick: () => void }`
  - `type Toast = { id: number; kind: ToastKind; message: string; action?: ToastAction; ttl: number }`
  - `const toast = { success(msg, action?), error(msg, action?), info(msg, action?) }`
  - `function subscribe(listener: (toasts: Toast[]) => void): () => void`
  - `function dismiss(id: number): void`
  - Consumed by `ToastHost` (B2) and every page (B3).

- [ ] **Step 1: Write the module**

Create `web/admin/src/toast.ts`:
```ts
export type ToastKind = 'success' | 'error' | 'info'
export type ToastAction = { label: string; onClick: () => void }
export type Toast = {
  id: number
  kind: ToastKind
  message: string
  action?: ToastAction
  ttl: number
}

let seq = 0
let toasts: Toast[] = []
const listeners = new Set<(t: Toast[]) => void>()

function emit() {
  for (const l of listeners) l(toasts)
}

export function subscribe(listener: (t: Toast[]) => void): () => void {
  listeners.add(listener)
  listener(toasts)
  return () => listeners.delete(listener)
}

export function dismiss(id: number) {
  toasts = toasts.filter((t) => t.id !== id)
  emit()
}

function push(kind: ToastKind, message: string, action?: ToastAction) {
  const id = ++seq
  // Errors linger (8s) and success/info auto-clear faster (4s).
  const ttl = kind === 'error' ? 8000 : 4000
  toasts = [...toasts, { id, kind, message, action, ttl }]
  emit()
  window.setTimeout(() => dismiss(id), ttl)
  return id
}

export const toast = {
  success: (message: string, action?: ToastAction) => push('success', message, action),
  error: (message: string, action?: ToastAction) => push('error', message, action),
  info: (message: string, action?: ToastAction) => push('info', message, action),
}
```

- [ ] **Step 2: Typecheck**

Run:
```bash
cd web/admin && npx tsc --noEmit
```
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add web/admin/src/toast.ts
git commit -m "feat(admin): toast pub/sub module"
```

### Task B2: ToastHost component + styles + mount

**Files:**
- Create: `web/admin/src/components/ToastHost.tsx`
- Modify: `web/admin/src/App.tsx` (import + render `<ToastHost />` once, inside the top-level return for authed AND unauthed states)
- Modify: `web/admin/src/styles.css` (toast styles)

**Interfaces:**
- Consumes: `subscribe`, `dismiss`, `Toast` from `../toast` (B1).
- Produces: a always-mounted host that renders the live toast stack. No props.

- [ ] **Step 1: Write the component**

Create `web/admin/src/components/ToastHost.tsx`:
```tsx
import { useEffect, useState } from 'react'
import { subscribe, dismiss, type Toast } from '../toast'

export default function ToastHost() {
  const [toasts, setToasts] = useState<Toast[]>([])
  useEffect(() => subscribe(setToasts), [])
  if (toasts.length === 0) return null
  return (
    <div className="toast-host" role="status" aria-live="polite">
      {toasts.map((t) => (
        <div key={t.id} className={`toast toast-${t.kind}`}>
          <span className="toast-msg">{t.message}</span>
          {t.action && (
            <button
              className="toast-action"
              onClick={() => {
                t.action!.onClick()
                dismiss(t.id)
              }}
            >
              {t.action.label}
            </button>
          )}
          <button className="toast-close" aria-label="Закрыть" onClick={() => dismiss(t.id)}>
            ×
          </button>
        </div>
      ))}
    </div>
  )
}
```

- [ ] **Step 2: Add toast styles to `styles.css`**

Append:
```css
.toast-host {
  position: fixed;
  right: 16px;
  bottom: 16px;
  z-index: 2000;
  display: grid;
  gap: 10px;
  width: min(380px, calc(100vw - 32px));
}
.toast {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 14px;
  border-radius: 10px;
  border: 1px solid var(--admin-border);
  background: var(--admin-surface);
  box-shadow: var(--admin-shadow-md);
  font-size: 14px;
  animation: toast-in 160ms ease;
}
@keyframes toast-in {
  from { opacity: 0; transform: translateY(8px); }
  to { opacity: 1; transform: translateY(0); }
}
.toast-msg { flex: 1; min-width: 0; }
.toast-success { border-left: 4px solid var(--ok); }
.toast-error { border-left: 4px solid var(--danger); }
.toast-info { border-left: 4px solid var(--accent); }
.toast-action {
  min-height: 32px;
  padding: 0 10px;
  border-radius: 8px;
  background: var(--accent);
  color: #fff;
  font-size: 13px;
  font-weight: 600;
}
.toast-close {
  min-height: 28px;
  width: 28px;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--admin-muted);
  font-size: 18px;
  line-height: 1;
}
.toast-close:hover { background: transparent; color: var(--admin-text); }
```

- [ ] **Step 3: Mount in `App.tsx`**

Add `import ToastHost from './components/ToastHost'` with the other imports. Render `<ToastHost />` so it exists in every mode. Simplest: wrap the three return branches. In the `loading`/unauth `return (<main className="auth-screen">…</main>)` blocks and the authed `return (<div className="app-shell">…</div>)`, wrap each in a fragment and add `<ToastHost />`. Example for the authed branch:
```tsx
  return (
    <>
      <div className="app-shell">
        {/* …existing sidebar + workspace… */}
      </div>
      <ToastHost />
    </>
  )
```
Do the same `<>…<ToastHost /></>` wrap for the `loading` and unauth returns.

- [ ] **Step 4: Build**

Run:
```bash
cd web/admin && npm run build
```
Expected: success.

- [ ] **Step 5: Manual check**

Temporarily add a `toast.success('тест')` on a button (or trigger a real save after B3) and confirm the toast appears bottom-right, auto-dismisses, and the × closes it. Remove the temporary call.

- [ ] **Step 6: Commit**

```bash
git add web/admin/src/components/ToastHost.tsx web/admin/src/App.tsx web/admin/src/styles.css
git commit -m "feat(admin): ToastHost renderer mounted app-wide"
```

### Task B3: Migrate page feedback to toasts

**Files:**
- Modify: `web/admin/src/pages/Settings.tsx`
- Modify: `web/admin/src/pages/Showcase.tsx`
- Modify: `web/admin/src/pages/Marketplaces.tsx`
- Modify: `web/admin/src/pages/Editor.tsx`
- Modify: `web/admin/src/pages/Reviews.tsx`
- Modify: `web/admin/src/pages/Questions.tsx`
- Modify: `web/admin/src/pages/Embed.tsx`

**Interfaces:**
- Consumes: `toast` from `../toast` (B1).
- Produces: no new exports. Success/error results surface as toasts instead of inline `<p className="muted">`/`<p className="error">`.

**Pattern (apply per page):** replace `setMessage('…')`/`setError('…')` success and error sites with `toast.success('…')` / `toast.error('…')`. Keep any *persistent inline* status that is not a transient action result (e.g. the catalog progress line `catalogStatusText(catalog)` in Marketplaces stays inline; the `insecureBase` warning in Embed stays inline). Remove now-unused `message`/`error` state only where nothing else renders it.

- [ ] **Step 1: Settings.tsx**

In `save()`, replace `setMessage('Сохранено')` with `toast.success('Сохранено')` and the catch `setError(...)` with `toast.error(...)`. Do the same in `dsrLookup`/`dsrDelete` (`toast.info(\`Найдено отзывов: ${data.reviews.length}\`)`, `toast.success(\`Удалено: ${r.deleted}\`)`, errors → `toast.error`). Remove the `{message && …}`/`{error && …}` JSX and the now-dead `message`/`error`/`dsrMsg`/`dsrError` state. Add `import { toast } from '../toast'`.

- [ ] **Step 2: Showcase.tsx**

`save()` success → `toast.success('Сохранено')`; error → `toast.error(...)`. Load error → `toast.error(...)`. Keep the `if (!rule) return …Загрузка…` guard (drop its dependence on `message`; show plain «Загрузка...»). Remove `message` state and its JSX. Add the import.

Also apply the spec's «Витрина» rewording so it is not confused with the widget editor: add an intro line at the top of the returned `<section className="stack">`, before the form panel:
```tsx
<p className="muted">
  Витрина — это <strong>отбор отзывов для главной страницы</strong> (какие отзывы попадают в
  подборку), а не оформление виджета. Внешний вид настраивается на вкладке «Виджет».
</p>
```

- [ ] **Step 3: Marketplaces.tsx**

`sync()` → `toast.success('Синхронизация запущена')`. `togglePublish` error → `toast.error`. `refreshCatalog` error → `toast.error`. Replace remaining `setMessage(err…)` with `toast.error(...)`. Keep `catalog` polling + `catalogStatusText(catalog)` inline. Remove the standalone `message` state + its `<p className="muted">{message}</p>`. Add the import.

For `save(item, enabled)` success, use the differentiated "applied" message from the spec — a toast that carries a one-click sync action for that marketplace:
```tsx
toast.success('Доступы сохранены. Запустите синхронизацию, чтобы подтянуть отзывы', {
  label: 'Синхронизировать',
  onClick: () => sync(item.id),
})
```
(`sync` and `item.id` are already in scope inside `save`.)

- [ ] **Step 4: Editor.tsx**

`publish()` success → `toast.success(\`Опубликована v${res.version} — уже на сайте\`)`; error → `toast.error(...)`. `rollback()` → `toast.success(\`Активна версия ${version}\`)`; error → `toast.error`. `load()` error → `toast.error`. Remove the `message` state + the `{message && <p className="muted">{message}</p>}` in the toolbar. Add the import. (The "live version" plaque comes in Task C1.)

- [ ] **Step 5: Reviews.tsx**

Replace the shared `setError(...)` catch sites in `moderate`, `deleteReview`, `restoreReview`, `purgeReview`, `saveReply`, `retryPublish`, `toggleArticlePin`, `bulkModerate` with `toast.error(...)`. `publishChanges` success → `toast.success(\`Опубликовано: ${result.reviews} отзывов, ${result.articles} артикулов\`)`; error → `toast.error`. `saveReply` success → `toast.success('Ответ сохранён')`. Remove the `error` and `publishMessage` state and their JSX (`{error && …}`, `{publishMessage && …}`). Keep `publishing` (button label). Add the import.

- [ ] **Step 6: Questions.tsx**

`saveAnswer` success → `toast.success('Ответ сохранён')`, error → `toast.error`; `retryPublish` error → `toast.error`; load error → `toast.error`. Remove `error` state + JSX. Add the import.

- [ ] **Step 7: Embed.tsx**

`copy()` → `toast.success('Скопировано')`. Remove `message` state + JSX. Keep the `insecureBase` inline warning. Add the import.

- [ ] **Step 8: Build + grep for stragglers**

Run:
```bash
cd web/admin && npm run build && grep -rn "className=\"muted\">{message}\|className=\"error\">{error}" src/pages ; echo "exit=$?"
```
Expected: build passes; grep finds no leftover inline message/error paragraphs (exit=1). (Auth error in `App.tsx` login form may remain inline — that is intentional and out of scope for this grep on `src/pages`.)

- [ ] **Step 9: Manual check**

Trigger a save on Settings and a publish on Editor; confirm toasts fire and the old grey text is gone.

- [ ] **Step 10: Commit**

```bash
git add web/admin/src/pages
git commit -m "feat(admin): route all page save/error feedback through toasts"
```

### Task B4: Dirty-state indicator + unsaved-changes guard

**Files:**
- Create: `web/admin/src/useDirty.ts`
- Modify: `web/admin/src/pages/Editor.tsx`, `web/admin/src/pages/Settings.tsx`, `web/admin/src/pages/Showcase.tsx`
- Modify: `web/admin/src/styles.css` (`.dirty-badge`)

**Interfaces:**
- Produces: `function useDirty(current: unknown, baseline: unknown): boolean` — deep-equality (via `JSON.stringify`) dirty flag that also installs a `beforeunload` guard while dirty.
- Consumes: nothing.

- [ ] **Step 1: Write the hook**

Create `web/admin/src/useDirty.ts`:
```ts
import { useEffect, useMemo } from 'react'

// Returns true when `current` differs from the last-saved `baseline`, and
// installs a beforeunload guard while dirty so a hard navigation warns.
export function useDirty(current: unknown, baseline: unknown): boolean {
  const dirty = useMemo(
    () => JSON.stringify(current) !== JSON.stringify(baseline),
    [current, baseline],
  )
  useEffect(() => {
    if (!dirty) return
    const handler = (e: BeforeUnloadEvent) => {
      e.preventDefault()
      e.returnValue = ''
    }
    window.addEventListener('beforeunload', handler)
    return () => window.removeEventListener('beforeunload', handler)
  }, [dirty])
  return dirty
}
```

- [ ] **Step 2: Add badge style**

Append to `styles.css`:
```css
.dirty-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 10px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--admin-warn) 14%, transparent);
  color: var(--admin-warn);
  font-size: 12px;
  font-weight: 600;
}
```

- [ ] **Step 3: Wire Editor.tsx**

Track a `baseline` snapshot of the published config. In `load()`, after `setCfg(mergeWidgetConfig(config))`, also `setBaseline(mergeWidgetConfig(config))` (add `const [baseline, setBaseline] = useState<WidgetConfig>(defaultWidgetConfig)`). In `publish()` success, `setBaseline(cfg)`. Compute `const dirty = useDirty(cfg, baseline)`. In the sticky toolbar, render `{dirty && <span className="dirty-badge">Есть изменения</span>}`.

- [ ] **Step 4: Wire Settings.tsx and Showcase.tsx**

Settings: build a `baseline` object `{ agreementUrl, reviewTermsUrl, shopOrigin, sitemapUrl }` set on load and on successful save; `const current = { agreementUrl, reviewTermsUrl, shopOrigin, sitemapUrl }`; `const dirty = useDirty(current, baseline)`; render the badge next to the Save button. Showcase: `baseline` = the loaded `rule`; set on load and after save; `useDirty(rule, baseline)`; badge by the Save button.

- [ ] **Step 5: Build**

Run:
```bash
cd web/admin && npm run build
```
Expected: success.

- [ ] **Step 6: Manual check**

Edit a field in Editor → badge appears + closing the tab warns; publish → badge clears, no warning. Repeat on Settings.

- [ ] **Step 7: Commit**

```bash
git add web/admin/src/useDirty.ts web/admin/src/pages/Editor.tsx web/admin/src/pages/Settings.tsx web/admin/src/pages/Showcase.tsx web/admin/src/styles.css
git commit -m "feat(admin): unsaved-changes badge + beforeunload guard"
```

---

## Phase C — Widget editor tabs

### Task C1: Tabbed Editor with sticky header + "live version" plaque

**Files:**
- Modify: `web/admin/src/pages/Editor.tsx`
- Modify: `web/admin/src/styles.css` (`.editor-toolbar`, `.editor-tabs`, `.live-badge`)

**Interfaces:**
- Consumes: existing `cfg`, `versions`, `dirty` (B4), `toast` (B3), all existing `set*` helpers.
- Produces: no new exports. Local `useState<'look' | 'content' | 'marketplaces' | 'versions'>('look')` drives which existing `<section className="panel">` blocks render.

**Note:** the Editor already contains four natural blocks. This task only *groups* them behind tabs — do not change their fields or the single `publish()` path.

- [ ] **Step 1: Add tab state + active-version lookup**

At the top of the component add:
```tsx
type EditorTab = 'look' | 'content' | 'marketplaces' | 'versions'
const [tab, setTab] = useState<EditorTab>('look')
const activeVersion = versions.find((v) => v.active)?.version ?? null
```

- [ ] **Step 2: Restructure the toolbar into a sticky header**

Replace the current `<div className="toolbar">` (context select + publish button + message) with:
```tsx
<div className="editor-toolbar">
  <select value={context} onChange={(e) => setContext(e.target.value as WidgetContext)}>
    <option value="product">Карточка товара</option>
    <option value="homepage">Главная</option>
  </select>
  {activeVersion !== null && <span className="live-badge">Сейчас в эфире: v{activeVersion}</span>}
  {dirty && <span className="dirty-badge">Есть изменения</span>}
  <button onClick={publish}>Опубликовать</button>
</div>
<nav className="editor-tabs">
  <button className={tab === 'look' ? 'active' : ''} onClick={() => setTab('look')}>Внешний вид</button>
  <button className={tab === 'content' ? 'active' : ''} onClick={() => setTab('content')}>Отбор отзывов</button>
  <button className={tab === 'marketplaces' ? 'active' : ''} onClick={() => setTab('marketplaces')}>Площадки</button>
  <button className={tab === 'versions' ? 'active' : ''} onClick={() => setTab('versions')}>Версии</button>
</nav>
```

- [ ] **Step 3: Gate the existing panels by tab**

Wrap the existing sections:
- `tab === 'look'` → the `form-grid` panel (title, colors, typography, layout) **and** the `check-grid` visibility panel.
- `tab === 'content'` → the «Выдача отзывов» panel (`defaults` + `ranking`).
- `tab === 'marketplaces'` → the «Площадки в публичном виджете» panel.
- `tab === 'versions'` → the «Версии» panel.

Use `{tab === 'look' && (<>…</>)}` wrappers around the corresponding existing JSX (do not rewrite the field JSX).

- [ ] **Step 4: Add styles**

Append to `styles.css`:
```css
.editor-toolbar {
  position: sticky;
  top: 0;
  z-index: 5;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
  padding: 12px 0;
  background: var(--admin-bg);
}
.editor-toolbar button:not(.secondary) { margin-left: auto; }
.editor-tabs {
  display: flex;
  gap: 4px;
  border-bottom: 1px solid var(--admin-border);
  margin-bottom: 16px;
}
.editor-tabs button {
  min-height: 36px;
  border-radius: 8px 8px 0 0;
  background: transparent;
  color: var(--admin-muted);
  font-weight: 500;
  padding: 0 14px;
}
.editor-tabs button:hover { background: var(--admin-warm); color: var(--admin-text); }
.editor-tabs button.active {
  color: var(--accent);
  background: var(--accent-tint);
  border-bottom: 2px solid var(--accent);
}
.live-badge {
  display: inline-flex;
  align-items: center;
  padding: 2px 10px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--ok) 12%, transparent);
  color: var(--ok);
  font-size: 12px;
  font-weight: 600;
}
```

- [ ] **Step 5: Build + manual check**

Run:
```bash
cd web/admin && npm run build
```
Then load the Editor: four tabs switch content, preview stays visible on the right, "Сейчас в эфире: vN" shows, publish still works and updates the plaque. Log what you saw.

- [ ] **Step 6: Commit**

```bash
git add web/admin/src/pages/Editor.tsx web/admin/src/styles.css
git commit -m "feat(admin): tabbed widget editor with live-version plaque"
```

### Task C2: Preview desktop/mobile toggle

**Files:**
- Modify: `web/admin/src/pages/Editor.tsx`
- Modify: `web/admin/src/styles.css` (`.preview-toolbar`, `.preview-pane.is-mobile`)

**Interfaces:**
- Consumes: existing preview iframe.
- Produces: local `useState<'desktop' | 'mobile'>('desktop')` constraining the preview width.

- [ ] **Step 1: Add state + toolbar above the preview**

In the component: `const [device, setDevice] = useState<'desktop' | 'mobile'>('desktop')`. Change the preview section to:
```tsx
<section className={`preview-pane${device === 'mobile' ? ' is-mobile' : ''}`}>
  <div className="preview-toolbar">
    <button className={`secondary${device === 'desktop' ? ' active' : ''}`} onClick={() => setDevice('desktop')}>
      Десктоп
    </button>
    <button className={`secondary${device === 'mobile' ? ' active' : ''}`} onClick={() => setDevice('mobile')}>
      Мобайл
    </button>
  </div>
  <iframe title="Предпросмотр виджета" srcDoc={preview} />
</section>
```

- [ ] **Step 2: Styles**

Append:
```css
.preview-toolbar {
  display: flex;
  gap: 6px;
  padding: 10px;
  border-bottom: 1px solid var(--admin-border);
}
.preview-toolbar .active { border-color: var(--accent); color: var(--accent); }
.preview-pane.is-mobile iframe {
  width: 390px;
  max-width: 100%;
  margin: 0 auto;
}
```

- [ ] **Step 3: Build + manual check**

`npm run build`; load Editor, toggle Мобайл → iframe narrows to 390px centred, Десктоп → full width.

- [ ] **Step 4: Commit**

```bash
git add web/admin/src/pages/Editor.tsx web/admin/src/styles.css
git commit -m "feat(admin): desktop/mobile toggle for widget preview"
```

---

## Phase D — Diagnostics + nav counts

### Task D1: Backend — `GET /admin/api/counts`

**Files:**
- Create: `internal/server/admin_counts.go`
- Create: `internal/server/admin_counts_test.go`
- Modify: `internal/server/server.go:214` area (register route)

**Interfaces:**
- Produces: `GET /admin/api/counts` → `{"pendingReviews": <int64>, "pendingQuestions": <int>}`. Consumed by the sidebar badges (D2).
- Consumes: `s.store.DashboardStats(ctx)` → `store.Stats` (`.PendingReviews int64`); `s.store.ListQuestions(ctx, store.QuestionFilter{Status: "pending"})` → `[]store.Question`.

- [ ] **Step 1: Write the failing test**

The server test harness (verified) is: `newAuthTestServer(t) *Server`, `loginTestAdmin(t, s) *http.Cookie`, `seedAdminReview(t, s, extID, rating) uint` (seeds a review with status `"imported"`), and requests are served via `s.adminMux().ServeHTTP(rec, req)`. Seeded reviews are `"imported"`, so promote one to `"pending"` via `s.store.SetReviewStatus(ctx, id, "pending")` to get a deterministic count.

Create `internal/server/admin_counts_test.go`:
```go
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleCounts(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)
	id := seedAdminReview(t, s, "w1", 5)
	if err := s.store.SetReviewStatus(context.Background(), id, "pending"); err != nil {
		t.Fatalf("set pending: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/api/counts", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		PendingReviews   int64 `json:"pendingReviews"`
		PendingQuestions int   `json:"pendingQuestions"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.PendingReviews != 1 {
		t.Fatalf("pendingReviews = %d, want 1", got.PendingReviews)
	}
	if got.PendingQuestions != 0 { // none seeded
		t.Fatalf("pendingQuestions = %d, want 0", got.PendingQuestions)
	}
}
```

- [ ] **Step 2: Run it — expect FAIL (route not found → 404)**

Run:
```bash
go test ./internal/server/ -run TestHandleCounts -v
```
Expected: FAIL (404 or compile error for the missing handler).

- [ ] **Step 3: Implement the handler**

Create `internal/server/admin_counts.go`:
```go
package server

import "net/http"

func (s *Server) handleCounts(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.DashboardStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	pendingQ, err := s.store.ListQuestions(r.Context(), storeQuestionPendingFilter())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"pendingReviews":   stats.PendingReviews,
		"pendingQuestions": len(pendingQ),
	})
}
```
Add a tiny helper in the same file (keeps the filter in one place):
```go
import "reviews/internal/store"

func storeQuestionPendingFilter() store.QuestionFilter {
	return store.QuestionFilter{Status: "pending", Limit: 1000}
}
```
(Merge the two `import` blocks into one.)

- [ ] **Step 4: Register the route**

In `internal/server/server.go`, next to the dashboard registration (line ~214) add:
```go
	protected.HandleFunc("GET /admin/api/counts", s.handleCounts)
```

- [ ] **Step 5: Run test — expect PASS**

Run:
```bash
go test ./internal/server/ -run TestHandleCounts -v
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/server/admin_counts.go internal/server/admin_counts_test.go internal/server/server.go
git commit -m "feat(server): GET /admin/api/counts for nav badges"
```

### Task D2: Frontend — sidebar count badges

**Files:**
- Modify: `web/admin/src/App.tsx`
- Modify: `web/admin/src/styles.css` (`.nav-count`)

**Interfaces:**
- Consumes: `GET /admin/api/counts` (D1) via `apiGet`.
- Produces: no exports; badges next to «Отзывы»/«Вопросы».

- [ ] **Step 1: Fetch counts when authed**

In `App.tsx`, add `const [counts, setCounts] = useState<{ pendingReviews: number; pendingQuestions: number }>({ pendingReviews: 0, pendingQuestions: 0 })`. Add an effect gated on `mode === 'authed'`:
```tsx
useEffect(() => {
  if (mode !== 'authed') return
  apiGet<{ pendingReviews: number; pendingQuestions: number }>('/admin/api/counts')
    .then(setCounts)
    .catch(() => {})
}, [mode, route])
```
(Refetch on `route` change so counts refresh after moderation without extra plumbing.)

- [ ] **Step 2: Render badges**

Update the Reviews and Questions nav links:
```tsx
<a className={route === 'reviews' ? 'active' : ''} href="#/reviews">
  Отзывы {counts.pendingReviews > 0 && <span className="nav-count">{counts.pendingReviews}</span>}
</a>
<a className={route === 'questions' ? 'active' : ''} href="#/questions">
  Вопросы {counts.pendingQuestions > 0 && <span className="nav-count">{counts.pendingQuestions}</span>}
</a>
```

- [ ] **Step 3: Style**

Append to `styles.css`:
```css
.nav-count {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 20px;
  height: 20px;
  padding: 0 6px;
  border-radius: 999px;
  background: var(--admin-warn);
  color: #fff;
  font-size: 12px;
  font-weight: 700;
}
```

- [ ] **Step 4: Build + manual check**

`npm run build`; with a pending review/question present, the badge shows a count; after approving all, it clears on next navigation.

- [ ] **Step 5: Commit**

```bash
git add web/admin/src/App.tsx web/admin/src/styles.css
git commit -m "feat(admin): pending-count badges in sidebar nav"
```

### Task D3: Backend — `GET /admin/api/diagnostics` (passive checklist + activity log)

**Files:**
- Create: `internal/server/admin_diagnostics.go`
- Create: `internal/server/admin_diagnostics_test.go`
- Modify: `internal/server/server.go` (register route)

**Interfaces:**
- Produces:
  - `type DiagItem struct { ID string \`json:"id"\`; Level string \`json:"level"\`; Title string \`json:"title"\`; Detail string \`json:"detail"\`; FixHref string \`json:"fixHref,omitempty"\` }` — `Level ∈ {"ok","warn","fail"}`.
  - `type ActivityItem struct { At time.Time \`json:"at"\`; Level string \`json:"level"\`; Source string \`json:"source"\`; Message string \`json:"message"\` }`.
  - `GET /admin/api/diagnostics` → `{"checks": []DiagItem, "activity": []ActivityItem}`.
- Consumes: `s.store.GetAppSetting(ctx, store.SettingShopOrigin)`, `s.store.ListWidgetConfigVersions(ctx, "product")`, `s.store.ExportDirtySince(ctx)`, `s.productCatalogLinks()`, `s.marketplaceStatuses(r)`, `s.store.RecentSyncRuns(ctx, 10)`, `s.store.DashboardStats(ctx)`.

- [ ] **Step 1: Write the failing test**

The test file is `package server`, so it can decode into the exported `DiagItem` type directly (defined in Step 3). Use the verified harness (`newAuthTestServer`, `loginTestAdmin`, `s.adminMux()`) and `s.store.SetAppSetting(ctx, store.SettingShopOrigin, …)` to set the origin.

Create `internal/server/admin_diagnostics_test.go`:
```go
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"reviews/internal/store"
)

// levelOf returns the Level for a check ID, or "" if absent.
func levelOf(items []DiagItem, id string) string {
	for _, c := range items {
		if c.ID == id {
			return c.Level
		}
	}
	return ""
}

func getDiagnostics(t *testing.T, s *Server, cookie *http.Cookie) []DiagItem {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/admin/api/diagnostics", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Checks []DiagItem `json:"checks"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	return got.Checks
}

func TestDiagnosticsFlagsMissingShopOrigin(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)
	if got := levelOf(getDiagnostics(t, s, cookie), "cors"); got != "fail" {
		t.Fatalf("cors level = %q, want fail when origin unset", got)
	}
}

func TestDiagnosticsCorsOkWhenOriginSet(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)
	if err := s.store.SetAppSetting(context.Background(), store.SettingShopOrigin, "https://shop.example"); err != nil {
		t.Fatal(err)
	}
	if got := levelOf(getDiagnostics(t, s, cookie), "cors"); got != "ok" {
		t.Fatalf("cors level = %q, want ok", got)
	}
}
```

- [ ] **Step 2: Run — expect FAIL (404/compile)**

Run:
```bash
go test ./internal/server/ -run TestDiagnostics -v
```
Expected: FAIL.

- [ ] **Step 3: Implement**

Create `internal/server/admin_diagnostics.go`:
```go
package server

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"reviews/internal/store"
)

type DiagItem struct {
	ID      string `json:"id"`
	Level   string `json:"level"` // ok | warn | fail
	Title   string `json:"title"`
	Detail  string `json:"detail"`
	FixHref string `json:"fixHref,omitempty"`
}

type ActivityItem struct {
	At      time.Time `json:"at"`
	Level   string    `json:"level"`
	Source  string    `json:"source"`
	Message string    `json:"message"`
}

func itoa(n int) string { return strconv.Itoa(n) }

func (s *Server) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	checks := []DiagItem{}

	// CORS / shop origin
	origin, _ := s.store.GetAppSetting(ctx, store.SettingShopOrigin)
	if strings.TrimSpace(origin) == "" {
		checks = append(checks, DiagItem{
			ID: "cors", Level: "fail",
			Title:  "Адрес магазина не задан",
			Detail: "Без адреса магазина браузер блокирует виджет (CORS): он останется без данных и стилей.",
			FixHref: "#/settings/general",
		})
	} else {
		checks = append(checks, DiagItem{
			ID: "cors", Level: "ok",
			Title: "Адрес магазина задан", Detail: origin,
			FixHref: "#/settings/general",
		})
	}

	// Active widget version (product context)
	versions, err := s.store.ListWidgetConfigVersions(ctx, "product")
	if err != nil || len(versions) == 0 {
		checks = append(checks, DiagItem{
			ID: "widget-version", Level: "warn",
			Title:  "Нет опубликованной версии виджета",
			Detail: "Опубликуйте оформление на вкладке «Виджет».",
			FixHref: "#/widget/editor",
		})
	} else {
		checks = append(checks, DiagItem{
			ID: "widget-version", Level: "ok",
			Title:  "Версия виджета опубликована",
			Detail: "Есть активная конфигурация карточки товара.",
			FixHref: "#/widget/editor",
		})
	}

	// Export freshness
	if _, dirty, derr := s.store.ExportDirtySince(ctx); derr == nil && dirty {
		checks = append(checks, DiagItem{
			ID: "export", Level: "warn",
			Title:  "Есть неопубликованные изменения отзывов",
			Detail: "Нажмите «Опубликовать изменения» на странице «Отзывы», чтобы обновить данные на сайте.",
			FixHref: "#/reviews",
		})
	} else {
		checks = append(checks, DiagItem{
			ID: "export", Level: "ok",
			Title: "Экспорт отзывов актуален", Detail: "",
			FixHref: "#/reviews",
		})
	}

	// Catalog coverage
	links, _ := s.productCatalogLinks()
	if len(links) == 0 {
		checks = append(checks, DiagItem{
			ID: "catalog", Level: "warn",
			Title:  "Каталог товаров пуст",
			Detail: "Обновите каталог на странице «Маркетплейсы», чтобы виджет находил товары по URL.",
			FixHref: "#/settings/marketplaces",
		})
	} else {
		checks = append(checks, DiagItem{
			ID: "catalog", Level: "ok",
			Title:  "Каталог заполнен",
			Detail: "Товаров в каталоге: " + itoa(len(links)),
			FixHref: "#/settings/marketplaces",
		})
	}

	// Per-marketplace sync
	for _, mp := range s.marketplaceStatuses(r) {
		if !mp.Enabled {
			continue
		}
		if !mp.Configured {
			checks = append(checks, DiagItem{
				ID: "mp-" + mp.ID, Level: "fail",
				Title:  "«" + mp.ID + "»: доступы не заданы",
				Detail: "Заполните доступы на странице «Маркетплейсы».",
				FixHref: "#/settings/marketplaces",
			})
			continue
		}
		checks = append(checks, DiagItem{
			ID: "mp-" + mp.ID, Level: "ok",
			Title:  "«" + mp.ID + "»: включён и настроен",
			Detail: "",
			FixHref: "#/settings/marketplaces",
		})
	}

	// Activity log from existing sync runs.
	activity := []ActivityItem{}
	if runs, rerr := s.store.RecentSyncRuns(ctx, 10); rerr == nil {
		for _, run := range runs {
			level := "ok"
			msg := "Синхронизация «" + run.Marketplace + "» — успешно, новых " + itoa(run.ReviewsUpserted)
			if run.Status == "error" {
				level = "fail"
				text := "ошибка"
				if run.ErrorText != nil {
					text = *run.ErrorText
				}
				msg = "Синхронизация «" + run.Marketplace + "» — " + text
			}
			activity = append(activity, ActivityItem{
				At: run.StartedAt, Level: level, Source: "sync", Message: msg,
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"checks": checks, "activity": activity})
}
```
(`itoa` is defined at the top of the file; `run.StartedAt` is already a `time.Time`, matching `ActivityItem.At`.)

- [ ] **Step 4: Register route**

In `server.go` near the dashboard line:
```go
	protected.HandleFunc("GET /admin/api/diagnostics", s.handleDiagnostics)
```

- [ ] **Step 5: Run tests — expect PASS**

Run:
```bash
go test ./internal/server/ -run TestDiagnostics -v
```
Expected: PASS.

- [ ] **Step 6: Full server package test**

Run:
```bash
go test ./internal/server/
```
Expected: PASS (no regressions).

- [ ] **Step 7: Commit**

```bash
git add internal/server/admin_diagnostics.go internal/server/admin_diagnostics_test.go internal/server/server.go
git commit -m "feat(server): GET /admin/api/diagnostics passive checklist + activity log"
```

### Task D4: Backend — `POST /admin/api/diagnostics/probe` (active checks)

**Files:**
- Modify: `internal/server/admin_diagnostics.go` (add probe handler + helpers)
- Modify: `internal/server/admin_diagnostics_test.go` (add probe tests)
- Modify: `internal/server/server.go` (register CSRF-wrapped POST route)

**Interfaces:**
- Produces: `POST /admin/api/diagnostics/probe` with body `{"productUrl": "<optional string>"}` → `{"checks": []DiagItem}`. Reuses `DiagItem` from D3.
- Consumes: `s.store.GetAppSetting(ctx, store.SettingShopOrigin)`; a `*http.Client` with a short timeout for the site-reachability probe; `s.productLinks()` (already used elsewhere) to resolve the article from a product URL.

**Design:** probes must not 500 on network failure — an unreachable target yields a `fail` DiagItem. Use a 8s client timeout. The `productUrl` article-resolution reuses the same product-links map the widget uses; if the URL is absent from the map, report `warn` ("артикул не резолвится"). Honesty rule: do NOT claim the snippet is present — the checklist only reports reachability + article resolution + review availability, and the frontend shows the Tag-Manager caveat.

- [ ] **Step 1: Write failing tests**

CSRF pattern (verified in `admin_dashboard_test.go`): `csrf := getCSRFToken(t, s, cookie)`, then on the request add the session cookie, a cookie `{Name: csrfCookieName, Value: csrf}`, and header `csrfHeaderName: csrf`. Add a small local helper for the probe POST and two cases to `admin_diagnostics_test.go` (`strings` is now also imported):
```go
func probe(t *testing.T, s *Server, cookie *http.Cookie, body string) []DiagItem {
	t.Helper()
	csrf := getCSRFToken(t, s, cookie)
	req := httptest.NewRequest(http.MethodPost, "/admin/api/diagnostics/probe", strings.NewReader(body))
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK { // must soft-degrade, never 500
		t.Fatalf("probe status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Checks []DiagItem `json:"checks"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	return got.Checks
}

func TestProbeReachableSite(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)
	shop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer shop.Close()
	if err := s.store.SetAppSetting(context.Background(), store.SettingShopOrigin, shop.URL); err != nil {
		t.Fatal(err)
	}
	if got := levelOf(probe(t, s, cookie, `{}`), "site-reachable"); got != "ok" {
		t.Fatalf("site-reachable = %q, want ok", got)
	}
}

func TestProbeUnreachableSiteDoesNotError(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)
	if err := s.store.SetAppSetting(context.Background(), store.SettingShopOrigin, "http://127.0.0.1:1"); err != nil {
		t.Fatal(err)
	}
	if got := levelOf(probe(t, s, cookie, `{}`), "site-reachable"); got != "fail" {
		t.Fatalf("site-reachable = %q, want fail (soft-degrade)", got)
	}
}
```
Add `"strings"` to the test file's import block.

- [ ] **Step 2: Run — expect FAIL**

Run:
```bash
go test ./internal/server/ -run TestProbe -v
```
Expected: FAIL.

- [ ] **Step 3: Implement the probe handler**

Add to `admin_diagnostics.go` (extend the existing import block with `"encoding/json"` — `net/http`, `strings`, `time` are already imported from Task D3):
```go
type probeRequest struct {
	ProductURL string `json:"productUrl"`
}

func (s *Server) handleDiagnosticsProbe(w http.ResponseWriter, r *http.Request) {
	var req probeRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req) // empty body is fine
	}
	ctx := r.Context()
	checks := []DiagItem{}

	origin, _ := s.store.GetAppSetting(ctx, store.SettingShopOrigin)
	origin = strings.TrimSpace(origin)
	if origin == "" {
		checks = append(checks, DiagItem{
			ID: "site-reachable", Level: "warn",
			Title:  "Адрес магазина не задан",
			Detail: "Укажите адрес магазина в «Настройках», чтобы проверить доступность сайта.",
			FixHref: "#/settings/general",
		})
	} else {
		client := &http.Client{Timeout: 8 * time.Second}
		resp, err := client.Get(origin)
		if err != nil {
			checks = append(checks, DiagItem{
				ID: "site-reachable", Level: "fail",
				Title:  "Сайт магазина недоступен",
				Detail: "Не удалось открыть " + origin + ": " + err.Error(),
			})
		} else {
			resp.Body.Close()
			level, detail := "ok", "Ответ "+itoa(resp.StatusCode)
			if resp.StatusCode >= 400 {
				level = "warn"
			}
			checks = append(checks, DiagItem{
				ID: "site-reachable", Level: level,
				Title: "Сайт магазина отвечает", Detail: detail,
			})
		}
	}

	if strings.TrimSpace(req.ProductURL) != "" {
		checks = append(checks, s.probeProductURL(req.ProductURL))
	}

	writeJSON(w, http.StatusOK, map[string]any{"checks": checks})
}

// probeProductURL resolves the article for a product URL against the same
// product-link map the widget uses. It never performs a "snippet present"
// check — that cannot be done reliably server-side when the snippet is
// injected by Tag Manager (documented limitation, surfaced in the UI).
func (s *Server) probeProductURL(productURL string) DiagItem {
	article := s.resolveArticleFromURL(productURL)
	if article == "" {
		return DiagItem{
			ID: "article-resolve", Level: "warn",
			Title:  "Артикул для этой страницы не найден",
			Detail: "URL нет в каталоге — обновите каталог или проверьте адрес. Виджет не сможет подобрать отзывы.",
			FixHref: "#/settings/marketplaces",
		}
	}
	return DiagItem{
		ID: "article-resolve", Level: "ok",
		Title:  "Артикул найден",
		Detail: "URL сопоставлен с артикулом " + article,
	}
}

// resolveArticleFromURL inverts the in-memory article→URL map (s.productLinks(),
// which is keyed by seller article — verified in server.go / site.ProductLinkMap)
// to find the article whose product page matches productURL.
func (s *Server) resolveArticleFromURL(productURL string) string {
	target := strings.TrimRight(strings.TrimSpace(productURL), "/")
	for article, u := range s.productLinks() {
		if strings.TrimRight(u, "/") == target {
			return article
		}
	}
	return ""
}
```
Note: `s.productLinks()` returns a `map[string]string` keyed by **article** with the product **URL** as value, so resolution is a linear scan inverting that map (catalog size is modest). Do NOT invent a store method for "reviews-for-article"; the probe intentionally degrades to reachability + article-resolution only. The `errors` import is not needed — remove it if your editor adds it.

- [ ] **Step 4: Register CSRF-wrapped route**

In `server.go`:
```go
	protected.Handle("POST /admin/api/diagnostics/probe", requireCSRF(http.HandlerFunc(s.handleDiagnosticsProbe)))
```

- [ ] **Step 5: Run — expect PASS**

Run:
```bash
go test ./internal/server/ -run TestProbe -v
```
Expected: PASS.

- [ ] **Step 6: Vet + full package test**

Run:
```bash
go vet ./internal/server/ && go test ./internal/server/
```
Expected: clean + PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/server/admin_diagnostics.go internal/server/admin_diagnostics_test.go internal/server/server.go
git commit -m "feat(server): POST /admin/api/diagnostics/probe active checks (soft-degrading)"
```

### Task D5: Frontend — «Состояние» page

**Files:**
- Create: `web/admin/src/pages/Status.tsx`
- Modify: `web/admin/src/App.tsx` (route + nav item)
- Modify: `web/admin/src/styles.css` (`.diag-item`, `.diag-ok/warn/fail`, `.activity-row`)

**Interfaces:**
- Consumes: `GET /admin/api/diagnostics` (D3), `POST /admin/api/diagnostics/probe` (D4) via `apiGet`/`apiWrite`; `toast` for probe errors.
- Produces: default-exported `Status` component. New `Route` value `'status'`.

- [ ] **Step 1: Write the page**

Create `web/admin/src/pages/Status.tsx`:
```tsx
import { useEffect, useState } from 'react'
import { apiGet, apiWrite } from '../api'
import { toast } from '../toast'

type DiagItem = { id: string; level: 'ok' | 'warn' | 'fail'; title: string; detail: string; fixHref?: string }
type ActivityItem = { at: string; level: string; source: string; message: string }
type Diagnostics = { checks: DiagItem[]; activity: ActivityItem[] }

const levelLabel: Record<DiagItem['level'], string> = { ok: '✓', warn: '⚠', fail: '✗' }

export default function Status() {
  const [data, setData] = useState<Diagnostics | null>(null)
  const [productUrl, setProductUrl] = useState('')
  const [probe, setProbe] = useState<DiagItem[] | null>(null)
  const [probing, setProbing] = useState(false)

  useEffect(() => {
    apiGet<Diagnostics>('/admin/api/diagnostics')
      .then(setData)
      .catch((e) => toast.error(e instanceof Error ? e.message : 'Не удалось загрузить диагностику'))
  }, [])

  async function runProbe() {
    setProbing(true)
    try {
      const res = await apiWrite<{ checks: DiagItem[] }>('POST', '/admin/api/diagnostics/probe', {
        productUrl: productUrl.trim(),
      })
      setProbe(res.checks)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Проверка не выполнена')
    } finally {
      setProbing(false)
    }
  }

  if (!data) return <p className="muted">Загрузка...</p>

  return (
    <section className="stack">
      <section className="panel">
        <h3>Проверка настройки</h3>
        <div className="stack">
          {data.checks.map((c) => (
            <div className={`diag-item diag-${c.level}`} key={c.id}>
              <span className="diag-mark">{levelLabel[c.level]}</span>
              <div>
                <strong>{c.title}</strong>
                {c.detail && <p className="muted">{c.detail}</p>}
              </div>
              {c.fixHref && <a className="diag-fix" href={c.fixHref}>Открыть</a>}
            </div>
          ))}
        </div>
      </section>

      <section className="panel">
        <h3>Проверить страницу товара</h3>
        <p className="muted">
          Вставьте адрес страницы товара — проверим доступность сайта и что виджет сможет
          подобрать отзывы. Если все проверки зелёные, а виджета нет — убедитесь, что контейнер
          в Тег Менеджере опубликован (вставленный через Тег Менеджер сниппет сервер проверить
          не может).
        </p>
        <div className="toolbar">
          <input
            className="search-input"
            value={productUrl}
            onChange={(e) => setProductUrl(e.target.value)}
            placeholder="https://ваш-магазин.ру/product/..."
          />
          <button onClick={runProbe} disabled={probing}>
            {probing ? 'Проверяем…' : 'Проверить'}
          </button>
        </div>
        {probe && (
          <div className="stack">
            {probe.map((c) => (
              <div className={`diag-item diag-${c.level}`} key={c.id}>
                <span className="diag-mark">{levelLabel[c.level]}</span>
                <div>
                  <strong>{c.title}</strong>
                  {c.detail && <p className="muted">{c.detail}</p>}
                </div>
              </div>
            ))}
          </div>
        )}
      </section>

      <section className="panel">
        <h3>Журнал</h3>
        <div className="rows">
          {data.activity.length === 0 && <p className="muted">Событий пока нет.</p>}
          {data.activity.map((a, i) => (
            <div className={`activity-row activity-${a.level}`} key={i}>
              <span className="muted">{new Date(a.at).toLocaleString()}</span>
              <span>{a.message}</span>
            </div>
          ))}
        </div>
      </section>
    </section>
  )
}
```

- [ ] **Step 2: Add styles**

Append to `styles.css`:
```css
.diag-item {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 12px;
  border: 1px solid var(--admin-border);
  border-radius: 10px;
  border-left-width: 4px;
}
.diag-item > div { flex: 1; min-width: 0; }
.diag-item p { margin: 4px 0 0; }
.diag-mark { font-size: 16px; font-weight: 700; line-height: 1.5; }
.diag-ok { border-left-color: var(--ok); }
.diag-ok .diag-mark { color: var(--ok); }
.diag-warn { border-left-color: var(--warn); }
.diag-warn .diag-mark { color: var(--warn); }
.diag-fail { border-left-color: var(--danger); }
.diag-fail .diag-mark { color: var(--danger); }
.diag-fix {
  align-self: center;
  color: var(--accent);
  text-decoration: none;
  font-weight: 600;
}
.activity-row {
  display: flex;
  gap: 12px;
  border-top: 1px solid var(--admin-border);
  padding-top: 10px;
}
.activity-fail span:last-child { color: var(--danger); }
```

- [ ] **Step 3: Wire route + nav in `App.tsx`**

Add `'status'` to the `Route` union and the `ROUTES` array. Add `import Status from './pages/Status'`. Add a `routeTitle` case (`case 'status': return 'Состояние'`). Add a nav link near the top (after «Вопросы»):
```tsx
<a className={route === 'status' ? 'active' : ''} href="#/status">
  Состояние
</a>
```
Add the render branch: `{route === 'status' && <Status />}`. Update `routeSection` if it must return a section for `status` (return `'dashboard'` group or add a case — keep it simple: `if (route === 'status') return 'dashboard'` is fine since sections only drive the widget/settings sub-nav highlight).

- [ ] **Step 4: Build**

Run:
```bash
cd web/admin && npm run build
```
Expected: success.

- [ ] **Step 5: Manual check (see D6 for full flow)**

Load `#/status`: checklist renders with ✓/⚠/✗; entering a URL + «Проверить» shows probe results; журнал lists recent syncs. Note the Tag-Manager caveat text is visible.

- [ ] **Step 6: Commit**

```bash
git add web/admin/src/pages/Status.tsx web/admin/src/App.tsx web/admin/src/styles.css
git commit -m "feat(admin): Состояние diagnostics page (checklist + probe + activity)"
```

### Task D6: Frontend — Dashboard health summary + end-to-end verification

**Files:**
- Modify: `web/admin/src/pages/Dashboard.tsx`
- Modify: `web/admin/src/styles.css` (`.health-strip`)

**Interfaces:**
- Consumes: `GET /admin/api/diagnostics` (D3).
- Produces: a compact traffic-light strip on the Dashboard linking to `#/status`.

- [ ] **Step 1: Fetch diagnostics summary in Dashboard**

In `Dashboard.tsx`, add a second fetch:
```tsx
const [diag, setDiag] = useState<{ checks: { id: string; level: string }[] } | null>(null)
useEffect(() => {
  apiGet<{ checks: { id: string; level: string }[] }>('/admin/api/diagnostics')
    .then(setDiag)
    .catch(() => {})
}, [])
```

- [ ] **Step 2: Render the strip above the metrics**

Add, before `<div className="metrics">`:
```tsx
{diag && (() => {
  const fails = diag.checks.filter((c) => c.level === 'fail').length
  const warns = diag.checks.filter((c) => c.level === 'warn').length
  const tone = fails > 0 ? 'fail' : warns > 0 ? 'warn' : 'ok'
  const label =
    tone === 'ok' ? 'Всё в порядке' : `${fails} проблем · ${warns} предупреждений`
  return (
    <a className={`health-strip health-${tone}`} href="#/status">
      <span>Состояние: {label}</span>
      <span className="muted">Подробнее →</span>
    </a>
  )
})()}
```

- [ ] **Step 3: Style**

Append:
```css
.health-strip {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 16px;
  border-radius: 12px;
  border: 1px solid var(--admin-border);
  border-left-width: 4px;
  text-decoration: none;
  color: var(--admin-text);
}
.health-ok { border-left-color: var(--ok); }
.health-warn { border-left-color: var(--warn); }
.health-fail { border-left-color: var(--danger); }
```

- [ ] **Step 4: Build**

Run:
```bash
cd web/admin && npm run build
```
Expected: success.

- [ ] **Step 5: End-to-end verification (whole sprint)**

Use the `run` skill to launch the server + admin. Walk the full surface and log observations:
1. Design: indigo/neutral theme, Inter, flat cards, no purple/cream/Prata.
2. Save a Setting → success toast; edit → «Есть изменения» badge + navigation guard.
3. Widget editor: four tabs switch, preview persists, desktop/mobile toggle works, «Сейчас в эфире: vN» shows, publish → toast «Опубликована v… — уже на сайте» and badge clears.
4. Sidebar badges show pending counts and clear after moderation.
5. Dashboard health strip reflects state and links to «Состояние».
6. «Состояние»: checklist ✓/⚠/✗ with fix links; «Проверить страницу товара» returns probe results on a reachable URL and a graceful fail on an unreachable one; журнал lists syncs; Tag-Manager caveat visible.

- [ ] **Step 6: Full backend test + build gate**

Run:
```bash
go test ./... && cd web/admin && npm run build
```
Expected: all Go tests PASS; admin build succeeds.

- [ ] **Step 7: Commit**

```bash
git add web/admin/src/pages/Dashboard.tsx web/admin/src/styles.css
git commit -m "feat(admin): dashboard health summary strip linking to Состояние"
```

---

## Rebuild embedded admin dist (before deploy)

The server embeds the built admin (`internal/server/admin_dist/`). After the frontend work, rebuild and refresh the embedded copy per the repo's existing build step (see `web/admin/build-embed.sh`). This is a release concern, not a per-task gate — do it once at the end:
```bash
cd web/admin && npm run build && ./build-embed.sh
```
Then commit the regenerated `internal/server/admin_dist/**` if the repo tracks it.

---

## Self-Review notes (author)

- **Spec coverage:** design tokens (A2) + Inter local (A1); widget tabs (C1) + preview toggle (C2); toasts (B1–B3) + dirty/guard (B4) + "live version" (C1) + differentiated "applied" MP toast (B3 step 3); «Витрина» rewording (B3 step 2); diagnostics L2 = passive checklist + activity (D3) + active probes (D4) + Status page (D5) + dashboard summary (D6); nav badges (D1–D2). Tag-Manager honesty caveat is in the D4 design note + D5 UI copy.
- **No new store methods / no schema changes** — every backend task consumes existing `store.Store` methods verified in the codebase (`DashboardStats`, `ListQuestions`, `GetAppSetting`/`SetAppSetting`, `SetReviewStatus`, `ListWidgetConfigVersions`, `ExportDirtySince`, `RecentSyncRuns`, `productCatalogLinks`, `productLinks`).
- **Frontend has no test runner** — gates are `npm run build` (tsc) + explicit manual verification steps; backend uses real Go table tests (TDD).
- **Test harness is verified** (from `admin_dashboard_test.go` / `admin_reviews_test.go` / `admin_auth_test.go`): `newAuthTestServer(t) *Server`, `loginTestAdmin(t, s) *http.Cookie`, `getCSRFToken(t, s, cookie) string`, constants `csrfCookieName`/`csrfHeaderName`, seed helper `seedAdminReview(t, s, extID, rating) uint`, and requests are served with `s.adminMux().ServeHTTP(rec, req)`. Use these names verbatim — do NOT use `srv.Handler()` / `newTestServer` (those don't exist).
