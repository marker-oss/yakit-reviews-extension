# Reviews — Фаза 1: Сборщик отзывов + БД (design)

**Дата:** 2026-06-04 · **Статус:** черновик на согласование · **Фаза:** 1 из 4

**Сопроводительные документы:**
[implementation plan](2026-06-04-reviews-collector-implementation-plan.md),
[runbook](2026-06-04-reviews-collector-runbook.md).

> Независимый опенсорс-сервис, который собирает отзывы о товарах селлера с
> маркетплейсов (Wildberries, Яндекс Маркет, Ozon) по официальным API и
> хранит их в собственной БД. Фаза 1 — только **сбор и хранение**. Виджет,
> публичный API, личный кабинет, ответы на отзывы и приём отзывов с сайта —
> отдельные фазы.

---

## 1. Контекст и цель

Селлер (бренд одежды, витрина на Yandex Kit) собирает отзывы на
маркетплейсах, но не может перенести их на свой сайт. Долгосрочная цель —
показывать отзывы с МП в виджете на сайте (через GTM) и принимать новые.

Сервис задуман как **независимый** и пригодный к выкладке в опенсорс для
других селлеров: разворачивается одним артефактом, без зависимости от
приватных проектов автора (PIM, analitica). Решения и паттерны из них
переиспользуются на уровне идей/кода, но не как библиотечная зависимость.

**Цель Фазы 1:** надёжно и идемпотентно забирать отзывы (текст, рейтинг,
автор, даты, медиа-ссылки, существующий ответ продавца) с включённых
маркетплейсов в БД, каждый час, с инкрементальной синхронизацией и
первичным backfill.

## 2. Что подтверждено исследованием API (фундамент)

| Возможность | Wildberries | Ozon | Яндекс Маркет |
|---|---|---|---|
| Чтение отзывов | `GET /api/v1/feedbacks` (+ `/feedbacks/archive`) | `POST /v1/review/list` + `/v1/review/info` | `POST /v2/businesses/{id}/goods-feedback` |
| Платная подписка | ❌ нет | ⚠️ **да — «Управление отзывами»** (Seller API только по подписке) | ❌ нет (нужен бизнес-аккаунт) |
| Медиа | прямые CDN `*.wbbasket.ru` (видео — HLS `.m3u8`) | URL фото/видео | `media.photos[]`, `media.videos[]` — URL |
| Привязка к товару | `nmId` (лучший ключ) / `supplierArticle` | `sku` | `offerId` (SKU продавца) |
| Авторизация | API-токен, scope «Вопросы и отзывы» | `Client-Id` + `Api-Key` | `Api-Key`, scope `communication` |
| Лимиты | ~1 req/sec | cursor `last_id` | ~5000 req/час, cursor `pageToken` |
| Ответ продавца (для Фазы 4) | `POST /api/v1/feedbacks/answer` | `/v1/review/comment/create` | `/goods-feedback/comments/update` |

**Следствие:** медиа храним **ссылками на CDN маркетплейса** (не качаем).
Ozon в Фазе 1 реализован, но **выключен feature-флагом** (требует платной
подписки; есть 7-дневный триал для теста адаптера).

## 3. Объём Фазы 1

**В объёме:**
- Адаптеры WB и YM (рабочие) + Ozon (за флагом, выключен по умолчанию).
- Единый интерфейс адаптера; нормализация ответа МП в общую модель отзыва.
- Схема БД (портируемая SQLite ↔ Postgres) с таблицами отзывов, медиа,
  товаров и связок «на вырост».
- Планировщик: часовой инкрементальный sync + первичный backfill.
- Идемпотентный upsert по `(marketplace, external_review_id)`.
- Захват уже существующего ответа продавца на МП (read-only).
- Конфиг (env/файл): выбор БД, включённые МП, ключи, интервал, глубина
  backfill.
- Структурное логирование и запись результатов прогонов (`sync_runs`).

**Вне объёма (другие фазы):**
- Виджет, публичный API, личный кабинет/админка (Фазы 2–4).
- Привязка отзывов к карточкам **сайта** и автосопоставление (Фаза 2).
- Отправка ответов на отзывы и приём отзывов с сайта (Фазы 3–4).
- Кэширование медиа в S3 (опционально, будущее).
- Управление ключами и подключение магазинов из UI (Фаза 4).

## 4. Архитектура

```
                    ┌──────────────────────────────────────┐
                    │           reviews (Go binary)         │
   каждый час       │                                       │
   ──────────────▶  │  scheduler ──▶ collector ──▶ store    │
                    │                  │            (GORM)   │
                    │            ┌─────┴─────┐         │     │
                    │            │ adapters  │         ▼     │
                    │            │ wb ym ozon│      SQLite /  │
                    │            └─────┬─────┘      Postgres  │
                    └──────────────────┼────────────────────┘
                                       ▼
              WB Feedbacks API · YM goods-feedback · (Ozon, flag)
```

**Модули (Go-пакеты):**
- `config` — загрузка/валидация конфига (env + файл), секреты МП.
- `store` — модели GORM, миграции, upsert-логика; абстракция SQLite/Postgres.
- `marketplace` — интерфейс `Adapter` + общая модель `Review`/`Media`;
  подпакеты `wb`, `ym`, `ozon` (HTTP-клиент, рейт-лимитер, маппинг полей).
- `collector` — оркестрация прогона: для каждого МП тянет инкремент,
  нормализует, резолвит товар, upsert, пишет `sync_run`.
- `scheduler` — тикер/cron, изоляция МП друг от друга, graceful shutdown.
- `cmd/reviews` — точка входа (CLI: `serve`, `sync --once`, `migrate`).

**Принцип изоляции:** падение одного МП не останавливает другие; каждый
адаптер инкапсулирует свой рейт-лимит и формат; `collector` не знает
деталей конкретного МП за пределами интерфейса.

## 5. Интерфейс адаптера

```go
// Нормализованная модель — общая для всех МП.
type Review struct {
    Marketplace       string    // "wb" | "ozon" | "ym"
    ExternalReviewID  string    // id отзыва в МП
    ExternalProductID string    // nmId / sku / offerId (как строка)
    Rating            *int      // 1..5, nil если нет
    AuthorName        string
    Text              string
    Pros              string
    Cons              string
    CreatedAtMP       time.Time  // когда отзыв создан в МП
    UpdatedAtMP       *time.Time // когда отзыв/ответ изменён в МП, если API отдаёт
    Answer            *Answer   // существующий ответ продавца, если есть
    Media             []Media
    Raw               []byte    // сырой payload (debug/forward-compat)
}

type Media struct {
    Kind       string // "photo" | "video"
    URL        string
    PreviewURL string // постер для видео; "" для фото
    Position   int
}

type Answer struct {
    Text  string
    State string
}

type Adapter interface {
    Marketplace() string
    // FetchReviews тянет отзывы, изменённые/созданные начиная с since.
    // cursor — непрозрачный маркер пагинации внутри одного прогона.
    // Возвращает страницу отзывов и nextCursor ("" — конец).
    FetchReviews(ctx context.Context, since time.Time, cursor string) (
        reviews []Review, nextCursor string, err error)
}
```

Phase 4 расширит интерфейс методом `PostAnswer(ctx, externalReviewID, text)`.

**Особенности маппинга по МП:**
- **WB:** объединять `GET /feedbacks?isAnswered=true|false` **и**
  `GET /feedbacks/archive` (отзыв уходит в архив после ответа, через 30
  дней без ответа, или если без текста/фото). Видео — HLS `.m3u8`
  (`PreviewURL` = постер). Поля: `text`, `pros`, `cons`,
  `productValuation`→Rating, `userName`, `createdDate`,
  `productDetails.nmId`/`supplierArticle`, `answer`.
- **YM:** `POST /v2/businesses/{businessId}/goods-feedback`, фильтр
  `dateTimeFrom`, пагинация `nextPageToken`. Поля: `identifiers.offerId`,
  `description.{advantages,disadvantages,comment}`, `statistics.rating`,
  `author`, `createdAt`, `media.{photos,videos}`.
- **Ozon (flag):** `POST /v1/review/list` (+ `/v1/review/info` за деталями,
  `/v1/review/comment/list` за ответами), курсор `last_id`. Только при
  активной подписке.

## 6. Модель данных (портируемая SQLite ↔ Postgres)

Ограничения переносимости: **без Postgres-массивов и jsonb-операторов**;
`raw` — `TEXT` (JSON-строка); времена — через GORM (`DATETIME`/`timestamptz`);
связь «товар↔МП» — отдельной таблицей, а не массивом.

```
products
  id              PK
  title           text  null      -- человекочитаемое имя (опц.)
  site_product_key text null      -- ключ карточки на сайте (заполняется в Фазе 2)
  created_at, updated_at

product_marketplace_links
  id              PK
  product_id      FK products
  marketplace     text            -- wb | ozon | ym
  external_product_id text         -- nmId / sku / offerId
  UNIQUE(marketplace, external_product_id)

reviews
  id                 PK
  marketplace        text
  external_review_id text
  UNIQUE(marketplace, external_review_id)   -- идемпотентность
  external_product_id text                  -- ключ товара в МП (всегда есть)
  product_id         FK products null       -- резолвится через links; null = orphan
  rating             int null
  author_name        text
  text               text
  pros               text
  cons               text
  created_at_mp      timestamp
  updated_at_mp      timestamp null
  mp_answer_text     text null              -- существующий ответ продавца на МП
  mp_answer_state    text null
  status             text  default 'imported' -- задел под модерацию (Фаза 4)
  raw                text                    -- сырой payload
  fetched_at, updated_at

review_media
  id          PK
  review_id   FK reviews
  kind        text          -- photo | video
  url         text
  preview_url text null
  position    int
  UNIQUE(review_id, url)

sync_state                         -- состояние инкремента per-MP
  marketplace     PK
  last_synced_at  timestamp null
  backfilled      bool default false

sync_runs                          -- журнал прогонов (наблюдаемость)
  id            PK
  marketplace   text
  started_at, finished_at
  status        text          -- running | ok | error
  reviews_seen     int
  reviews_upserted int
  error_text    text null
```

**Резолв товара:** при upsert отзыва ищем
`product_marketplace_links(marketplace, external_product_id)`; если найдено —
проставляем `reviews.product_id`. Если нет — отзыв сохраняется как **orphan**
(`product_id = null`). Когда селлер заведёт связь (Фаза 2), фоновая
до-привязка проставит `product_id` у накопленных orphan-отзывов. Сбор
**никогда не падает** из-за отсутствия маппинга.

## 7. Поведение синхронизации

- **Планировщик:** тикер с интервалом `sync.interval` (по умолчанию `1h`).
  Также CLI `sync --once` для ручного/cron-запуска.
- **Инкремент:** для каждого МП тянем отзывы с `since = last_synced_at`
  минус небольшой overlap (напр. 1 ч) для надёжности; пагинируем по курсору.
- **Первичный backfill:** если `sync_state.backfilled = false`, `since = now -
  backfill.months` (по умолчанию **6–12 мес**, параметр конфига; «вся
  история» — отдельным значением). По завершении — `backfilled = true`.
- **Идемпотентность:** upsert по `(marketplace, external_review_id)`. При
  повторной встрече обновляем изменяемые поля: `text`, `rating`, `pros`,
  `cons`, `mp_answer_*`, медиа. (Отзыв мог получить ответ продавца или
  быть отредактирован.)
- **Рейт-лимиты:** per-adapter limiter (WB ~1 rps; YM ~5000/час; Ozon —
  default). Backoff на 429/блокировки.
- **Изоляция и устойчивость:** горутина на МП; транзиентные HTTP-ошибки —
  ретрай с экспоненциальным backoff; неустранимая ошибка одного МП пишется
  в `sync_runs` и не валит остальные.

### 7.1 Инварианты sync

- **Cursor не является долговременным состоянием.** Cursor/page token живёт
  только внутри одного прогона и не используется как источник истины после
  рестарта. Между прогонами сохраняем только `last_synced_at` и `backfilled`.
  Исключение возможно только если конкретный API требует durable cursor; тогда
  это фиксируется в адаптере и покрывается тестом восстановления.
- **Watermark двигается только после успешного прогона конкретного МП.**
  Если МП упал на любой странице после ретраев, `last_synced_at` не меняется,
  `sync_runs.status = error`, следующий прогон повторяет окно с overlap.
- **Новое значение `last_synced_at` — не просто `now`.** Для успешного прогона
  сохраняем безопасный watermark: максимум обработанных `UpdatedAtMP`, если он
  есть, иначе максимум `CreatedAtMP`; верхняя граница не должна быть позже
  `run.started_at`. Если страница пуста, можно сохранить `run.started_at`.
- **Overlap обязателен для всех инкрементов.** Повторно увиденные отзывы
  безвредны из-за upsert. Overlap должен быть конфигурируемым и по умолчанию
  достаточно большим, чтобы пережить задержки индексации МП и расхождение часов.
- **Медиа синхронизируются как snapshot.** При upsert отзыва набор
  `review_media` для этого отзыва приводится к текущему ответу МП в одной
  транзакции: новые URL добавляются, изменившиеся `kind`/`preview_url`/`position`
  обновляются, исчезнувшие URL удаляются.
- **Ответ продавца обновляется при повторной встрече отзыва.** Если API не
  отдаёт `UpdatedAtMP` и фильтр работает только по дате создания, адаптер обязан
  иметь стратегию refresh для старых отзывов: например WB читает archive, Ozon
  добирает детали через `review/info`, YM переобходит конфигурируемое окно.
  Низколатентное обновление ответов вне возможностей API не обещаем.
- **WB active/archive дедуплицируются до записи.** Адаптер WB объединяет
  answered, unanswered и archive в один поток нормализованных отзывов; если
  один `external_review_id` встретился дважды, выбирается версия с более полным
  payload/ответом.
- **Частичный успех не смешивается.** Успех или ошибка считаются отдельно для
  каждого marketplace. `sync --once` возвращает ненулевой exit code, если хотя
  бы один включённый МП завершился ошибкой, но успешные МП сохраняют свой
  watermark.

### 7.2 CLI-поведение

- `reviews migrate` применяет GORM-миграции и завершает процесс.
- `reviews sync --once` запускает один прогон по всем включённым МП.
- `reviews sync --once --marketplace wb` запускает один МП; полезно для
  отладки и ручного восстановления.
- `reviews serve` запускает планировщик и корректно завершает активные прогоны
  по `SIGINT`/`SIGTERM`.
- CLI не печатает секреты и не пишет API-ключи в `sync_runs.error_text`.

## 8. Конфигурация (env + файл)

```
DB:        driver (sqlite|postgres) + dsn/path
sync:      interval (1h), backfill_months (12), overlap (1h)
log:       level, format
marketplaces:
  wb:   enabled=true,  token
  ym:   enabled=true,  api_key, business_id
  ozon: enabled=false, client_id, api_key      # flag, требует подписки
```

Ключи — только из env/файла (не в БД, не в логах). Управление ключами из
UI — Фаза 4.

## 9. Обработка ошибок и наблюдаемость

- Структурные логи (по образцу mulog/slog): по прогону и по МП — счётчики
  `seen`/`upserted`, длительность, ошибки.
- `sync_runs` — персистентный журнал (видно историю прогонов и сбои).
- Чёткое разделение транзиентных (ретраи) и фатальных (стоп данного МП)
  ошибок.

## 10. Стратегия тестирования

- **Адаптеры:** unit-тесты нормализации на **зафиксированных фикстурах**
  (примеры реальных ответов WB/YM/Ozon) → проверяем маппинг в `Review`.
- **Пагинация/границы окна:** фикстуры с несколькими страницами, пустой
  страницей, дублем на overlap и ошибкой после частично обработанных страниц.
- **Идемпотентность/upsert:** против in-memory SQLite — повторный прогон не
  плодит дубли, обновляет ответ продавца, не теряет медиа.
- **Snapshot медиа:** исчезнувшее медиа удаляется, изменившаяся позиция или
  `preview_url` обновляется, повторный прогон остаётся идемпотентным.
- **Резолв товара:** orphan → до-привязка при появлении link.
- **Dual-DB:** прогон миграций и базовых сценариев и на SQLite, и на Postgres
  (CI-матрица).
- **Рейт-лимитер:** не превышает заданный rps.

## 11. Технологический стек

- **Go** (single static binary; ассеты будущих фаз — через `embed`).
- **GORM** — модели + миграции, одна схема → SQLite (`modernc.org/sqlite`,
  pure-Go) и Postgres (`pgx`).
- HTTP — стандартная библиотека + per-host rate limiter
  (`golang.org/x/time/rate`).
- Логирование — `log/slog`.
- Конфиг — env + файл (напр. `koanf`/`viper`).
- Виджет (будущие фазы) — **Preact/vanilla**, Shadow DOM, лёгкий bundle.

## 12. Открытые вопросы (на будущие фазы, не блокируют Фазу 1)

- Как виджет на сайте определяет текущий товар (URL / data-атрибут / id в
  DOM Yandex Kit) и как селлер заполняет `site_product_key` ↔ МП-ID
  (Фаза 2).
- Нужен ли отдельный механизм авто-сопоставления товаров по
  артикулу продавца (если у селлера единый vendorCode) vs ручная связка.
- Авторизация клиента для отзывов с сайта — Яндекс ID или иное (Фаза 3).
- Кэшировать ли медиа в S3 (риск «протухания» CDN-ссылок МП) — измерить
  после Фазы 1.
- Опциональная интеграция с PIM как источником маппинга — после стабилизации
  PIM.
