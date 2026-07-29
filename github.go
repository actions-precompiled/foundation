package foundation

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// DefaultGitHub uses curl for API JSON and gh for release mutations when available.
type DefaultGitHub struct {
	Runner Runner
	Env    Environ
	Stderr io.Writer
	// UserAgent for curl requests.
	UserAgent string
}

// NewDefaultGitHub constructs a GitHub client.
func NewDefaultGitHub(runner Runner, env Environ, stderr io.Writer) *DefaultGitHub {
	return &DefaultGitHub{
		Runner:    runner,
		Env:       env,
		Stderr:    stderr,
		UserAgent: "actions-precompiled-foundation",
	}
}

func (g *DefaultGitHub) token() string {
	return EnvFirst(g.Env, EnvGitHubToken, EnvGitHubTokenAlt)
}

func (g *DefaultGitHub) apiJSON(ctx context.Context, path string) (json.RawMessage, error) {
	url := "https://api.github.com" + path
	args := []string{
		"--fail", "--silent", "--show-error", "--location",
		"--max-time", "60",
		"-H", "Accept: application/vnd.github+json",
		"-H", "User-Agent: " + g.UserAgent,
		"-H", "X-GitHub-Api-Version: 2022-11-28",
	}
	if tok := g.token(); tok != "" {
		args = append(args, "-H", "Authorization: Bearer "+tok)
	}
	args = append(args, url)

	rw, ok := g.Runner.(RunnerWithOpts)
	var out string
	var err error
	if ok {
		out, err = rw.OutputWith(ctx, RunOpts{Stderr: g.Stderr}, "curl", args...)
	} else {
		out, err = g.Runner.Output(ctx, "curl", args...)
	}
	if err != nil {
		return nil, fmt.Errorf("curl %s: %w", path, err)
	}
	return json.RawMessage(out), nil
}

func (g *DefaultGitHub) ListUpstreamTags(ctx context.Context, ownerRepo string) ([]string, error) {
	var tags []string
	page := 1
	for {
		raw, err := g.apiJSON(ctx, fmt.Sprintf("/repos/%s/releases?per_page=100&page=%d", ownerRepo, page))
		if err != nil {
			break
		}
		var items []struct {
			TagName string `json:"tag_name"`
		}
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, err
		}
		if len(items) == 0 {
			break
		}
		for _, it := range items {
			if it.TagName != "" {
				tags = append(tags, it.TagName)
			}
		}
		if len(items) < 100 {
			break
		}
		page++
	}
	if len(tags) > 0 {
		return tags, nil
	}

	page = 1
	for {
		raw, err := g.apiJSON(ctx, fmt.Sprintf("/repos/%s/tags?per_page=100&page=%d", ownerRepo, page))
		if err != nil {
			return nil, err
		}
		var items []struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, err
		}
		if len(items) == 0 {
			break
		}
		for _, it := range items {
			if it.Name != "" {
				tags = append(tags, it.Name)
			}
		}
		if len(items) < 100 {
			break
		}
		page++
	}
	return tags, nil
}

func (g *DefaultGitHub) ListReleasedTags(ctx context.Context) ([]string, error) {
	if _, err := exec.LookPath("gh"); err == nil {
		out, err := g.Runner.Output(ctx, "gh", "release", "list", "--limit", "1000", "--json", "tagName", "--jq", ".[].tagName")
		if err == nil {
			var tags []string
			for _, line := range strings.Split(out, "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					tags = append(tags, line)
				}
			}
			return tags, nil
		}
	}

	repo := g.Env.Get(EnvGitHubRepo)
	if repo == "" {
		g.logf("No gh and no GITHUB_REPOSITORY — treating released set as empty")
		return nil, nil
	}

	var tags []string
	page := 1
	for {
		raw, err := g.apiJSON(ctx, fmt.Sprintf("/repos/%s/releases?per_page=100&page=%d", repo, page))
		if err != nil {
			g.logf("Warning: could not list releases (%v); assuming none", err)
			return nil, nil
		}
		var items []struct {
			TagName string `json:"tag_name"`
		}
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, err
		}
		if len(items) == 0 {
			break
		}
		for _, it := range items {
			if it.TagName != "" {
				tags = append(tags, it.TagName)
			}
		}
		if len(items) < 100 {
			break
		}
		page++
	}
	return tags, nil
}

func (g *DefaultGitHub) LatestReleaseTag(ctx context.Context, ownerRepo string) (string, error) {
	raw, err := g.apiJSON(ctx, fmt.Sprintf("/repos/%s/releases/latest", ownerRepo))
	if err != nil {
		return "", err
	}
	var item struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(raw, &item); err != nil {
		return "", err
	}
	if item.TagName == "" {
		return "", fmt.Errorf("%w for %s", ErrEmptyReleaseTag, ownerRepo)
	}
	return item.TagName, nil
}

func (g *DefaultGitHub) CreateRelease(ctx context.Context, req ReleaseRequest) error {
	args := []string{
		"release", "create", req.Tag,
		"--title", req.Title,
		"--notes", req.Notes,
	}
	if !req.Latest {
		args = append(args, "--latest=false")
	}
	if err := g.Runner.Run(ctx, "gh", args...); err != nil {
		g.logf("gh release create: %v (will try upload)", err)
	}
	for _, asset := range req.Assets {
		if err := g.Runner.Run(ctx, "gh", "release", "upload", req.Tag, "--clobber", asset); err != nil {
			return fmt.Errorf("upload %s: %w", asset, err)
		}
	}
	return nil
}

func (g *DefaultGitHub) DeleteRelease(ctx context.Context, tag string) error {
	return g.Runner.Run(ctx, "gh", "release", "delete", tag, "--yes", "--cleanup-tag")
}

func (g *DefaultGitHub) logf(format string, args ...any) {
	w := g.Stderr
	if w == nil {
		w = os.Stderr
	}
	fmt.Fprintf(w, format+"\n", args...)
}
