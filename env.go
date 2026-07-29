package foundation

import (
	"os"
	"strings"
	"unicode"
)

// APC_* environment keys. The foundation never templates shell from package.toml;
// configuration is explicit flags or these env vars.

const (
	EnvTargets        = "APC_TARGETS"
	EnvPublish        = "APC_PUBLISH"
	EnvSkipSmoke      = "APC_SKIP_SMOKE"
	EnvSkipImageBuild = "APC_SKIP_IMAGE_BUILD"
	EnvRecreate       = "APC_RECREATE"
	EnvForceAll       = "APC_FORCE_ALL"
	EnvDryRun         = "APC_DRY_RUN"
	EnvImageName      = "APC_IMAGE_NAME"
	EnvImageTag       = "APC_IMAGE_TAG"
	EnvBuildOutputDir = "APC_BUILD_OUTPUT_DIR"
	EnvWorkDir        = "APC_WORK_DIR"
	EnvGitHubToken    = "GH_TOKEN" // also GITHUB_TOKEN via Lookup chain
	EnvGitHubTokenAlt = "GITHUB_TOKEN"
	EnvGitHubRepo     = "GITHUB_REPOSITORY"
	EnvVersion        = "APC_VERSION"
	EnvTarget         = "APC_TARGET"
	EnvOutputDir      = "APC_OUTPUT_DIR"
	EnvInContainer    = "APC_IN_CONTAINER"
)

// OSEnviron is the real process environment.
type OSEnviron struct{}

func (OSEnviron) Get(key string) string {
	return os.Getenv(key)
}

func (OSEnviron) Lookup(key string) (string, bool) {
	return os.LookupEnv(key)
}

func (OSEnviron) Environ() []string {
	return os.Environ()
}

// MapEnviron is a mutable env for tests.
type MapEnviron map[string]string

func (m MapEnviron) Get(key string) string {
	return m[key]
}

func (m MapEnviron) Lookup(key string) (string, bool) {
	v, ok := m[key]
	return v, ok
}

func (m MapEnviron) Environ() []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}

// EnvFlag is true when key is set and not a falsey value ("" / 0 / false / False).
func EnvFlag(env Environ, key string) bool {
	v, ok := env.Lookup(key)
	if !ok {
		return false
	}
	switch v {
	case "", "0", "false", "False", "FALSE", "no", "NO":
		return false
	default:
		return true
	}
}

// EnvFirst returns the first non-empty value among keys.
func EnvFirst(env Environ, keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(env.Get(k)); v != "" {
			return v
		}
	}
	return ""
}

// envNameForPackage turns "quickshell" into "QUICKSHELL" for default VersionEnv.
func envNameForPackage(name string) string {
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToUpper(r))
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}
