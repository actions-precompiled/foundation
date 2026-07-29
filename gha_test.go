package foundation_test

import (
	"context"
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
	if err := foundation.WriteBuildPlanSummary(env, fs, "llvm", "llvmorg-21.1.0", false, false); err != nil {
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
	if !strings.Contains(s, "Version: `llvmorg-21.1.0`") {
		t.Fatalf("summary missing version: %q", s)
	}
}

func TestRunCIPlanUsesLatestTag(t *testing.T) {
	dir := t.TempDir()
	summary := filepath.Join(dir, "summary.md")
	gout := filepath.Join(dir, "github_output")
	env := foundation.MapEnviron{
		foundation.EnvGitHubStepSummary: summary,
		foundation.EnvGitHubOutput:      gout,
		foundation.EnvGitHubEventName:   "push",
	}
	fs := foundation.OSFileSystem{}
	deps := foundation.Deps{
		Env:    env,
		FS:     fs,
		GitHub: fakeGitHub{upstream: []string{"llvmorg-20.1.0", "llvmorg-21.1.0"}},
	}
	meta := foundation.Meta{Name: "llvm", UpstreamRepoAPI: "llvm/llvm-project", ImageName: "x"}.Normalize()
	if err := foundation.RunCIPlan(context.Background(), deps, meta, foundation.CIPlanInput{
		EventName: "push",
	}); err != nil {
		t.Fatal(err)
	}
	o, _ := fs.ReadFile(gout)
	os := string(o)
	// LatestReleaseTag on fake returns last upstream
	if !strings.Contains(os, "version=llvmorg-21.1.0") {
		t.Fatalf("output %q", os)
	}
	s, _ := fs.ReadFile(summary)
	if !strings.Contains(string(s), "Version: `llvmorg-21.1.0`") {
		t.Fatalf("summary %q", string(s))
	}
}

func TestResolveCIPlanDispatchRequiresVersion(t *testing.T) {
	_, _, _, _, err := foundation.ResolveCIPlan(foundation.CIPlanInput{EventName: "workflow_dispatch"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestIsPublishableTag(t *testing.T) {
	if foundation.IsPublishableTag("trunk") || foundation.IsPublishableTag("main") || foundation.IsPublishableTag("latest") {
		t.Fatal("trunk/main/latest must not be publishable")
	}
	if !foundation.IsPublishableTag("llvmorg-21.1.0") || !foundation.IsPublishableTag("v1.2.3") {
		t.Fatal("real tags should be publishable")
	}
}
