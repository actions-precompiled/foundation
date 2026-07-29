# actions-precompiled foundation

Go library + **Cobra CLI** for package repositories that build relocatable
tarballs. No `package.toml` magic, no shell packaging scripts as the container
entrypoint.

## Package contract

```go
type Package interface {
    Meta() Meta
    Work(ctx context.Context, deps Deps, req BuildRequest) error // real build
    Smoke(ctx context.Context, deps Deps, req SmokeRequest) error
}
```

`Work` is pure Go and runs:

- **natively** on Windows hosts for `windows-*` targets
- **inside Docker** for Linux targets — the host builds this binary and
  bind-mounts it as `/apc`, then runs `/apc work`

## CLI (Cobra)

```bash
go run . plan                  # CI plan job → GITHUB_OUTPUT + step summary
go run . list                 # missing upstream tags (one per line)
go run . list --all           # all upstream tags
go run . build v1.2.3          # build (injects binary into docker for linux)
go run . build --dry-run v1.2.3
go run . smoke v1.2.3
go run . publish --recreate v1.2.3
go run . work                 # single cell via APC_VERSION / APC_TARGET / APC_OUTPUT_DIR
go run . generate workflow --force
```

### Docker inject

Host `build` for `linux-*`:

1. `go build` (or reuse current executable) for the target GOOS/GOARCH
2. `docker run --entrypoint /apc -v binary:/apc:ro -v out:/out image work`

The image is only a dependency environment (cmake, ninja, compilers) — not a
shell `build_and_package.sh` entrypoint.

### Workflow generation

```bash
go run . generate workflow --force
```

Writes:

- `.github/workflows/build.yml` — matrix from `Meta.DefaultTargets`, thin
  `go run . build <version>`
- `.github/workflows/dispatch-missing.yml` — `go run . list` → `gh workflow run Build`

## Version type

`ParseVersion` / `Compare` (from workspaced semver ideas) sorts tags for
planning and listing.

## Config

Flags or `APC_*` env (`APC_TARGETS`, `APC_PUBLISH`, `APC_VERSION`, `APC_TARGET`, …).

## Develop

```bash
mise install
mise exec -- go test ./...
```

Module: `github.com/actions-precompiled/foundation`

## License

MIT
