#!/usr/bin/env sh
set -eu

OUT="${1:-./dist/reviews-linux-amd64}"
mkdir -p "$(dirname "$OUT")"

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -trimpath \
  -ldflags "-s -w" \
  -o "$OUT" \
  ./cmd/reviews

echo "built $OUT"
ls -lh "$OUT"
