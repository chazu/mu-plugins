#!/usr/bin/env bash
set -euo pipefail

if [[ -n "${GOROOT:-}" && ! -d "$GOROOT" ]]; then
  unset GOROOT
fi

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
mu_bin="${MU_BIN:-mu}"

if ! mu_path="$(command -v "$mu_bin")"; then
  echo "test-plugins: mu not found; set MU_BIN or install mu" >&2
  exit 1
fi

bb_plugins=(docker doppler go k8s lint sops terraform)
go_plugins=(file keypair-gen pass remote-exec remote-file)

# mu runs Babashka plugins with ~/.mu/toolchains/bb/bb. CI installs it there;
# locally, fall back to bb on PATH.
bb_path="$HOME/.mu/toolchains/bb/bb"
if [[ ! -x "$bb_path" ]]; then
  bb_path="$(command -v bb || true)"
fi
if [[ -z "$bb_path" ]]; then
  echo "test-plugins: bb not found under ~/.mu/toolchains/bb or on PATH" >&2
  exit 1
fi

# Resolving a plugin bundles it into ~/.mu, so run mu against a throwaway HOME
# that only carries bb. Keep Go's caches where they already are.
export GOCACHE="$(go env GOCACHE)" GOMODCACHE="$(go env GOMODCACHE)" GOPATH="$(go env GOPATH)"
work_dir="$(mktemp -d)"
cleanup() {
  for name in "${go_plugins[@]}"; do
    rm -f "$root_dir/plugins/$name/plugin"
  done
  rm -rf "$work_dir"
}
trap cleanup EXIT

mu_home="$work_dir/home"
mkdir -p "$mu_home/.mu/toolchains/bb"
ln -s "$bb_path" "$mu_home/.mu/toolchains/bb/bb"

# doppler's suite runs the plugin against a fake CLI and checks that no
# secret value reaches an error or stderr. It starts the plugin with `bb` from
# PATH.
PATH="$PATH:$(dirname "$bb_path")" "$bb_path" "$root_dir/plugins/doppler/test/plugin_test.bb"

# Go plugins are source-only nested modules; their unit tests live with the
# canonical source in the mu repo. Build each exactly as `mu plugin install`
# does.
for name in "${go_plugins[@]}"; do
  (
    cd "$root_dir/plugins/$name"
    go vet .
    go build -trimpath -o plugin .
  )
done

# Register every package directory as a project plugin and run discover through
# `mu plugin info`, which resolves and bundles it exactly as a build would. mu
# rejects a discover response that advertises removed capabilities.
project="$work_dir/project"
mkdir -p "$project/cue.mod"
printf 'module: "example.com/plugin-contract"\nlanguage: version: "v0.16.0"\n' \
  >"$project/cue.mod/module.cue"
{
  echo 'package mu'
  echo 'plugins: ['
  for name in "${bb_plugins[@]}" "${go_plugins[@]}"; do
    printf '\t{name: "%s", script: "%s"},\n' "$name" "$root_dir/plugins/$name"
  done
  echo ']'
} >"$project/mu.cue"

for name in "${bb_plugins[@]}" "${go_plugins[@]}"; do
  info="$(cd "$project" && HOME="$mu_home" "$mu_path" plugin info --json --config "$project/mu.cue" "$name")"
  if ! grep -q "^  \"name\": \"$name\",\$" <<<"$info"; then
    echo "test-plugins: $name: discover reported a different name" >&2
    echo "$info" >&2
    exit 1
  fi
  version="$(sed -n 's/^  "version": "\(.*\)",$/\1/p' <<<"$info")"
  echo "ok $name $version"
done
