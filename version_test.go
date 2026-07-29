package foundation_test

import (
	"testing"

	"github.com/actions-precompiled/foundation"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantLen int
		wantNum string
	}{
		{name: "simple", input: "1.2.3", wantLen: 3, wantNum: "1.2.3"},
		{name: "v prefix", input: "v1.2.3", wantLen: 3, wantNum: "1.2.3"},
		{name: "latest", input: "latest", wantLen: 0, wantNum: ""},
		{name: "trunk", input: "trunk", wantLen: 0, wantNum: ""},
		{name: "prerelease", input: "1.0.0-beta", wantLen: 3, wantNum: "1.0.0"},
		{name: "two parts", input: "3.14", wantLen: 2, wantNum: "3.14"},
		{name: "llvmorg", input: "llvmorg-18.1.0", wantLen: 3, wantNum: "18.1.0"},
		{name: "trunk sha", input: "trunk-f1ba92abeffc", wantLen: 0, wantNum: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := foundation.ParseVersion(tt.input)
			if got.Original != tt.input {
				t.Errorf("Original = %q, want %q", got.Original, tt.input)
			}
			if len(got.Parts) != tt.wantLen {
				t.Errorf("len(Parts) = %d, want %d (%v)", len(got.Parts), tt.wantLen, got.Parts)
			}
			if got.NumericString() != tt.wantNum {
				t.Errorf("NumericString = %q, want %q", got.NumericString(), tt.wantNum)
			}
		})
	}
}

func TestVersionCompare(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want int
	}{
		{name: "equal", a: "1.2.3", b: "1.2.3", want: 0},
		{name: "less", a: "1.2.3", b: "1.2.4", want: -1},
		{name: "greater", a: "2.0.0", b: "1.9.9", want: 1},
		{name: "v prefix equal", a: "v1.0.0", b: "1.0.0", want: 0},
		{name: "latest vs version", a: "latest", b: "99.0.0", want: 1},
		{name: "version vs latest", a: "1.0.0", b: "latest", want: -1},
		{name: "both latest", a: "latest", b: "latest", want: 0},
		{name: "different lengths", a: "1.0", b: "1.0.0", want: 0},
		{name: "different lengths unequal", a: "1.0", b: "1.0.1", want: -1},
		{name: "llvmorg order", a: "llvmorg-17.0.6", b: "llvmorg-18.1.0", want: -1},
		{name: "trunk > release", a: "trunk", b: "llvmorg-99.0.0", want: 1},
		{name: "latest > trunk", a: "latest", b: "trunk", want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := foundation.ParseVersion(tt.a).Compare(foundation.ParseVersion(tt.b))
			if got != tt.want {
				t.Errorf("Compare(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestSortVersionStrings(t *testing.T) {
	in := []string{"v2.0.0", "1.0.0", "llvmorg-18.1.0", "llvmorg-17.0.6", "trunk"}
	got := foundation.SortVersionStrings(in)
	// oldest first by numeric core: 1 < 2 < 17 < 18; trunk last
	want := []string{"1.0.0", "v2.0.0", "llvmorg-17.0.6", "llvmorg-18.1.0", "trunk"}
	if len(got) != len(want) {
		t.Fatalf("len=%d got=%v", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v\nwant %v", got, want)
		}
	}
}
