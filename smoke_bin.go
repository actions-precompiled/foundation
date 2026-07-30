package foundation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// BinHelpOpts configures [SmokeBinDirHelp].
type BinHelpOpts struct {
	// Env is the process environment for invocations. If nil, CleanSmokeEnv is used.
	Env []string
	// Timeout per binary (0 = 30s).
	Timeout time.Duration
	// Skip basenames (exact match). Use for multi-call drivers like bare "lld".
	Skip []string
	// SkipSuffixes skips names with these suffixes (e.g. ".dll", ".py").
	SkipSuffixes []string
	// MaxBins caps how many files are exercised (0 = unlimited).
	MaxBins int
	// StrictHelp requires --help or -h to succeed (no --version fallback).
	StrictHelp bool
}

// SmokeBinDirHelp walks prefix/bin and runs each executable with --help (then
// fallbacks) under a clean loader env. Dynamic-linker / missing-DLL failures
// always fail the check. Packages use this to ensure the whole kitchen-sink
// is self-contained, not only Meta.Binary.
func SmokeBinDirHelp(ctx context.Context, deps Deps, prefix string, opts BinHelpOpts) error {
	binDir := filepath.Join(prefix, "bin")
	entries, err := os.ReadDir(binDir)
	if err != nil {
		return fmt.Errorf("smoke bin dir: %w", err)
	}
	env := opts.Env
	if env == nil {
		if deps.Env != nil {
			env = CleanSmokeEnv(deps.Env.Environ())
		} else {
			env = CleanSmokeEnv(nil)
		}
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	skip := map[string]bool{}
	for _, s := range opts.Skip {
		skip[s] = true
	}
	suffixes := opts.SkipSuffixes
	if len(suffixes) == 0 {
		suffixes = defaultSkipSuffixes()
	}

	var tried, okN int
	var problems []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if skip[name] {
			deps.Logf("smoke bin: skip %s", name)
			continue
		}
		if hasAnySuffix(name, suffixes) {
			continue
		}
		path := filepath.Join(binDir, name)
		if !isSmokeCandidate(path, name) {
			continue
		}
		if opts.MaxBins > 0 && tried >= opts.MaxBins {
			break
		}
		tried++
		flag, out, err := tryBinHelpFlags(ctx, deps, env, timeout, path, opts.StrictHelp)
		if dynLinkFailure(out, err) {
			problems = append(problems, fmt.Sprintf("%s: missing shared lib / dynamic link failure\n%s", name, firstNonEmptyLine(out, err)))
			continue
		}
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: no usable --help/--version (%v)\n%s", name, err, firstNonEmptyLine(out, err)))
			continue
		}
		okN++
		deps.Logf("ok bin/%s %s", name, flag)
	}
	if tried == 0 {
		return fmt.Errorf("%w: no executables under bin/", ErrSmokeNoOutput)
	}
	if len(problems) > 0 {
		n := len(problems)
		show := problems
		if n > 20 {
			show = append(problems[:20], fmt.Sprintf("... and %d more", n-20))
		}
		// Prefer dynlink sentinel only when any problem mentions missing libs.
		sent := ErrSmokeNoOutput
		for _, p := range problems {
			if strings.Contains(p, "missing shared lib") {
				sent = ErrSmokeDynamicLink
				break
			}
		}
		return fmt.Errorf("%w: %d/%d bin tools failed self-contained start:\n  - %s",
			sent, n, tried, strings.Join(show, "\n  - "))
	}
	deps.Logf("smoke bin: %d/%d tools OK under clean env (--help/--version)", okN, tried)
	return nil
}

func defaultSkipSuffixes() []string {
	if runtime.GOOS == "windows" {
		return []string{".dll", ".lib", ".pdb", ".py", ".cmake", ".txt", ".md", ".bat", ".cmd", ".ps1"}
	}
	return []string{".so", ".a", ".py", ".cmake", ".txt", ".md", ".sh", ".pl", ".td", ".inc"}
}

func hasAnySuffix(name string, suffixes []string) bool {
	lower := strings.ToLower(name)
	for _, s := range suffixes {
		if strings.HasSuffix(lower, strings.ToLower(s)) {
			return true
		}
	}
	return false
}

func isSmokeCandidate(path, name string) bool {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return false
	}
	// Symlinks to tools are fine (clang -> clang-22).
	if runtime.GOOS == "windows" {
		lower := strings.ToLower(name)
		return strings.HasSuffix(lower, ".exe") || !strings.Contains(name, ".")
	}
	// Prefer executable bit; also accept ELFs without +x (rare install glitch).
	if st.Mode()&0o111 != 0 {
		return true
	}
	return isDynamicELF(path) // defined in relocatable.go
}

func tryBinHelpFlags(ctx context.Context, deps Deps, env []string, timeout time.Duration, path string, strictHelp bool) (flag string, out string, err error) {
	flags := []string{"--help", "-h"}
	if !strictHelp {
		flags = append(flags, "--version", "-version", "-V")
	}
	var lastOut string
	var lastErr error
	for _, f := range flags {
		o, e := CombinedOutputWithEnvTimeout(ctx, deps, env, timeout, path, f)
		lastOut, lastErr = o, e
		if dynLinkFailure(o, e) {
			return f, o, e
		}
		// Success: exit 0, or produced output that is not a linker error.
		// Many tools exit 1 on --help when they want a subcommand; still count
		// if they printed something and the loader was happy.
		if e == nil {
			return f, o, nil
		}
		if strings.TrimSpace(o) != "" && !dynLinkFailure(o, e) {
			// Loader OK; tool rejected the flag but started — good enough for missing-lib detection.
			if f == "--help" || f == "-h" || !strictHelp {
				return f, o, nil
			}
		}
	}
	return "", lastOut, lastErr
}

func dynLinkFailure(out string, err error) bool {
	low := strings.ToLower(out)
	if err != nil {
		low += " " + strings.ToLower(err.Error())
	}
	needles := []string{
		"error while loading shared libraries",
		"cannot open shared object",
		"image not found", // macOS
		"the code execution cannot proceed because", // Windows missing DLL
		"unable to find dll",
		"library not loaded",
		"lib*.so", // too broad — skip
	}
	for _, n := range needles {
		if n == "lib*.so" {
			continue
		}
		if strings.Contains(low, n) {
			return true
		}
	}
	// Linux: "bin/foo: error while loading..." sometimes only on stderr via err text
	if strings.Contains(low, "shared libraries") && strings.Contains(low, "no such file") {
		return true
	}
	return false
}

func firstNonEmptyLine(out string, err error) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) != "" {
			return strings.TrimSpace(line)
		}
	}
	if err != nil {
		return err.Error()
	}
	return ""
}
