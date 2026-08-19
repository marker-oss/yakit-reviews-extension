# Фиксы виджета к коммерческому демо

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

Дата: 2026-08-18
Статус: **READY — следующий спринт (2 дня, не блокируется ничем)**
Источник: Этап 0 из `2026-08-18-subscription-model-rollout.md` (вынесен как независимый план)

**Goal:** Починить два бага виджета, без которых SaaS-демо не продастся — счётчик на главной и чужеродную палитру. Не требует решений по SaaS/биллингу/инфре.

**Why standalone:** Не зависит от мультитенанта, шифрования, Prodamus и доменов. Можно делать прямо сейчас параллельно с любыми продуктовыми решениями. Результат — виджет без стыда показывать в коммерческом демо.

**Tech Stack:** `web/reviews-widget/*` (vanilla JS, no build), `internal/server/admin_showcase.go`, `internal/store/widget_config_models.go`, `web/admin/src/pages/Editor.tsx`.

## Global Constraints

- Виджет — plain unbundled JS (`web/reviews-widget/reviews-widget.js`), без сборки. Проверять `node --check`.
- Админка — `web/admin` (React 19 + Vite); после изменений `npm run build` + `rm -rf internal/server/admin_dist && cp -r web/admin/dist internal/server/admin_dist`.
- Go из корня репо (`/home/mama/DEV/Reviews`), npm из `web/admin`.
- UI-копи на русском.

---

### Task 1: Баг счётчика на главной («12 отзывов», видно 3)

**Проблема:** `GET /api/showcase` отдаёт 12 отзывов + агрегат по всей базе; виджет в `homepage` капает показ до `min(pageSize,4)` и прячет остальное за кнопку «Посмотреть отзывы» внизу экрана. Заголовок врёт.

**Files:**
- Modify: `web/reviews-widget/reviews-widget.js` — `initialVisible()` (~1378), `mount()`, `renderSummary()` (~590), `effectiveVisibleCount()`
- Modify: `internal/server/admin_showcase.go` — `handleShowcase`
- Modify: `internal/server/admin_showcase_test.go` — assertion на агрегат

**Interfaces:**
- Produces: витрина показывает все отзывы витринного набора без капа; агрегат в `/api/showcase` считается по витринному набору (`publicReviewAggregate(items)`), а не по всей базе (`publicShowcaseAggregate`). Глобальный агрегат остаётся для dashboard.

- [ ] **Step 1: Починить виджет — убрать кап homepage**

В `web/reviews-widget/reviews-widget.js`:
- `initialVisible(config, context)` — убрать ветку `if (context === "homepage") return min(pageSize,4)`, оставить единый `return config.layout.pageSize` (или `Math.max(1, pageSize)`).
- `mount()` — `expanded: context !== "homepage"` → `expanded: true` (витрина сразу раскрыта).
- `renderSummary()` (~590) — единый текст `«показано N из M»` для обоих контекстов вместо специального homepage-варианта.
- `effectiveVisibleCount()` — ветка `homepage && !expanded` становится мёртвой; удалить.

- [ ] **Step 2: Починить бэкенд — агрегат по витринному набору**

В `internal/server/admin_showcase.go` `handleShowcase`: считать агрегат через `publicReviewAggregate(items)` по уже отфильтрованному витринному набору, а не `publicShowcaseAggregate()` по всей базе. Глобальный `publicShowcaseAggregate` оставить для dashboard.

- [ ] **Step 3: Тест**

Расширить `admin_showcase_test.go`: `aggregate.TotalReviews == len(Reviews)` в ответе `/api/showcase`.

- [ ] **Step 4: Проверка в браузере**

На живой витрине заголовок совпадает с числом карточек; «Показать ещё» появляется только при наличии отзывов за пределами витрины.

- [ ] **Step 5: Commit**

```bash
git add web/reviews-widget/reviews-widget.js internal/server/admin_showcase.go internal/server/admin_showcase_test.go
git commit -m "fix(widget): showcase counter — aggregate from showcase set, no homepage cap"
```

---

### Task 2: Виджет в палитре магазина (inheritSite + пресеты)

**Проблема:** Дефолт — фиолет `#68478D` + Prata/Onest на бежевом фоне; на типовом Кит-магазине (`brandPrimary` из settings, напр. `#079CDF`) выглядит чужеродно.

**Files:**
- Modify: `web/reviews-widget/reviews-widget.css` — `font-family: inherit` как опция
- Modify: `web/reviews-widget/reviews-widget.js` — `normalizeConfig()` / `applyConfig()` (+ флаг `typography.inheritSite`)
- Modify: `web/admin/src/pages/Editor.tsx` — 3 пресета
- Modify: `internal/store/widget_config_models.go` — схема Payload не меняется (флаг внутри JSON)

**Interfaces:**
- Produces: `typography.inheritSite: bool` — при `true` виджет наследует шрифт/цвета страницы (`--rw-accent` из `getComputedStyle` хоста, фолбэк на текущий). Пресеты — подстановка в форму Editor, конфиг-объект не меняется.

- [ ] **Step 1: CSS — inherit как опция**

В `reviews-widget.css`: `font-family: inherit` как опция; убрать обязательный `@import` шрифтов при `inheritSite`.

- [ ] **Step 2: JS — флаг inheritSite**

В `reviews-widget.js` `normalizeConfig()`/`applyConfig()`: флаг `typography.inheritSite: bool` — наследовать шрифт/цвета страницы (`--rw-accent` берётся из `getComputedStyle` хоста, фолбэк на текущий).

- [ ] **Step 3: Пресеты в админке**

В `web/admin/src/pages/Editor.tsx` 3 пресета: «Нативный Кит» (inheritSite + radius 8), «Минимализм», «Текущий» (как есть). Пресет — просто подстановка в форму, конфиг-объект не меняется.

- [ ] **Step 4: Store — без миграции схемы**

`internal/store/widget_config_models.go` — схема Payload не меняется (флаг внутри JSON).

- [ ] **Step 5: Проверка**

На демо-странице с палитрой `#079CDF` пресет «Нативный Кит» не выбивается; переключение пресетов в админке live-preview работает. `node --check web/reviews-widget/reviews-widget.js` OK.

- [ ] **Step 6: Commit**

```bash
git add web/reviews-widget/ web/admin/src/pages/Editor.tsx internal/store/widget_config_models.go internal/server/admin_dist
git commit -m "feat(widget): inheritSite + presets (Нативный Кит / Минимализм)"
```

---

## Приёмка спринта

- [ ] Витрина: заголовок = числу карточек; «Показать ещё» только при наличии скрытых отзывов.
- [ ] Пресет «Нативный Кит» на синей палитре (`#079CDF`) визуально вписывается.
- [ ] `go test ./...` зелёный; `node --check web/reviews-widget/reviews-widget.js` OK.
