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

bb_plugins=(docker doppler go k8s lint sops terraform)
go_plugins=(file keypair-gen pass remote-exec remote-file)

for name in "${bb_plugins[@]}"; do
  "$mu_bin" plugin test --no-color --config "$root_dir/mu.cue" "$root_dir/plugins/$name"
done

# doppler's suite runs the plugin against a fake CLI and checks that no
# secret value reaches an error or stderr. CI installs bb under ~/.mu.
PATH="$PATH:$HOME/.mu/toolchains/bb" bb "$root_dir/plugins/doppler/test/plugin_test.bb"

# Go plugins are source-only nested modules; their unit tests live with the
# canonical source in the mu repo. Build each exactly as `mu plugin install`
# does, then run the discover contract against the binary.
cleanup_go_plugins() {
  for name in "${go_plugins[@]}"; do
    rm -f "$root_dir/plugins/$name/plugin"
  done
}
trap cleanup_go_plugins EXIT

for name in "${go_plugins[@]}"; do
  (
    cd "$root_dir/plugins/$name"
    go vet .
    go build -trimpath -o plugin .
  )
  "$mu_bin" plugin test --no-color --config "$root_dir/mu.cue" "$root_dir/plugins/$name"
done
