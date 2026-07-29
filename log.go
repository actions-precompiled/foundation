package foundation

import (
	"fmt"
	"io"
	"os"
)

// WriteLine writes one formatted line to w, falling back to fallback when w is nil.
// Atom used by Deps.Logf/Outf and other clients that need consistent line logging.
func WriteLine(w, fallback io.Writer, format string, args ...any) {
	if w == nil {
		w = fallback
	}
	if w == nil {
		w = os.Stderr
	}
	fmt.Fprintf(w, format+"\n", args...)
}

// Logf writes a formatted line to Deps.Stderr (or os.Stderr).
func (d Deps) Logf(format string, args ...any) {
	WriteLine(d.Stderr, os.Stderr, format, args...)
}

// Outf writes a formatted line to Deps.Stdout (or os.Stdout).
func (d Deps) Outf(format string, args ...any) {
	WriteLine(d.Stdout, os.Stdout, format, args...)
}

// RemoveAllLog removes path and logs failures (best-effort cleanup).
func (d Deps) RemoveAllLog(path, what string) {
	if d.FS == nil {
		return
	}
	if err := d.FS.RemoveAll(path); err != nil {
		d.Logf("%s: %v", what, err)
	}
}
