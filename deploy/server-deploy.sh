#!/usr/bin/env sh
set -eu

REPO_URL="${REPO_URL:-}"
DEPLOY_REF="${DEPLOY_REF:-main}"
SRC_DIR="${SRC_DIR:-/srv/reviews-src}"
APP_DIR="${APP_DIR:-/srv/reviews}"
GO_BIN="${GO_BIN:-/usr/local/go/bin/go}"
RUN_SYNC="${RUN_SYNC:-true}"
SYNC_MARKETPLACE="${SYNC_MARKETPLACE:-}"

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

if command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet caddy; then
  systemctl reload caddy
fi

echo "deploy complete"
echo "ref=$DEPLOY_REF"
echo "commit=$(git -C "$SRC_DIR" rev-parse --short HEAD)"
