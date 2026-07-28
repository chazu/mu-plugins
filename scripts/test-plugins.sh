#!/usr/bin/env bash
set -euo pipefail

if [[ -n "${GOROOT:-}" && ! -d "$GOROOT" ]]; then
  unset GOROOT
fi

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
mu_bin="${MU_BIN:-mu}"

if ! command -v "$mu_bin" >/dev/null 2>&1; then
  echo "test-plugins: mu not found; set MU_BIN or install mu" >&2
  exit 1
fi

plugins=(
  aws cowsay docker file go host k8s keypair-gen lint pass
  remote-exec remote-file scratch sops terraform void zig
)

for name in "${plugins[@]}"; do
  if [[ "$name" == aws ]]; then
    MU_AWS_FIXTURE=1 \
      PATH="$root_dir/plugins/aws/testdata/bin:$PATH" \
      "$mu_bin" plugin test --no-color --config "$root_dir/mu.cue" "$root_dir/plugins/$name"
  else
    "$mu_bin" plugin test --no-color --config "$root_dir/mu.cue" "$root_dir/plugins/$name"
  fi
done

# envsecret is the one source-only Go plugin. mu's generic scenario runner
# currently discovers Babashka or prebuilt executable entrypoints, so exercise
# this package through its source entrypoint until Go-plugin package builds land.
printf '%s\n' '{"method":"discover"}' \
  | (cd "$root_dir" && go run ./plugins/envsecret) \
  | grep -q '"name":"env"'
echo "envsecret discover passed"
