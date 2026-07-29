package foundation

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// ArtifactName returns {name}-{bareVersion}-{target}.tar.gz
func ArtifactName(packageName, version, target string) string {
	return fmt.Sprintf("%s-%s-%s.tar.gz", packageName, VersionBare(version), target)
}

// FindTarballs locates package tarballs for a version/target under outDir.
func FindTarballs(fs FileSystem, packageName, version, target, outDir string) ([]string, error) {
	if fs == nil {
		return nil, fmt.Errorf("FindTarballs: FS is nil")
	}
	if _, err := fs.Stat(outDir); err != nil {
		return nil, nil
	}

	exact := filepath.Join(outDir, ArtifactName(packageName, version, target))
	if _, err := fs.Stat(exact); err == nil {
		return []string{exact}, nil
	}

	matches, err := fs.Glob(filepath.Join(outDir, "*.tar.gz"))
	if err != nil {
		return nil, err
	}
	bare := VersionBare(version)
	var found []string
	for _, p := range matches {
		base := filepath.Base(p)
		if strings.Contains(base, bare) || strings.Contains(base, version) {
			if strings.Contains(base, target) || target == "" {
				found = append(found, p)
			}
		}
	}
	sort.Strings(found)
	return found, nil
}
