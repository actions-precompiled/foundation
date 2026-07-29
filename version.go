package foundation

import (
	"sort"
	"strconv"
	"strings"
)

// Version is a parseable/comparable package version string.
//
// Adapted from workspaced's internal/semver (github.com/lucasew/workspaced):
// dotted numeric parts, optional "v" prefix, special tokens, and per-segment
// numeric prefixes so "1.0.0-beta" compares as 1.0.0.
//
// Extra for actions-precompiled tags: leading non-numeric product prefixes
// (e.g. "llvmorg-18.1.0") are skipped so comparison uses the real numbers.
type Version struct {
	// Original is the input string (e.g. "v1.2.3", "llvmorg-18.1.0", "trunk").
	Original string
	// Parts are dotted numeric components used for ordering (empty for
	// specials like "latest" / "trunk").
	Parts []int
}

// ParseVersion parses s into a Version. Always succeeds; unparseable inputs
// keep Original and empty Parts.
func ParseVersion(s string) Version {
	original := s
	s = strings.TrimSpace(s)
	if s == "" {
		return Version{Original: original}
	}

	lower := strings.ToLower(s)
	switch lower {
	case "latest", "trunk", "main", "master", "git-main":
		return Version{Original: original, Parts: nil}
	}
	if strings.HasPrefix(lower, "trunk-") {
		return Version{Original: original, Parts: nil}
	}

	core := stripVersionNoise(s)
	if core == "" {
		return Version{Original: original}
	}

	segments := strings.Split(core, ".")
	nums := make([]int, 0, len(segments))
	for _, seg := range segments {
		nums = append(nums, ParseNumericPrefix(seg))
	}
	return Version{Original: original, Parts: nums}
}

// stripVersionNoise trims a leading "v" and any non-digit product prefix.
func stripVersionNoise(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	s = strings.TrimPrefix(s, "V")
	for i, r := range s {
		if r >= '0' && r <= '9' {
			return s[i:]
		}
	}
	return ""
}

// String returns the original version string.
func (v Version) String() string { return v.Original }

// Bare returns the version without a leading "v"/"V".
func (v Version) Bare() string {
	s := strings.TrimPrefix(v.Original, "v")
	return strings.TrimPrefix(s, "V")
}

// NumericString joins Parts as "major.minor.patch…" (empty if no parts).
func (v Version) NumericString() string {
	if len(v.Parts) == 0 {
		return ""
	}
	var b strings.Builder
	for i, p := range v.Parts {
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(strconv.Itoa(p))
	}
	return b.String()
}

// IsLatest reports whether this is the "latest" sentinel.
func (v Version) IsLatest() bool {
	return strings.EqualFold(strings.TrimSpace(v.Original), "latest")
}

// IsTrunk reports trunk/main-style moving tips (including trunk-<sha>).
func (v Version) IsTrunk() bool {
	s := strings.ToLower(strings.TrimSpace(v.Original))
	switch s {
	case "trunk", "main", "master", "git-main":
		return true
	}
	return strings.HasPrefix(s, "trunk-")
}

// Compare returns -1 if v < other, 0 if equal, 1 if v > other.
//
//	latest > trunk > numeric tags
//	numeric parts compared left-to-right; missing parts treated as 0
//	tie → Original Bare() string compare
func (v Version) Compare(other Version) int {
	if v.IsLatest() || other.IsLatest() {
		switch {
		case v.IsLatest() && other.IsLatest():
			return 0
		case v.IsLatest():
			return 1
		default:
			return -1
		}
	}

	if v.IsTrunk() || other.IsTrunk() {
		switch {
		case v.IsTrunk() && other.IsTrunk():
			return strings.Compare(v.Original, other.Original)
		case v.IsTrunk():
			return 1
		default:
			return -1
		}
	}

	maxLen := max(len(v.Parts), len(other.Parts))
	for i := range maxLen {
		a, b := 0, 0
		if i < len(v.Parts) {
			a = v.Parts[i]
		}
		if i < len(other.Parts) {
			b = other.Parts[i]
		}
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
	}
	// Numeric equality (workspaced): 1.0 == 1.0.0. Non-numeric leftovers only.
	if len(v.Parts) > 0 || len(other.Parts) > 0 {
		return 0
	}
	return strings.Compare(v.Bare(), other.Bare())
}

// Less reports v < other.
func (v Version) Less(other Version) bool { return v.Compare(other) < 0 }

// Greater reports v > other.
func (v Version) Greater(other Version) bool { return v.Compare(other) > 0 }

// Equal reports equality via Compare.
func (v Version) Equal(other Version) bool { return v.Compare(other) == 0 }

// ParseNumericPrefix returns the leading integer in s ("3-beta" → 3, "" → 0).
// From workspaced/internal/semver.
func ParseNumericPrefix(s string) int {
	numPart := ""
	for _, r := range s {
		if r >= '0' && r <= '9' {
			numPart += string(r)
		} else {
			break
		}
	}
	if numPart == "" {
		return 0
	}
	n, err := strconv.Atoi(numPart)
	if err != nil {
		return 0
	}
	return n
}

// Versions is a sortable slice of Version (ascending, oldest first).
type Versions []Version

func (vs Versions) Len() int           { return len(vs) }
func (vs Versions) Swap(i, j int)      { vs[i], vs[j] = vs[j], vs[i] }
func (vs Versions) Less(i, j int) bool { return vs[i].Less(vs[j]) }

// ParseVersions parses each string into a Version.
func ParseVersions(ss []string) Versions {
	out := make(Versions, len(ss))
	for i, s := range ss {
		out[i] = ParseVersion(s)
	}
	return out
}

// SortVersionStrings returns a new slice sorted ascending by [Version.Compare].
func SortVersionStrings(ss []string) []string {
	vs := ParseVersions(append([]string(nil), ss...))
	sort.Sort(vs)
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = v.Original
	}
	return out
}

// VersionBare strips a leading "v" from a tag for artifact names.
// Prefer [ParseVersion](s).Bare() when you already hold a Version.
func VersionBare(version string) string {
	return ParseVersion(version).Bare()
}
