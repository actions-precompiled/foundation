package foundation

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCleanSmokeEnvStripsLoaderOverrides(t *testing.T) {
	in := []string{
		"PATH=/usr/bin",
		"LD_LIBRARY_PATH=/evil/lib",
		"LD_PRELOAD=/evil.so",
		"DYLD_LIBRARY_PATH=/evil",
		"HOME=/home/u",
	}
	out := CleanSmokeEnv(in)
	for _, e := range out {
		if e == "LD_LIBRARY_PATH=/evil/lib" || e == "LD_PRELOAD=/evil.so" || e == "DYLD_LIBRARY_PATH=/evil" {
			t.Fatalf("loader override leaked: %s", e)
		}
	}
	if !containsEnv(out, "PATH=/usr/bin") || !containsEnv(out, "HOME=/home/u") {
		t.Fatalf("expected host PATH/HOME kept, got %v", out)
	}
}

func containsEnv(env []string, want string) bool {
	for _, e := range env {
		if e == want {
			return true
		}
	}
	return false
}

func TestExpandRPathOrigin(t *testing.T) {
	dirs := expandRPath("$ORIGIN/../lib:$ORIGIN/../lib64", "/opt/pkg/bin")
	if len(dirs) != 2 {
		t.Fatalf("got %v", dirs)
	}
	if dirs[0] != "/opt/pkg/lib" || dirs[1] != "/opt/pkg/lib64" {
		t.Fatalf("expanded: %v", dirs)
	}
}

func TestCheckLinuxRelocatableEmptyPrefix(t *testing.T) {
	dir := t.TempDir()
	if err := CheckLinuxRelocatable(dir, RelocatableOpts{}); err != nil {
		t.Fatal(err)
	}
}

func TestCheckLinuxRelocatableDetectsBrokenRPath(t *testing.T) {
	cc := firstAvailable("gcc", "clang", "cc")
	if cc == "" {
		t.Skip("no C compiler")
	}
	dir := t.TempDir()
	prefix := filepath.Join(dir, "pkg")
	binDir := filepath.Join(prefix, "bin")
	libDir := filepath.Join(prefix, "lib")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatal(err)
	}

	libSrc := filepath.Join(dir, "foo.c")
	mainSrc := filepath.Join(dir, "main.c")
	if err := os.WriteFile(libSrc, []byte("int foo(void){return 7;}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainSrc, []byte("int foo(void); int main(void){return foo();}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	libSo := filepath.Join(libDir, "libfoo.so")
	bin := filepath.Join(binDir, "tool")
	if out, err := exec.Command(cc, "-shared", "-fPIC", "-o", libSo, libSrc).CombinedOutput(); err != nil {
		t.Skipf("cannot build lib with %s: %v\n%s", cc, err, out)
	}
	// Intentionally no -Wl,-rpath
	if out, err := exec.Command(cc, "-o", bin, mainSrc, "-L"+libDir, "-lfoo", "-Wl,-rpath-link,"+libDir).CombinedOutput(); err != nil {
		// some linkers need the lib at link time via rpath-link; if still fails skip
		if out2, err2 := exec.Command(cc, "-o", bin, mainSrc, libSo).CombinedOutput(); err2 != nil {
			t.Skipf("cannot build bin: %v\n%s\n%v\n%s", err, out, err2, out2)
		}
	}

	if err := CheckLinuxRelocatable(prefix, RelocatableOpts{RequiredBins: []string{"tool"}}); err == nil {
		t.Fatal("expected broken RPATH to fail check")
	}

	if _, err := exec.LookPath("patchelf"); err != nil {
		t.Logf("patchelf missing; skip fix-up half")
		return
	}
	deps := DefaultDeps(dir)
	if err := PatchLinuxOriginRPath(t.Context(), deps, prefix); err != nil {
		t.Fatalf("patchelf: %v", err)
	}
	if err := CheckLinuxRelocatable(prefix, RelocatableOpts{RequiredBins: []string{"tool"}}); err != nil {
		t.Fatalf("after patchelf: %v", err)
	}
	cmd := exec.Command(bin)
	cmd.Env = CleanSmokeEnv(os.Environ())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run without LD_LIBRARY_PATH: %v\n%s", err, out)
	}
}

func firstAvailable(names ...string) string {
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	return ""
}
