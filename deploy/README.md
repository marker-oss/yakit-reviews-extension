# Deploy

This project is intended to run on a small VPS as one Go binary plus static
files served by Caddy.

## DNS

Point `reviews.shegida.ru` to the VPS IP address. Caddy will request and renew
TLS certificates automatically.

## VPS Layout

```text
/srv/reviews/
  reviews
  reviews.db
  .env
  data/
    shegida-product-links.json
  web/
    loader.js
    reviews-widget.js
    reviews-widget.css
    reviews-data/
      index.json
      by-article/*.json
```

## First-Time Server Setup

Install Caddy and create the project directories:

```sh
sudo mkdir -p /srv/reviews/data /srv/reviews/web/reviews-data
sudo chown -R "$USER":"$USER" /srv/reviews
sudo cp deploy/Caddyfile /etc/caddy/Caddyfile
sudo systemctl reload caddy
```

Copy service files if you want systemd-managed hourly sync/export:

```sh
sudo cp deploy/systemd/reviews-sync.service /etc/systemd/system/
sudo cp deploy/systemd/reviews-sync.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now reviews-sync.timer
```

## Environment

Create `/srv/reviews/.env` on the VPS. Minimal SQLite example:

```sh
REVIEWS_DB_DRIVER=sqlite
REVIEWS_DB_DSN=./reviews.db
REVIEWS_SITE_PRODUCT_LINKS=./data/shegida-product-links.json
REVIEWS_SITE_PRODUCT_URL_TEMPLATE=https://shegida.ru/search?query={seller_article_url}
REVIEWS_WB_ENABLED=true
REVIEWS_WB_TOKEN=...
REVIEWS_YM_ENABLED=false
REVIEWS_OZON_ENABLED=false
```

## Local Build

```sh
./deploy/build.sh ./dist/reviews-linux-amd64
```

## Manual Ship

```sh
scp ./dist/reviews-linux-amd64 vps:/srv/reviews/reviews
scp web/reviews-widget/loader.js \
    web/reviews-widget/reviews-widget.js \
    web/reviews-widget/reviews-widget.css \
    vps:/srv/reviews/web/
scp data/shegida-product-links.json vps:/srv/reviews/data/
```

On the VPS:

```sh
cd /srv/reviews
chmod +x ./reviews
./reviews migrate
./reviews export --out ./web/reviews-data
sudo systemctl reload caddy
```

## Yandex Tag Manager

After deploy, add a Custom HTML tag firing on page view / DOM Ready / all pages:

```html
<script>
window.REVIEWS_EMBED_CONFIG = {
  dataBase: "https://reviews.shegida.ru/reviews-data",
  widgetJsUrl: "https://reviews.shegida.ru/reviews-widget.js",
  widgetCssUrl: "https://reviews.shegida.ru/reviews-widget.css",
  debug: false
};
</script>
<script src="https://reviews.shegida.ru/loader.js"></script>
```

## GitHub Actions Deploy

The deploy workflow expects these repository secrets:

- `VPS_HOST`
- `VPS_USER`
- `VPS_SSH_KEY`
- `VPS_PORT` (optional, defaults to `22`)

Run the `Deploy VPS` workflow manually from GitHub Actions.
