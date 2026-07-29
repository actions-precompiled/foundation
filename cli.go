package foundation

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// Main is the production entrypoint: DefaultDeps + os.Args + process exit.
// Package repos typically call foundation.Main(myPackage{}) from main.
func Main(p Package) {
	if err := MainWith(p, DefaultDeps("."), os.Args[1:]); err != nil {
		code := 1
		var ee *ExitError
		if AsExitError(err, &ee) {
			code = ee.Code
			if ee.Err != nil {
				fmt.Fprintln(os.Stderr, ee.Err)
			}
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(code)
	}
}

// AsExitError is a tiny helper so we do not force errors.As import on callers.
func AsExitError(err error, target **ExitError) bool {
	if err == nil {
		return false
	}
	e, ok := err.(*ExitError)
	if !ok {
		return false
	}
	*target = e
	return true
}

// MainWith runs the CLI against an injected Package, Deps, and argv (no os.Exit).
// Returns an error for failures; callers map it to exit codes.
func MainWith(p Package, deps Deps, args []string) error {
	flags, err := parseFlags(args, deps.Stderr)
	if err != nil {
		return err
	}

	env := deps.Env
	if env == nil {
		env = OSEnviron{}
	}

	meta := p.Meta()
	cfg, err := ResolveConfig(env, meta, flags)
	if err != nil {
		return err
	}

	if cfg.WorkDir != "" {
		deps.WorkDir = cfg.WorkDir
	}
	if deps.WorkDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		deps.WorkDir = wd
	}

	// Rebind GitHub/Docker default clients with workdir/stderr if still default-shaped.
	deps = finalizeDeps(deps)

	ctx := context.Background()
	return Run(ctx, p, deps, cfg)
}

// flagsWithValue are long option names that consume the next argv token.
var flagsWithValue = map[string]bool{
	"targets":    true,
	"output-dir": true,
	"image-name": true,
	"image-tag":  true,
	"workdir":    true,
}

// reorderArgs moves flags before positional versions so flag.FlagSet can parse
// invocations like: create_releases v1.2.3 --skip-smoke
func reorderArgs(args []string) []string {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			positional = append(positional, a)
			continue
		}
		flags = append(flags, a)
		// --name=value already self-contained
		if strings.Contains(a, "=") {
			continue
		}
		name := strings.TrimLeft(a, "-")
		if flagsWithValue[name] && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			i++
			flags = append(flags, args[i])
		}
	}
	return append(flags, positional...)
}

func parseFlags(args []string, errW io.Writer) (Flags, error) {
	if errW == nil {
		errW = os.Stderr
	}
	fs := flag.NewFlagSet("foundation", flag.ContinueOnError)
	fs.SetOutput(errW)

	var f Flags
	fs.BoolVar(&f.Publish, "publish", false, "create GitHub releases and upload tarballs (also: APC_PUBLISH=1)")
	fs.BoolVar(&f.DryRun, "dry-run", false, "list versions/targets and exit (also: APC_DRY_RUN=1)")
	fs.BoolVar(&f.SkipSmoke, "skip-smoke", false, "skip post-build smoke tests (also: APC_SKIP_SMOKE=1)")
	fs.BoolVar(&f.SmokeOnly, "smoke-only", false, "only smoke-test existing tarballs under target/")
	fs.BoolVar(&f.ListToBuild, "list-to-build", false, "print versions that would be built, one per line")
	fs.BoolVar(&f.All, "all", false, "with empty version list: all upstream tags (also: APC_RECREATE / APC_FORCE_ALL)")
	fs.BoolVar(&f.SkipImageBuild, "skip-image-build", false, "use existing docker image (also: APC_SKIP_IMAGE_BUILD=1)")
	fs.BoolVar(&f.Recreate, "recreate", false, "delete existing release before publish (also: APC_RECREATE=1)")
	fs.StringVar(&f.Targets, "targets", "", "space-separated targets (also: APC_TARGETS)")
	fs.StringVar(&f.BuildOutputDir, "output-dir", "", "artifact root directory (also: APC_BUILD_OUTPUT_DIR)")
	fs.StringVar(&f.ImageName, "image-name", "", "docker image name override (also: APC_IMAGE_NAME)")
	fs.StringVar(&f.ImageTag, "image-tag", "", "docker image tag (also: APC_IMAGE_TAG, default local)")
	fs.StringVar(&f.WorkDir, "workdir", "", "package repo root (also: APC_WORK_DIR)")

	fs.Usage = func() {
		fmt.Fprintf(errW, "Usage: %s [flags] [versions...]\n\n", fs.Name())
		fmt.Fprintf(errW, "Build precompiled package tarballs. Local by default; --publish uploads releases.\n\n")
		fmt.Fprintf(errW, "Configuration uses flags or APC_* environment variables (no package.toml templating).\n\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(reorderArgs(args)); err != nil {
		return Flags{}, err
	}
	f.Versions = fs.Args()
	out := f.Versions[:0]
	for _, v := range f.Versions {
		if strings.TrimSpace(v) != "" {
			out = append(out, strings.TrimSpace(v))
		}
	}
	f.Versions = out
	return f, nil
}
