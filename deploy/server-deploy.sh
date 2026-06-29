#!/usr/bin/env sh
set -eu

REPO_URL="${REPO_URL:-}"
DEPLOY_REF="${DEPLOY_REF:-main}"
SRC_DIR="${SRC_DIR:-/srv/reviews-src}"
APP_DIR="${APP_DIR:-/srv/reviews}"
GO_BIN="${GO_BIN:-/usr/local/go/bin/go}"
RUN_SYNC="${RUN_SYNC:-true}"
SYNC_MARKETPLACE="${SYNC_MARKETPLACE:-}"

env_value() {
  key="$1"
  file="$2"
  if [ ! -f "$file" ]; then
    return 0
  fi
  grep -E "^${key}=" "$file" | tail -n 1 | cut -d= -f2- | sed 's/^"//; s/"$//; s/^'\''//; s/'\''$//'
}

caddy_cert_domain() {
  cert_root="/var/lib/caddy/.local/share/caddy/certificates"
  if [ ! -d "$cert_root" ]; then
    return 0
  fi
  find "$cert_root" -mindepth 2 -maxdepth 2 -type d 2>/dev/null |
    awk -F/ '{print $NF}' |
    grep -Ev '^(localhost|)$' |
    head -n 1
}

write_caddyfile() {
  env_file="$APP_DIR/.env"
  public_domain="$(env_value REVIEWS_PUBLIC_DOMAIN "$env_file")"
  shop_origin="$(env_value REVIEWS_SHOP_ORIGIN "$env_file")"
  if [ "$public_domain" = "" ]; then
    public_domain="$(caddy_cert_domain)"
  fi
  if [ "$shop_origin" = "" ]; then
    shop_origin="*"
  fi
  if [ "$public_domain" = "" ]; then
    echo "skip Caddyfile update: REVIEWS_PUBLIC_DOMAIN is missing and no Caddy certificate domain was found" >&2
    return 0
  fi
  cat > "$SRC_DIR/.deploy/Caddyfile" <<EOF
$public_domain {
	encode zstd gzip

	root * $APP_DIR/web

	@cors path /reviews-data /reviews-data/* /loader.js /reviews-widget.js /reviews-widget.css
	header @cors {
		Access-Control-Allow-Origin "$shop_origin"
		Access-Control-Allow-Methods "GET, OPTIONS"
		Access-Control-Allow-Headers "Accept, Content-Type"
		Vary "Origin"
	}

	@options method OPTIONS
	header @options {
		Access-Control-Allow-Origin "$shop_origin"
		Access-Control-Allow-Methods "GET, OPTIONS"
		Access-Control-Allow-Headers "Accept, Content-Type"
		Vary "Origin"
	}
	respond @options 204

	@json path /reviews-data/* /reviews-data
	header @json {
		Content-Type "application/json; charset=utf-8"
		Cache-Control "public, max-age=300"
	}

	@assets path /loader.js /reviews-widget.js /reviews-widget.css
	header @assets Cache-Control "public, max-age=3600"

	@backend path /api /api/* /admin /admin/* /healthz
	reverse_proxy @backend 127.0.0.1:8080

	@media path /media
	reverse_proxy @media 127.0.0.1:8080

	file_server
}
EOF
  install -m 0644 "$SRC_DIR/.deploy/Caddyfile" /etc/caddy/Caddyfile
}

if [ "$(id -u)" -ne 0 ]; then
  echo "server-deploy.sh must run as root" >&2
  exit 1
fi

if [ ! -x "$GO_BIN" ]; then
  echo "Go binary not found at $GO_BIN. Run deploy/server-bootstrap.sh first." >&2
  exit 1
fi

if [ -d "$SRC_DIR/.git" ]; then
  cd "$SRC_DIR"
  git fetch --prune origin
else
  if [ -z "$REPO_URL" ]; then
    echo "REPO_URL is required for the first deploy (no checkout at $SRC_DIR yet)." >&2
    echo "Re-run with: REPO_URL=git@github.com:your-org/your-repo.git sh server-deploy.sh" >&2
    exit 1
  fi
  rm -rf "$SRC_DIR"
  git clone "$REPO_URL" "$SRC_DIR"
  cd "$SRC_DIR"
fi

git checkout -B "$DEPLOY_REF" "origin/$DEPLOY_REF"
git reset --hard "origin/$DEPLOY_REF"

mkdir -p "$APP_DIR/data" "$APP_DIR/web" "$APP_DIR/web/reviews-data" "$SRC_DIR/.deploy"

GOCACHE="${GOCACHE:-/tmp/reviews-go-cache}" \
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 "$GO_BIN" build \
  -trimpath \
  -ldflags "-s -w" \
  -o "$SRC_DIR/.deploy/reviews" \
  ./cmd/reviews

install -m 0755 "$SRC_DIR/.deploy/reviews" "$APP_DIR/reviews"
# Seed the product-links map if one is committed; otherwise it is generated on
# the server by `discover-site-urls` (see reviews-links.timer).
if [ -f "$SRC_DIR/data/product-links.json" ]; then
  install -m 0644 "$SRC_DIR/data/product-links.json" "$APP_DIR/data/product-links.json"
fi
install -m 0644 "$SRC_DIR/web/reviews-widget/loader.js" "$APP_DIR/web/loader.js"
install -m 0644 "$SRC_DIR/web/reviews-widget/reviews-widget.js" "$APP_DIR/web/reviews-widget.js"
install -m 0644 "$SRC_DIR/web/reviews-widget/reviews-widget.css" "$APP_DIR/web/reviews-widget.css"
if command -v systemctl >/dev/null 2>&1; then
  install -m 0644 "$SRC_DIR/deploy/systemd/reviews.service" /etc/systemd/system/reviews.service
  systemctl daemon-reload
fi

cd "$APP_DIR"
./reviews migrate

if [ "$RUN_SYNC" = "true" ]; then
  if [ "$SYNC_MARKETPLACE" = "" ]; then
    ./reviews sync --once
  else
    ./reviews sync --once --marketplace "$SYNC_MARKETPLACE"
  fi
fi

./reviews export --out "$APP_DIR/web/reviews-data"

if command -v systemctl >/dev/null 2>&1; then
  systemctl enable --now reviews.service
  systemctl restart reviews.service
fi

if command -v systemctl >/dev/null 2>&1 && [ -f /etc/caddy/Caddyfile ]; then
  write_caddyfile
  if systemctl is-active --quiet caddy; then
    systemctl reload caddy
  else
    systemctl restart caddy
  fi
fi

echo "deploy complete"
echo "ref=$DEPLOY_REF"
echo "commit=$(git -C "$SRC_DIR" rev-parse --short HEAD)"
