// Package version exposes the build identity of the binaries. The values are
// injected at build time with -ldflags (see the Makefile) and default to
// "dev" for local builds.
package version

import "fmt"

// Set at build time via:
//
//	-ldflags "-X github.com/beark7/ember/internal/version.Version=v0.1.0 ..."
var (
	// Version is the semantic version or tag of the build, e.g. "v0.1.0".
	Version = "dev"
	// Commit is the short git commit hash of the build.
	Commit = "none"
	// Date is the build date in RFC 3339 format.
	Date = "unknown"
)

// Info is a serializable snapshot of the build identity.
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

// Get returns the build identity of the running binary.
func Get() Info {
	return Info{Version: Version, Commit: Commit, Date: Date}
}

// String renders the identity as "version (commit, date)".
func (i Info) String() string {
	return fmt.Sprintf("%s (%s, %s)", i.Version, i.Commit, i.Date)
}
