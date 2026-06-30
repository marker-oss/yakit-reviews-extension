# Admin Navigation Consolidation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Collapse the 7 flat admin tabs into 4 logical top-level sections (Сводка / Отзывы / Виджет / Настройки) with sub-tabs, without changing any page's behavior.

**Architecture:** Pure frontend refactor of `web/admin/src/App.tsx`. Existing page components (`Dashboard`, `Reviews`, `Showcase`, `Editor`, `Embed`, `Marketplaces`, `Settings`) are reused unchanged — only the routing model, the sidebar nav, and the page-render switch change. Hash routes become two-segment (`#/widget/editor`, `#/settings/marketplaces`); single-segment legacy routes (`#/showcase`, `#/marketplaces`, etc.) are normalized to their new home so old bookmarks keep working.

**Tech Stack:** React 18 + TypeScript, Vite, hash-based routing (no router lib). Admin bundle is built with `npm run build` (outputs `web/admin/dist`) and embedded into the Go binary by copying to `internal/server/admin_dist` (`//go:embed all:admin_dist`).

## Global Constraints

- UI copy is Russian — match existing tone (e.g. "Сводка", "Отзывы", "Маркетплейсы", "Витрина", "Редактор", "Встраивание", "Настройки").
- No frontend unit-test runner exists in this repo. The per-task gate is: `npm run build` succeeds (TypeScript typecheck + bundle) **plus** the manual verification step listed in the task. Do not invent a test framework.
- Do not modify page component files (`pages/*.tsx`) except `App.tsx`; their internals stay as-is.
- The embedded bundle must be refreshed (`rm -rf internal/server/admin_dist && cp -r web/admin/dist internal/server/admin_dist`) before committing any admin change, or the Go binary serves a stale UI.
- Build/copy commands run from repo root `/home/mama/DEV/Reviews`; npm commands run from `web/admin`.

## Target navigation map

| Top-level | Sub-tab | Hash route | Component | Legacy hash normalized from |
|---|---|---|---|---|
| Сводка | — | `#/dashboard` | `Dashboard` | `#/`, `#` |
| Отзывы | — | `#/reviews` | `Reviews` | `#/reviews` |
| Виджет | Витрина | `#/widget/showcase` | `Showcase` | `#/showcase` |
| Виджет | Редактор | `#/widget/editor` | `Editor` | `#/editor` |
| Виджет | Встраивание | `#/widget/embed` | `Embed` | `#/embed` |
| Настройки | Общие | `#/settings/general` | `Settings` | `#/settings` |
| Настройки | Маркетплейсы | `#/settings/marketplaces` | `Marketplaces` | `#/marketplaces` |

## File Structure

- Modify: `web/admin/src/App.tsx` — routing model (`Route` keys), `currentRoute()` parser with legacy normalization, `title`/group helpers, sidebar markup, render switch.
- Modify: `web/admin/src/styles.css` — sidebar group + sub-nav styling.
- Regenerate: `internal/server/admin_dist/*` — rebuilt bundle (not hand-edited).

---

### Task 1: Routing model + legacy-route normalization

**Files:**
- Modify: `web/admin/src/App.tsx` (the `Page` type at line 11, `currentPage()` at lines 25-29, `title` useMemo at lines 61-68, `page` state at line 40)

**Interfaces:**
- Produces: `type Route = 'dashboard' | 'reviews' | 'widget/showcase' | 'widget/editor' | 'widget/embed' | 'settings/general' | 'settings/marketplaces'`
- Produces: `currentRoute(): Route` — parses `window.location.hash`, normalizing legacy single-segment hashes per the target map; unknown → `'dashboard'`.
- Produces: `routeTitle(route: Route): string` and `routeSection(route: Route): 'dashboard' | 'reviews' | 'widget' | 'settings'`.

- [ ] **Step 1: Replace the `Page` type and `currentPage()` with the `Route` model**

Replace line 11:

```tsx
type Route =
  | 'dashboard'
  | 'reviews'
  | 'widget/showcase'
  | 'widget/editor'
  | 'widget/embed'
  | 'settings/general'
  | 'settings/marketplaces'
```

Replace `currentPage()` (lines 25-29) with:

```tsx
const LEGACY_ROUTES: Record<string, Route> = {
  '': 'dashboard',
  dashboard: 'dashboard',
  reviews: 'reviews',
  showcase: 'widget/showcase',
  editor: 'widget/editor',
  embed: 'widget/embed',
  settings: 'settings/general',
  marketplaces: 'settings/marketplaces',
}

const ROUTES: Route[] = [
  'dashboard',
  'reviews',
  'widget/showcase',
  'widget/editor',
  'widget/embed',
  'settings/general',
  'settings/marketplaces',
]

function currentRoute(): Route {
  const raw = window.location.hash.replace(/^#\/?/, '')
  if ((ROUTES as string[]).includes(raw)) return raw as Route
  if (raw in LEGACY_ROUTES) return LEGACY_ROUTES[raw]
  return 'dashboard'
}

function routeSection(route: Route): 'dashboard' | 'reviews' | 'widget' | 'settings' {
  if (route === 'dashboard') return 'dashboard'
  if (route === 'reviews') return 'reviews'
  return route.startsWith('widget/') ? 'widget' : 'settings'
}

function routeTitle(route: Route): string {
  switch (route) {
    case 'reviews':
      return 'Отзывы'
    case 'widget/showcase':
      return 'Виджет · Витрина'
    case 'widget/editor':
      return 'Виджет · Редактор'
    case 'widget/embed':
      return 'Виджет · Встраивание'
    case 'settings/general':
      return 'Настройки'
    case 'settings/marketplaces':
      return 'Настройки · Маркетплейсы'
    default:
      return 'Сводка'
  }
}
```

- [ ] **Step 2: Update the `page` state + title to use the route model**

Change line 40 from `const [page, setPage] = useState<Page>(currentPage)` to:

```tsx
const [route, setRoute] = useState<Route>(currentRoute)
```

Change the hashchange effect (lines 56-60) body `setPage(currentPage())` to `setRoute(currentRoute())`.

Replace the `title` useMemo (lines 61-68) with:

```tsx
const title = useMemo(() => routeTitle(route), [route])
```

(The sidebar and render switch still reference `page`; they are rewritten in Tasks 2-3. The build will fail until then — that is expected and verified in the next step.)

- [ ] **Step 3: Verify the typecheck fails on the not-yet-updated references**

Run: `cd web/admin && npm run build`
Expected: FAIL — TypeScript errors referencing `page` / `setPage` / `Page` in the sidebar and render switch (those are fixed in Tasks 2-3).

- [ ] **Step 4: Commit the routing model**

```bash
cd /home/mama/DEV/Reviews
git add web/admin/src/App.tsx
git commit -m "refactor(admin): two-segment route model with legacy normalization"
```

---

### Task 2: Sidebar nav with 4 grouped sections

**Files:**
- Modify: `web/admin/src/App.tsx` (the `<nav>` block, currently lines ~152-163)

**Interfaces:**
- Consumes: `route`, `routeSection` from Task 1.
- Produces: grouped sidebar markup using existing CSS classes plus new `nav-group` / `nav-sub` classes styled in Task 4.

- [ ] **Step 1: Replace the `<nav>` markup**

Replace the entire `<nav>...</nav>` block with:

```tsx
<nav>
  <a className={route === 'dashboard' ? 'active' : ''} href="#/dashboard">
    Сводка
  </a>
  <a className={route === 'reviews' ? 'active' : ''} href="#/reviews">
    Отзывы
  </a>
  <div className="nav-group">
    <span className={`nav-group-label${routeSection(route) === 'widget' ? ' active' : ''}`}>
      Виджет
    </span>
    <a className={`nav-sub${route === 'widget/showcase' ? ' active' : ''}`} href="#/widget/showcase">
      Витрина
    </a>
    <a className={`nav-sub${route === 'widget/editor' ? ' active' : ''}`} href="#/widget/editor">
      Редактор
    </a>
    <a className={`nav-sub${route === 'widget/embed' ? ' active' : ''}`} href="#/widget/embed">
      Встраивание
    </a>
  </div>
  <div className="nav-group">
    <span className={`nav-group-label${routeSection(route) === 'settings' ? ' active' : ''}`}>
      Настройки
    </span>
    <a className={`nav-sub${route === 'settings/general' ? ' active' : ''}`} href="#/settings/general">
      Общие
    </a>
    <a className={`nav-sub${route === 'settings/marketplaces' ? ' active' : ''}`} href="#/settings/marketplaces">
      Маркетплейсы
    </a>
  </div>
</nav>
```

- [ ] **Step 2: Verify (still expected to fail on the render switch only)**

Run: `cd web/admin && npm run build`
Expected: FAIL — remaining TypeScript errors only in the page-render switch (`page === ...`), fixed in Task 3. No errors in the nav block.

- [ ] **Step 3: Commit**

```bash
cd /home/mama/DEV/Reviews
git add web/admin/src/App.tsx
git commit -m "refactor(admin): grouped sidebar (Сводка/Отзывы/Виджет/Настройки)"
```

---

### Task 3: Page render switch on routes

**Files:**
- Modify: `web/admin/src/App.tsx` (the render switch, currently lines ~166-173)

**Interfaces:**
- Consumes: `route` from Task 1; existing imported components `Dashboard`, `Reviews`, `Showcase`, `Editor`, `Embed`, `Marketplaces`, `Settings`.

- [ ] **Step 1: Replace the render switch**

Replace the block:

```tsx
{page === 'dashboard' && <Dashboard />}
{page === 'reviews' && <Reviews />}
{page === 'marketplaces' && <Marketplaces />}
{page === 'showcase' && <Showcase />}
{page === 'editor' && <Editor />}
{page === 'embed' && <Embed />}
{page === 'settings' && <Settings />}
```

with:

```tsx
{route === 'dashboard' && <Dashboard />}
{route === 'reviews' && <Reviews />}
{route === 'widget/showcase' && <Showcase />}
{route === 'widget/editor' && <Editor />}
{route === 'widget/embed' && <Embed />}
{route === 'settings/general' && <Settings />}
{route === 'settings/marketplaces' && <Marketplaces />}
```

- [ ] **Step 2: Verify the full app typechecks and builds**

Run: `cd web/admin && npm run build`
Expected: PASS — no TypeScript errors, bundle written to `web/admin/dist`.

- [ ] **Step 3: Commit**

```bash
cd /home/mama/DEV/Reviews
git add web/admin/src/App.tsx
git commit -m "refactor(admin): render pages from route keys"
```

---

### Task 4: Sidebar group styling

**Files:**
- Modify: `web/admin/src/styles.css` (append; sidebar `<a>` rules already exist — reuse their look)

**Interfaces:**
- Consumes: `nav-group`, `nav-group-label`, `nav-sub` classes emitted in Task 2.

- [ ] **Step 1: Add the group/sub-nav rules**

Append to `styles.css`:

```css
.sidebar nav .nav-group {
  display: flex;
  flex-direction: column;
}
.sidebar nav .nav-group-label {
  font-size: 0.75rem;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  opacity: 0.6;
  padding: 0.75rem 0 0.25rem;
}
.sidebar nav .nav-group-label.active {
  opacity: 1;
}
.sidebar nav a.nav-sub {
  padding-left: 1.5rem;
  font-size: 0.95rem;
}
```

- [ ] **Step 2: Build and visually verify navigation**

Run: `cd web/admin && npm run build`
Expected: PASS.

Then run the dev server and click through every nav item:

Run: `cd web/admin && npm run dev` (open the printed URL, log in)
Manual checks:
- Sidebar shows: Сводка, Отзывы, then "Виджет" group with Витрина/Редактор/Встраивание, then "Настройки" group with Общие/Маркетплейсы.
- Each link loads the correct page and the topbar title matches `routeTitle`.
- Visiting a legacy hash directly (e.g. `#/marketplaces`, `#/showcase`, `#/settings`) lands on the correct new page.
- Active styling highlights both the group label and the current sub-item.

- [ ] **Step 3: Commit**

```bash
cd /home/mama/DEV/Reviews
git add web/admin/src/styles.css
git commit -m "style(admin): grouped sidebar navigation"
```

---

### Task 5: Rebuild and embed the admin bundle

**Files:**
- Regenerate: `internal/server/admin_dist/index.html`, `internal/server/admin_dist/assets/index.js`, `internal/server/admin_dist/assets/index.css`

- [ ] **Step 1: Build and copy into the embed dir**

```bash
cd /home/mama/DEV/Reviews/web/admin && npm run build
cd /home/mama/DEV/Reviews && rm -rf internal/server/admin_dist && cp -r web/admin/dist internal/server/admin_dist
```

- [ ] **Step 2: Verify the new nav is in the embedded bundle and Go still builds**

Run: `cd /home/mama/DEV/Reviews && grep -c "nav-group" internal/server/admin_dist/assets/index.js`
Expected: `1` (≥1).

Run: `cd /home/mama/DEV/Reviews && go build ./...`
Expected: PASS (no output).

- [ ] **Step 3: Commit**

```bash
cd /home/mama/DEV/Reviews
git add internal/server/admin_dist
git commit -m "build(admin): rebuild embedded bundle with grouped nav"
```

---

## Self-Review

- **Spec coverage:** All 7 current pages are reachable in the target map (Task render switch covers dashboard, reviews, showcase, editor, embed, settings, marketplaces). Legacy hashes normalized (Task 1 `LEGACY_ROUTES`). ✓
- **Placeholder scan:** No TBD/TODO; every code step shows full code. ✓
- **Type consistency:** `Route` union, `currentRoute`, `routeSection`, `routeTitle` names are identical across Tasks 1-3; nav classes `nav-group`/`nav-group-label`/`nav-sub` defined in Task 2 and styled in Task 4 match. ✓
- **Note:** This plan deliberately uses `npm run build` + manual verification as the gate because the repo has no frontend test runner (honest adaptation of the TDD cycle, per Global Constraints).
