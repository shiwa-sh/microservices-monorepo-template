// Command affected emits a JSON manifest describing which services, apps,
// libraries, and infra are affected by the current diff against the merge base
// with origin/master. Consumed by CI workflows (ADR-0002 §"Affected detection").
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
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
	files, err := changedFiles(*baseRef)
	if err != nil {
		fail("git diff failed: %v", err)
	}
	m := classify(files, *all)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	err = enc.Encode(m)
	if err != nil {
		fail("encode: %v", err)
	}
}

func changedFiles(base string) ([]string, error) {
	out, err := exec.Command("git", "diff", "--name-only", base+"...HEAD").Output()
	if err != nil {
		return nil, err
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
		case f == "go.mod" || f == "go.sum" ||
			f == "package.json" || f == "bun.lockb" ||
			f == ".mise.toml" ||
			strings.HasPrefix(f, "infra/") ||
			strings.HasPrefix(f, "tools/") ||
			strings.HasPrefix(f, ".github/"):
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
	// Cheap stable sort without pulling in "sort": tiny insertion sort is fine here.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
	return keys
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
