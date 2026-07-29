package foundation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// EnsureWorkerBinary returns a path to a package binary for goos/goarch suitable
// to bind-mount into Docker (or run natively). When the current process is already
// a real binary for that platform, it is reused; otherwise `go build` writes a temp file.
//
// cleanup must be called when done (no-op if reusing the current executable).
func EnsureWorkerBinary(ctx context.Context, deps Deps, goos, goarch string) (path string, cleanup func(), err error) {
	cleanup = func() {}
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}

	if goos == runtime.GOOS && goarch == runtime.GOARCH {
		if exe, err := os.Executable(); err == nil {
			// Prefer a built binary over `go run` temp paths when possible.
			// go run still produces a real executable we can mount.
			if st, err := os.Stat(exe); err == nil && st.Mode().IsRegular() && st.Size() > 0 {
				return exe, cleanup, nil
			}
		}
	}

	if deps.Runner == nil {
		return "", cleanup, fmt.Errorf("EnsureWorkerBinary: %w", ErrRunnerNil)
	}
	if deps.WorkDir == "" {
		return "", cleanup, fmt.Errorf("%w", ErrWorkerWorkDir)
	}

	dir, err := deps.FS.TempDir("", "apc-worker-")
	if err != nil {
		return "", cleanup, err
	}
	cleanup = func() { deps.RemoveAllLog(dir, "worker cleanup") }

	out := filepath.Join(dir, "apc")
	if goos == "windows" {
		out += ".exe"
	}

	// go build from package module root
	env := append([]string{}, deps.Env.Environ()...)
	env = setEnvKV(env, "GOOS", goos)
	env = setEnvKV(env, "GOARCH", goarch)
	env = setEnvKV(env, "CGO_ENABLED", "0")

	args := []string{"build", "-o", out, "."}
	if rw, ok := deps.Runner.(RunnerWithOpts); ok {
		if err := rw.RunWith(ctx, RunOpts{Dir: deps.WorkDir, Env: env, Stderr: deps.Stderr}, "go", args...); err != nil {
			cleanup()
			return "", func() {}, fmt.Errorf("go build worker (%s/%s): %w", goos, goarch, err)
		}
	} else {
		// best-effort without env override
		if err := deps.Runner.Run(ctx, "go", args...); err != nil {
			cleanup()
			return "", func() {}, fmt.Errorf("go build worker: %w", err)
		}
	}
	return out, cleanup, nil
}

func setEnvKV(env []string, key, value string) []string {
	prefix := key + "="
	for i, e := range env {
		if strings.HasPrefix(e, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

// TargetGOOSGOARCH maps package targets to GOOS/GOARCH for worker builds.
func TargetGOOSGOARCH(target string) (goos, goarch string) {
	switch {
	case strings.HasPrefix(target, "windows-"):
		goos = "windows"
	default:
		goos = "linux"
	}
	switch {
	case strings.Contains(target, "aarch64") || strings.Contains(target, "arm64"):
		goarch = "arm64"
	default:
		goarch = "amd64"
	}
	return goos, goarch
}

// RunWorkInDocker mounts the worker binary and runs `work` inside the image.
func RunWorkInDocker(ctx context.Context, deps Deps, meta Meta, imageName, imageTag string, req BuildRequest, workerPath string) error {
	meta = meta.Normalize()
	if deps.Docker == nil {
		return fmt.Errorf("RunWorkInDocker: %w", ErrDockerNil)
	}
	image := imageName + ":" + imageTag
	if imageTag == "" {
		image = imageName + ":local"
	}

	env := map[string]string{
		EnvInContainer:    "1",
		EnvVersion:        req.Version,
		"PACKAGE_VERSION": req.Version,
		"PACKAGE_NAME":    meta.Name,
		"UPSTREAM_REPO":   meta.UpstreamGit,
		EnvTarget:         req.Target,
		"BUILD_TARGET":    req.Target,
		EnvOutputDir:      "/out",
		"OUTPUT_DIR":      "/out",
		"HOME":            "/tmp",
		"XDG_CACHE_HOME":  "/tmp/.cache",
		"LANG":            "C.UTF-8",
		"LC_ALL":          "C.UTF-8",
	}
	if meta.VersionEnv != "" {
		env[meta.VersionEnv] = req.Version
	}
	// Forward optional knobs from host.
	for _, key := range []string{
		"LLVM_ENABLE_PROJECTS", "LLVM_ENABLE_RUNTIMES", "LLVM_TARGETS_TO_BUILD",
		"LLVM_PARALLEL_LINK_JOBS", "JOBS", "CCACHE_DIR",
	} {
		if v := deps.Env.Get(key); v != "" {
			env[key] = v
		}
	}

	return deps.Docker.Run(ctx, DockerRunRequest{
		Image:      image,
		User:       HostDockerUser(),
		Binds:      []string{req.OutDir + ":/out", workerPath + ":/apc:ro"},
		Env:        env,
		Entrypoint: []string{"/apc"},
		Cmd:        []string{"work"},
	})
}
