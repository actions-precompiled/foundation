package foundation

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Main is the production entrypoint: DefaultDeps + os.Args + process exit.
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

// MainWith runs the Cobra CLI against an injected Package, Deps, and argv (no os.Exit).
func MainWith(p Package, deps Deps, args []string) error {
	if deps.Env == nil {
		deps.Env = OSEnviron{}
	}
	if deps.WorkDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		deps.WorkDir = wd
	}
	deps = finalizeDeps(deps)

	root := newRootCmd(p, deps)
	root.SetArgs(args)
	if deps.Stdout != nil {
		root.SetOut(deps.Stdout)
	}
	if deps.Stderr != nil {
		root.SetErr(deps.Stderr)
	}
	return root.Execute()
}

func newRootCmd(p Package, deps Deps) *cobra.Command {
	var f Flags

	root := &cobra.Command{
		Use:   "apc",
		Short: "actions-precompiled package CLI (foundation)",
		Long: `Build, list, smoke-test, publish, and generate CI for a package.

The same Go binary is the host orchestrator and the in-container worker:
  host:   apc build v1.2.3
  docker: mounts this binary as /apc and runs: /apc work
`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// persistent flags
	pf := root.PersistentFlags()
	pf.StringVar(&f.WorkDir, "workdir", "", "package repo root (APC_WORK_DIR)")
	pf.StringVar(&f.Targets, "targets", "", "space-separated targets (APC_TARGETS)")
	pf.StringVar(&f.BuildOutputDir, "output-dir", "", "artifact root (APC_BUILD_OUTPUT_DIR)")
	pf.StringVar(&f.ImageName, "image-name", "", "docker image name (APC_IMAGE_NAME)")
	pf.StringVar(&f.ImageTag, "image-tag", "", "docker image tag (APC_IMAGE_TAG)")

	bindVersions := func(cmd *cobra.Command) {
		cmd.Flags().BoolVar(&f.All, "all", false, "with empty versions: all upstream tags (APC_RECREATE / APC_FORCE_ALL)")
	}
	bindBuildFlags := func(cmd *cobra.Command) {
		cmd.Flags().BoolVar(&f.Publish, "publish", false, "create GitHub releases (APC_PUBLISH)")
		cmd.Flags().BoolVar(&f.DryRun, "dry-run", false, "plan only (APC_DRY_RUN)")
		cmd.Flags().BoolVar(&f.SkipSmoke, "skip-smoke", false, "skip smoke (APC_SKIP_SMOKE)")
		cmd.Flags().BoolVar(&f.SkipImageBuild, "skip-image-build", false, "reuse docker image (APC_SKIP_IMAGE_BUILD)")
		cmd.Flags().BoolVar(&f.Recreate, "recreate", false, "delete release before publish (APC_RECREATE)")
	}

	run := func(cmd Command) func(*cobra.Command, []string) error {
		return func(c *cobra.Command, args []string) error {
			f.Versions = args
			d := deps
			if f.WorkDir != "" {
				d.WorkDir = f.WorkDir
			}
			d = finalizeDeps(d)
			cfg, err := ResolveConfig(d.Env, p.Meta(), f, cmd)
			if err != nil {
				return err
			}
			if cfg.WorkDir != "" {
				d.WorkDir = cfg.WorkDir
			}
			return Run(context.Background(), p, d, cfg)
		}
	}

	planCmd := &cobra.Command{
		Use:   "plan [version]",
		Short: "CI plan job: write GITHUB_OUTPUT + step summary (all in Go)",
		Long: `Used by generated build.yml plan step:

  mise exec -- go run . plan

Reads GITHUB_EVENT_NAME, INPUT_VERSION, INPUT_PUBLISH, INPUT_RECREATE
(or flags/APC_*) and writes version/publish/recreate to GITHUB_OUTPUT
plus a markdown Build plan to GITHUB_STEP_SUMMARY.`,
		Args: cobra.MaximumNArgs(1),
		RunE: run(CommandPlan),
	}
	planCmd.Flags().BoolVar(&f.Publish, "publish", false, "publish (or INPUT_PUBLISH)")
	planCmd.Flags().BoolVar(&f.Recreate, "recreate", false, "recreate (or INPUT_RECREATE)")

	listCmd := &cobra.Command{
		Use:     "list [versions...]",
		Aliases: []string{"list-to-build"},
		Short:   "Print versions to build, one per line (missing upstream tags by default)",
		Long: `List versions for dispatch fan-out.

  apc list              # upstream tags not yet released here
  apc list --all        # every upstream tag
  apc list v1.2.3 v2.0  # echo explicit versions (planning pass-through)

Stdout is only version lines (for mapfile/xargs). Logs go to stderr.`,
		RunE: run(CommandList),
	}
	bindVersions(listCmd)

	buildCmd := &cobra.Command{
		Use:   "build [versions...]",
		Short: "Build package tarballs (docker work binary for Linux; native Work on Windows)",
		RunE:  run(CommandBuild),
	}
	bindVersions(buildCmd)
	bindBuildFlags(buildCmd)

	smokeCmd := &cobra.Command{
		Use:   "smoke [versions...]",
		Short: "Smoke-test existing tarballs under target/",
		RunE:  run(CommandSmoke),
	}
	bindVersions(smokeCmd)

	publishCmd := &cobra.Command{
		Use:   "publish [versions...]",
		Short: "Create GitHub releases from existing target/ artifacts",
		RunE:  run(CommandPublish),
	}
	bindVersions(publishCmd)
	publishCmd.Flags().BoolVar(&f.Recreate, "recreate", false, "delete existing release first")

	workCmd := &cobra.Command{
		Use:   "work",
		Short: "In-container/native unit of work for one version+target (reads APC_* env)",
		Long: `Runs Package.Work for a single matrix cell.

Intended entrypoint when the binary is mounted into Docker:

  docker run ... -v $PWD/apc:/apc:ro -v $out:/out --entrypoint /apc image work

Environment:
  APC_VERSION / PACKAGE_VERSION / <Meta.VersionEnv>
  APC_TARGET / BUILD_TARGET
  APC_OUTPUT_DIR / OUTPUT_DIR  (default /out in container)`,
		Args: cobra.NoArgs,
		RunE: run(CommandWork),
	}

	genCmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate package repo assets",
	}
	genWorkflowCmd := &cobra.Command{
		Use:   "workflow",
		Short: "Write GitHub Actions workflows (build + dispatch-missing)",
		Long: `Writes:

  .github/workflows/build.yml
  .github/workflows/dispatch-missing.yml

build.yml is a thin matrix: checkout → mise → go run . build <version>
dispatch-missing.yml fans out: go run . list | gh workflow run Build

Matrix targets come from Meta.DefaultTargets (or linux-amd64/aarch64 defaults).`,
		RunE: func(c *cobra.Command, args []string) error {
			d := deps
			if f.WorkDir != "" {
				d.WorkDir = f.WorkDir
			}
			d = finalizeDeps(d)
			cfg, err := ResolveConfig(d.Env, p.Meta(), f, CommandGenerateWorkflow)
			if err != nil {
				return err
			}
			if cfg.WorkDir != "" {
				d.WorkDir = cfg.WorkDir
			}
			return Run(context.Background(), p, d, cfg)
		},
	}
	genWorkflowCmd.Flags().StringVar(&f.WorkflowDir, "dir", ".github/workflows", "output directory for workflow YAML")
	genWorkflowCmd.Flags().BoolVar(&f.ForceWrite, "force", false, "overwrite existing workflow files")
	genCmd.AddCommand(genWorkflowCmd)

	root.AddCommand(planCmd, listCmd, buildCmd, smokeCmd, publishCmd, workCmd, genCmd)
	return root
}
