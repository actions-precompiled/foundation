package foundation

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
)

// Run executes a host-side command (list/build/smoke/publish) from Config.
func Run(ctx context.Context, p Package, deps Deps, cfg Config) error {
	meta := p.Meta().Normalize()
	if err := meta.Validate(); err != nil {
		return err
	}

	if cfg.WorkDir != "" {
		deps.WorkDir = cfg.WorkDir
	}
	if deps.WorkDir == "" {
		return fmt.Errorf("WorkDir is required on Deps or Config")
	}

	switch cfg.Command {
	case CommandList:
		return runList(ctx, deps, meta, cfg)
	case CommandWork:
		return runWork(ctx, p, deps, cfg)
	case CommandGenerateWorkflow:
		return GenerateWorkflows(deps, meta, cfg)
	case CommandBuild, CommandSmoke, CommandPublish:
		// fall through
	default:
		return fmt.Errorf("unknown command %q", cfg.Command)
	}

	versions, err := PlanVersions(ctx, deps, meta, cfg.Versions, cfg.ForceAll)
	if err != nil {
		return err
	}
	if len(versions) == 0 {
		deps.Logf("No versions to build.")
		return nil
	}

	deps.Logf("package=%s upstream=%s command=%s", meta.Name, meta.UpstreamRepoAPI, cfg.Command)
	deps.Logf("Versions (%d):", len(versions))
	for _, v := range versions {
		deps.Logf("  - %s  [targets: %s]", v, joinSpace(cfg.Targets))
	}

	if cfg.DryRun {
		deps.Logf("Dry-run — exiting without building")
		return nil
	}

	if hp, ok := p.(HostPrep); ok && cfg.Command == CommandBuild {
		if err := hp.PrepHost(ctx, deps, cfg); err != nil {
			return fmt.Errorf("prep host: %w", err)
		}
	}

	if cfg.Command == CommandSmoke {
		for _, version := range versions {
			if err := smokeVersion(ctx, p, deps, meta, cfg, version); err != nil {
				return err
			}
		}
		deps.Logf("✓ Smoke finished")
		return nil
	}

	if cfg.Command == CommandPublish {
		for _, version := range versions {
			if err := publishVersion(ctx, deps, meta, cfg, version); err != nil {
				return err
			}
		}
		deps.Logf("✓ Publish finished")
		return nil
	}

	// CommandBuild
	needDocker := TargetsNeedDocker(cfg.Targets)
	switch {
	case cfg.SkipImageBuild:
		deps.Logf("APC_SKIP_IMAGE_BUILD set — using existing %s:%s", cfg.ImageName, cfg.ImageTag)
	case !needDocker:
		deps.Logf("no docker targets — skipping image build")
	default:
		if err := ensureImage(ctx, deps, meta, cfg); err != nil {
			return err
		}
	}

	for _, version := range versions {
		if err := buildRelease(ctx, p, deps, meta, cfg, version); err != nil {
			return err
		}
	}
	deps.Logf("✓ All requested builds finished")
	return nil
}

func runList(ctx context.Context, deps Deps, meta Meta, cfg Config) error {
	versions, err := PlanVersions(ctx, deps, meta, cfg.Versions, cfg.ForceAll)
	if err != nil {
		return err
	}
	for _, v := range versions {
		deps.Outf("%s", v)
	}
	return nil
}

func runWork(ctx context.Context, p Package, deps Deps, cfg Config) error {
	if len(cfg.Versions) != 1 {
		return fmt.Errorf("work requires exactly one version (got %v); set APC_VERSION", cfg.Versions)
	}
	if len(cfg.Targets) != 1 {
		return fmt.Errorf("work requires exactly one target (got %v); set APC_TARGET", cfg.Targets)
	}
	req := BuildRequest{
		Version: cfg.Versions[0],
		Target:  cfg.Targets[0],
		OutDir:  cfg.BuildOutputDir,
	}
	if err := deps.FS.MkdirAll(req.OutDir, 0o755); err != nil {
		return err
	}
	deps.Logf("work version=%s target=%s out=%s goos=%s", req.Version, req.Target, req.OutDir, runtime.GOOS)
	return p.Work(ctx, deps, req)
}

func ensureImage(ctx context.Context, deps Deps, meta Meta, cfg Config) error {
	if deps.Docker == nil {
		return fmt.Errorf("ensure image: Docker client is nil")
	}
	image := cfg.ImageName + ":" + cfg.ImageTag
	arch := HostDockerArch()
	deps.Logf("Building Docker image %s (TARGETARCH=%s)...", image, arch)
	return deps.Docker.BuildImage(ctx, DockerBuildRequest{
		Context:    deps.WorkDir,
		Image:      image,
		TargetArch: arch,
	})
}

func buildRelease(ctx context.Context, p Package, deps Deps, meta Meta, cfg Config, version string) error {
	deps.Logf("")
	deps.Logf("========================================")
	deps.Logf("Building %s version %s", meta.Name, version)
	deps.Logf("========================================")

	for _, target := range cfg.Targets {
		outDir := filepath.Join(cfg.BuildOutputDir, target)
		if err := deps.FS.MkdirAll(outDir, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", outDir, err)
		}
		req := BuildRequest{Version: version, Target: target, OutDir: outDir}
		deps.Logf("")
		deps.Logf("Work %s / %s...", version, target)

		if IsWindowsTarget(target) {
			if err := p.Work(ctx, deps, req); err != nil {
				return fmt.Errorf("work %s/%s: %w", version, target, err)
			}
			continue
		}

		// Linux: compile this package binary and run it as `work` inside Docker.
		goos, goarch := TargetGOOSGOARCH(target)
		worker, cleanup, err := EnsureWorkerBinary(ctx, deps, goos, goarch)
		if err != nil {
			return err
		}
		err = RunWorkInDocker(ctx, deps, meta, cfg.ImageName, cfg.ImageTag, req, worker)
		cleanup()
		if err != nil {
			return fmt.Errorf("work docker %s/%s: %w", version, target, err)
		}
	}

	if !cfg.SkipSmoke {
		if err := smokeVersion(ctx, p, deps, meta, cfg, version); err != nil {
			return err
		}
	}
	if cfg.Publish {
		return publishVersion(ctx, deps, meta, cfg, version)
	}
	deps.Logf("Local build only — pass --publish to create a GitHub release")
	return nil
}

func smokeVersion(ctx context.Context, p Package, deps Deps, meta Meta, cfg Config, version string) error {
	deps.Logf("")
	deps.Logf("========================================")
	deps.Logf("Smoke testing %s", version)
	deps.Logf("========================================")
	for _, target := range cfg.Targets {
		outDir := filepath.Join(cfg.BuildOutputDir, target)
		tarballs, err := FindTarballs(deps.FS, meta.Name, version, target, outDir)
		if err != nil {
			return err
		}
		if len(tarballs) == 0 {
			return fmt.Errorf("no tarball for %s / %s under %s", version, target, outDir)
		}
		req := SmokeRequest{Version: version, Target: target, OutDir: outDir, Tarballs: tarballs}
		if err := p.Smoke(ctx, deps, req); err != nil {
			return fmt.Errorf("smoke %s/%s: %w", version, target, err)
		}
	}
	return nil
}

func publishVersion(ctx context.Context, deps Deps, meta Meta, cfg Config, version string) error {
	if deps.GitHub == nil {
		return fmt.Errorf("publish: GitHub client is nil")
	}
	var assets []string
	for _, target := range cfg.Targets {
		outDir := filepath.Join(cfg.BuildOutputDir, target)
		found, err := FindTarballs(deps.FS, meta.Name, version, target, outDir)
		if err != nil {
			return err
		}
		assets = append(assets, found...)
	}
	if len(assets) == 0 {
		return fmt.Errorf("publish %s: no assets", version)
	}
	if cfg.Recreate {
		deps.Logf("Recreate: deleting existing release %s (if any)", version)
		if err := deps.GitHub.DeleteRelease(ctx, version); err != nil {
			deps.Logf("  delete release: %v (continuing)", err)
		}
	}
	notes := fmt.Sprintf("%s\n\nInstall: extract the tarball and put `bin/` on your PATH.\n\nAssets:\n", meta.Description)
	for _, a := range assets {
		notes += fmt.Sprintf("- `%s`\n", filepath.Base(a))
	}
	notes += fmt.Sprintf("\nUpstream: %s\n", meta.Homepage)
	deps.Logf("Creating GitHub release %s...", version)
	if err := deps.GitHub.CreateRelease(ctx, ReleaseRequest{
		Tag: version, Title: meta.Name + " " + version, Notes: notes, Assets: assets, Latest: false, Recreate: cfg.Recreate,
	}); err != nil {
		return fmt.Errorf("create release %s: %w", version, err)
	}
	deps.Logf("✓ Released %s", version)
	return nil
}

func joinSpace(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += " "
		}
		out += s
	}
	return out
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no (local only)"
}
