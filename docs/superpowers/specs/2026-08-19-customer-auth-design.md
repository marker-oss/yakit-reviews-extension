# Авторизация покупателей в виджете — дизайн

Дата: 2026-08-19. Статус: утверждён к планированию.

## 1. Контекст и решения

Виджет сейчас принимает отзывы анонимно (`authorName + authorEmail` без проверки).
Проблема: спам, отсутствие доверия, нет связи «отзыв ← покупатель».

Зафиксированные решения сессии 2026-08-19:

- **Логин обязателен для отзывов.** Без сессии — стена логина, а не форма. Анонимный путь депрекейтим (`allowAnonymousReviews=false` по умолчанию; флаг для отката). Комментарии/Q&A — вне скоупа (только отзывы).
- **Провайдеры:** Yandex ID — приоритет (как на Ките, popup OAuth), Email OTP — fallback (если нет Яндекс ID). Абстракция `CustomerAuthProvider` → легко добавить VK/другой.
- **Скоуп:** только отзывы; self-host пока не трогаем — фича под подпиской (SaaS). После появления `tenants.plan` — paywall `customerAuthEnabled` (pro/pro+), trial/base — off с фолбэком «Доступно в Pro».
- **Доверие:** верифицированный отзыв получает бейдж «Проверенный покупатель» / «Через Яндекс ID» в виджете и админке.
- **152-ФЗ:** храним `EmailHash` + `YandexUID` (hash-опционально), `VerifiedAt`; DSR попадает в существующий `SubjectExport`.

Не-цели: обязательная привязка к заказу Kit (`GetOrders` верификация покупки) — отложено; соцсети кроме Yandex ID — позже.

## 2. Модель данных

Расширение существующего `ReviewerIdentity` + новая таблица сессий.

### 2.1 ReviewerIdentity (расширение)

```go
// internal/store/models.go — добавить поля:
YandexUID   *string `gorm:"size:64;uniqueIndex:idx_tenant_yandex_uid,where:yandex_uid IS NOT NULL"`
Provider    string  `gorm:"size:16;not null;default:'email_otp'"` // yandex | email_otp | legacy
DisplayName string  `gorm:"size:128"`
AvatarURL   string  `gorm:"size:512"`
```

Существующие `VerifiedAt *time.Time` и `ShopCustomerID` переиспользуются: `ShopCustomerID = YandexUID` для Yandex, `VerifiedAt = now()` при успешном OAuth/OTP. `EmailNormalized` остаётся ключом для OTP; для Yandex — `default_email` из `/info` (может быть пустым → identity без email, поиск по `YandexUID`).

Индекс: `(tenant_id, yandex_uid) unique where not null`, `(tenant_id, email_normalized) unique` уже есть.

### 2.2 CustomerSession

```go
type CustomerSession struct {
    Token       string    `gorm:"primaryKey;size:64"` // 32 bytes hex, crypto/rand
    TenantID    uint      `gorm:"not null;index"`
    IdentityID  uint      `gorm:"not null;index"`
    Identity    ReviewerIdentity `gorm:"constraint:OnDelete:CASCADE"`
    Provider    string    `gorm:"size:16;not null"` // yandex | email_otp
    YandexUID   *string   `gorm:"size:64;index"`
    Email       string    `gorm:"size:320;index"` // denormalized for quick lookup
    DisplayName string    `gorm:"size:128"`
    AvatarURL   string    `gorm:"size:512"`
    ExpiresAt   time.Time `gorm:"not null;index"`
    CreatedAt   time.Time
}
```

TTL 30 дней, скользящее продление при `GET /api/auth/me` (опционально). Джоб чистит `ExpiresAt < now()` (как `Session`).

### 2.3 Review — верификация

```go
// internal/store/models.go — добавить в Review:
Verified          bool       `gorm:"not null;default:false;index"`
VerifiedProvider  *string    `gorm:"size:16"` // yandex | email_otp
CustomerSessionID *string    `gorm:"size:64;index"`
```

При `CreateSiteReview` с сессией: `Verified = true`, `VerifiedProvider = session.Provider`, `AuthorName/Email` берутся из сессии (игнор полей формы), `AuthorEmailHash` как раньше. Без сессии — `401` (когда флаг `requireCustomerAuth=true`).

### 2.4 Email OTP коды (эфемерно)

```go
type EmailOTPCode struct {
    ID             uint      `gorm:"primaryKey"`
    TenantID       uint      `gorm:"not null;index"`
    EmailNormalized string   `gorm:"size:320;not null;index"`
    CodeHash       string    `gorm:"size:64;not null"` // sha256(code)
    Attempts       int       `gorm:"not null;default:0"`
    ExpiresAt      time.Time `gorm:"not null;index"`
    CreatedAt      time.Time
}
```

TTL 10 мин, 3 попытки, rate-limit 3 старта/час на email+IP.

### 2.5 OAuth state (эфемерно)

```go
type YandexOAuthState struct {
    State        string    `gorm:"primaryKey;size:64"`
    TenantID     uint      `gorm:"not null;index"`
    PKCEVerifier string    `gorm:"size:128;not null"`
    RedirectURI  string    `gorm:"size:512"`
    CreatedAt    time.Time `gorm:"not null;index"`
}
```

TTL 10 мин, чистится джобом или при callback.

## 3. Провайдеры — абстракция

```go
// internal/customer_auth/provider.go
type Provider interface {
    Name() string // "yandex" | "email_otp"
    Start(ctx context.Context, tenantID uint, r *http.Request) (redirectURL string, err error)
    Callback(ctx context.Context, r *http.Request) (*Identity, error)
}

type Identity struct {
    Provider    string
    YandexUID   *string
    Email       string // normalized, may be ""
    DisplayName string
    AvatarURL   string
}
```

- `yandex`: `Start` — генерит `state`+`PKCE`, пишет `YandexOAuthState`, 302 на `https://oauth.yandex.ru/authorize?response_type=code&client_id=&state=&code_challenge=&scope=login:email login:info`.
- `Callback` — валидирует `state`, `POST https://oauth.yandex.ru/token` (client_id/secret), `GET https://login.yandex.ru/info?format=json` (Bearer), парсит `id`, `default_email`, `display_name || real_name`, `default_avatar_id` → `https://avatars.yandex.net/get-yapic/{id}/islands-200`.
- `email_otp`: `Start` — генерит 6-значный код, `POST /api/auth/email-otp/start` (не OAuth redirect, а JSON). `Callback` — `POST /api/auth/email-otp/verify`.

Upsert identity: `Find by (tenant_id, yandex_uid)` → если есть, обновить; иначе `Find by (tenant_id, email_normalized)` → привязать `YandexUID`; иначе создать.

## 4. HTTP API

Все под `public` mux (CORS с `AllowCredentials`), кроме админских.

| Метод | Путь | Auth | Описание |
|---|---|---|---|
| GET | `/api/auth/yandex/start?public_key=&redirect_uri=` | — | Создаёт state, 302 на Yandex |
| GET | `/api/auth/yandex/callback?code=&state=` | — | Обмен, upsert, `Set-Cookie: customer_session=...; HttpOnly; Secure; SameSite=None; Partitioned; Path=/; Max-Age=2592000`, 302 на `redirect_uri` или `postMessage` HTML |
| POST | `/api/auth/email-otp/start` | — | `{email, public_key}` → 200, rate-limit 429 |
| POST | `/api/auth/email-otp/verify` | — | `{email, code, public_key}` → 200 + cookie как выше |
| GET | `/api/auth/me` | cookie | `{authenticated, provider, displayName, avatarUrl, email}` или 401 |
| POST | `/api/auth/logout` | cookie | Чистит сессию, `Set-Cookie: customer_session=; Max-Age=0` |
| POST | `/api/review-submissions` | **customer cookie required** | Берёт identity из сессии, игнор `authorName/authorEmail` из формы; 401 без сессии |

**CORS:** `AllowCredentials: true`, `AllowedOrigins` per-tenant (как сейчас), `Access-Control-Allow-Credentials: true`. Виджет в Shadow DOM на чужом домене — `SameSite=None; Secure` критично; fallback — JWT в `localStorage` + `Authorization: Bearer` header (виджет шлёт оба).

**Конфиг:** `REVIEWS_YANDEX_OAUTH_CLIENT_ID`, `REVIEWS_YANDEX_OAUTH_SECRET`, `REVIEWS_YANDEX_OAUTH_REDIRECT` (дефолт `https://{SAAS_DOMAIN}/api/auth/yandex/callback`), `REVIEWS_CUSTOMER_AUTH_ENABLED` (bool, до появления `tenants.plan`), `REVIEWS_SMTP_*` для OTP (или `log` в dev).

**Paywall:** middleware `requireCustomerAuth` проверяет `customerAuthEnabled(tenant)` — если `false`, `GET /api/auth/*` → 403 `{error:"Доступно в Pro"}`, `POST /api/review-submissions` без сессии → анонимный legacy только если `allowAnonymousReviews=true` (для self-host отката), иначе 403.

## 5. Виджет (`web/reviews-widget/reviews-widget.js`)

Состояние: `state.auth = {loading, authenticated, provider, displayName, avatarUrl} | null`.

- `loadAuth()` — `fetch(${apiBase}/api/auth/me, {credentials:'include'})` при монтировании; кэш в `localStorage` опционально.
- `renderSubmission`: если `!auth.authenticated` → стена:

```html
<div class="rw-auth-wall">
  <p>Войдите чтобы оставить отзыв</p>
  <button data-role="auth-yandex">Продолжить с Яндекс ID</button>
  <button data-role="auth-email">Войти по email</button>
  <div data-role="auth-email-form" hidden> <!-- email input + code input --></div>
</div>
```

Yandex кнопка: `window.open(/api/auth/yandex/start?public_key=..., '_blank', 'popup')` + `window.addEventListener('message', ...)` — callback отдаёт `postMessage({type:'yandex_auth', token})` HTML.
Email кнопка: инлайн форма — `POST /api/auth/email-otp/start` → поле кода → `POST /api/auth/email-otp/verify`.

После auth: скрыть `authorName/authorEmail` поля, показать `Вы вошли как {displayName}` + аватар + «Выйти», сабмит шлёт `FormData` + `credentials:'include'` (cookie) и игнор email/name.

- `renderList`: если `review.verified` → бейдж `<span class="rw-verified">Проверенный покупатель</span>` или `Через Яндекс ID` (по `verifiedProvider`).
- `loader.js`: хинт `requestContext.user.id` только для UX-подсказки («Вы вошли на Ките как ... — войдите через Яндекс ID одним кликом»), не для доверия.
- `submitReview`: `fetch(state.submissionUrl, {method:'POST', body:formData, credentials:'include'})`; 401 → показать стену.

Стили: `.rw-auth-wall`, `.rw-verified` (зелёный бейдж, как `trust` токен).

## 6. Админка (`web/admin`)

- `Reviews.tsx`: колонка/фильтр `Верифицирован` (boolean), бейдж в карточке отзыва (`Verified` + `VerifiedProvider`), сортировка по `verified` (опционально).
- `Settings.tsx` или `Status.tsx`: блок «Авторизация покупателей» — статус Yandex OAuth (client_id настроен?), тумблер `customerAuthEnabled` (до paywall — env), инструкция по подключению.
- Paywall UI: если `customerAuthEnabled=false` — баннер «Доступно в тарифе Pro» + ссылка на биллинг (после появления `tenants.plan`).

## 7. Подписка / paywall

- До `tenants` таблицы: `REVIEWS_CUSTOMER_AUTH_ENABLED=true` для SaaS, `false` для self-host (фича под подпиской сразу).
- После Этапа 1 подписки: `tenants.plan` → `customerAuthEnabled = plan in (pro, pro+)`; trial/base → 403 на auth эндпоинты, виджет показывает фолбэк.
- Биллинг не меняется; фича — аргумент апгрейда.

## 8. Безопасность и ПД (152-ФЗ)

- OAuth: `state` random 32 bytes hex, `PKCE S256`, TTL 10м, one-time use, `redirect_uri` валидируется против allowlist (SAAS_DOMAIN).
- Cookie: `HttpOnly; Secure; SameSite=None; Partitioned; Path=/` (CHIPS для 3PC), `Max-Age=30d`. CSRF не нужен для auth cookie (не меняет админ-данные), но `POST /api/review-submissions` проверяет `Origin` против `AllowedOrigins`.
- Rate-limit: `email-otp/start` — 3/час per email+IP, `yandex/start` — 10/мин per IP, `verify` — 5 попыток на код.
- ПД: `EmailHash` как сейчас, `YandexUID` — хранить как есть (ID Паспорта, не ПД прямого), `DisplayName/AvatarURL` — кэш, удаляется по DSR. DSR `FindSubjectByEmail` и новый `FindSubjectByYandexUID` → `PurgeSubjectByYandexUID`.
- Логи: не писать `code`, `token`, `email` в plaintext; только `tenant_id`, `provider`, `yandex_uid` hash.

## 9. Тестирование

- Store: `customer_auth_test.go` — upsert по yandex_uid, по email, привязка, TTL чистка, verified флаг.
- Server: `customer_auth_test.go` — `TestYandexCallback` (httptest mock oauth/token/info), `TestEmailOTPFlow`, `TestReviewSubmissionRequiresAuth` (401 без сессии, 201 с), `TestCORSWithCredentials`, 152-ФЗ DSR с yandex_uid.
- Widget: `node --check`, ручная приёмка popup/postMessage, стена логина, бейдж.
- Админка: `npm run build` + ручная проверка фильтра/бейджа.

## 10. Следующие фазы (вне скоупа)

- Верификация покупки Kit (`GetOrders` по email/yandex_uid) → бейдж «Покупка подтверждена».
- Комментарии/Q&A под той же сессией.
- VK ID провайдер (тот же интерфейс).
