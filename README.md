# actions-precompiled foundation

Go library that package repositories implement to build, smoke-test, and
optionally publish relocatable Linux tarballs.

This replaces the copy-pasted `create_releases` Python scripts and
`package.toml` templating with **code + dependency injection**.

## Principles

| Rule | Meaning |
|------|---------|
| **No magic** | No runtime `package.toml`, no shell-line templating. Identity is `Meta()`; config is flags or `APC_*` env. |
| **Package interface** | Each repo implements `Meta`, `Build`, `Smoke`. |
| **DI everywhere** | `Deps` carries `Runner`, `Environ`, `GitHub`, `Docker`, `FS`. Tests inject fakes. |
| **`MainWith` returns error** | No `os.Exit` inside the library path. `Main` is a thin wrapper for binaries. |
| **Targets drive the matrix** | `APC_TARGETS` / `Meta.DefaultTargets` / host-native. |

## Install / develop

```bash
mise install          # pins Go (see mise.toml)
mise exec -- go test ./...
```

Module path:

```text
github.com/actions-precompiled/foundation
```

## Package repo shape

```go
package main

import (
	"context"

	"github.com/actions-precompiled/foundation"
)

type pkg struct{}

func (pkg) Meta() foundation.Meta {
	return foundation.Meta{
		Name:            "example",
		UpstreamRepoAPI: "example-org/example",
		Binary:          "example",
		DefaultTargets: []string{
			foundation.TargetLinuxAMD64,
			foundation.TargetLinuxAArch64,
		},
	}
}

func (pkg) Build(ctx context.Context, deps foundation.Deps, req foundation.BuildRequest) error {
	// Option A: shared docker run helper
	return foundation.StandardDockerBuild(ctx, deps, pkg{}.Meta(), "example-buildenv", "local", req)
}

func (pkg) Smoke(ctx context.Context, deps foundation.Deps, req foundation.SmokeRequest) error {
	// Option A: default --help/--version smoke
	return foundation.SmokeBinaryHelp(ctx, deps, pkg{}.Meta(), req)
	// Option B: custom (Xvfb, QML fixtures, …)
}

func main() {
	foundation.Main(pkg{})
}
```

Local CLI (same flags as the old scripts):

```bash
./create_releases --list-to-build
./create_releases --list-to-build --all
./create_releases --dry-run v1.2.3
./create_releases v1.2.3
./create_releases --publish v1.2.3
./create_releases --smoke-only v1.2.3
```

## Configuration (`APC_*`)

| Variable | Role |
|----------|------|
| `APC_TARGETS` | Space-separated targets (`linux-amd64 linux-aarch64`) |
| `APC_PUBLISH` | Create GitHub releases after build |
| `APC_SKIP_SMOKE` | Skip smoke |
| `APC_SKIP_IMAGE_BUILD` | Reuse existing Docker image |
| `APC_RECREATE` / `APC_FORCE_ALL` | Plan all upstream tags; recreate also deletes release on publish |
| `APC_DRY_RUN` | Plan only |
| `APC_IMAGE_NAME` / `APC_IMAGE_TAG` | Image overrides |
| `APC_BUILD_OUTPUT_DIR` | Artifact root (default `{workdir}/target`) |
| `APC_WORK_DIR` | Package repo root |

Also read (not `APC_*`): `GH_TOKEN` / `GITHUB_TOKEN`, `GITHUB_REPOSITORY`.

## Engine flow

1. **Plan** — CLI versions, or upstream − released, or all upstream (`--all` / recreate).
2. **`--list-to-build`** — print versions and exit.
3. **`--dry-run`** — print plan and exit.
4. **`--smoke-only`** — `Smoke` only.
5. Else **build image** (unless skipped) → for each version × target: `Build` → `Smoke` → optional publish.

## Version type

`foundation.Version` (inspired by workspaced `internal/semver`) parses and
compares package tags:

```go
a := foundation.ParseVersion("v1.2.3")
b := foundation.ParseVersion("llvmorg-18.1.0")
if a.Less(b) { /* … */ }
foundation.SortVersionStrings(tags) // ascending
```

Specials: `latest`, `trunk` / `main` / `trunk-<sha>`. Bare artifact names via
`Version.Bare()` or `VersionBare`.

## Status

Foundation only. Workflow generators and migration of gettext/tesseract/quickshell
are follow-ups.

## License

MIT
