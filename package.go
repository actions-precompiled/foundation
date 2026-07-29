// Package foundation is the shared runtime for actions-precompiled package
// repositories. Each package implements [Package]; orchestration lives here.
package foundation

import "context"

// Package is the contract a precompiled package repository implements.
// There is no package.toml runtime: identity and build/smoke logic are code.
type Package interface {
	// Meta returns static package identity used for planning, artifacts, and releases.
	Meta() Meta

	// Build produces artifacts for one version+target into req.OutDir.
	// The foundation calls this once per matrix cell; packages usually puppet Docker.
	Build(ctx context.Context, deps Deps, req BuildRequest) error

	// Smoke verifies artifacts for one version+target after Build (or --smoke-only).
	// Packages own their smoke logic (binary --help, Xvfb, etc.).
	Smoke(ctx context.Context, deps Deps, req SmokeRequest) error
}

// Meta is static package identity. Prefer filling fields in code over env overrides.
type Meta struct {
	// Name is the short package name used in artifact names and release titles.
	// Example: "quickshell" → quickshell-0.3.0-linux-amd64.tar.gz
	Name string

	// UpstreamRepoAPI is the GitHub "owner/repo" used to list tags/releases.
	UpstreamRepoAPI string

	// UpstreamGit is the git URL cloned inside the build container.
	// If empty, defaults to https://github.com/{UpstreamRepoAPI}.git.
	UpstreamGit string

	// ImageName is the Docker image name built for this package.
	ImageName string

	// Binary is the expected executable under bin/ (used by helpers, not required).
	Binary string

	// VersionEnv is the env var name passed into the container with the version.
	// Example: "QUICKSHELL_VERSION".
	VersionEnv string

	// Description appears in release notes.
	Description string

	// Homepage appears in release notes.
	Homepage string

	// DefaultTargets is the multi-arch matrix when APC_TARGETS is unset.
	// Empty means host-native single target only (local-friendly).
	DefaultTargets []string
}

// Normalize fills derived Meta defaults. Does not invent missing Name/upstream.
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

// BuildRequest is one matrix cell for [Package.Build].
type BuildRequest struct {
	Version string
	Target  string
	// OutDir is the host directory mounted/written for this target (usually
	// {BuildOutputDir}/{target}). Absolute path.
	OutDir string
}

// SmokeRequest is one matrix cell for [Package.Smoke].
type SmokeRequest struct {
	Version  string
	Target   string
	OutDir   string
	Tarballs []string // absolute paths; may be empty if package looks them up
}

// ReleaseRequest is used by the default GitHub release helper.
type ReleaseRequest struct {
	Tag    string
	Title  string
	Notes  string
	Assets []string // absolute paths
	Latest bool
	// Recreate deletes an existing release+tag before create when true.
	Recreate bool
}
