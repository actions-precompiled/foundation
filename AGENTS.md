# foundation (actions-precompiled)

Shared Go library + Cobra CLI for package repos that build **relocatable**
tarballs. No `package.toml` runtime; no shell packaging entrypoint in the
image.

## Commands

```bash
mise install
mise exec -- go test ./...
mise exec -- go test ./... -run TestName
mise exec -- go build ./...
```

Package consumers (e.g. llvm-bin): `go get github.com/actions-precompiled/foundation@<commit>` — prefer full pseudo-versions; `GOPROXY=direct` if the proxy lags.

## Layout (atoms live here)

| Area | Files (examples) | Kind |
|------|------------------|------|
| Contract | `package.go` | `Package` / `Meta` / requests |
| Engine / CLI | `engine.go`, `cli.go`, `config.go` | orchestration |
| Process / env | `runner.go`, `runenv.go`, `env.go` | **atoms** |
| Log / FS | `log.go`, `fs.go`, `deps.go` | **atoms** |
| Relocatable | `relocatable.go` | **atoms** (Linux ELF) |
| Plan / GHA | `plan.go`, `gha.go`, `workflow_gen.go` | CI atoms |
| Smoke default | `smoke_default.go` | generic molecule using atoms |
| Errors | `errors.go` | tabled `Err*`, wrap with `%w` |

Human overview: [README.md](README.md) (API surface, Docker inject, atoms table).

## Atoms vs molecules (coupling)

**Foundation owns atoms** — small, standardized primitives packages compose.
**Packages own molecules** — product policy (cmake flags, which tools to smoke,
Windows sysroot, project lists).

| Do in foundation | Do in the package repo |
|------------------|------------------------|
| `OutputWithEnv`, `CleanSmokeEnv`, `SmokeBinDirHelp` | Which product extras beyond bin-wide --help |
| `CheckLinuxRelocatable`, `PatchLinuxOriginRPath` | When to call them after install |
| `WriteLine` / `Logf` / `RemoveAllLog` | Package-specific log wording is fine |
| `DefaultRunner` process wiring | Upstream clone/build steps for *this* product |
| plan / list / publish / workflow gen | `Meta`, `Work`, custom `Smoke` |

**Coupling rules**

1. **No package identity in foundation** — no llvm/quickshell branches, no
   product cmake flags, no “kitchen sink” util lists.
2. **Prefer new atoms over special-case engines** — if two packages need the
   same primitive, extract an atom; if only one package needs a flow, keep the
   molecule in that package.
3. **Smoke is self-contained** — never teach packages to rely on
   `LD_LIBRARY_PATH` or prepending package `bin/` to `PATH`. Absolute paths +
   `$ORIGIN` RPATH + `CleanSmokeEnv`.
4. **Tagged releases only** — plan defaults to latest upstream **tag**;
   publish refuses `trunk` / `main` / `latest` (`IsPublishableTag`).
5. **DI stays shallow** — inject via `Deps`; do not grow a service locator or
   package-specific hooks into the engine unless many packages need them.

**Good**

```go
// package molecule
env := foundation.CleanSmokeEnv(deps.Env.Environ())
out, err := foundation.OutputWithEnv(ctx, deps, env, clang, "--version")
```

**Bad**

```go
// foundation learning about one product
if meta.Name == "llvm" { smokeKitchenSink(...) }
// or foundation setting LD_LIBRARY_PATH so broken RPATH “passes”
```

## Boundaries

**Always**

- Keep foundation atoms package-agnostic and reusable.
- Wrap errors with tabled `Err*` + `%w` (see `errors.go`).
- Run `mise exec -- go test ./...` after non-trivial edits.
- Linux artifacts must be relocatable without loader env hacks.

**Ask first**

- New public API surface on `Package` / `Deps` / `Meta`.
- Changing workflow generation shape (`workflow_gen.go`) for all consumers.
- Dropping or renaming `APC_*` env keys.

**Never**

- Push product-specific build/smoke policy into this module “for convenience”.
- Add `package.toml` templating or shell packaging as the container entrypoint.
- Use `context.Background` outside `main`/CLI entry — pass `ctx` from callers.
- Blank-discard errors from calls that can fail (handle or log).

## More context

- [README.md](README.md) — when onboarding or changing the public story
- Package repos (llvm-bin, …) — when implementing molecules; pin foundation by commit

User instructions override this file. Prefer existing patterns in this tree.
