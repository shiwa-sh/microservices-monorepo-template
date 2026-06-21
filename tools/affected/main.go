// Command affected emits a JSON manifest describing which services, apps,
// libraries, and infra are affected by the current diff against the merge base
// with origin/master. Consumed by CI workflows (ADR-0002 §"Affected detection").
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
)

// Manifest is the JSON shape printed to stdout.
type Manifest struct {
	Global   bool     `json:"global"`
	Services []string `json:"services"`
	Apps     []string `json:"apps"`
	Libs     []string `json:"libs"`
	Reason   string   `json:"reason,omitempty"`
}

var (
	baseRef = flag.String("base", "origin/master", "git ref to diff against (merge-base)")
	all     = flag.Bool("all", false, "force --global=true (override affected detection)")
)

func main() {
	flag.Parse()
	files, err := changedFiles(context.Background(), *baseRef)
	if err != nil {
		failf("git diff failed: %v", err)
	}
	m := classify(files, *all)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	err = enc.Encode(m)
	if err != nil {
		failf("encode: %v", err)
	}
}

func changedFiles(ctx context.Context, base string) ([]string, error) {
	// #nosec G204 -- base is an operator-supplied git ref; this is a local CI helper, not a server.
	out, err := exec.CommandContext(ctx, "git", "diff", "--name-only", base+"...HEAD").Output()
	if err != nil {
		return nil, fmt.Errorf("git diff: %w", err)
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil, nil
	}
	return lines, nil
}

// classify maps the given list of changed paths onto the affected scopes.
// Exported (well — unexported but accessible within the package) for testing.
func classify(files []string, forceAll bool) Manifest {
	m := Manifest{
		Services: []string{},
		Apps:     []string{},
		Libs:     []string{},
	}

	if forceAll {
		m.Global = true
		m.Reason = "--all flag"
		return m
	}

	svcSet := map[string]struct{}{}
	appSet := map[string]struct{}{}
	libSet := map[string]struct{}{}

	for _, f := range files {
		switch {
		// Anything that affects the whole repo's build graph.
		case isGlobalTrigger(f):
			m.Global = true
			m.Reason = "global trigger: " + f
			return Manifest{Global: true, Reason: m.Reason, Services: []string{}, Apps: []string{}, Libs: []string{}}

		case strings.HasPrefix(f, "services/"):
			p := segment(f, 1)
			if p != "" {
				svcSet[p] = struct{}{}
			}

		case strings.HasPrefix(f, "apps/"):
			p := segment(f, 1)
			if p != "" {
				appSet[p] = struct{}{}
			}

		case strings.HasPrefix(f, "libs/go/sdks/") || strings.HasPrefix(f, "libs/ts/sdks/"):
			// A change under libs/{go,ts}/sdks/<service>/ affects consumers of that service's client.
			// We mark the underlying service as affected; downstream consumers are handled by
			// `go list -deps` in CI when this turns out to be insufficient.
			p := segment(f, 3)
			if p != "" {
				svcSet[p] = struct{}{}
			}

		case strings.HasPrefix(f, "libs/go/") || strings.HasPrefix(f, "libs/ts/"):
			p := segment(f, 2)
			if p != "" {
				libSet[p] = struct{}{}
			}
		}
	}

	m.Services = sortedKeys(svcSet)
	m.Apps = sortedKeys(appSet)
	m.Libs = sortedKeys(libSet)
	return m
}

// isGlobalTrigger reports whether a changed path forces a full-repo build.
func isGlobalTrigger(f string) bool {
	exact := []string{"go.mod", "go.sum", "package.json", "bun.lockb", ".mise.toml"}
	if slices.Contains(exact, f) {
		return true
	}
	return strings.HasPrefix(f, "infra/") ||
		strings.HasPrefix(f, "tools/") ||
		strings.HasPrefix(f, ".github/")
}

// segment returns the n-th path segment (0-indexed), or "" if absent.
func segment(p string, n int) string {
	parts := strings.Split(p, "/")
	if len(parts) <= n {
		return ""
	}
	return parts[n]
}

func sortedKeys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func failf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
