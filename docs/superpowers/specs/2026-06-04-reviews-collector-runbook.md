# Reviews — runbook Фазы 1

**Дата:** 2026-06-04 · **Статус:** черновик эксплуатации · **Связанный дизайн:**
[reviews-collector-design](2026-06-04-reviews-collector-design.md)

Runbook описывает, как сервис должен настраиваться, запускаться и
диагностироваться. Реализация пока частичная: `migrate` уже работает, `sync`
и `serve` имеют CLI-каркас, но collector/scheduler ещё не реализованы.

## 1. Режимы запуска

```sh
reviews migrate
reviews sync --once
reviews sync --once --marketplace wb
reviews sync --once --marketplace ym
reviews serve --addr 127.0.0.1:8080
reviews discover-site-urls --out data/shegida-product-links.json
```

- `migrate` применяет миграции и завершает процесс.
- `sync --once` запускает один сбор и завершает процесс.
- `sync --once --marketplace <id>` запускает только один marketplace.
- `serve` запускает локальный HTTP-сервер: `/api/reviews` и статический
  прототип виджета из `web/reviews-widget`.
- `discover-site-urls` обновляет JSON-карту прямых ссылок на карточки товаров
  в магазине продавца по sitemap shegida.ru.

Exit code:
- `0` — команда завершилась успешно;
- `1` — хотя бы один включённый marketplace завершился ошибкой;
- `2` — невалидный конфиг или аргументы CLI.

## 2. Конфигурация

Сервис читает настройки из файла и env. Env имеет приоритет над файлом.
Секреты можно хранить в env или локальном config-файле, но они не должны
попадать в БД, логи, `sync_runs.error_text` или fixture files.

На старте сервис также читает локальный `.env` из текущей директории. Значения,
которые уже выставлены в окружении процесса, имеют приоритет над `.env`.

Пример YAML:

```yaml
db:
  driver: sqlite
  dsn: ./reviews.db

sync:
  interval: 1h
  backfill_months: 12
  overlap: 1h

log:
  level: info
  format: text

marketplaces:
  wb:
    enabled: true
    token: ${REVIEWS_WB_TOKEN}
  ym:
    enabled: true
    api_key: ${REVIEWS_YM_API_KEY}
    business_id: ${REVIEWS_YM_BUSINESS_ID}
  ozon:
    enabled: false
    client_id: ${REVIEWS_OZON_CLIENT_ID}
    api_key: ${REVIEWS_OZON_API_KEY}
```

Рекомендуемые env-переменные:

```sh
REVIEWS_CONFIG
REVIEWS_DB_DRIVER
REVIEWS_DB_DSN
REVIEWS_SYNC_INTERVAL
REVIEWS_SYNC_BACKFILL_MONTHS
REVIEWS_SYNC_OVERLAP
REVIEWS_LOG_LEVEL
REVIEWS_LOG_FORMAT
REVIEWS_SITE_PRODUCT_URL_TEMPLATE
REVIEWS_SITE_PRODUCT_LINKS
REVIEWS_WB_ENABLED
REVIEWS_WB_TOKEN
WB_API_TOKEN
REVIEWS_YM_ENABLED
REVIEWS_YM_API_KEY
REVIEWS_YM_OAUTH_TOKEN
REVIEWS_YM_BUSINESS_ID
REVIEWS_OZON_ENABLED
REVIEWS_OZON_CLIENT_ID
REVIEWS_OZON_API_KEY
YM_API_KEY
YM_OAUTH_TOKEN
YM_BUSINESS_ID
YM_CAMPAIGN_ID
OZON_CLIENT_ID
OZON_API_KEY
```

## 3. Marketplace prerequisites

Wildberries:
- API token with scope for questions/reviews;
- expected rate limit around 1 request/sec;
- adapter must read answered, unanswered and archive feedbacks.

Yandex Market:
- business account;
- API key with `communication` scope;
- configured `business_id`;
- adapter uses `goods-feedback` and `nextPageToken`.

Ozon:
- `Client-Id` and `Api-Key`;
- paid review-management subscription;
- disabled by default until explicitly enabled.

## 4. Sync state

The durable sync state per marketplace is:
- `last_synced_at`;
- `backfilled`.

Cursor/page token is runtime-only unless a specific API forces durable cursor
storage. A failed marketplace run must not advance `last_synced_at`.

Safe operational expectations:
- repeated reviews are normal because overlap is intentional;
- missed reviews are not acceptable;
- a marketplace can fail while others succeed;
- successful marketplaces keep their own watermark even when `sync --once`
  exits non-zero due to another marketplace.

## 5. Observability

Each run writes one `sync_runs` row per marketplace:
- `marketplace`;
- `started_at`;
- `finished_at`;
- `status`;
- `reviews_seen`;
- `reviews_upserted`;
- `error_text`.

Useful SQLite checks:

```sql
select marketplace, status, started_at, finished_at, reviews_seen, reviews_upserted
from sync_runs
order by started_at desc
limit 20;

select marketplace, last_synced_at, backfilled
from sync_state
order by marketplace;

select marketplace, count(*) as reviews
from reviews
group by marketplace;

select marketplace, count(*) as orphan_reviews
from reviews
where product_id is null
group by marketplace;
```

## 6. Failure handling

HTTP 429 or temporary network errors:
- adapter retries with exponential backoff;
- if retries are exhausted, marketplace run becomes `error`;
- `last_synced_at` is not advanced.

Auth errors:
- run becomes `error`;
- logs should identify marketplace and auth class, not the token value;
- operator should rotate/fix credentials and rerun `sync --once --marketplace`.

Mapping errors:
- missing product mapping is not an error;
- review is stored as orphan with `product_id = null`;
- later phases can attach orphan reviews after product links are added.

Seller site links:
- `serve` loads `REVIEWS_SITE_PRODUCT_LINKS`, default
  `data/shegida-product-links.json`;
- for WB seller articles with color suffixes, such as `3467/Белый`, the server
  tries the exact article and then the base article `3467`;
- when no exact storefront product is known, the API returns the configured
  `REVIEWS_SITE_PRODUCT_URL_TEMPLATE` fallback.

Schema errors:
- stop the command;
- do not attempt sync before successful `migrate`.

## 7. Backup notes

For SQLite deployments:
- stop `serve` before copying the DB, or use SQLite online backup tooling;
- keep backups outside the repository;
- do not commit real DB files.

For Postgres deployments:
- use normal `pg_dump`/managed database backups;
- verify restore against migration tests before relying on it.

## 8. Release checklist

- `go test ./...` passes.
- `reviews migrate` succeeds on empty SQLite.
- `reviews sync --once --marketplace wb` succeeds with real WB credentials.
- `reviews sync --once --marketplace ym` succeeds with real YM credentials.
- `reviews discover-site-urls --out data/shegida-product-links.json` refreshes
  seller-store product links when the storefront catalog changes.
- Ozon remains disabled unless explicitly configured.
- Logs contain no API keys.
- `sync_runs` has one row per enabled marketplace.
- Re-running sync does not duplicate reviews or media.
