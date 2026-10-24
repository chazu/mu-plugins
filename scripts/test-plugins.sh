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
  aws cowsay docker doppler file go host k8s keypair-gen lint pass
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

# doppler's suite runs the plugin against a fake CLI and checks that no
# secret value reaches an error or stderr. CI installs bb under ~/.mu.
PATH="$PATH:$HOME/.mu/toolchains/bb" bb "$root_dir/plugins/doppler/test/plugin_test.bb"

# envsecret is the one source-only Go plugin. Exercise its nested module
# directly so the contract job verifies the package build boundary as well.
printf '%s\n' '{"method":"discover"}' \
  | (cd "$root_dir/plugins/envsecret" && go run .) \
  | grep -q '"name":"env"'
echo "envsecret discover passed"
