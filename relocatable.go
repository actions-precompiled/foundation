package foundation

import (
	"context"
	"debug/elf"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultLinuxRPathBin is the DT_RUNPATH used for executables under bin/.
// $ORIGIN is the directory containing the ELF file.
const DefaultLinuxRPathBin = "$ORIGIN/../lib:$ORIGIN/../lib64:$ORIGIN"

// DefaultLinuxRPathLib is the DT_RUNPATH used for shared objects under lib/.
const DefaultLinuxRPathLib = "$ORIGIN:$ORIGIN/../lib:$ORIGIN/../lib64"

// RelocatableOpts configures [CheckLinuxRelocatable] / [PatchLinuxOriginRPath].
type RelocatableOpts struct {
	// MaxFiles caps how many ELF files are inspected (0 = 256).
	MaxFiles int
	// RequiredBins are basenames under prefix/bin that must exist and pass.
	// Empty means: check every regular file under bin/ (up to MaxFiles).
	RequiredBins []string
}

// CleanSmokeEnv returns a copy of env suitable for relocatable smoke tests:
// package libs must resolve via RPATH/$ORIGIN, never LD_LIBRARY_PATH.
// Also strips DYLD_* (macOS). Does not prepend package bin/ to PATH —
// smoke must invoke package tools by absolute path.
func CleanSmokeEnv(env []string) []string {
	if env == nil {
		env = os.Environ()
	}
	out := make([]string, 0, len(env))
	for _, e := range env {
		key, _, ok := strings.Cut(e, "=")
		if !ok {
			continue
		}
		switch key {
		case "LD_LIBRARY_PATH",
			"LD_PRELOAD",
			"DYLD_LIBRARY_PATH",
			"DYLD_FALLBACK_LIBRARY_PATH",
			"DYLD_INSERT_LIBRARIES":
			continue
		}
		out = append(out, e)
	}
	return out
}

// CheckLinuxRelocatable verifies that dynamic ELFs under prefix resolve any
// libraries that ship inside the package via DT_RUNPATH/DT_RPATH ($ORIGIN),
// without consulting LD_LIBRARY_PATH.
//
// System libs (libc, libstdc++, …) that are not present under prefix are ignored:
// the package may still depend on a normal glibc userland. Package-private libs
// (e.g. libLLVM.so.*) must be findable via RPATH alone.
func CheckLinuxRelocatable(prefix string, opts RelocatableOpts) error {
	prefix = filepath.Clean(prefix)
	if st, err := os.Stat(prefix); err != nil || !st.IsDir() {
		return fmt.Errorf("relocatable check: prefix %s: %w", prefix, err)
	}
	max := opts.MaxFiles
	if max <= 0 {
		max = 256
	}

	bundled := indexBundledSONAMEs(prefix)
	var files []string
	if len(opts.RequiredBins) > 0 {
		for _, b := range opts.RequiredBins {
			p := filepath.Join(prefix, "bin", b)
			if _, err := os.Stat(p); err != nil {
				return fmt.Errorf("%w: bin/%s", ErrRelocatableBinMissing, b)
			}
			files = append(files, p)
		}
	} else {
		binDir := filepath.Join(prefix, "bin")
		if entries, err := os.ReadDir(binDir); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				files = append(files, filepath.Join(binDir, e.Name()))
			}
		}
	}
	// Always inspect package shared libraries too (sibling $ORIGIN resolution).
	if err := filepath.WalkDir(filepath.Join(prefix, "lib"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.Contains(name, ".so") {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("walk lib: %w", err)
	}

	checked := 0
	var problems []string
	for _, path := range files {
		if checked >= max {
			break
		}
		info, err := inspectELFRelocatable(path, bundled)
		if err != nil {
			// not an ELF / static / unreadable — skip
			continue
		}
		if info == nil {
			continue
		}
		checked++
		if len(info.Missing) > 0 {
			rel, relErr := filepath.Rel(prefix, path)
			if relErr != nil {
				rel = path
			}
			problems = append(problems, fmt.Sprintf(
				"%s: needs %v but RPATH/RUNPATH %q does not resolve them under the package (bundled libs must use $ORIGIN)",
				rel, info.Missing, info.RPath,
			))
		}
	}
	if checked == 0 && len(bundled) > 0 {
		return fmt.Errorf("%w (%d shared libs)", ErrRelocatableNoELF, len(bundled))
	}
	if len(problems) > 0 {
		// Cap error size
		n := len(problems)
		if n > 12 {
			problems = problems[:12]
			problems = append(problems, fmt.Sprintf("... and %d more", n-12))
		}
		return fmt.Errorf("%w (%d file(s)):\n  - %s", ErrRelocatableFailed, n, strings.Join(problems, "\n  - "))
	}
	return nil
}

// PatchLinuxOriginRPath runs patchelf on dynamic ELFs under prefix/bin and
// prefix/lib so DT_RUNPATH uses $ORIGIN-relative paths. Call after install,
// before tarring. Requires patchelf on PATH (build image should provide it).
func PatchLinuxOriginRPath(ctx context.Context, deps Deps, prefix string) error {
	prefix = filepath.Clean(prefix)
	if deps.Runner == nil {
		return fmt.Errorf("patchelf: %w", ErrRunnerNil)
	}
	if _, err := deps.Runner.Output(ctx, "patchelf", "--version"); err != nil {
		return fmt.Errorf("patchelf not available: %w", err)
	}

	var targets []string
	for _, sub := range []string{"bin", "lib"} {
		root := filepath.Join(prefix, sub)
		if err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d == nil || d.IsDir() {
				return nil
			}
			// Skip obvious non-binaries.
			name := d.Name()
			if strings.HasSuffix(name, ".py") || strings.HasSuffix(name, ".txt") ||
				strings.HasSuffix(name, ".cmake") || strings.HasSuffix(name, ".td") {
				return nil
			}
			if isDynamicELF(path) {
				targets = append(targets, path)
			}
			return nil
		}); err != nil {
			return fmt.Errorf("walk %s: %w", sub, err)
		}
	}

	for _, path := range targets {
		rel, relErr := filepath.Rel(prefix, path)
		if relErr != nil {
			rel = path
		}
		rpath := DefaultLinuxRPathBin
		if strings.HasPrefix(rel, "lib"+string(filepath.Separator)) || rel == "lib" {
			rpath = DefaultLinuxRPathLib
		}
		// --force-rpath is obsolete on modern patchelf; --set-rpath sets RUNPATH.
		if err := deps.Runner.Run(ctx, "patchelf", "--set-rpath", rpath, path); err != nil {
			return fmt.Errorf("patchelf %s: %w", rel, err)
		}
	}
	deps.Logf("patchelf: set $ORIGIN rpath on %d ELF file(s) under %s", len(targets), prefix)
	return nil
}

// CheckSmokeTarballRelocatable extracts nothing — operates on an already-extracted
// package prefix. Convenience for package Smoke implementations.
func CheckSmokeTarballRelocatable(deps Deps, prefix string, requiredBins ...string) error {
	opts := RelocatableOpts{RequiredBins: requiredBins}
	if err := CheckLinuxRelocatable(prefix, opts); err != nil {
		return err
	}
	deps.Logf("relocatable: OK (%s)", prefix)
	return nil
}

type elfRelocInfo struct {
	RPath   string
	Missing []string
}

func inspectELFRelocatable(path string, bundled map[string]string) (*elfRelocInfo, error) {
	f, err := elf.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if f.Type != elf.ET_EXEC && f.Type != elf.ET_DYN {
		return nil, nil // not a dynamic ELF — caller skips
	}
	needed, err := f.DynString(elf.DT_NEEDED)
	if err != nil || len(needed) == 0 {
		// static or no dynamic deps — caller skips
		return nil, nil
	}

	runpath, errRun := f.DynString(elf.DT_RUNPATH)
	if errRun != nil {
		runpath = nil
	}
	rpath, errRP := f.DynString(elf.DT_RPATH)
	if errRP != nil {
		rpath = nil
	}
	// ELF prefers RUNPATH over RPATH when both exist.
	search := strings.Join(runpath, ":")
	if search == "" {
		search = strings.Join(rpath, ":")
	}

	origin := filepath.Dir(path)
	dirs := expandRPath(search, origin)

	var missing []string
	for _, soname := range needed {
		base := filepath.Base(soname)
		// Only enforce libs that ship inside this package.
		if _, ok := bundled[base]; !ok {
			continue
		}
		if !resolveInDirs(base, dirs) {
			missing = append(missing, base)
		}
	}
	return &elfRelocInfo{RPath: search, Missing: missing}, nil
}

func isDynamicELF(path string) bool {
	f, err := elf.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	if f.Type != elf.ET_EXEC && f.Type != elf.ET_DYN {
		return false
	}
	needed, err := f.DynString(elf.DT_NEEDED)
	return err == nil && len(needed) > 0
}

// indexBundledSONAMEs maps "libfoo.so.1" → absolute path for every .so* under prefix.
func indexBundledSONAMEs(prefix string) map[string]string {
	out := make(map[string]string)
	if err := walkIgnore(prefix, func(path string, d os.DirEntry) {
		name := d.Name()
		if !strings.Contains(name, ".so") {
			return
		}
		// Index basename and common realpath basename.
		out[name] = path
		if fi, err := os.Lstat(path); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			if target, err := filepath.EvalSymlinks(path); err == nil {
				out[filepath.Base(target)] = path
			}
		}
	}); err != nil {
		// best-effort: partial index is still useful
		return out
	}
	return out
}

// walkIgnore walks root and calls fn for each non-dir entry; walk errors are ignored
// (best-effort indexing during checks).
func walkIgnore(root string, fn func(path string, d os.DirEntry)) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		fn(path, d)
		return nil
	})
}

// expandRPath splits a DT_RUNPATH/RPATH value and expands $ORIGIN / ${ORIGIN}.
func expandRPath(rpath, origin string) []string {
	if rpath == "" {
		return nil
	}
	var dirs []string
	for _, part := range strings.Split(rpath, ":") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		part = strings.ReplaceAll(part, "$ORIGIN", origin)
		part = strings.ReplaceAll(part, "${ORIGIN}", origin)
		// $LIB is rare; leave unexpanded if present.
		if !filepath.IsAbs(part) {
			part = filepath.Join(origin, part)
		}
		dirs = append(dirs, filepath.Clean(part))
	}
	return dirs
}

func resolveInDirs(soname string, dirs []string) bool {
	for _, d := range dirs {
		p := filepath.Join(d, soname)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return true
		}
	}
	return false
}
