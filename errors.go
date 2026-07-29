package foundation

import (
	"errors"
	"fmt"
)

// Sentinel and typed errors for callers and tests.

var (
	// ErrNoVersions means plan selected nothing to build.
	ErrNoVersions = errors.New("no versions to build")

	// ErrSkip is returned by optional hooks to mean "not implemented, skip".
	// Not used by the core engine today; reserved for optional interfaces.
	ErrSkip = errors.New("skip")
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
