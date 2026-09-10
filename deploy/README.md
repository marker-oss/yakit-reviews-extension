# Deploy

This project is intended to run on a small VPS as one Go binary plus static
files served by Caddy.

## Install Wizard

The easiest install path is the local TUI wizard:

```sh
reviews install
```

Enter the reviews domain, shop origin, VPS SSH access, first admin credentials,
and marketplace credentials, then let the installer run the setup for you. The
manual SSH commands below are the transparent fallback.

Before starting the wizard, prepare a fresh Linux VPS, its public IP, SSH port,
root/sudo login, SSH password or private key, a reviews domain pointing to that
IP, the shop origin, and marketplace credentials. The user should not need to
open an SSH session manually; the wizard will use those access details to run
the setup. Wizard v1 targets fresh Ubuntu/Debian servers and deploys through
Docker Compose behind Caddy.

## DNS

Point `reviews.myshop.example` to the VPS IP address. Caddy will request and renew
TLS certificates automatically.

## VPS Layout

```text
/srv/reviews/
  reviews
  reviews.db
  .env
  data/
    product-links.json
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
sudo cp deploy/systemd/reviews.service /etc/systemd/system/
sudo cp deploy/systemd/reviews-sync.service /etc/systemd/system/
sudo cp deploy/systemd/reviews-sync.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now reviews.service
sudo systemctl enable --now reviews-sync.timer
```

## Environment

Create `/srv/reviews/.env` on the VPS. Minimal SQLite example:

```sh
REVIEWS_DB_DRIVER=sqlite
REVIEWS_DB_DSN=./reviews.db
REVIEWS_SITE_PRODUCT_LINKS=./data/product-links.json
REVIEWS_SITE_PRODUCT_URL_TEMPLATE=https://myshop.example/search?query={seller_article_url}
REVIEWS_WB_ENABLED=true
REVIEWS_WB_TOKEN=...
REVIEWS_YM_ENABLED=false
REVIEWS_OZON_ENABLED=false
```

## Local Build

```sh
./deploy/build.sh ./dist/reviews-linux-amd64
```

## Run with Docker

Prerequisites: Docker with the Compose plugin.

1. Copy and edit configuration:

   ```sh
   cp .env.example .env
   # fill in marketplace tokens, then enable the marketplaces you want to sync
   ```

   Keep `REVIEWS_INSECURE_COOKIES=1` only for local HTTP preview. Remove it or
   leave it empty when the admin panel is served over HTTPS.

2. Start the service (builds the image on first run):

   ```sh
   docker compose up -d --build
   ```

3. Verify it is healthy:

   ```sh
   curl -fsS http://localhost:8080/healthz
   ```

The server runs database migrations on startup and, because the image's default
command is `serve --with-sync`, it also runs review sync on `REVIEWS_SYNC_INTERVAL`
inside the same process, so no systemd timer is required. The SQLite database
persists in the `reviews-data` named volume.

Note: `docker compose config` expands values from `.env` into its output. Avoid
pasting that output into logs or tickets when real marketplace tokens are set.

To run a one-off sync manually:

```sh
docker compose run --rm reviews sync --once
```

## Server-Pull Deploy

For repeatable deploys where the VPS pulls from GitHub and builds in place,
use a source checkout at `/srv/reviews-src` and keep runtime state in
`/srv/reviews`.

One-time bootstrap on the VPS:

```sh
scp deploy/server-bootstrap.sh your-vps:/tmp/server-bootstrap.sh
ssh your-vps 'sh /tmp/server-bootstrap.sh'
```

Deploy the latest commit of a branch:

```sh
ssh your-vps 'DEPLOY_REF=main sh /srv/reviews-src/deploy/server-deploy.sh'
```

For the first run, when `/srv/reviews-src` does not exist yet, copy the deploy
script once and pass the repository URL:

```sh
scp deploy/server-deploy.sh your-vps:/tmp/server-deploy.sh
ssh your-vps 'REPO_URL=git@github.com:your-org/your-repo.git DEPLOY_REF=main sh /tmp/server-deploy.sh'
```

The deploy script does:

- `git fetch` / hard reset to `origin/$DEPLOY_REF`;
- server-side Go build;
- install binary, widget assets, and product links into `/srv/reviews`;
- run `migrate`, `sync --once`, `export`;
- install/restart `reviews.service` so dynamic helpers such as `/media` work;
- reload Caddy if it is active.

## Manual Ship

```sh
scp ./dist/reviews-linux-amd64 vps:/srv/reviews/reviews
scp web/reviews-widget/loader.js \
    web/reviews-widget/reviews-widget.js \
    web/reviews-widget/reviews-widget.css \
    vps:/srv/reviews/web/
scp data/product-links.json vps:/srv/reviews/data/
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
  dataBase: "https://reviews.myshop.example/reviews-data",
  widgetJsUrl: "https://reviews.myshop.example/reviews-widget.js",
  widgetCssUrl: "https://reviews.myshop.example/reviews-widget.css",
  debug: false
};
</script>
<script src="https://reviews.myshop.example/loader.js"></script>
```

## GitHub Actions Deploy

The deploy workflow expects these repository secrets:

- `VPS_HOST`
- `VPS_USER`
- `VPS_SSH_KEY`
- `VPS_PORT` (optional, defaults to `22`)

Run the `Deploy VPS` workflow manually from GitHub Actions.
