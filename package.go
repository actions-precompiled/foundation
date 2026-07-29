// Package foundation is the shared runtime for actions-precompiled package
// repositories. Each package implements [Package]; orchestration lives here.
package foundation

import "context"

// Package is the contract a precompiled package repository implements.
// There is no package.toml runtime and no shell build scripts: identity and
// build/smoke logic are Go. The host CLI mounts this binary into Docker and
// runs [Package.Work] inside the container for Linux targets.
type Package interface {
	// Meta returns static package identity used for planning, artifacts, and releases.
	Meta() Meta

	// Work produces artifacts for one version+target into req.OutDir.
	// Called in-process on the host (Windows) or inside the build container (Linux)
	// after the package binary is bind-mounted. Pure Go — no bash packaging scripts.
	Work(ctx context.Context, deps Deps, req BuildRequest) error

	// Smoke verifies artifacts for one version+target after Work.
	Smoke(ctx context.Context, deps Deps, req SmokeRequest) error
}

// HostPrep is an optional extension packages may implement.
// Host build orchestration calls PrepHost once before image/work.
type HostPrep interface {
	PrepHost(ctx context.Context, deps Deps, cfg Config) error
}

// Meta is static package identity. Prefer filling fields in code over env overrides.
type Meta struct {
	Name            string
	UpstreamRepoAPI string
	UpstreamGit     string
	ImageName       string
	Binary          string
	VersionEnv      string
	Description     string
	Homepage        string
	// DefaultTargets drives CI matrix generation and APC_TARGETS default.
	DefaultTargets []string
}

// Normalize fills derived Meta defaults.
func (m Meta) Normalize() Meta {
	if m.UpstreamGit == "" && m.UpstreamRepoAPI != "" {
		m.UpstreamGit = "https://github.com/" + m.UpstreamRepoAPI + ".git"
	}
	if m.Description == "" && m.Name != "" {
		m.Description = "Prebuilt " + m.Name + " (relocatable tree)."
	}
	if m.Homepage == "" && m.UpstreamRepoAPI != "" {
		m.Homepage = "https://github.com/" + m.UpstreamRepoAPI
	}
	if m.ImageName == "" && m.Name != "" {
		m.ImageName = m.Name + "-buildenv"
	}
	if m.VersionEnv == "" && m.Name != "" {
		m.VersionEnv = envNameForPackage(m.Name) + "_VERSION"
	}
	return m
}

// Validate reports required Meta fields.
func (m Meta) Validate() error {
	switch {
	case m.Name == "":
		return errMeta("Name")
	case m.UpstreamRepoAPI == "":
		return errMeta("UpstreamRepoAPI")
	case m.ImageName == "":
		return errMeta("ImageName")
	default:
		return nil
	}
}

// BuildRequest is one matrix cell for [Package.Work].
type BuildRequest struct {
	Version string
	Target  string
	// OutDir is where artifacts are written (host path or /out in container).
	OutDir string
}

// SmokeRequest is one matrix cell for [Package.Smoke].
type SmokeRequest struct {
	Version  string
	Target   string
	OutDir   string
	Tarballs []string
}

// ReleaseRequest is used by the default GitHub release helper.
type ReleaseRequest struct {
	Tag      string
	Title    string
	Notes    string
	Assets   []string
	Latest   bool
	Recreate bool
}
