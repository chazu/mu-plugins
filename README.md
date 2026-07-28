# mu-plugins

Official plugins for [chazu/mu](https://github.com/chazu/mu). This repository
is the source catalog for the default plugin set; each package is independently
bundleable and installable by mu.

## Packages

| Plugin | Role |
| --- | --- |
| `aws` | AWS EC2/VPC/subnet observer |
| `cowsay` | Demo text transformation |
| `docker` | Docker image builder |
| `envsecret` | Read-only environment secret provider |
| `file` | File convergence |
| `go` | Go build toolchain |
| `host` | Remote host observer over SSH |
| `k8s` | Kubernetes convergence and drift observer |
| `keypair-gen` | Ed25519/ECDSA keypair generator |
| `lint` | Linter wrapper |
| `pass` | `pass` secret provider |
| `remote-exec` | Remote SSH command execution |
| `remote-file` | Remote file convergence |
| `scratch` | Toolchain bootstrap helper |
| `sops` | SOPS secret provider |
| `terraform` | Terraform plan/apply and drift observer |
| `void` | Build webhook reporter |
| `zig` | Zig build toolchain |

The canonical implementation for each package is the entrypoint declared by
its `plugins/<name>/mu.cue`. Unselected Go ports remain in the mu repository
until they are deliberately promoted or removed.

## Package layout

```text
plugins/<name>/
  mu.cue       package manifest
  GUIDE.md     operator and configuration guide
  plugin.bb    executable entrypoint (where selected)
  pudl.cue     optional PUDL semantic mappings
  schemas/     optional plugin-owned CUE wire schemas
```

Plugin-owned schemas use the `mu/<plugin>` namespace. PUDL semantic schemas
remain in PUDL; a package's optional `pudl.cue` maps emitted `_schema` resource
types to those semantic schemas. The AWS package is the first complete example.

## Development

From this repository:

```bash
mu plugin test plugins/aws
mu verify
go test ./...
```

The AWS observer has a deterministic fake-CLI fixture:

```bash
MU_AWS_FIXTURE=1 \
  PATH="$PWD/plugins/aws/testdata/bin:$PATH" \
  mu plugin test plugins/aws
```
