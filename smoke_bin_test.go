package foundation

import "testing"

func TestDynLinkFailure(t *testing.T) {
	if !dynLinkFailure("error while loading shared libraries: libLLVM.so: cannot open shared object file", nil) {
		t.Fatal("expected dyn link fail")
	}
	if dynLinkFailure("usage: tool [options]", nil) {
		t.Fatal("usage is not dyn link fail")
	}
}

func TestSmokeBinDirHelpEmpty(t *testing.T) {
	dir := t.TempDir()
	// no bin/
	if err := SmokeBinDirHelp(t.Context(), DefaultDeps(dir), dir, BinHelpOpts{}); err == nil {
		t.Fatal("expected error without bin/")
	}
}
