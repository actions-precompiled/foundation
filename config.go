package foundation

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Config is the fully resolved CLI/engine configuration for one invocation.
// Built from argv + APC_* env — never from templated run lines.
type Config struct {
	// Versions are explicit tags from argv. Empty means plan from upstream.
	Versions []string

	Targets []string

	Publish        bool
	DryRun         bool
	SkipSmoke      bool
	SmokeOnly      bool
	ListToBuild    bool
	ForceAll       bool // all upstream tags (recreate fan-out)
	SkipImageBuild bool
	Recreate       bool // delete+recreate release when publishing

	BuildOutputDir string
	ImageName      string // override Meta.ImageName when set
	ImageTag       string

	// WorkDir package root; empty → deps.WorkDir / cwd.
	WorkDir string
}

// Flags are parsed CLI switches before merging with environment.
type Flags struct {
	Versions       []string
	Publish        bool
	DryRun         bool
	SkipSmoke      bool
	SmokeOnly      bool
	ListToBuild    bool
	All            bool
	SkipImageBuild bool
	Recreate       bool
	// Targets raw override (optional flag); empty means use env/meta.
	Targets string
	// BuildOutputDir optional flag.
	BuildOutputDir string
	ImageName      string
	ImageTag       string
	WorkDir        string
}

// ResolveConfig merges flags, APC_* env, and package meta into a Config.
func ResolveConfig(env Environ, meta Meta, flags Flags) (Config, error) {
	meta = meta.Normalize()
	if err := meta.Validate(); err != nil {
		return Config{}, err
	}

	cfg := Config{
		Versions:       append([]string(nil), flags.Versions...),
		Publish:        flags.Publish || EnvFlag(env, EnvPublish),
		DryRun:         flags.DryRun || EnvFlag(env, EnvDryRun),
		SkipSmoke:      flags.SkipSmoke || EnvFlag(env, EnvSkipSmoke),
		SmokeOnly:      flags.SmokeOnly,
		ListToBuild:    flags.ListToBuild,
		ForceAll:       flags.All || EnvFlag(env, EnvRecreate) || EnvFlag(env, EnvForceAll),
		SkipImageBuild: flags.SkipImageBuild || EnvFlag(env, EnvSkipImageBuild),
		Recreate:       flags.Recreate || EnvFlag(env, EnvRecreate),
		ImageTag:       firstNonEmpty(flags.ImageTag, env.Get(EnvImageTag), "local"),
		WorkDir:        firstNonEmpty(flags.WorkDir, env.Get(EnvWorkDir)),
	}

	cfg.ImageName = firstNonEmpty(flags.ImageName, env.Get(EnvImageName), meta.ImageName)

	outDir := firstNonEmpty(flags.BuildOutputDir, env.Get(EnvBuildOutputDir))
	if outDir == "" {
		wd := cfg.WorkDir
		if wd == "" {
			wd = "."
		}
		outDir = filepath.Join(wd, "target")
	}
	abs, err := filepath.Abs(outDir)
	if err != nil {
		return Config{}, fmt.Errorf("build output dir: %w", err)
	}
	cfg.BuildOutputDir = abs

	if flags.Targets != "" {
		cfg.Targets = ParseTargets(flags.Targets)
	} else {
		cfg.Targets = ResolveTargets(env, meta)
	}
	if len(cfg.Targets) == 0 {
		return Config{}, fmt.Errorf("no targets resolved")
	}

	return cfg, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
