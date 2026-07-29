package foundation_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/actions-precompiled/foundation"
)

func TestGenerateWorkflow(t *testing.T) {
	pkg := &fakePkg{}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	wd := t.TempDir()
	deps := foundation.Deps{
		WorkDir: wd,
		Runner:  fakeRunner{},
		Env:     foundation.MapEnviron{},
		GitHub:  fakeGitHub{},
		Docker:  &fakeDocker{},
		FS:      foundation.OSFileSystem{},
		Stdout:  stdout,
		Stderr:  stderr,
	}
	err := foundation.MainWith(pkg, deps, []string{"generate", "workflow", "--dir", filepath.Join(wd, ".github/workflows"), "--force"})
	if err != nil {
		t.Fatal(err)
	}
	build := filepath.Join(wd, ".github/workflows/build.yml")
	disp := filepath.Join(wd, ".github/workflows/dispatch-missing.yml")
	b, err := deps.FS.ReadFile(build)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "go run . build") {
		t.Fatalf("build.yml missing go run . build:\n%s", s[:min(500, len(s))])
	}
	if !strings.Contains(s, "${{ matrix.target }}") && !strings.Contains(s, "${{  matrix.target  }}") {
		// gha() adds spaces: ${{ matrix.target }}
		if !strings.Contains(s, "matrix.target") {
			t.Fatalf("build.yml missing matrix.target:\n%s", s[:min(800, len(s))])
		}
	}
	d, err := deps.FS.ReadFile(disp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(d), "go run . list") {
		t.Fatalf("dispatch missing list: %s", string(d)[:min(400, len(d))])
	}
}
