package foundation

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// SmokeBinaryHelp is a default smoke: extract each tarball, require bin/<Binary>,
// verify Linux packages are relocatable (RPATH/$ORIGIN, no LD_LIBRARY_PATH),
// then run the binary by absolute path with a clean loader env.
// Packages with custom needs implement Smoke themselves.
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

	prefix, err := singleTopDir(deps, tmp)
	if err != nil {
		return err
	}
	binPath := filepath.Join(prefix, "bin", meta.Binary)
	if _, err := deps.FS.Stat(binPath); err != nil {
		return fmt.Errorf("missing bin/%s in package", meta.Binary)
	}

	buildinfo := filepath.Join(prefix, "BUILDINFO.txt")
	if data, err := deps.FS.ReadFile(buildinfo); err == nil {
		deps.Logf("--- BUILDINFO ---")
		deps.Logf("%s", strings.TrimSpace(string(data)))
	}

	// Self-contained: never set LD_LIBRARY_PATH / never put package bin on PATH.
	if runtime.GOOS != "windows" && !IsWindowsTarget(guessTargetFromTarball(tarball)) {
		if err := CheckLinuxRelocatable(prefix, RelocatableOpts{
			RequiredBins: []string{meta.Binary},
		}); err != nil {
			return err
		}
		deps.Logf("relocatable: RPATH/$ORIGIN OK for bin/%s", meta.Binary)
	}

	env := CleanSmokeEnv(deps.Env.Environ())

	var lastOut string
	var lastCode error
	tried := [][]string{{"--help"}, {"--version"}, {"-V"}, {"-h"}}
	for _, args := range tried {
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
			return fmt.Errorf("smoke failed for %v: dynamic linker error (package must be relocatable without LD_LIBRARY_PATH)", args)
		}
		if strings.TrimSpace(out) != "" || err == nil {
			deps.Logf("✓ Smoke test passed: %s", filepath.Base(tarball))
			return nil
		}
	}
	if lastCode != nil && strings.TrimSpace(lastOut) == "" {
		return fmt.Errorf("smoke failed: binary produced no output (%v)", tried[len(tried)-1])
	}
	deps.Logf("✓ Smoke test passed: %s", filepath.Base(tarball))
	return nil
}

func singleTopDir(deps Deps, tmp string) (string, error) {
	entries, err := deps.FS.ReadDir(tmp)
	if err != nil {
		return "", err
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
		return "", fmt.Errorf("expected one top-level dir, got: %v", names)
	}
	return roots[0], nil
}

func guessTargetFromTarball(tarball string) string {
	base := filepath.Base(tarball)
	for _, t := range []string{
		"windows-amd64", "windows-arm64",
		TargetLinuxAMD64, TargetLinuxAArch64,
	} {
		if strings.Contains(base, t) {
			return t
		}
	}
	return ""
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
