package foundation

import (
	"runtime"
	"strings"
)

// Known matrix targets used by actions-precompiled packages.
const (
	TargetLinuxAMD64    = "linux-amd64"
	TargetLinuxAArch64  = "linux-aarch64"
	TargetWindowsAMD64  = "windows-amd64"
	TargetWindowsARM64  = "windows-arm64"
	TargetDarwinAMD64   = "darwin-amd64"
	TargetDarwinAArch64 = "darwin-aarch64"
)

// HostTarget returns the native matrix target for the current machine.
func HostTarget() string {
	switch runtime.GOOS {
	case "windows":
		if runtime.GOARCH == "arm64" {
			return TargetWindowsARM64
		}
		return TargetWindowsAMD64
	case "darwin":
		if runtime.GOARCH == "arm64" {
			return TargetDarwinAArch64
		}
		return TargetDarwinAMD64
	default:
		if runtime.GOARCH == "arm64" {
			return TargetLinuxAArch64
		}
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

// IsWindowsTarget reports windows-* matrix targets.
func IsWindowsTarget(target string) bool {
	return strings.HasPrefix(target, "windows-")
}

// IsDarwinTarget reports darwin-* / macos-* matrix targets.
func IsDarwinTarget(target string) bool {
	return strings.HasPrefix(target, "darwin-") || strings.HasPrefix(target, "macos-")
}

// IsNativeTarget reports targets whose Work runs on the host (not Docker).
func IsNativeTarget(target string) bool {
	return IsWindowsTarget(target) || IsDarwinTarget(target)
}

// TargetsNeedDocker is true when any target is built via the Linux docker path.
// Used to skip docker image builds on pure Windows/macOS runs.
func TargetsNeedDocker(targets []string) bool {
	for _, t := range targets {
		if t != "" && !IsNativeTarget(t) {
			return true
		}
	}
	return false
}
