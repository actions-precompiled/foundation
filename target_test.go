package foundation_test

import (
	"testing"

	"github.com/actions-precompiled/foundation"
)

func TestIsNativeTarget(t *testing.T) {
	t.Parallel()
	cases := []struct {
		target string
		native bool
		win    bool
		darwin bool
	}{
		{foundation.TargetLinuxAMD64, false, false, false},
		{foundation.TargetLinuxAArch64, false, false, false},
		{foundation.TargetWindowsAMD64, true, true, false},
		{foundation.TargetWindowsARM64, true, true, false},
		{foundation.TargetDarwinAMD64, true, false, true},
		{foundation.TargetDarwinAArch64, true, false, true},
		{"macos-arm64", true, false, true},
		{"", false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.target, func(t *testing.T) {
			t.Parallel()
			if got := foundation.IsNativeTarget(tc.target); got != tc.native {
				t.Fatalf("IsNativeTarget(%q)=%v want %v", tc.target, got, tc.native)
			}
			if got := foundation.IsWindowsTarget(tc.target); got != tc.win {
				t.Fatalf("IsWindowsTarget(%q)=%v want %v", tc.target, got, tc.win)
			}
			if got := foundation.IsDarwinTarget(tc.target); got != tc.darwin {
				t.Fatalf("IsDarwinTarget(%q)=%v want %v", tc.target, got, tc.darwin)
			}
		})
	}
}

func TestTargetsNeedDocker(t *testing.T) {
	t.Parallel()
	if foundation.TargetsNeedDocker([]string{foundation.TargetWindowsAMD64, foundation.TargetDarwinAArch64}) {
		t.Fatal("native-only targets should not need docker")
	}
	if !foundation.TargetsNeedDocker([]string{foundation.TargetLinuxAMD64}) {
		t.Fatal("linux target needs docker")
	}
	if !foundation.TargetsNeedDocker([]string{foundation.TargetWindowsAMD64, foundation.TargetLinuxAMD64}) {
		t.Fatal("mixed targets need docker")
	}
}

func TestTargetGOOSGOARCH(t *testing.T) {
	t.Parallel()
	cases := []struct {
		target, goos, goarch string
	}{
		{foundation.TargetLinuxAMD64, "linux", "amd64"},
		{foundation.TargetLinuxAArch64, "linux", "arm64"},
		{foundation.TargetWindowsAMD64, "windows", "amd64"},
		{foundation.TargetWindowsARM64, "windows", "arm64"},
		{foundation.TargetDarwinAMD64, "darwin", "amd64"},
		{foundation.TargetDarwinAArch64, "darwin", "arm64"},
		{"macos-arm64", "darwin", "arm64"},
	}
	for _, tc := range cases {
		t.Run(tc.target, func(t *testing.T) {
			t.Parallel()
			goos, goarch := foundation.TargetGOOSGOARCH(tc.target)
			if goos != tc.goos || goarch != tc.goarch {
				t.Fatalf("TargetGOOSGOARCH(%q)=%s/%s want %s/%s", tc.target, goos, goarch, tc.goos, tc.goarch)
			}
		})
	}
}

func TestRunnerForTarget(t *testing.T) {
	t.Parallel()
	cases := []struct {
		target, runner string
	}{
		{foundation.TargetLinuxAMD64, "ubuntu-latest"},
		{foundation.TargetLinuxAArch64, "ubuntu-24.04-arm"},
		{foundation.TargetWindowsAMD64, "windows-latest"},
		{foundation.TargetWindowsARM64, "windows-11-arm"},
		{foundation.TargetDarwinAMD64, "macos-13"},
		{foundation.TargetDarwinAArch64, "macos-latest"},
		{"macos-arm64", "macos-latest"},
	}
	for _, tc := range cases {
		t.Run(tc.target, func(t *testing.T) {
			t.Parallel()
			if got := foundation.RunnerForTarget(tc.target); got != tc.runner {
				t.Fatalf("RunnerForTarget(%q)=%q want %q", tc.target, got, tc.runner)
			}
		})
	}
}
