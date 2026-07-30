package foundation

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// DefaultRunner shells out with exec.CommandContext and logs "+ cmd".
type DefaultRunner struct {
	Stderr io.Writer
}

// NewDefaultRunner returns a runner that logs to stderrW (or os.Stderr).
func NewDefaultRunner(stderrW io.Writer) *DefaultRunner {
	if stderrW == nil {
		stderrW = os.Stderr
	}
	return &DefaultRunner{Stderr: stderrW}
}

func (r *DefaultRunner) log(name string, args []string) {
	WriteLine(r.Stderr, os.Stderr, "+ %s", strings.Join(append([]string{name}, args...), " "))
}

// command builds an *exec.Cmd with RunOpts applied (shared by RunWith/OutputWith).
func (r *DefaultRunner) command(ctx context.Context, opts RunOpts, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}
	if opts.Env != nil {
		cmd.Env = opts.Env
	}
	cmd.Stdin = opts.Stdin
	return cmd
}

func (r *DefaultRunner) stderrFor(opts RunOpts) io.Writer {
	if opts.Stderr != nil {
		return opts.Stderr
	}
	if r.Stderr != nil {
		return r.Stderr
	}
	return os.Stderr
}

func (r *DefaultRunner) Run(ctx context.Context, name string, args ...string) error {
	return r.RunWith(ctx, RunOpts{}, name, args...)
}

func (r *DefaultRunner) Output(ctx context.Context, name string, args ...string) (string, error) {
	return r.OutputWith(ctx, RunOpts{}, name, args...)
}

func (r *DefaultRunner) RunWith(ctx context.Context, opts RunOpts, name string, args ...string) error {
	r.log(name, args)
	cmd := r.command(ctx, opts, name, args...)
	if opts.Stdout != nil {
		cmd.Stdout = opts.Stdout
	} else {
		cmd.Stdout = os.Stdout
	}
	cmd.Stderr = r.stderrFor(opts)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func (r *DefaultRunner) OutputWith(ctx context.Context, opts RunOpts, name string, args ...string) (string, error) {
	r.log(name, args)
	cmd := r.command(ctx, opts, name, args...)
	var stdout bytes.Buffer
	if opts.Stdout != nil {
		cmd.Stdout = io.MultiWriter(&stdout, opts.Stdout)
	} else {
		cmd.Stdout = &stdout
	}
	cmd.Stderr = r.stderrFor(opts)
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("%s: %w", name, err)
	}
	return stdout.String(), nil
}
