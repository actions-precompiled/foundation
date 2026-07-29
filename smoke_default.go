package foundation

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SmokeBinaryHelp is a default smoke: extract each tarball, require bin/<Binary>,
// try --help / --version / -V / -h, fail on dynamic linker errors.
// Packages with custom needs implement Smoke themselves (e.g. Xvfb for quickshell).
func SmokeBinaryHelp(ctx context.Context, deps Deps, meta Meta, req SmokeRequest) error {
	meta = meta.Normalize()
	if meta.Binary == "" {
		return fmt.Errorf("SmokeBinaryHelp: Meta.Binary is required")
	}
	if len(req.Tarballs) == 0 {
		return fmt.Errorf("SmokeBinaryHelp: no tarballs")
	}
	for _, tb := range req.Tarballs {
		if err := smokeOneTarball(ctx, deps, meta, tb); err != nil {
			return err
		}
	}
	return nil
}

func smokeOneTarball(ctx context.Context, deps Deps, meta Meta, tarball string) error {
	deps.Logf("")
	deps.Logf("Smoke test: %s", filepath.Base(tarball))
	if _, err := deps.FS.Stat(tarball); err != nil {
		return fmt.Errorf("tarball missing: %s", tarball)
	}

	tmp, err := deps.FS.TempDir("", meta.Name+"-smoke-")
	if err != nil {
		return err
	}
	defer func() { _ = deps.FS.RemoveAll(tmp) }()

	if err := extractTarGz(tarball, tmp); err != nil {
		return fmt.Errorf("extract %s: %w", tarball, err)
	}

	entries, err := deps.FS.ReadDir(tmp)
	if err != nil {
		return err
	}
	var roots []string
	for _, e := range entries {
		if e.IsDir() {
			roots = append(roots, filepath.Join(tmp, e.Name()))
		}
	}
	if len(roots) != 1 {
		names := make([]string, 0, len(roots))
		for _, r := range roots {
			names = append(names, filepath.Base(r))
		}
		return fmt.Errorf("expected one top-level dir, got: %v", names)
	}
	prefix := roots[0]
	binPath := filepath.Join(prefix, "bin", meta.Binary)
	if _, err := deps.FS.Stat(binPath); err != nil {
		return fmt.Errorf("missing bin/%s in package", meta.Binary)
	}

	buildinfo := filepath.Join(prefix, "BUILDINFO.txt")
	if data, err := deps.FS.ReadFile(buildinfo); err == nil {
		deps.Logf("--- BUILDINFO ---")
		deps.Logf("%s", strings.TrimSpace(string(data)))
	}

	libDir := filepath.Join(prefix, "lib")
	env := append([]string(nil), deps.Env.Environ()...)
	if st, err := deps.FS.Stat(libDir); err == nil && st.IsDir() {
		env = prependEnv(env, "LD_LIBRARY_PATH", libDir)
	}

	var lastOut string
	var lastCode error
	tried := [][]string{{"--help"}, {"--version"}, {"-V"}, {"-h"}}
	for _, args := range tried {
		cmdArgs := append([]string{binPath}, args...)
		// Use runner with env
		out, err := runWithEnv(ctx, deps, env, binPath, args...)
		lastOut = out
		lastCode = err
		for i, line := range strings.Split(out, "\n") {
			if i >= 12 {
				break
			}
			if strings.TrimSpace(line) != "" {
				deps.Logf("  %s", line)
			}
		}
		low := strings.ToLower(out)
		if strings.Contains(low, "error while loading shared libraries") ||
			strings.Contains(low, "cannot open shared object") {
			return fmt.Errorf("smoke failed for %v: dynamic linker error", args)
		}
		if strings.TrimSpace(out) != "" || err == nil {
			deps.Logf("✓ Smoke test passed: %s", filepath.Base(tarball))
			return nil
		}
		_ = cmdArgs
	}
	if lastCode != nil && strings.TrimSpace(lastOut) == "" {
		return fmt.Errorf("smoke failed: binary produced no output (%v)", tried[len(tried)-1])
	}
	deps.Logf("✓ Smoke test passed: %s", filepath.Base(tarball))
	return nil
}

func runWithEnv(ctx context.Context, deps Deps, env []string, name string, args ...string) (string, error) {
	// Bound runtime for smoke.
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	if rw, ok := deps.Runner.(RunnerWithOpts); ok {
		return rw.OutputWith(ctx, RunOpts{Env: env}, name, args...)
	}
	return deps.Runner.Output(ctx, name, args...)
}

func prependEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, e := range env {
		if strings.HasPrefix(e, prefix) {
			env[i] = prefix + value + ":" + strings.TrimPrefix(e, prefix)
			return env
		}
	}
	return append(env, prefix+value)
}

func extractTarGz(src, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// Basic path hygiene
		name := filepath.Clean(hdr.Name)
		if strings.HasPrefix(name, "..") || filepath.IsAbs(name) {
			return fmt.Errorf("refusing unsafe path in tarball: %s", hdr.Name)
		}
		target := filepath.Join(dst, name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			mode := os.FileMode(hdr.Mode)
			if mode == 0 {
				mode = 0o644
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			_ = os.Remove(target)
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		}
	}
	return nil
}
