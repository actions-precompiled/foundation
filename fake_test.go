package foundation_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/actions-precompiled/foundation"
)

// fakePkg is a minimal Package used to exercise the engine without Docker.
type fakePkg struct {
	builds []string
	smokes []string
}

func (f *fakePkg) Meta() foundation.Meta {
	return foundation.Meta{
		Name:            "example",
		UpstreamRepoAPI: "example-org/example",
		ImageName:       "example-buildenv",
		Binary:          "example",
		VersionEnv:      "EXAMPLE_VERSION",
		DefaultTargets:  []string{"windows-amd64"},
	}
}

func (f *fakePkg) Work(_ context.Context, deps foundation.Deps, req foundation.BuildRequest) error {
	f.builds = append(f.builds, req.Version+"/"+req.Target)
	// Write a placeholder tarball path content (empty gz is fine for find; smoke skipped)
	name := foundation.ArtifactName("example", req.Version, req.Target)
	path := filepath.Join(req.OutDir, name)
	return deps.FS.WriteFile(path, []byte("not-a-real-tarball"), 0o644)
}

func (f *fakePkg) Smoke(_ context.Context, _ foundation.Deps, req foundation.SmokeRequest) error {
	f.smokes = append(f.smokes, req.Version+"/"+req.Target)
	if len(req.Tarballs) == 0 {
		return context.Canceled // distinctive
	}
	return nil
}

type fakeGitHub struct {
	upstream []string
	released []string
}

func (g fakeGitHub) ListUpstreamTags(context.Context, string) ([]string, error) {
	return append([]string(nil), g.upstream...), nil
}
func (g fakeGitHub) ListReleasedTags(context.Context) ([]string, error) {
	return append([]string(nil), g.released...), nil
}
func (g fakeGitHub) LatestReleaseTag(context.Context, string) (string, error) {
	if len(g.upstream) == 0 {
		return "", os.ErrNotExist
	}
	return g.upstream[len(g.upstream)-1], nil
}
func (g fakeGitHub) CreateRelease(context.Context, foundation.ReleaseRequest) error { return nil }
func (g fakeGitHub) DeleteRelease(context.Context, string) error                    { return nil }

type fakeDocker struct {
	builds int
	runs   int
}

func (d *fakeDocker) BuildImage(context.Context, foundation.DockerBuildRequest) error {
	d.builds++
	return nil
}
func (d *fakeDocker) Run(context.Context, foundation.DockerRunRequest) error {
	d.runs++
	return nil
}

type fakeRunner struct{}

func (fakeRunner) Run(context.Context, string, ...string) error { return nil }
func (fakeRunner) Output(context.Context, string, ...string) (string, error) {
	return "", nil
}

func testDeps(t *testing.T, env foundation.MapEnviron, gh foundation.GitHub, docker foundation.Docker) (foundation.Deps, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	wd := t.TempDir()
	return foundation.Deps{
		WorkDir: wd,
		Runner:  fakeRunner{},
		Env:     env,
		GitHub:  gh,
		Docker:  docker,
		FS:      foundation.OSFileSystem{},
		Stdout:  stdout,
		Stderr:  stderr,
	}, stdout, stderr
}

func TestMainWithListToBuildMissing(t *testing.T) {
	pkg := &fakePkg{}
	gh := fakeGitHub{
		upstream: []string{"v1.0.0", "v1.1.0", "v2.0.0"},
		released: []string{"v1.0.0"},
	}
	docker := &fakeDocker{}
	deps, stdout, _ := testDeps(t, foundation.MapEnviron{}, gh, docker)

	err := foundation.MainWith(pkg, deps, []string{"list"})
	if err != nil {
		t.Fatalf("MainWith: %v", err)
	}
	got := strings.TrimSpace(stdout.String())
	want := "v1.1.0\nv2.0.0"
	if got != want {
		t.Fatalf("list-to-build:\n got %q\nwant %q", got, want)
	}
	if len(pkg.builds) != 0 {
		t.Fatalf("expected no builds, got %v", pkg.builds)
	}
}

func TestMainWithListToBuildAll(t *testing.T) {
	pkg := &fakePkg{}
	gh := fakeGitHub{
		upstream: []string{"v1.0.0", "v1.1.0"},
		released: []string{"v1.0.0"},
	}
	deps, stdout, _ := testDeps(t, foundation.MapEnviron{}, gh, &fakeDocker{})

	err := foundation.MainWith(pkg, deps, []string{"list", "--all"})
	if err != nil {
		t.Fatalf("MainWith: %v", err)
	}
	got := strings.TrimSpace(stdout.String())
	if got != "v1.0.0\nv1.1.0" {
		t.Fatalf("got %q", got)
	}
}

func TestMainWithDryRunExplicitVersion(t *testing.T) {
	pkg := &fakePkg{}
	deps, _, stderr := testDeps(t, foundation.MapEnviron{}, fakeGitHub{}, &fakeDocker{})

	err := foundation.MainWith(pkg, deps, []string{"build", "--dry-run", "v9.9.9"})
	if err != nil {
		t.Fatalf("MainWith: %v", err)
	}
	if !strings.Contains(stderr.String(), "v9.9.9") {
		t.Fatalf("stderr missing version: %s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "Dry-run") {
		t.Fatalf("stderr missing dry-run: %s", stderr.String())
	}
	if len(pkg.builds) != 0 {
		t.Fatalf("builds should be empty: %v", pkg.builds)
	}
}

func TestMainWithBuildAndSmoke(t *testing.T) {
	pkg := &fakePkg{}
	docker := &fakeDocker{}
	env := foundation.MapEnviron{
		foundation.EnvSkipImageBuild: "1",
	}
	deps, _, _ := testDeps(t, env, fakeGitHub{}, docker)

	err := foundation.MainWith(pkg, deps, []string{"build", "--skip-image-build", "v1.2.3"})
	if err != nil {
		t.Fatalf("MainWith: %v", err)
	}
	if len(pkg.builds) != 1 || pkg.builds[0] != "v1.2.3/windows-amd64" {
		t.Fatalf("builds: %v", pkg.builds)
	}
	if len(pkg.smokes) != 1 {
		t.Fatalf("smokes: %v", pkg.smokes)
	}
	if docker.builds != 0 {
		t.Fatalf("image builds should be skipped, got %d", docker.builds)
	}
}

func TestMainWithAPCTargetsEnv(t *testing.T) {
	pkg := &fakePkg{}
	env := foundation.MapEnviron{
		foundation.EnvTargets:        "windows-amd64 windows-arm64",
		foundation.EnvSkipImageBuild: "1",
		foundation.EnvSkipSmoke:      "1",
	}
	deps, _, _ := testDeps(t, env, fakeGitHub{}, &fakeDocker{})

	err := foundation.MainWith(pkg, deps, []string{"build", "--skip-smoke", "v0.1.0"})
	if err != nil {
		t.Fatalf("MainWith: %v", err)
	}
	if len(pkg.builds) != 2 {
		t.Fatalf("expected 2 builds, got %v", pkg.builds)
	}
}

func TestResolveConfigRequiresMeta(t *testing.T) {
	_, err := foundation.ResolveConfig(foundation.MapEnviron{}, foundation.Meta{}, foundation.Flags{}, foundation.CommandBuild)
	if err == nil {
		t.Fatal("expected error for empty meta")
	}
}

func TestVersionBare(t *testing.T) {
	if foundation.VersionBare("v1.2.3") != "1.2.3" {
		t.Fatal(foundation.VersionBare("v1.2.3"))
	}
	if foundation.VersionBare("1.2.3") != "1.2.3" {
		t.Fatal(foundation.VersionBare("1.2.3"))
	}
	if foundation.ParseVersion("v1.2.3").Bare() != "1.2.3" {
		t.Fatal("ParseVersion.Bare")
	}
}

func TestArtifactName(t *testing.T) {
	got := foundation.ArtifactName("quickshell", "v0.3.0", "linux-amd64")
	want := "quickshell-0.3.0-linux-amd64.tar.gz"
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestEnvFlag(t *testing.T) {
	env := foundation.MapEnviron{
		"A": "1",
		"B": "0",
		"C": "false",
		"D": "yes",
	}
	if !foundation.EnvFlag(env, "A") {
		t.Fatal("A")
	}
	if foundation.EnvFlag(env, "B") {
		t.Fatal("B")
	}
	if foundation.EnvFlag(env, "C") {
		t.Fatal("C")
	}
	if !foundation.EnvFlag(env, "D") {
		t.Fatal("D")
	}
	if foundation.EnvFlag(env, "missing") {
		t.Fatal("missing")
	}
}

func TestMetaNormalize(t *testing.T) {
	m := foundation.Meta{
		Name:            "gettext",
		UpstreamRepoAPI: "gnu-mirror/gettext",
	}.Normalize()
	if m.UpstreamGit == "" {
		t.Fatal("UpstreamGit")
	}
	if m.ImageName != "gettext-buildenv" {
		t.Fatalf("ImageName %s", m.ImageName)
	}
	if m.VersionEnv != "GETTEXT_VERSION" {
		t.Fatalf("VersionEnv %s", m.VersionEnv)
	}
}
