#!/bin/sh
# Updates the vendored Swagger UI assets served by the backend at /swagger/.
# Usage: scripts/update-swagger-ui.sh v5.33.3
set -eu

version="${1:?usage: $0 <swagger-ui tag, e.g. v5.33.3>}"
dest="$(cd "$(dirname "$0")/.." && pwd)/apps/backend/internal/docs/swaggerui"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

git clone --quiet --depth 1 --branch "$version" --filter=blob:none --no-checkout \
  https://github.com/swagger-api/swagger-ui "$tmp/swagger-ui"
git -C "$tmp/swagger-ui" sparse-checkout set --no-cone dist LICENSE NOTICE
git -C "$tmp/swagger-ui" checkout --quiet

for f in swagger-ui-bundle.js swagger-ui-bundle.js.LICENSE.txt swagger-ui.css favicon-32x32.png; do
  cp "$tmp/swagger-ui/dist/$f" "$dest/$f"
done
cp "$tmp/swagger-ui/LICENSE" "$dest/LICENSE"
cp "$tmp/swagger-ui/NOTICE" "$dest/NOTICE"

commit="$(git -C "$tmp/swagger-ui" rev-parse HEAD)"
cat > "$dest/VERSION" <<EOF
swagger-ui $version (commit $commit)
files: dist/swagger-ui-bundle.js, dist/swagger-ui-bundle.js.LICENSE.txt, dist/swagger-ui.css, dist/favicon-32x32.png, LICENSE, NOTICE
EOF
echo "Swagger UI updated to $version ($commit). Review the diff and run the backend tests."
