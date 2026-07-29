package foundation

import (
	"errors"
	"fmt"
)

// Sentinel errors for callers, tests, and fmt.Errorf %w wrapping.
var (
	// ErrNoVersions means plan selected nothing to build.
	ErrNoVersions = errors.New("no versions to build")

	// ErrSkip is returned by optional hooks to mean "not implemented, skip".
	ErrSkip = errors.New("skip")

	ErrWorkDirRequired       = errors.New("WorkDir is required on Deps or Config")
	ErrUnknownCommand        = errors.New("unknown command")
	ErrWorkVersionRequired   = errors.New("work requires exactly one version; set APC_VERSION")
	ErrWorkTargetRequired    = errors.New("work requires exactly one target; set APC_TARGET")
	ErrDockerNil             = errors.New("Docker client is nil")
	ErrGitHubNil             = errors.New("GitHub client is nil")
	ErrNoTarball             = errors.New("no tarball for version/target")
	ErrNotPublishableTag     = errors.New("not a tagged release (refusing trunk/main/latest)")
	ErrNoPublishAssets       = errors.New("no assets to publish")
	ErrNoTargets             = errors.New("no targets resolved")
	ErrVersionRequired       = errors.New("version required on workflow_dispatch (use a real upstream tag)")
	ErrNonTagPublish         = errors.New("refusing to publish non-tag (tagged releases only)")
	ErrEmptyPlanVersion      = errors.New("plan: empty version")
	ErrNoUpstreamTags        = errors.New("no upstream tags")
	ErrLatestTagNeedsGitHub  = errors.New("plan: need latest upstream tag but GitHub client is nil")
	ErrFSNil                 = errors.New("FileSystem is nil")
	ErrRunnerNil             = errors.New("Runner is nil")
	ErrWorkerWorkDir         = errors.New("EnsureWorkerBinary: WorkDir is required")
	ErrSmokeBinaryRequired   = errors.New("SmokeBinaryHelp: Meta.Binary is required")
	ErrSmokeNoTarballs       = errors.New("SmokeBinaryHelp: no tarballs")
	ErrTarballMissing        = errors.New("tarball missing")
	ErrMissingPackageBinary  = errors.New("missing package binary under bin/")
	ErrSmokeDynamicLink      = errors.New("dynamic linker error (package must be relocatable without LD_LIBRARY_PATH)")
	ErrSmokeNoOutput         = errors.New("smoke failed: binary produced no output")
	ErrBadTarballLayout      = errors.New("expected one top-level dir in tarball")
	ErrUnsafeTarballPath     = errors.New("refusing unsafe path in tarball")
	ErrWorkflowExists        = errors.New("workflow file exists (pass --force to overwrite)")
	ErrDockerImageRequired   = errors.New("docker build: Image is required")
	ErrDockerContextRequired = errors.New("docker build: Context is required")
	ErrDockerRunImage        = errors.New("docker run: Image is required")
	ErrRelocatablePrefix     = errors.New("relocatable check: invalid prefix")
	ErrRelocatableBinMissing = errors.New("relocatable check: required bin missing")
	ErrRelocatableNoELF      = errors.New("relocatable check: package ships shared libs but no dynamic ELF under bin/ was inspected")
	ErrRelocatableFailed     = errors.New("relocatable check failed")
	ErrEmptyReleaseTag       = errors.New("latest release has empty tag_name")
)

type metaError struct {
	field string
}

func (e metaError) Error() string {
	return fmt.Sprintf("meta.%s is required", e.field)
}

func errMeta(field string) error {
	return metaError{field: field}
}

// ExitError carries a process exit code for Main.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit %d", e.Code)
	}
	return e.Err.Error()
}

func (e *ExitError) Unwrap() error { return e.Err }
