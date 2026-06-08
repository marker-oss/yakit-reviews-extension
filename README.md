# Reviews

Reviews is an independent open-source service for collecting seller product
reviews from marketplaces and storing them in a local database.

Phase 1 focuses only on ingestion and persistence: Wildberries and Yandex
Market are enabled targets, Ozon is implemented behind a feature flag because
review API access requires a paid seller subscription.

## Current Status

Implementation has started on the Phase 1 collector foundation. The current
code includes:
- Go module and CLI entrypoint;
- config loading from env and local `.env`;
- GORM store models and migrations;
- SQLite-backed store tests for review upsert, media snapshotting, product link
  resolution, and sync state;
- WB adapter and collector path verified against the real WB API.
- static frontend prototype for rendering imported reviews.
- SHEGIDA product URL discovery from `https://shegida.ru/sitemap.xml` for
  exact seller-store links where the storefront publishes matching SKUs.

Primary spec:
- [Phase 1 design](docs/superpowers/specs/2026-06-04-reviews-collector-design.md)

Companion docs:
- [Implementation plan](docs/superpowers/specs/2026-06-04-reviews-collector-implementation-plan.md)
- [Operations runbook](docs/superpowers/specs/2026-06-04-reviews-collector-runbook.md)
- [Widget rendering prototype](docs/superpowers/specs/2026-06-04-reviews-widget-rendering.md)
- [Site embedding design](docs/superpowers/specs/2026-06-08-reviews-site-embedding-design.md)
- [Site embedding implementation plan](docs/superpowers/plans/2026-06-08-reviews-site-embedding.md)

Frontend prototype:
- [Widget demo](web/reviews-widget/demo.html)
- Dynamic URL after `serve`: `http://127.0.0.1:8080/dynamic.html`
- Seller product links use `data/shegida-product-links.json` first, then fall
  back to `REVIEWS_SITE_PRODUCT_URL_TEMPLATE`.

## Phase 1 Scope

In scope:
- marketplace adapters for WB and Yandex Market;
- Ozon adapter behind a disabled-by-default feature flag;
- normalized review model;
- portable SQLite/Postgres schema;
- idempotent upsert by `(marketplace, external_review_id)`;
- hourly incremental sync and primary backfill;
- persisted sync history in `sync_runs`;
- existing marketplace seller answers imported read-only.

Out of scope for Phase 1:
- public API;
- production website widget;
- dashboard/admin UI;
- posting seller answers back to marketplaces;
- accepting reviews from the seller website.

## Expected Binary Commands

The current skeleton exposes:

```sh
reviews migrate
reviews sync --once
reviews sync --once --marketplace wb
reviews serve --addr 127.0.0.1:8080
reviews discover-site-urls --out data/shegida-product-links.json
reviews export --out web/reviews-data
```

See the runbook for configuration shape, environment variables, and operational
behavior.

## Site Embedding

The production embed path is static-first:

1. `reviews export` writes per-article JSON bundles to `web/reviews-data`.
2. Caddy serves `loader.js`, `reviews-widget.js`, `reviews-widget.css`, and
   `reviews-data/` over HTTPS from `reviews.shegida.ru`.
3. Yandex Tag Manager loads `loader.js` with a Custom HTML tag on page view /
   DOM Ready / all pages.
4. `loader.js` watches SPA navigation, extracts the product SKU from the Yandex
   Kit page, fetches `reviews-data/by-article/<sku>.json`, mounts the widget in
   Shadow DOM after the product details grid, and injects JSON-LD.

Deploy files and CI/CD templates live in [deploy](deploy/README.md) and
[.github/workflows](.github/workflows).

## Design Principles

- One deployable Go binary.
- No dependency on private projects.
- Marketplace adapters isolate API quirks, rate limits, pagination, and mapping.
- A failed marketplace sync must not stop other enabled marketplaces.
- Sync state is conservative: repeated reads are acceptable, missed reviews are
  not.
