package foundation

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
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
		return fmt.Errorf("%w", ErrSmokeBinaryRequired)
	}
	if len(req.Tarballs) == 0 {
		return fmt.Errorf("%w", ErrSmokeNoTarballs)
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
		return fmt.Errorf("%w: %s", ErrTarballMissing, tarball)
	}

	tmp, err := deps.FS.TempDir("", meta.Name+"-smoke-")
	if err != nil {
		return err
	}
	defer deps.RemoveAllLog(tmp, "smoke cleanup")

	if err := extractTarGz(tarball, tmp); err != nil {
		return fmt.Errorf("extract %s: %w", tarball, err)
	}

	prefix, err := singleTopDir(deps, tmp)
	if err != nil {
		return err
	}
	binPath := filepath.Join(prefix, "bin", meta.Binary)
	if _, err := deps.FS.Stat(binPath); err != nil {
		return fmt.Errorf("%w: bin/%s", ErrMissingPackageBinary, meta.Binary)
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

	// Exercise every bin/* under clean env so missing shared libs surface.
	if err := SmokeBinDirHelp(ctx, deps, prefix, BinHelpOpts{Env: env}); err != nil {
		return err
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
		return "", fmt.Errorf("%w, got: %v", ErrBadTarballLayout, names)
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
			return fmt.Errorf("%w: %s", ErrUnsafeTarballPath, hdr.Name)
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
			if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
				// replace existing path
			}
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		}
	}
	return nil
}
