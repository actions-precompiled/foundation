package foundation

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ArtifactName returns {name}-{bareVersion}-{target}.tar.gz
func ArtifactName(packageName, version, target string) string {
	return ArtifactNameExt(packageName, version, target, ".tar.gz")
}

// ArtifactNameExt is ArtifactName with a caller-chosen suffix (".tar.gz", ".dmg", ".zip").
func ArtifactNameExt(packageName, version, target, ext string) string {
	if ext == "" {
		ext = ".tar.gz"
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return fmt.Sprintf("%s-%s-%s%s", packageName, VersionBare(version), target, ext)
}

// artifactGlobs is the set of release files FindTarballs will pick up.
var artifactGlobs = []string{"*.tar.gz", "*.zip", "*.dmg"}

// FindTarballs locates package artifacts for a version/target under outDir.
// Matches .tar.gz, .zip, and .dmg.
func FindTarballs(fs FileSystem, packageName, version, target, outDir string) ([]string, error) {
	if fs == nil {
		return nil, fmt.Errorf("FindTarballs: %w", ErrFSNil)
	}
	if _, err := fs.Stat(outDir); err != nil {
		return nil, nil
	}

	for _, ext := range []string{".tar.gz", ".zip", ".dmg"} {
		exact := filepath.Join(outDir, ArtifactNameExt(packageName, version, target, ext))
		if _, err := fs.Stat(exact); err == nil {
			return []string{exact}, nil
		}
	}

	var matches []string
	for _, g := range artifactGlobs {
		got, err := fs.Glob(filepath.Join(outDir, g))
		if err != nil {
			return nil, err
		}
		matches = append(matches, got...)
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

// WriteTarGz writes dest as gzip+tar of dir/base, same as
// `tar -czf dest -C dir base`. Paths in the archive use slashes and start
// with base/.
func WriteTarGz(dest, dir, base string) error {
	root := filepath.Join(dir, base)
	if st, err := os.Stat(root); err != nil {
		return fmt.Errorf("WriteTarGz: %w", err)
	} else if !st.IsDir() {
		return fmt.Errorf("WriteTarGz: %s is not a directory", root)
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	gz := gzip.NewWriter(out)
	tw := tar.NewWriter(gz)
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel)
		link := ""
		if info.Mode()&os.ModeSymlink != 0 {
			link, err = os.Readlink(path)
			if err != nil {
				return err
			}
		}
		hdr, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		hdr.Name = name
		if info.IsDir() && !strings.HasSuffix(hdr.Name, "/") {
			hdr.Name += "/"
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tw, f)
		closeErr := f.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if err != nil {
		_ = tw.Close()
		_ = gz.Close()
		return err
	}
	if err := tw.Close(); err != nil {
		_ = gz.Close()
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	return out.Close()
}

// ExtractTarGz unpacks a gzip tarball into dst.
func ExtractTarGz(src, dst string) error {
	return extractTarGz(src, dst)
}
