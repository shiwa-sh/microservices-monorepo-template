// Package buildinfo exposes the identity of the running binary (ADR-0103): the
// git SHA it was built from, the release version, and the build time. It lets
// "what is actually deployed" be answered from the artifact itself, not inferred
// from an image tag or a ConfigMap — which is the whole point, since those can
// drift from the binary they claim to describe.
//
// Values are injected at build time via -ldflags -X (see the service Dockerfiles).
// When unset — e.g. `go run` or a `go build` without ldflags — they fall back to
// the Go VCS stamp from debug.ReadBuildInfo so local runs still self-report.
package buildinfo

import "runtime/debug"

// Set via -ldflags "-X .../libs/go/buildinfo.SHA=<sha> ...". Do not assign at runtime.
var (
	// Version is the release tag (CalVer, ADR-0103), e.g. v2026.07.0; "dev" off-release.
	Version = "dev"
	// SHA is the git commit the binary was built from.
	SHA = ""
	// BuiltAt is the build timestamp (RFC 3339).
	BuiltAt = ""
)

//nolint:gochecknoinits // one-time fallback to the Go VCS stamp at load when ldflags didn't inject.
func init() {
	if SHA != "" && BuiltAt != "" {
		return
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if SHA == "" {
				SHA = s.Value
			}
		case "vcs.time":
			if BuiltAt == "" {
				BuiltAt = s.Value
			}
		}
	}
}

// Info is the structured build identity, e.g. for the /version admin endpoint.
type Info struct {
	Version string `json:"version"`
	SHA     string `json:"sha"`
	BuiltAt string `json:"builtAt"`
}

// Get returns the current build identity.
func Get() Info { return Info{Version: Version, SHA: SHA, BuiltAt: BuiltAt} }
