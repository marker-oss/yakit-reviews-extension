# Reviews — Фаза 1: план реализации

**Дата:** 2026-06-04 · **Статус:** рабочий план · **Связанный дизайн:**
[reviews-collector-design](2026-06-04-reviews-collector-design.md)

**Текущий прогресс:** начат skeleton Phase 1: Go module, CLI entrypoint,
env-config, GORM store, SQLite migrations, store-тесты, WB adapter и реальный
WB e2e sync в SQLite.

Этот документ переводит дизайн Фазы 1 в порядок разработки. Он не заменяет
design spec: здесь фиксируется последовательность работ, контрольные точки и
критерии готовности.

## 1. Цель первой реализации

Получить один Go-binary `reviews`, который умеет:
- применять миграции;
- читать конфиг;
- запускать один синк вручную;
- запускать планировщик;
- собирать отзывы минимум из WB и YM;
- сохранять отзывы, медиа, ответы продавца и историю прогонов в SQLite;
- иметь переносимую схему и тесты, достаточные для будущего Postgres.

## 2. Предлагаемая структура

```text
cmd/reviews/
  main.go
internal/config/
internal/store/
internal/marketplace/
internal/marketplace/wb/
internal/marketplace/ym/
internal/marketplace/ozon/
internal/collector/
internal/scheduler/
testdata/marketplace/
```

Публичных Go-пакетов на старте не нужно: сервис пока не является библиотекой.
Если позже появится public API или SDK, границу можно вынести отдельно.

## 3. Порядок разработки

### 3.1 Project skeleton

Status: started.

- Инициализировать Go module.
- Добавить `cmd/reviews`.
- Подключить CLI-команды `migrate`, `sync --once`, `serve`.
- Добавить базовый `slog` logger.
- Добавить минимальный config loader.

Acceptance:
- `go test ./...` проходит;
- `reviews --help` показывает команды;
- сервис стартует без marketplace keys только в командах, где они не нужны.

### 3.2 Store layer

Status: started.

- Описать GORM-модели: `products`, `product_marketplace_links`, `reviews`,
  `review_media`, `sync_state`, `sync_runs`.
- Реализовать `Migrate`.
- Реализовать upsert отзыва по `(marketplace, external_review_id)`.
- Реализовать snapshot-обновление медиа в транзакции.
- Реализовать чтение/запись `sync_state`.
- Реализовать запись `sync_runs`.

Acceptance:
- повторный upsert не создаёт дубли;
- изменённый seller answer обновляется;
- исчезнувшие media удаляются;
- orphan review сохраняется без `product_id`;
- review получает `product_id`, если link уже существует.

### 3.3 Marketplace contract

- Зафиксировать общую модель `Review`, `Media`, `Answer`.
- Зафиксировать интерфейс `Adapter`.
- Добавить ошибки/типы для transient vs fatal cases.
- Добавить shared HTTP helper: auth headers, request id, retries, rate limit.

Acceptance:
- unit-тесты контрактов проходят без реальных API;
- адаптеры не зависят от `store`;
- collector не знает деталей конкретных API.

### 3.4 WB adapter

Status: started; first real API fetch + SQLite save verified.

- Реализовать чтение answered/unanswered feedbacks.
- Реализовать чтение archive.
- Дедуплицировать active/archive до передачи в collector.
- Нормализовать rating, text/pros/cons, author, product id, answer, media.
- Добавить fixtures для активного, отвеченного, архивного и media-отзыва.

Acceptance:
- fixture-тесты покрывают нормализацию;
- archive не создаёт дубль active-отзыва;
- видео получает `Kind=video`, `URL`, `PreviewURL`, `Position`.

### 3.5 Yandex Market adapter

- Реализовать `goods-feedback`.
- Поддержать `dateTimeFrom` и `nextPageToken`.
- Нормализовать `offerId`, rating, author, description, media.
- Добавить fixtures для нескольких страниц и пустого результата.

Acceptance:
- pagination-тест проходит;
- дубли на границе overlap не ломают upsert;
- отсутствие media/answer корректно маппится в пустые значения.

### 3.6 Ozon adapter behind flag

- Реализовать структуру адаптера и config validation.
- Поддержать `review/list`, `review/info`, `comment/list`.
- По умолчанию не включать адаптер.
- Если нет ключей или подписки, ошибка должна быть понятной и без секретов.

Acceptance:
- при `enabled=false` Ozon не инициализируется;
- при `enabled=true` без обязательных ключей конфиг невалиден;
- fixture-тесты проходят без доступа к реальному Ozon.

### 3.7 Collector and scheduler

- Реализовать прогон одного marketplace.
- Реализовать прогон всех включённых marketplace с изоляцией ошибок.
- Реализовать conservative watermark из design spec.
- Реализовать overlap.
- Реализовать graceful shutdown.

Acceptance:
- ошибка одного adapter не останавливает другие;
- `last_synced_at` двигается только после успешного прогона конкретного МП;
- `sync --once` возвращает ненулевой exit code при любой ошибке включённого МП;
- `serve` завершает активный прогон по сигналу.

### 3.8 Dual-DB readiness

- Основные тесты store проходят на SQLite.
- Postgres добавить в CI-матрицу, когда появится CI.
- Не использовать Postgres-only типы и операторы.

Acceptance:
- миграции не используют массивы/jsonb-операторы;
- timestamps и nullable поля работают одинаково на SQLite и Postgres.

## 4. Definition of Done для Фазы 1

Фаза 1 считается готовой, когда:
- WB и YM собирают отзывы через официальные API;
- Ozon adapter существует, но выключен по умолчанию;
- primary backfill и hourly incremental sync работают идемпотентно;
- `sync_runs` показывает историю успехов и ошибок;
- retry/backoff работает для transient HTTP failures;
- секреты не попадают в логи и БД;
- есть fixture-тесты адаптеров и store-тесты upsert/idempotency;
- README и runbook соответствуют фактическому CLI/config.

## 5. Что не делать в этой фазе

- Не строить widget/public API/admin UI.
- Не вводить multi-tenant модель ключей в БД.
- Не кэшировать media в S3.
- Не делать авто-сопоставление товаров с сайтом.
- Не добавлять зависимость от private PIM/analitica packages.
