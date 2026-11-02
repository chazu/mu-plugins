# mu-plugins

Official plugins for [chazu/mu](https://github.com/chazu/mu). This repository
is the source catalog for the default plugin set; each package is independently
bundleable and installable by mu.

## Packages

| Plugin | Language | Role |
| --- | --- | --- |
| `docker` | Babashka | Docker image builder |
| `doppler` | Babashka | Read-only Doppler secret provider |
| `file` | Go | File convergence |
| `go` | Babashka | Go build toolchain |
| `k8s` | Babashka | Kubernetes convergence and drift observer |
| `keypair-gen` | Go | Ed25519/ECDSA keypair generator |
| `lint` | Babashka | Linter wrapper |
| `pass` | Go | `pass` secret provider |
| `remote-exec` | Go | Remote SSH command execution |
| `remote-file` | Go | Remote file convergence |
| `sops` | Babashka | SOPS secret provider |
| `terraform` | Babashka | Terraform plan/apply and drift observer |

The canonical implementation for each package is the entrypoint declared by
its `plugins/<name>/mu.cue`.

The Go plugins are vendored from `github.com/chazu/mu/plugins/<name>`, which
owns their tests. Each is a source-only nested module (`main.go`, `go.mod`,
`go.sum`, plus `internal/ssh` for the SSH plugins). The catalog declares
`entrypoint: "plugin"` and the build command
`go build -trimpath -o plugin .`; `mu plugin install` runs it in the
extracted package directory, so installing one requires Go 1.26 or newer. The
built binary is ignored by git and never included in a release archive.

## Package layout

```text
plugins/<name>/
  mu.cue       package manifest
  GUIDE.md     operator and configuration guide
  plugin.bb    executable entrypoint (Babashka plugins)
  main.go      source of the built `plugin` entrypoint (Go plugins)
  go.mod       nested Go module (Go plugins)
```

## Development

From this repository:

```bash
go test ./...
bash scripts/check-catalog.sh   # catalog.json is generated and reproducible
bash scripts/test-plugins.sh    # discover contract for every plugin
```

To test one Go plugin by hand, build it first:

```bash
(cd plugins/pass && go build -trimpath -o plugin .)
mu plugin test plugins/pass
```

## Catalog and releases

`catalog.source.json` is the hand-authored metadata source. The generator
validates every package manifest, checks plugin versions against their
discover source, creates deterministic source archives, and writes the
generated [`catalog.json`](catalog.json).

Regenerate it locally with:

```bash
tmp_dir="$(mktemp -d)"
go run ./cmd/catalog \
  --source catalog.source.json \
  --output catalog.json \
  --assets-dir "$tmp_dir" \
  --release-tag catalog-v0.2.0
```

Pushing a `catalog-v*` tag runs the release workflow. It uploads
`catalog.json`, `SHA256SUMS`, and one immutable `<plugin>-<version>.tar.gz`
source asset per package. The catalog records the release URL and hash that
`mu plugin install` verifies before it extracts, builds, and bundles a
package. Keep `release_tag` in `catalog.source.json` equal to the tag you push,
so the committed `catalog.json` matches the published one.
