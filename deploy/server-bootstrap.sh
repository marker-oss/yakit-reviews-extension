#!/usr/bin/env sh
set -eu

GO_VERSION="${GO_VERSION:-1.26.4}"
SRC_DIR="${SRC_DIR:-/srv/reviews-src}"
APP_DIR="${APP_DIR:-/srv/reviews}"

if [ "$(id -u)" -ne 0 ]; then
  echo "server-bootstrap.sh must run as root" >&2
  exit 1
fi

apt-get update
apt-get install -y ca-certificates curl git tar

installed_go=""
if [ -x /usr/local/go/bin/go ]; then
  installed_go="$(/usr/local/go/bin/go version | awk '{print $3}' | sed 's/^go//')"
fi

if [ "$installed_go" != "$GO_VERSION" ]; then
  tmp="/tmp/go${GO_VERSION}.linux-amd64.tar.gz"
  curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -o "$tmp"
  rm -rf /usr/local/go
  tar -C /usr/local -xzf "$tmp"
fi

mkdir -p "$SRC_DIR" "$APP_DIR/data" "$APP_DIR/web/reviews-data"
chmod 0755 "$SRC_DIR" "$APP_DIR" "$APP_DIR/data" "$APP_DIR/web" "$APP_DIR/web/reviews-data"

echo "bootstrap complete"
echo "go=$(/usr/local/go/bin/go version)"
echo "src_dir=$SRC_DIR"
echo "app_dir=$APP_DIR"
