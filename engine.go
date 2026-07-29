package foundation

import (
	"context"
	"fmt"
	"path/filepath"
)

// Run executes one foundation invocation: plan, dry-run, smoke-only, or build.
// Returns a nil error on success (including "nothing to do").
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

	versions, err := PlanVersions(ctx, deps, meta, cfg.Versions, cfg.ForceAll)
	if err != nil {
		return err
	}

	if cfg.ListToBuild {
		for _, v := range versions {
			deps.Outf("%s", v)
		}
		return nil
	}

	if len(versions) == 0 {
		deps.Logf("No versions to build.")
		return nil
	}

	deps.Logf("package=%s upstream=%s", meta.Name, meta.UpstreamRepoAPI)
	deps.Logf("")
	deps.Logf("Versions to build (%d):", len(versions))
	for _, v := range versions {
		deps.Logf("  - %s  [targets: %s]", v, joinSpace(cfg.Targets))
	}
	deps.Logf("Publish: %s", yesNo(cfg.Publish))
	smokeLabel := "yes"
	if cfg.SkipSmoke {
		smokeLabel = "no"
	}
	if cfg.SmokeOnly {
		smokeLabel = "only"
	}
	deps.Logf("Smoke: %s", smokeLabel)

	if cfg.DryRun {
		deps.Logf("Dry-run — exiting without building")
		return nil
	}

	if cfg.SmokeOnly {
		for _, version := range versions {
			if err := smokeVersion(ctx, p, deps, meta, cfg, version); err != nil {
				return err
			}
		}
		deps.Logf("")
		deps.Logf("✓ Smoke-only finished")
		return nil
	}

	if !cfg.SkipImageBuild {
		if err := ensureImage(ctx, deps, meta, cfg); err != nil {
			return err
		}
	} else {
		deps.Logf("APC_SKIP_IMAGE_BUILD set — using existing %s:%s", cfg.ImageName, cfg.ImageTag)
	}

	for _, version := range versions {
		if err := buildRelease(ctx, p, deps, meta, cfg, version); err != nil {
			return err
		}
	}

	deps.Logf("")
	deps.Logf("✓ All requested builds finished")
	return nil
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
		deps.Logf("")
		deps.Logf("Building %s for %s...", version, target)
		req := BuildRequest{
			Version: version,
			Target:  target,
			OutDir:  outDir,
		}
		if err := p.Build(ctx, deps, req); err != nil {
			return fmt.Errorf("build %s/%s: %w", version, target, err)
		}
	}

	deps.Logf("")
	deps.Logf("Built artifacts for %s:", version)
	for _, target := range cfg.Targets {
		d := filepath.Join(cfg.BuildOutputDir, target)
		if deps.Runner != nil {
			_ = deps.Runner.Run(ctx, "ls", "-lh", d)
		}
	}

	if !cfg.SkipSmoke {
		if err := smokeVersion(ctx, p, deps, meta, cfg, version); err != nil {
			return err
		}
	}

	if !cfg.Publish {
		deps.Logf("Local build only — pass --publish to create a GitHub release")
		return nil
	}

	return publishVersion(ctx, deps, meta, cfg, version)
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
		req := SmokeRequest{
			Version:  version,
			Target:   target,
			OutDir:   outDir,
			Tarballs: tarballs,
		}
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
		Tag:      version,
		Title:    meta.Name + " " + version,
		Notes:    notes,
		Assets:   assets,
		Latest:   false,
		Recreate: cfg.Recreate,
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
