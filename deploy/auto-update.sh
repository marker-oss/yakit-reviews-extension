#!/bin/sh
# Reviews auto-update: pull new image, health-check, roll back on failure.
# Standalone copy for manual installs; the installer embeds the same script
# (internal/installer/render.go, keep them in sync) and runs it daily via
# reviews-update.timer. Usage: set REVIEWS_COMPOSE_DIR to the directory with
# docker-compose.yml (default /srv/reviews-src) and run from cron/systemd.
set -eu

DIR="${REVIEWS_COMPOSE_DIR:-/srv/reviews-src}"
SERVICE="${REVIEWS_SERVICE:-reviews}"
HEALTH_URL="${REVIEWS_HEALTH_URL:-http://127.0.0.1:8080/healthz}"

cd "$DIR"

exec 9>/var/lock/reviews-auto-update.lock
if ! flock -n 9; then
    echo "another update run is in progress, exiting"
    exit 0
fi

container="$(docker compose ps -q "$SERVICE")"
if [ -z "$container" ]; then
    echo "service $SERVICE is not running, skipping update"
    exit 0
fi
old_image="$(docker inspect --format '{{.Image}}' "$container")"
image_ref="$(docker inspect --format '{{.Config.Image}}' "$container")"

docker compose pull --quiet "$SERVICE"
new_image="$(docker image inspect --format '{{.Id}}' "$image_ref")"

if [ "$old_image" = "$new_image" ]; then
    echo "already up to date ($image_ref)"
    exit 0
fi

echo "updating $image_ref"
docker compose up -d "$SERVICE"

healthy=0
for _ in $(seq 1 12); do
    sleep 5
    if curl -fsS --max-time 5 "$HEALTH_URL" >/dev/null 2>&1; then
        healthy=1
        break
    fi
done

if [ "$healthy" = 1 ]; then
    echo "update ok: $image_ref is healthy"
    docker image prune -f >/dev/null 2>&1 || true
    exit 0
fi

echo "update failed: $HEALTH_URL did not come up, rolling back"
docker tag "$old_image" "$image_ref"
docker compose up -d --pull never "$SERVICE"
exit 1
