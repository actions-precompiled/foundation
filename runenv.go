package foundation

import (
	"context"
	"time"
)

// OutputWithEnv runs name with an explicit process environment (replaces os.Environ).
// This is the standard atom for smoke and tool checks: pair with CleanSmokeEnv so
// packages never inject LD_LIBRARY_PATH. Invokes tools by absolute path.
func OutputWithEnv(ctx context.Context, deps Deps, env []string, name string, args ...string) (string, error) {
	if rw, ok := deps.Runner.(RunnerWithOpts); ok {
		return rw.OutputWith(ctx, RunOpts{Env: env}, name, args...)
	}
	return deps.Runner.Output(ctx, name, args...)
}

// OutputWithEnvTimeout is OutputWithEnv with a deadline (smoke helpers).
func OutputWithEnvTimeout(ctx context.Context, deps Deps, env []string, timeout time.Duration, name string, args ...string) (string, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	return OutputWithEnv(ctx, deps, env, name, args...)
}
