// Command lint-thin-workflows enforces ADR-0102's rule that workflow YAML checks
// out, sets up the toolchain, and calls `mise run ci:*` — pipeline logic is not
// written in YAML.
//
// The gate is on `run:` steps only. A `uses:` step delegates to an action and
// carries no logic of its own, so checkout, the composite setup action, and the
// docker and pull-request actions are outside the rule rather than allow-listed
// exceptions to it.
//
// A `run:` block may contain, per line: a `mise run` invocation, `set -euo
// pipefail`, or a comment. A trailing redirect into $GITHUB_OUTPUT or $GITHUB_ENV
// is permitted on a `mise run` line, because passing a value between steps is
// forge plumbing rather than logic — the task still decides what the value is.
// Everything else is logic that belongs in a task: the point is that a pipeline
// can be reproduced locally and survives the move to another forge (ADR-0102).
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// A finding is one offending line inside one run: block.
type finding struct {
	file string
	line int
	text string
}

func main() {
	files, err := workflowFiles()
	if err != nil {
		failf("%v", err)
	}
	if len(files) == 0 {
		failf("no workflow files found under .github/")
	}

	var findings []finding
	for _, f := range files {
		fs, err := check(f)
		if err != nil {
			failf("%s: %v", f, err)
		}
		findings = append(findings, fs...)
	}

	if len(findings) > 0 {
		_, _ = fmt.Fprintln(os.Stderr, "✗ workflow steps carry pipeline logic (ADR-0102):")
		for _, f := range findings {
			_, _ = fmt.Fprintf(os.Stderr, "  %s:%d: %s\n", f.file, f.line, f.text)
		}
		_, _ = fmt.Fprintln(os.Stderr, "\n  Move the logic into a mise task and call it from the step.")
		os.Exit(1)
	}
	_, _ = fmt.Fprintf(os.Stdout, "✓ workflow steps call mise tasks (%d files)\n", len(files))
}

func workflowFiles() ([]string, error) {
	var out []string
	for _, pattern := range []string{
		filepath.Join(".github", "workflows", "*.yml"),
		filepath.Join(".github", "workflows", "*.yaml"),
		filepath.Join(".github", "actions", "*", "action.yml"),
		filepath.Join(".github", "actions", "*", "action.yaml"),
	} {
		m, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("glob %s: %w", pattern, err)
		}
		out = append(out, m...)
	}
	return out, nil
}

func check(path string) ([]finding, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var root yaml.Node
	err = yaml.Unmarshal(data, &root)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	var findings []finding
	// script.Line is the line of the scalar's first content line for a block
	// scalar, which is what makes the offsets below point at the real source.
	collect := func(script *yaml.Node) {
		for i, line := range strings.Split(script.Value, "\n") {
			bad := offending(line)
			if bad != "" {
				findings = append(findings, finding{path, script.Line + i, bad})
			}
		}
	}
	walk(&root, collect)
	return findings, nil
}

// walk calls fn with the script of every step under a `steps:` sequence. Scoping
// to steps matters: a reusable workflow may declare an output literally named
// `run`, which is a value rather than a script.
func walk(n *yaml.Node, fn func(*yaml.Node)) {
	if n.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].Value != "steps" || n.Content[i+1].Kind != yaml.SequenceNode {
				continue
			}
			for _, step := range n.Content[i+1].Content {
				if step.Kind != yaml.MappingNode {
					continue
				}
				for j := 0; j+1 < len(step.Content); j += 2 {
					if step.Content[j].Value == "run" && step.Content[j+1].Kind == yaml.ScalarNode {
						fn(step.Content[j+1])
					}
				}
			}
		}
	}
	for _, c := range n.Content {
		walk(c, fn)
	}
}

// offending returns the trimmed line if it is logic, or "" if it is permitted.
func offending(line string) string {
	s := strings.TrimSpace(line)
	if s == "" || strings.HasPrefix(s, "#") || s == "set -euo pipefail" {
		return ""
	}
	// A forge step-output redirect is plumbing; strip it before judging the command.
	for _, sink := range []string{`>> "$GITHUB_OUTPUT"`, `>> "$GITHUB_ENV"`} {
		cut, found := strings.CutSuffix(s, sink)
		if found {
			s = strings.TrimSpace(cut)
			break
		}
	}
	if strings.HasPrefix(s, "mise run ") {
		return ""
	}
	return strings.TrimSpace(line)
}

func failf(format string, a ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "✗ "+format+"\n", a...)
	os.Exit(1)
}
