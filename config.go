package foundation

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Command is the CLI verb after cobra routing.
type Command string

const (
	CommandList             Command = "list"
	CommandBuild            Command = "build"
	CommandSmoke            Command = "smoke"
	CommandPublish          Command = "publish"
	CommandWork             Command = "work"
	CommandGenerateWorkflow Command = "generate-workflow"
	CommandPlan             Command = "plan"
)

// Config is the fully resolved configuration for one invocation.
type Config struct {
	Command Command

	Versions []string
	Targets  []string

	Publish        bool
	DryRun         bool
	SkipSmoke      bool
	ForceAll       bool
	SkipImageBuild bool
	Recreate       bool

	BuildOutputDir string
	ImageName      string
	ImageTag       string
	WorkDir        string

	// Generate workflow options
	WorkflowDir string
	ForceWrite  bool
}

// Flags are shared optional flags merged with APC_* env.
type Flags struct {
	Versions       []string
	Publish        bool
	DryRun         bool
	SkipSmoke      bool
	All            bool
	SkipImageBuild bool
	Recreate       bool
	Targets        string
	BuildOutputDir string
	ImageName      string
	ImageTag       string
	WorkDir        string
	WorkflowDir    string
	ForceWrite     bool
}

// ResolveConfig merges flags, APC_* env, and package meta into a Config.
func ResolveConfig(env Environ, meta Meta, flags Flags, cmd Command) (Config, error) {
	meta = meta.Normalize()
	if err := meta.Validate(); err != nil {
		return Config{}, err
	}

	cfg := Config{
		Command:        cmd,
		Versions:       append([]string(nil), flags.Versions...),
		Publish:        flags.Publish || EnvFlag(env, EnvPublish),
		DryRun:         flags.DryRun || EnvFlag(env, EnvDryRun),
		SkipSmoke:      flags.SkipSmoke || EnvFlag(env, EnvSkipSmoke),
		ForceAll:       flags.All || EnvFlag(env, EnvRecreate) || EnvFlag(env, EnvForceAll),
		SkipImageBuild: flags.SkipImageBuild || EnvFlag(env, EnvSkipImageBuild),
		Recreate:       flags.Recreate || EnvFlag(env, EnvRecreate),
		ImageTag:       firstNonEmpty(flags.ImageTag, env.Get(EnvImageTag), "local"),
		WorkDir:        firstNonEmpty(flags.WorkDir, env.Get(EnvWorkDir)),
		WorkflowDir:    firstNonEmpty(flags.WorkflowDir, ".github/workflows"),
		ForceWrite:     flags.ForceWrite,
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
	} else if t := env.Get(EnvTargets); t != "" {
		cfg.Targets = ParseTargets(t)
	} else if cmd == CommandWork {
		// work reads single target from BUILD_TARGET / APC_TARGET
		if bt := firstNonEmpty(env.Get(EnvTarget), env.Get("BUILD_TARGET")); bt != "" {
			cfg.Targets = []string{bt}
		}
	} else {
		cfg.Targets = ResolveTargets(env, meta)
	}
	if cmd != CommandList && cmd != CommandGenerateWorkflow && cmd != CommandWork && cmd != CommandPlan {
		if len(cfg.Targets) == 0 {
			return Config{}, fmt.Errorf("%w", ErrNoTargets)
		}
	}

	// work: version from env if not on argv
	if cmd == CommandWork && len(cfg.Versions) == 0 {
		if v := firstNonEmpty(env.Get(EnvVersion), env.Get("PACKAGE_VERSION"), env.Get(meta.VersionEnv)); v != "" {
			cfg.Versions = []string{v}
		}
	}
	if cmd == CommandWork {
		if od := firstNonEmpty(env.Get(EnvOutputDir), env.Get("OUTPUT_DIR")); od != "" {
			abs, err := filepath.Abs(od)
			if err != nil {
				return Config{}, err
			}
			cfg.BuildOutputDir = abs
		}
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
