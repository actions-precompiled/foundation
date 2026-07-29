package foundation

import (
	"runtime"
	"strings"
)

// Known Linux targets used by actions-precompiled packages.
const (
	TargetLinuxAMD64   = "linux-amd64"
	TargetLinuxAArch64 = "linux-aarch64"
)

// HostTarget returns the native linux-* target for the current machine.
func HostTarget() string {
	switch runtime.GOARCH {
	case "arm64":
		return TargetLinuxAArch64
	default:
		return TargetLinuxAMD64
	}
}

// HostDockerArch returns amd64 / arm64 for Docker TARGETARCH.
func HostDockerArch() string {
	switch runtime.GOARCH {
	case "arm64":
		return "arm64"
	default:
		return "amd64"
	}
}

// TargetDockerArch maps a package target string to Docker TARGETARCH.
func TargetDockerArch(target string) string {
	switch target {
	case TargetLinuxAArch64, "linux-arm64":
		return "arm64"
	default:
		return "amd64"
	}
}

// ParseTargets splits a space-separated TARGETS string.
func ParseTargets(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	fields := strings.Fields(raw)
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

// ResolveTargets picks targets from env, then Meta.DefaultTargets, then host.
func ResolveTargets(env Environ, meta Meta) []string {
	if t := ParseTargets(env.Get(EnvTargets)); len(t) > 0 {
		return t
	}
	if len(meta.DefaultTargets) > 0 {
		return append([]string(nil), meta.DefaultTargets...)
	}
	return []string{HostTarget()}
}

// VersionBare strips a leading "v" from a tag for artifact names.
func VersionBare(version string) string {
	return strings.TrimPrefix(version, "v")
}
