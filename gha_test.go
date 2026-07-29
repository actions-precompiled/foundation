package foundation_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/actions-precompiled/foundation"
)

func TestWriteBuildPlanSummary(t *testing.T) {
	dir := t.TempDir()
	summary := filepath.Join(dir, "summary.md")
	env := foundation.MapEnviron{
		foundation.EnvGitHubStepSummary: summary,
	}
	fs := foundation.OSFileSystem{}
	if err := foundation.WriteBuildPlanSummary(env, fs, "llvm", "trunk", false, false); err != nil {
		t.Fatal(err)
	}
	b, err := fs.ReadFile(summary)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "Package: `llvm`") {
		t.Fatalf("summary missing package: %q", s)
	}
	if !strings.Contains(s, "Version: `trunk`") {
		t.Fatalf("summary missing version: %q", s)
	}
	if !strings.Contains(s, "Publish: **false**") {
		t.Fatalf("summary publish: %q", s)
	}
}

func TestRunCIPlan(t *testing.T) {
	dir := t.TempDir()
	summary := filepath.Join(dir, "summary.md")
	gout := filepath.Join(dir, "github_output")
	env := foundation.MapEnviron{
		foundation.EnvGitHubStepSummary: summary,
		foundation.EnvGitHubOutput:      gout,
		foundation.EnvGitHubEventName:   "push",
	}
	fs := foundation.OSFileSystem{}
	deps := foundation.Deps{Env: env, FS: fs}
	meta := foundation.Meta{Name: "llvm", UpstreamRepoAPI: "llvm/llvm-project", ImageName: "x"}.Normalize()
	if err := foundation.RunCIPlan(deps, meta, foundation.CIPlanInput{
		EventName:      "push",
		DefaultVersion: "trunk",
	}); err != nil {
		t.Fatal(err)
	}
	o, _ := fs.ReadFile(gout)
	os := string(o)
	if !strings.Contains(os, "version=trunk") || !strings.Contains(os, "publish=false") {
		t.Fatalf("output %q", os)
	}
	s, _ := fs.ReadFile(summary)
	if !strings.Contains(string(s), "Package: `llvm`") || !strings.Contains(string(s), "Version: `trunk`") {
		t.Fatalf("summary %q", string(s))
	}
}

func TestResolveCIPlanDispatchRequiresVersion(t *testing.T) {
	_, _, _, err := foundation.ResolveCIPlan(foundation.CIPlanInput{EventName: "workflow_dispatch"})
	if err == nil {
		t.Fatal("expected error")
	}
}
