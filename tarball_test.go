package foundation_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/actions-precompiled/foundation"
)

func TestWriteExtractTarGz(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	root := filepath.Join(dir, "csmith")
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin", "csmith"), []byte("exe"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "BUILDINFO.txt"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(dir, "out.tar.gz")
	if err := foundation.WriteTarGz(archive, dir, "csmith"); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "unpacked")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := foundation.ExtractTarGz(archive, dst); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dst, "csmith", "BUILDINFO.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "ok\n" {
		t.Fatalf("got %q", got)
	}
	if _, err := os.Stat(filepath.Join(dst, "csmith", "bin", "csmith")); err != nil {
		t.Fatal(err)
	}
}
