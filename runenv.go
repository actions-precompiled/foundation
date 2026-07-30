package foundation

import (
	"bytes"
	"context"
	"io"
	"os"
	"time"
)

// OutputWithEnv runs name with an explicit process environment (replaces os.Environ).
// Returns stdout only. Prefer CombinedOutputWithEnv for smoke (--help often on stderr).
func OutputWithEnv(ctx context.Context, deps Deps, env []string, name string, args ...string) (string, error) {
	if rw, ok := deps.Runner.(RunnerWithOpts); ok {
		return rw.OutputWith(ctx, RunOpts{Env: env}, name, args...)
	}
	return deps.Runner.Output(ctx, name, args...)
}

// OutputWithEnvTimeout is OutputWithEnv with a deadline (stdout only).
func OutputWithEnvTimeout(ctx context.Context, deps Deps, env []string, timeout time.Duration, name string, args ...string) (string, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	return OutputWithEnv(ctx, deps, env, name, args...)
}

// CombinedOutputWithEnv returns stdout+stderr under an explicit env.
// Used by SmokeBinDirHelp: tools that print usage to stderr and exit 1 still "start".
func CombinedOutputWithEnv(ctx context.Context, deps Deps, env []string, name string, args ...string) (string, error) {
	return CombinedOutputWithEnvTimeout(ctx, deps, env, 0, name, args...)
}

// CombinedOutputWithEnvTimeout is CombinedOutputWithEnv with an optional deadline.
func CombinedOutputWithEnvTimeout(ctx context.Context, deps Deps, env []string, timeout time.Duration, name string, args ...string) (string, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	rw, ok := deps.Runner.(RunnerWithOpts)
	if !ok {
		return deps.Runner.Output(ctx, name, args...)
	}
	var stderrBuf bytes.Buffer
	logW := deps.Stderr
	if logW == nil {
		logW = os.Stderr
	}
	stdout, err := rw.OutputWith(ctx, RunOpts{
		Env:    env,
		Stderr: io.MultiWriter(&stderrBuf, logW),
	}, name, args...)
	return stdout + stderrBuf.String(), err
}
