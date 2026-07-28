#!/usr/bin/env bash
set -euo pipefail

if [[ -n "${GOROOT:-}" && ! -d "$GOROOT" ]]; then
  unset GOROOT
fi

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

(
  cd "$root_dir"
  go run ./cmd/catalog \
    --source catalog.source.json \
    --output "$tmp_dir/catalog.json" \
    --assets-dir "$tmp_dir/assets"
)

cmp "$root_dir/catalog.json" "$tmp_dir/catalog.json"
asset_count="$(find "$tmp_dir/assets" -type f -name '*.tar.gz' | wc -l | tr -d ' ')"
test "$asset_count" = 18
echo "catalog is generated and reproducible (18 assets)"
