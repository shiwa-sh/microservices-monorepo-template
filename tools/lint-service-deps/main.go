// Command lint-service-deps checks that each service's declared dependencies match
// the ones its code actually has (ADR-0600, ADR-0205).
//
// A service declares what it needs in its own `.mise.toml`:
//
//	[tasks.worker]
//	depends = ["env", "dep:postgres", "dep:temporal", "svc:catalog", "svc:payment"]
//
// That list is what `mise run worker` brings up, and it is the whole inner loop.
// ADR-0600 names the omitted edge as the most likely way to regress it: a worker
// whose saga dials payment, with no `svc:payment`, starts cleanly and dies on the
// first checkout — in a way that looks like payment being broken rather than like
// payment being absent. The declaration is prose about the code, and prose drifts.
//
// So the code is asked directly. For each `cmd/<task>` the package closure is taken
// from `go list -deps`, and four signals are read out of it:
//
//	go.temporal.io/sdk/client   → dep:temporal
//	libs/go/authz               → dep:openfga
//	"DATABASE_URL"              → dep:postgres
//	"<SERVICE>_URL"             → svc:<service>, when a sibling service has that name
//
// The first two are imports, so they are exact. The last two are string literals in
// the closure's own source, because a call to another service over plain HTTP has
// no import to find — the base URL is the only thing in the code that names the
// callee at all. That is a deliberate limit, not an oversight: an activity that
// hardcodes a URL rather than reading it from the environment is invisible here,
// and is also a defect ADR-0205 forbids for its own reasons.
//
// # Both directions are reported
//
// A missing declaration costs a confusing failure. An extra one costs inner-loop
// time on every run — `dep:temporal` on a service that never opens a client waits
// for a database and a schema Job nobody is going to use. Both are drift between
// the same two things, so both are findings.
//
// # What is deliberately NOT a signal
//
// KRATOS_ADMIN_URL. Kratos is not a `deps.yaml` component — the base tier runs the
// real one — so there is no `dep:` to declare, and reporting it would mean adding a
// name that no task can satisfy.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
)

// listTimeout bounds one `go list` call.
const listTimeout = 2 * time.Minute

// The two entrypoints a service can have. Each gets its own comparison, because
// each has its own dependency list — orders' server has no cross-service calls and
// its worker has two.
var commands = []string{"server", "worker"}

// urlEnv matches an environment variable naming a base URL, e.g. "CATALOG_URL".
var urlEnv = regexp.MustCompile(`"([A-Z][A-Z0-9_]*)_URL"`)

// dependsBlock captures a `depends = [ … ]` array, which may span lines.
var dependsBlock = regexp.MustCompile(`(?s)depends\s*=\s*\[(.*?)\]`)

// taskHeader matches a `[tasks.<name>]` section header.
var taskHeader = regexp.MustCompile(`(?m)^\[tasks\.([a-z0-9:-]+)\]`)

// listed matches each quoted entry inside a depends array.
var listed = regexp.MustCompile(`"([^"]+)"`)

func main() {
	module, err := modulePath()
	if err != nil {
		failf(err)
	}
	services, err := serviceNames()
	if err != nil {
		failf(err)
	}

	var problems []string
	checked := 0

	for _, svc := range services {
		declared, err := declaredDeps(filepath.Join("services", svc, ".mise.toml"))
		if err != nil {
			failf(err)
		}
		for _, cmd := range commands {
			dir := filepath.Join("services", svc, "cmd", cmd)
			if !exists(dir) {
				continue
			}
			checked++

			required, err := requiredDeps(module, svc, services, dir)
			if err != nil {
				failf(err)
			}
			problems = append(problems, compare(svc, cmd, required, declared[cmd])...)
		}
	}

	if len(problems) > 0 {
		sort.Strings(problems)
		for _, p := range problems {
			_, _ = fmt.Fprintf(os.Stderr, "✗ %s\n", p)
		}
		_, _ = fmt.Fprintf(os.Stderr, "\n  `depends` is what `mise run <task>` brings up. A missing entry is a\n")
		_, _ = fmt.Fprintf(os.Stderr, "  service that starts and then fails on its first real request.\n")
		os.Exit(1)
	}

	_, _ = fmt.Fprintf(os.Stdout, "✓ %d service entrypoints declare exactly what they use\n", checked)
}

// compare produces one finding per direction of drift.
func compare(svc, cmd string, required, declared map[string]bool) []string {
	var out []string
	task := fmt.Sprintf("services/%s/.mise.toml [tasks.%s]", svc, cmd)

	for _, dep := range sortedKeys(required) {
		if !declared[dep] {
			out = append(out, fmt.Sprintf("%s: uses %s but does not declare it", task, dep))
		}
	}
	for _, dep := range sortedKeys(declared) {
		// Only the two namespaces this can speak to. `env` and any future task name
		// in the list are somebody else's business.
		if !strings.HasPrefix(dep, "dep:") && !strings.HasPrefix(dep, "svc:") {
			continue
		}
		if !required[dep] {
			out = append(out, fmt.Sprintf("%s: declares %s but nothing in the closure uses it", task, dep))
		}
	}
	return out
}

// requiredDeps reads the package closure of one entrypoint and returns the
// dependency names the code implies.
func requiredDeps(module, svc string, services []string, dir string) (map[string]bool, error) {
	packages, err := closure(dir)
	if err != nil {
		return nil, err
	}

	out := map[string]bool{}
	for _, pkg := range packages {
		switch pkg.ImportPath {
		case "go.temporal.io/sdk/client":
			out["dep:temporal"] = true
		case module + "/libs/go/authz":
			out["dep:openfga"] = true
		}
		// Only first-party packages have source worth reading: a string literal in a
		// dependency is not this repository's declaration of anything.
		if !strings.HasPrefix(pkg.ImportPath, module) {
			continue
		}
		names, err := envNames(pkg.Dir)
		if err != nil {
			return nil, err
		}
		for _, name := range names {
			if name == "DATABASE" {
				out["dep:postgres"] = true
				continue
			}
			callee := strings.ToLower(name)
			if callee != svc && slices.Contains(services, callee) {
				out["svc:"+callee] = true
			}
		}
	}
	return out, nil
}

// A pkg is the part of `go list -json` this needs. The tags are explicit because
// the field names happen to match: `go list` emits Go-style keys, and relying on
// the case-insensitive fallback would break silently if it ever stopped.
type pkg struct {
	ImportPath string `json:"ImportPath"`
	Dir        string `json:"Dir"`
}

// closure runs `go list -deps -json` for one entrypoint. Shelling out to the go
// tool rather than reimplementing import resolution: build tags, vendoring and the
// module graph are its job, and it is already pinned in .mise.toml.
func closure(dir string) ([]pkg, error) {
	// The argument is a path this tool constructed from a directory listing under
	// services/, never anything a caller supplied — there are no flags and no input.
	// A context so the gate cannot hang on a wedged toolchain: `go list` on a cold
	// module cache can reach the network, and a lint run that never returns is worse
	// than one that fails.
	ctx, cancel := context.WithTimeout(context.Background(), listTimeout)
	defer cancel()
	//nolint:gosec // fixed argv; the path comes from a repo directory walk
	cmd := exec.CommandContext(ctx, "go", "list", "-deps", "-json", "./"+filepath.ToSlash(dir))
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list %s: %w", dir, err)
	}

	var packages []pkg
	decoder := json.NewDecoder(strings.NewReader(string(out)))
	for decoder.More() {
		var p pkg
		err = decoder.Decode(&p)
		if err != nil {
			return nil, fmt.Errorf("decode go list output for %s: %w", dir, err)
		}
		packages = append(packages, p)
	}
	return packages, nil
}

// envNames returns the `X` of every "X_URL" string literal in a package's
// non-test source.
func envNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	var out []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", filepath.Join(dir, name), err)
		}
		for _, m := range urlEnv.FindAllStringSubmatch(string(data), -1) {
			out = append(out, m[1])
		}
	}
	return out, nil
}

// declaredDeps maps a task name to the `depends` entries it lists. The parse is a
// regex rather than a TOML library: the module has no TOML dependency, and adding
// one to read two array literals is a poor trade. The shape it accepts is the shape
// mise itself documents — a `depends` array inside a `[tasks.x]` section.
func declaredDeps(path string) (map[string]map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	text := string(data)

	out := map[string]map[string]bool{}
	headers := taskHeader.FindAllStringSubmatchIndex(text, -1)
	for i, header := range headers {
		name := text[header[2]:header[3]]
		end := len(text)
		if i+1 < len(headers) {
			end = headers[i+1][0]
		}
		section := text[header[1]:end]

		deps := map[string]bool{}
		block := dependsBlock.FindStringSubmatch(section)
		if block != nil {
			for _, m := range listed.FindAllStringSubmatch(block[1], -1) {
				deps[m[1]] = true
			}
		}
		out[name] = deps
	}
	return out, nil
}

// serviceNames lists the deployable services. `_`-prefixed directories are
// scaffolding: `_template` is built under its own tag by lint:template-build and
// its tasks are examples rather than declarations.
func serviceNames() ([]string, error) {
	entries, err := os.ReadDir("services")
	if err != nil {
		return nil, fmt.Errorf("read services: %w", err)
	}
	var out []string
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), "_") {
			continue
		}
		out = append(out, entry.Name())
	}
	sort.Strings(out)
	return out, nil
}

func modulePath() (string, error) {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return "", fmt.Errorf("read go.mod: %w", err)
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		path, ok := strings.CutPrefix(line, "module ")
		if ok {
			return strings.TrimSpace(path), nil
		}
	}
	return "", errors.New("go.mod: no module line")
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func failf(err error) {
	_, _ = fmt.Fprintf(os.Stderr, "✗ %v\n", err)
	os.Exit(1)
}
