package foundation

import (
	"context"
	"fmt"
)

// PlanVersions returns the ordered list of versions to build.
//
// Rules (no magic):
//   - non-empty cliVersions → use them as-is
//   - forceAll → every upstream tag
//   - else → upstream tags not present in this repo's releases
func PlanVersions(ctx context.Context, deps Deps, meta Meta, cliVersions []string, forceAll bool) ([]string, error) {
	meta = meta.Normalize()
	if len(cliVersions) > 0 {
		return append([]string(nil), cliVersions...), nil
	}

	if deps.GitHub == nil {
		return nil, fmt.Errorf("plan versions: %w", ErrGitHubNil)
	}

	deps.Logf("Fetching upstream from %s...", meta.UpstreamRepoAPI)
	upstream, err := deps.GitHub.ListUpstreamTags(ctx, meta.UpstreamRepoAPI)
	if err != nil {
		return nil, fmt.Errorf("list upstream tags: %w", err)
	}
	upstream = uniqueSorted(upstream)
	deps.Logf("  upstream: %d", len(upstream))

	if forceAll {
		deps.Logf("  force_all/recreate: all upstream tags (including already released)")
		return upstream, nil
	}

	deps.Logf("Fetching existing releases from this repository...")
	released, err := deps.GitHub.ListReleasedTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("list released tags: %w", err)
	}
	releasedSet := make(map[string]struct{}, len(released))
	for _, t := range released {
		releasedSet[t] = struct{}{}
	}
	deps.Logf("  released: %d", len(releasedSet))

	var missing []string
	for _, t := range upstream {
		if _, ok := releasedSet[t]; !ok {
			missing = append(missing, t)
		}
	}
	deps.Logf("  missing: %d", len(missing))
	return missing, nil
}

func uniqueSorted(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return SortVersionStrings(out)
}
