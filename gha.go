package foundation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

// GitHub Actions environment file keys (see docs.github.com/actions/writing-workflows).
const (
	EnvGitHubOutput      = "GITHUB_OUTPUT"
	EnvGitHubStepSummary = "GITHUB_STEP_SUMMARY"
	EnvGitHubEventName   = "GITHUB_EVENT_NAME"
)

// AppendGitHubOutput appends key=value lines to $GITHUB_OUTPUT (no-op if unset).
// Values with newlines use the multiline delimiter form.
func AppendGitHubOutput(env Environ, fs FileSystem, pairs map[string]string) error {
	path := env.Get(EnvGitHubOutput)
	if path == "" {
		return nil
	}
	var b strings.Builder
	for k, v := range pairs {
		if strings.ContainsAny(v, "\n\r") {
			delim := "APC_EOF"
			fmt.Fprintf(&b, "%s<<%s\n%s\n%s\n", k, delim, v, delim)
			continue
		}
		fmt.Fprintf(&b, "%s=%s\n", k, v)
	}
	return appendFile(fs, path, b.String())
}

// AppendStepSummary appends markdown to $GITHUB_STEP_SUMMARY (no-op if unset).
// Callers pass plain markdown — do not wrap values in shell backticks.
func AppendStepSummary(env Environ, fs FileSystem, markdown string) error {
	path := env.Get(EnvGitHubStepSummary)
	if path == "" {
		return nil
	}
	if !strings.HasSuffix(markdown, "\n") {
		markdown += "\n"
	}
	return appendFile(fs, path, markdown)
}

// WriteBuildPlanSummary writes a standard package build plan block to the step summary.
func WriteBuildPlanSummary(env Environ, fs FileSystem, packageName, version string, publish, recreate bool) error {
	pub := "false"
	if publish {
		pub = "true"
	}
	rec := "false"
	if recreate {
		rec = "true"
	}
	// Markdown code spans without shell-interpreted backticks in YAML: build string in Go.
	md := fmt.Sprintf("## Build plan\n\n- Package: `%s`\n- Version: `%s`\n- Publish: **%s**\n- Recreate: **%s**\n",
		packageName, version, pub, rec)
	return AppendStepSummary(env, fs, md)
}

// WriteDispatchPlanSummary writes a dispatch fan-out summary.
func WriteDispatchPlanSummary(env Environ, fs FileSystem, packageName string, versions []string, publish, recreate bool, maxN int) error {
	pub := "false"
	if publish {
		pub = "true"
	}
	rec := "false"
	if recreate {
		rec = "true"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Dispatch plan (%s)\n\n", packageName)
	fmt.Fprintf(&b, "- Count: **%d**\n", len(versions))
	fmt.Fprintf(&b, "- Publish: **%s**\n", pub)
	fmt.Fprintf(&b, "- Recreate: **%s**\n", rec)
	if maxN > 0 {
		fmt.Fprintf(&b, "- Max cap: `%d`\n", maxN)
	}
	if len(versions) == 0 {
		b.WriteString("\n_Nothing to dispatch._\n")
	} else {
		b.WriteString("\nVersions:\n")
		for _, v := range versions {
			fmt.Fprintf(&b, "- `%s`\n", v)
		}
	}
	return AppendStepSummary(env, fs, b.String())
}

func appendFile(fs FileSystem, path, content string) error {
	prev, err := fs.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) && !os.IsNotExist(err) {
		// Some FS mocks return empty error; try write fresh
		prev = nil
	}
	data := append(prev, []byte(content)...)
	return fs.WriteFile(path, data, 0o644)
}

// CIPlanInput is read from the environment for the `plan` subcommand (GHA plan job).
type CIPlanInput struct {
	EventName string // GITHUB_EVENT_NAME
	Version   string // INPUT_VERSION / APC_VERSION (must be a real tag for publish)
	Publish   bool   // INPUT_PUBLISH
	Recreate  bool   // INPUT_RECREATE
	// DefaultVersion, if set, is used on push/PR when no input.
	// Empty means: resolve the latest upstream release tag (not trunk/main).
	DefaultVersion string
}

// ResolveCIPlan picks version/publish/recreate for a Build workflow plan job.
// When version is empty on push/PR, needLatest is true and RunCIPlan fetches the
// latest upstream release tag. Trunk/main are never default release identities.
func ResolveCIPlan(in CIPlanInput) (version string, publish, recreate, needLatest bool, err error) {
	version = strings.TrimSpace(in.Version)
	if in.EventName == "workflow_dispatch" {
		if version == "" {
			return "", false, false, false, fmt.Errorf("version required on workflow_dispatch (use a real upstream tag)")
		}
		if !IsPublishableTag(version) && in.Publish {
			return "", false, false, false, fmt.Errorf("refusing to publish non-tag %q (tagged releases only)", version)
		}
		publish = in.Publish
		recreate = in.Recreate && publish
		return version, publish, recreate, false, nil
	}
	// push / pull_request: smoke-build one tag, never publish by default
	if version != "" {
		return version, false, false, false, nil
	}
	if in.DefaultVersion != "" {
		return strings.TrimSpace(in.DefaultVersion), false, false, false, nil
	}
	return "", false, false, true, nil
}

// RunCIPlan writes GITHUB_OUTPUT + step summary for the thin Build plan job.
func RunCIPlan(ctx context.Context, deps Deps, meta Meta, in CIPlanInput) error {
	meta = meta.Normalize()
	version, publish, recreate, needLatest, err := ResolveCIPlan(in)
	if err != nil {
		return err
	}
	if needLatest {
		if deps.GitHub == nil {
			return fmt.Errorf("plan: need latest upstream tag but GitHub client is nil")
		}
		version, err = deps.GitHub.LatestReleaseTag(ctx, meta.UpstreamRepoAPI)
		if err != nil {
			// Fallback: list tags and take newest by Version sort
			tags, err2 := deps.GitHub.ListUpstreamTags(ctx, meta.UpstreamRepoAPI)
			if err2 != nil {
				return fmt.Errorf("latest upstream tag: %w (fallback list: %v)", err, err2)
			}
			tags = SortVersionStrings(tags)
			if len(tags) == 0 {
				return fmt.Errorf("no upstream tags for %s", meta.UpstreamRepoAPI)
			}
			version = tags[len(tags)-1]
			deps.Logf("plan: LatestReleaseTag failed (%v); using newest listed tag %s", err, version)
		}
	}
	if version == "" {
		return fmt.Errorf("plan: empty version")
	}
	if err := AppendGitHubOutput(deps.Env, deps.FS, map[string]string{
		"version":  version,
		"publish":  boolStr(publish),
		"recreate": boolStr(recreate),
	}); err != nil {
		return fmt.Errorf("GITHUB_OUTPUT: %w", err)
	}
	if err := WriteBuildPlanSummary(deps.Env, deps.FS, meta.Name, version, publish, recreate); err != nil {
		return fmt.Errorf("step summary: %w", err)
	}
	deps.Logf("plan package=%s version=%s publish=%v recreate=%v", meta.Name, version, publish, recreate)
	return nil
}

// IsPublishableTag reports whether s is a real release tag (not trunk/main/latest).
func IsPublishableTag(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	v := ParseVersion(s)
	if v.IsTrunk() || v.IsLatest() {
		return false
	}
	// bare branch names
	switch strings.ToLower(s) {
	case "main", "master", "head", "develop", "development":
		return false
	}
	return true
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// ParseEnvBool is true for true/1/yes (case-insensitive).
func ParseEnvBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes":
		return true
	default:
		return false
	}
}
