// Package version holds release metadata injected at link time (-ldflags -X).
package version

import "fmt"

// Variables are set via -ldflags at build time.
var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

// String returns a single-line description for CLI --version output.
func String() string {
	v := Version
	if v == "" {
		v = "dev"
	}
	return fmt.Sprintf("%s (commit %s, built %s)", v, Commit, Date)
}
