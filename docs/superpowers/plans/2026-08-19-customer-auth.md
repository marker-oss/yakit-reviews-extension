# Авторизация покупателей в виджете — план

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

Дата: 2026-08-19
Статус: READY — после `widget-demo-fixes`, параллельно с `marketplace-api-coordination`
Спек: `docs/superpowers/specs/2026-08-19-customer-auth-design.md`

**Goal:** Покупатель логинится в виджете (Yandex ID popup или Email OTP) чтобы оставить отзыв; без сессии — стена логина, а не форма. Верифицированный отзыв получает бейдж. Фича под флагом подписки (SaaS pro/pro+, self-host off).

**Why gated:** отзыв = доверие + антиспам. Анонимный путь депрекейтим; только отзывы (Q&A/комментарии вне скоупа).

**Tech Stack:** Go 1.24+ stdlib, GORM/SQLite, `net/http` (popup+postMessage), `crypto/rand`, `x/oauth2` не добавляем (чистый `net/http` для Yandex), `web/reviews-widget/*` (vanilla JS, Shadow DOM), React 19 admin.

## Global Constraints

- Self-host пока не трогаем — `REVIEWS_CUSTOMER_AUTH_ENABLED=false` для OSS, `true` для SaaS (до `tenants.plan`).
- Paywall: после появления `tenants` → `customerAuthEnabled = plan in (pro, pro+)`; до того — env флаг.
- Виджет — cross-origin (чужой домен Кит) → `SameSite=None; Secure; Partitioned` + fallback `Authorization: Bearer` из localStorage.
- CORS: `AllowCredentials: true`, `AllowedOrigins` per-tenant как сейчас.
- Виджет — plain unbundled JS, `node --check` после изменений. Админка — `npm run build` + `cp -r web/admin/dist → internal/server/admin_dist`.
- Go из корня, npm из `web/admin`. UI-копи на русском. Тесты `go test ./...`.
- 152-ФЗ: `EmailHash` + `YandexUID` hash-опционально, `VerifiedAt`; DSR расширяется на `yandex_uid`.

---

## Phase A — каркас сессии и провайдеры (1.5 дня)

### Task A1: Модели — CustomerSession, EmailOTP, YandexOAuthState + расширение ReviewerIdentity

**Files:**
- Modify: `internal/store/models.go` — расширить `ReviewerIdentity` (+YandexUID/Provider/DisplayName/AvatarURL), добавить `Review.Verified/VerifiedProvider/CustomerSessionID`
- Create: `internal/store/customer_auth_models.go` — `CustomerSession`, `EmailOTPCode`, `YandexOAuthState`
- Modify: `internal/store/store.go` — `AutoMigrate` для 3 новых таблиц
- Create: `internal/store/customer_auth_test.go` — upsert по yandex_uid, по email, привязка, TTL

**Interfaces:**
- Produces: `CustomerSession`, `EmailOTPCode`, `YandexOAuthState`, расширенный `ReviewerIdentity`; `FindOrCreateCustomerIdentity`, `CreateCustomerSession`, `FindCustomerSessionByToken`, `DeleteExpiredCustomerSessions`.

- [ ] **Step 1: Расширить модели**

`ReviewerIdentity` добавить `YandexUID *string` (uniqueIndex where not null), `Provider string`, `DisplayName`, `AvatarURL`. `Review` добавить `Verified bool`, `VerifiedProvider *string`, `CustomerSessionID *string`.

- [ ] **Step 2: Новые таблицы**

`CustomerSession{Token PK, TenantID, IdentityID, Provider, YandexUID, Email, DisplayName, AvatarURL, ExpiresAt}`, `EmailOTPCode{ID, TenantID, EmailNormalized, CodeHash, Attempts, ExpiresAt}`, `YandexOAuthState{State PK, TenantID, PKCEVerifier, RedirectURI, CreatedAt}`.

- [ ] **Step 3: Миграция**

Добавить 3 таблицы в `store.go` AutoMigrate. Проверить `go vet`.

- [ ] **Step 4: Commit**

```bash
git add internal/store/models.go internal/store/customer_auth_models.go internal/store/store.go internal/store/customer_auth_test.go
git commit -m "feat(store): customer auth models — session, OTP, OAuth state, verified review"
```

---

### Task A2: Провайдер-интерфейс + Yandex ID (start/callback)

**Files:**
- Create: `internal/customer_auth/provider.go` — `Provider` interface, `Identity` struct
- Create: `internal/customer_auth/yandex.go` — `YandexProvider: Start, Callback` (oauth.yandex.ru + login.yandex.ru/info)
- Create: `internal/customer_auth/yandex_test.go` — httptest mocks для token/info

**Interfaces:**
- Produces: `Provider{Name, Start(tenantID) (url), Callback(r) (*Identity)}`, `YandexProvider` с `ClientID/Secret/RedirectURI`, PKCE S256, state 32 bytes hex.

- [ ] **Step 1: Интерфейс**

`type Identity struct { Provider, YandexUID *string, Email, DisplayName, AvatarURL }`.

- [ ] **Step 2: Yandex Start**

Генерит `state`+`PKCE verifier/challenge`, пишет `YandexOAuthState`, возвращает `https://oauth.yandex.ru/authorize?response_type=code&client_id=&state=&code_challenge=&scope=login:email login:info`.

- [ ] **Step 3: Yandex Callback**

Валидирует `state` (one-time, TTL 10м), `POST /token` (client_id/secret), `GET /info?format=json` (Bearer), парсит `id`, `default_email`, `display_name`, `default_avatar_id` → avatar URL.

- [ ] **Step 4: Commit**

```bash
git add internal/customer_auth/
git commit -m "feat(auth): Yandex ID provider — PKCE, state, token+info"
```

---

### Task A3: Email OTP провайдер

**Files:**
- Create: `internal/customer_auth/email_otp.go` — `EmailOTPProvider: Start(email), Verify(email,code)`
- Create: `internal/customer_auth/email_otp_test.go`

**Interfaces:**
- Produces: `Start` генерит 6-значный код, `sha256` hash, TTL 10м, 3/час per email+IP; `Verify` — 3 попытки, издатель `Identity{Email, DisplayName=email}`.

- [ ] **Step 1: Start**

Валидировать `NormalizeEmail`, rate-limit, генерить код, писать `EmailOTPCode{CodeHash}`, отправлять через `REVIEWS_SMTP_*` (dev — log).

- [ ] **Step 2: Verify**

Сверять `CodeHash`, инкремент `Attempts`, TTL, удалить при успехе, вернуть `Identity`.

- [ ] **Step 3: Commit**

```bash
git add internal/customer_auth/email_otp.go internal/customer_auth/email_otp_test.go
git commit -m "feat(auth): email OTP provider — 6-digit code, 10m TTL"
```

---

### Task A4: HTTP — /api/auth/* и cookie

**Files:**
- Create: `internal/server/customer_auth.go` — `handleYandexStart, handleYandexCallback, handleEmailOTPStart, handleEmailOTPVerify, handleAuthMe, handleAuthLogout`, `customerAuthEnabled(tenant)` paywall
- Modify: `internal/server/server.go` — `handler()` public routes `/api/auth/*`, `Config{YandexOAuth, CustomerAuthEnabled, SMTP}`, `adminMux` не трогать
- Modify: `internal/config/config.go` — `REVIEWS_YANDEX_OAUTH_CLIENT_ID/SECRET/REDIRECT`, `REVIEWS_CUSTOMER_AUTH_ENABLED`, `REVIEWS_SMTP_*`
- Create: `internal/server/customer_auth_test.go` — 401/200, CORS credentials, paywall 403, OTP flow

**Interfaces:**
- Produces: `GET /api/auth/yandex/start?public_key=&redirect_uri=` → 302; `GET /api/auth/yandex/callback?code=&state=` → Set-Cookie `customer_session=token; HttpOnly; Secure; SameSite=None; Partitioned; Path=/; Max-Age=2592000` + postMessage HTML; `POST /api/auth/email-otp/start|verify` → JSON + cookie; `GET /api/auth/me` / `POST /api/auth/logout`.

- [ ] **Step 1: Config**

Добавить 3 env в `config.LoadFromEnv` + `Config` struct.

- [ ] **Step 2: Handlers**

`yandexStart` проверяет paywall, генерит state via provider, 302. `yandexCallback` — upsert `ReviewerIdentity` (yandex_uid → email → create), `CreateCustomerSession` (30д), `SetCookie` + HTML `postMessage`. `email-otp/*` — JSON, та же сессия.

- [ ] **Step 3: Paywall middleware**

`customerAuthEnabled()` — до tenants: `cfg.CustomerAuthEnabled`; после: `tenant.Plan in (pro,pro+)`. При `false` → 403 на `/api/auth/*`.

- [ ] **Step 4: CORS AllowCredentials**

`cors()` добавить `Access-Control-Allow-Credentials: true` когда `customerAuthEnabled`.

- [ ] **Step 5: Tests + Commit**

`go test ./...` зелёный.

```bash
git add internal/server/customer_auth.go internal/server/server.go internal/config/config.go internal/server/customer_auth_test.go
git commit -m "feat(server): /api/auth/* — Yandex + OTP, cookie SameSite=None, paywall"
```

---

## Phase B — виджет gated (1.5 дня)

### Task B1: Auth state + стена логина

**Files:**
- Modify: `web/reviews-widget/reviews-widget.js` — `state.auth`, `loadAuth()`, `renderSubmission` gated, `submitReview` с credentials
- Modify: `web/reviews-widget/reviews-widget.css` — `.rw-auth-wall`, `.rw-verified`, `.rw-auth-email-form`

**Interfaces:**
- Produces: без сессии — `renderSubmission` показывает `.rw-auth-wall` с кнопками Yandex/Email вместо формы; с сессией — скрывает `authorName/authorEmail`, показывает `Вы вошли как {displayName}` + «Выйти».

- [ ] **Step 1: loadAuth**

`fetch(${apiBase}/api/auth/me, {credentials:'include'})` на mount, `state.auth = {loading, authenticated, provider, displayName, avatarUrl}`.

- [ ] **Step 2: Стена логина**

`renderSubmission` ветка `!auth.authenticated` → `.rw-auth-wall` + 2 кнопки. Yandex: `window.open(.../yandex/start?public_key=..., 'yandex_auth', 'popup')` + `addEventListener('message')`. Email: инлайн `email → code` форма.

- [ ] **Step 3: submitReview gated**

`fetch(submissionUrl, {method:'POST', body:formData, credentials:'include'})`; 401 → показать стену; игнор `authorName/authorEmail` из формы (берутся из сессии на сервере).

- [ ] **Step 4: Стили + node --check**

`web/reviews-widget/reviews-widget.css` — `.rw-auth-wall`, `.rw-verified`.

```bash
node --check web/reviews-widget/reviews-widget.js
```

- [ ] **Step 5: Commit**

```bash
git add web/reviews-widget/reviews-widget.js web/reviews-widget/reviews-widget.css
git commit -m "feat(widget): gated reviews — Yandex ID + email OTP wall, verified badge"
```

---

### Task B2: Submission требует сессии

**Files:**
- Modify: `internal/server/review_submissions.go` — `handleCreateReviewSubmission` требует `CustomerSession` cookie, `requireCustomerAuth` guard, флаг `REVIEWS_REQUIRE_CUSTOMER_AUTH`
- Modify: `internal/store/submissions.go` — `CreateSiteReview` принимает `SessionID` → `Verified=true`
- Create: `internal/server/review_submissions_auth_test.go` — 401 без сессии, 201 с, paywall

**Interfaces:**
- Produces: без валидной `customer_session` → 401 `{error:"требуется вход"}`; с сессией — `AuthorName/Email` из сессии, `Verified=true`, `VerifiedProvider`, `AuthorEmailHash` как раньше.

- [ ] **Step 1: Guard**

`handleCreateReviewSubmission` → `FindCustomerSessionByToken(cookie)`, 401 если нет/expired, `r.WithContext(WithCustomerSession(ctx, session))`.

- [ ] **Step 2: Store**

`SiteReviewInput` + `CustomerSessionID *string`, `Review.Verified=true` при сессии.

- [ ] **Step 3: Флаг отката**

`REVIEWS_REQUIRE_CUSTOMER_AUTH` (default true); при false — legacy анонимный путь за флагом (для self-host).

- [ ] **Step 4: Commit**

```bash
git add internal/server/review_submissions.go internal/store/submissions.go internal/server/review_submissions_auth_test.go
git commit -m "feat(server): review submissions require customer session, verified flag"
```

---

## Phase C — доверие + админка (1 день)

### Task C1: Бейдж верификации в виджете

**Files:**
- Modify: `web/reviews-widget/reviews-widget.js` — `renderList` бейдж `Проверенный покупатель` / `Через Яндекс ID`
- Modify: `internal/reviewjson/reviewjson.go` — `Review.Verified` в public JSON

**Interfaces:**
- Produces: `review.verified` bool в `/api/reviews` → виджет рендерит `.rw-verified` рядом с author.

- [ ] **Step 1: JSON**

`reviewjson.Mapper.ToReview` добавить `Verified: rv.Verified`.

- [ ] **Step 2: Render**

`renderList` — если `review.verified` → `<span class="rw-verified">Проверенный покупатель</span>` (или `Через Яндекс ID` по `verifiedProvider`).

- [ ] **Step 3: Commit**

```bash
git add internal/reviewjson/reviewjson.go web/reviews-widget/reviews-widget.js
git commit -m "feat(widget): verified badge for customer-auth reviews"
```

---

### Task C2: Админка — фильтр и бейдж

**Files:**
- Modify: `web/admin/src/pages/Reviews.tsx` — колонка/фильтр `Верифицирован`, бейдж в карточке
- Modify: `web/admin/src/types.ts` — `Review.verified`
- Modify: `internal/server/admin_reviews.go` — `AdminReview.Verified` в JSON, фильтр `?verified=true/false`

**Interfaces:**
- Produces: админка показывает бейдж верификации, фильтрует по `verified`, отдаёт поле в API.

- [ ] **Step 1: API**

`handleAdminReviews` добавить `verified` query param → `store.ListReviews` фильтр.

- [ ] **Step 2: UI**

`Reviews.tsx` — фильтр сегмент `Верифицирован`, бейдж в карточке + tooltip provider.

- [ ] **Step 3: Build + Commit**

```bash
cd web/admin && npm run build && cp -r dist ../internal/server/admin_dist
git add web/admin/ internal/server/admin_dist internal/server/admin_reviews.go web/admin/src/types.ts
git commit -m "feat(admin): verified filter + badge for customer reviews"
```

---

## Phase D — Kit-хинт (0.5 дня, опционально)

### Task D1: Loader hint

**Files:**
- Modify: `web/reviews-widget/loader.js` — читать `requestContext.user` если доступен, подсказка в виджет

**Interfaces:**
- Produces: если `requestContext.user.isRegistered` → виджет показывает «Вы вошли на Ките как ... — войдите через Яндекс ID одним кликом» (только UX, не доверие).

- [ ] **Step 1: Hint**

`loader.js` → `window.REVIEWS_CUSTOMER_HINT = {isRegistered, displayName}`.

- [ ] **Step 2: Commit**

```bash
git add web/reviews-widget/loader.js
git commit -m "feat(widget): Kit session hint for Yandex ID UX"
```

---

## Приёмка спринта

- [ ] Без логина — стена с 2 кнопками, форма скрыта.
- [ ] Yandex popup → postMessage → сессия, Email OTP → сессия (6 цифр, 10м, 3 попытки).
- [ ] С сессией — отзыв уходит 201, `Verified=true`, бейдж в виджете и админке.
- [ ] Без сессии — 401 на `POST /api/review-submissions`.
- [ ] Paywall: `CustomerAuthEnabled=false` → 403 на `/api/auth/*`.
- [ ] `go test ./...` зелёный; `node --check` OK; `npm run build` OK.
- [ ] 152-ФЗ: DSR находит по email и по yandex_uid, экспорт включает Verified.
