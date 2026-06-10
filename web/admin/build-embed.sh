#!/usr/bin/env sh
set -eu
npm run build
rm -rf ../../internal/server/admin_dist
cp -r dist ../../internal/server/admin_dist
echo "embedded admin SPA into internal/server/admin_dist"
