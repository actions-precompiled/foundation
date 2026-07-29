package foundation

import (
	"context"
	"fmt"
	"os"
	"strconv"
)

// DefaultDocker puppets the docker CLI via Runner.
type DefaultDocker struct {
	Runner Runner
}

// NewDefaultDocker returns a Docker client.
func NewDefaultDocker(runner Runner) *DefaultDocker {
	return &DefaultDocker{Runner: runner}
}

func (d *DefaultDocker) BuildImage(ctx context.Context, req DockerBuildRequest) error {
	if req.Image == "" {
		return fmt.Errorf("docker build: Image is required")
	}
	if req.Context == "" {
		return fmt.Errorf("docker build: Context is required")
	}
	args := []string{"build", "-t", req.Image}
	if req.TargetArch != "" {
		args = append(args, "--build-arg", "TARGETARCH="+req.TargetArch)
	}
	for k, v := range req.BuildArgs {
		args = append(args, "--build-arg", k+"="+v)
	}
	args = append(args, req.Context)
	return d.Runner.Run(ctx, "docker", args...)
}

func (d *DefaultDocker) Run(ctx context.Context, req DockerRunRequest) error {
	if req.Image == "" {
		return fmt.Errorf("docker run: Image is required")
	}
	args := []string{"run", "--rm"}
	if req.User != "" {
		args = append(args, "--user", req.User)
	}
	for _, b := range req.Binds {
		args = append(args, "-v", b)
	}
	for k, v := range req.Env {
		args = append(args, "-e", k+"="+v)
	}
	if req.WorkDir != "" {
		args = append(args, "-w", req.WorkDir)
	}
	if len(req.Entrypoint) > 0 {
		args = append(args, "--entrypoint", req.Entrypoint[0])
		// remaining entrypoint parts become args after image if needed — keep simple: first only
	}
	args = append(args, req.Image)
	args = append(args, req.Cmd...)
	return d.Runner.Run(ctx, "docker", args...)
}

// HostDockerUser returns "uid:gid" for the current user (Linux).
func HostDockerUser() string {
	return strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid())
}

// StandardDockerBuild is deprecated: prefer engine inject via RunWorkInDocker + Package.Work.
// StandardDockerBuild runs the common actions-precompiled container build:
// mounts OutDir at /out, passes version env vars, uses Meta + BuildRequest.
// Packages can call this from Package.Build instead of hand-rolling docker args.
func StandardDockerBuild(ctx context.Context, deps Deps, meta Meta, cfgImageName, cfgImageTag string, req BuildRequest) error {
	meta = meta.Normalize()
	if deps.Docker == nil {
		return fmt.Errorf("StandardDockerBuild: Docker is nil")
	}
	image := cfgImageName + ":" + cfgImageTag
	if cfgImageTag == "" {
		image = cfgImageName + ":local"
	}

	env := map[string]string{
		"PACKAGE_VERSION": req.Version,
		"PACKAGE_NAME":    meta.Name,
		"UPSTREAM_REPO":   meta.UpstreamGit,
		"BUILD_TARGET":    req.Target,
		"OUTPUT_DIR":      "/out",
		"HOME":            "/tmp",
		"XDG_CACHE_HOME":  "/tmp/.cache",
		"LANG":            "C.UTF-8",
		"LC_ALL":          "C.UTF-8",
	}
	if meta.VersionEnv != "" {
		env[meta.VersionEnv] = req.Version
	}

	return deps.Docker.Run(ctx, DockerRunRequest{
		Image: image,
		User:  HostDockerUser(),
		Binds: []string{req.OutDir + ":/out"},
		Env:   env,
	})
}
