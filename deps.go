package foundation

import (
	"context"
	"io"
	"os"
)

// Deps is the dependency bag injected into Package methods and the engine.
// Tests replace individual interfaces; production uses [DefaultDeps].
type Deps struct {
	// WorkDir is the package repository root (docker build context, etc.).
	WorkDir string

	Runner Runner
	Env    Environ
	GitHub GitHub
	Docker Docker
	FS     FileSystem

	Stdout io.Writer
	Stderr io.Writer
}

// Runner executes host processes (docker, gh, curl, binaries under test).
type Runner interface {
	// Run prints "+ cmd args" to stderr and runs the process.
	Run(ctx context.Context, name string, args ...string) error

	// Output runs a process and returns stdout.
	Output(ctx context.Context, name string, args ...string) (string, error)
}

// RunOpts optional overrides for a single invocation.
type RunOpts struct {
	Dir    string
	Env    []string // if non-nil, replaces environment
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// RunnerWithOpts is an optional extension; DefaultRunner implements it.
type RunnerWithOpts interface {
	Runner
	RunWith(ctx context.Context, opts RunOpts, name string, args ...string) error
	OutputWith(ctx context.Context, opts RunOpts, name string, args ...string) (string, error)
}

// Environ is an abstract environment. Prefer reading APC_* keys via config helpers.
type Environ interface {
	Get(key string) string
	Lookup(key string) (string, bool)
	// Environ returns KEY=value pairs (like os.Environ).
	Environ() []string
}

// GitHub talks to GitHub for planning and publishing.
// Default implementation uses curl + gh so TLS uses the system CA store.
type GitHub interface {
	ListUpstreamTags(ctx context.Context, ownerRepo string) ([]string, error)
	ListReleasedTags(ctx context.Context) ([]string, error)
	LatestReleaseTag(ctx context.Context, ownerRepo string) (string, error)
	CreateRelease(ctx context.Context, req ReleaseRequest) error
	DeleteRelease(ctx context.Context, tag string) error
}

// Docker builds and runs package build containers.
type Docker interface {
	BuildImage(ctx context.Context, req DockerBuildRequest) error
	Run(ctx context.Context, req DockerRunRequest) error
}

// DockerBuildRequest is a docker build invocation.
type DockerBuildRequest struct {
	Context    string // directory
	Image      string // name:tag
	TargetArch string // amd64 / arm64 → TARGETARCH build-arg
	BuildArgs  map[string]string
}

// DockerRunRequest is a docker run invocation for a package build.
type DockerRunRequest struct {
	Image      string
	User       string // uid:gid, empty = default
	Binds      []string
	Env        map[string]string
	WorkDir    string
	Entrypoint []string
	Cmd        []string
}

// FileSystem is a narrow FS surface for tests.
type FileSystem interface {
	MkdirAll(path string, perm os.FileMode) error
	Stat(name string) (os.FileInfo, error)
	TempDir(dir, prefix string) (string, error)
	RemoveAll(path string) error
	ReadDir(name string) ([]os.DirEntry, error)
	Glob(pattern string) ([]string, error)
	WriteFile(name string, data []byte, perm os.FileMode) error
	ReadFile(name string) ([]byte, error)
}
