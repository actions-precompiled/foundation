package foundation

import (
	"os"
	"path/filepath"
)

// DefaultDeps constructs production dependencies rooted at workDir.
// workDir may be "." ; it is cleaned to an absolute path when possible.
func DefaultDeps(workDir string) Deps {
	if workDir == "" {
		workDir = "."
	}
	if abs, err := filepath.Abs(workDir); err == nil {
		workDir = abs
	}

	stderr := os.Stderr
	runner := NewDefaultRunner(stderr)
	env := OSEnviron{}

	return Deps{
		WorkDir: workDir,
		Runner:  runner,
		Env:     env,
		GitHub:  NewDefaultGitHub(runner, env, stderr),
		Docker:  NewDefaultDocker(runner),
		FS:      OSFileSystem{},
		Stdout:  os.Stdout,
		Stderr:  stderr,
	}
}

func finalizeDeps(d Deps) Deps {
	if d.Stdout == nil {
		d.Stdout = os.Stdout
	}
	if d.Stderr == nil {
		d.Stderr = os.Stderr
	}
	if d.Env == nil {
		d.Env = OSEnviron{}
	}
	if d.FS == nil {
		d.FS = OSFileSystem{}
	}
	if d.Runner == nil {
		d.Runner = NewDefaultRunner(d.Stderr)
	}
	if d.GitHub == nil {
		d.GitHub = NewDefaultGitHub(d.Runner, d.Env, d.Stderr)
	}
	if d.Docker == nil {
		d.Docker = NewDefaultDocker(d.Runner)
	}
	return d
}
