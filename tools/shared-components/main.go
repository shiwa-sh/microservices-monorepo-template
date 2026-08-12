// Command shared-components keeps the canonical OpenAPI components identical
// across every service spec (ADR-0303).
//
//	shared-components          rewrite each spec's shared region from the source
//	shared-components -check   fail if any spec's region has drifted
//
// Why the components are COPIED into each spec rather than $ref'd across files:
// a spec stays self-contained, so ogen and vacuum each read one document with no
// external resolution. The cost is duplication, and this tool is the reason the
// duplication is safe — "identical across specs" is a fact the check establishes
// rather than a habit reviewers maintain.
//
// The region is delimited by sentinel comments inside `components.schemas` and
// `components.responses`. Splicing text between sentinels, rather than re-emitting
// the document through a YAML marshaller, is deliberate: a marshaller rewrites the
// whole file — flow-style spacing collapses, comments move — and every spec change
// would arrive as a formatting diff.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const sourceFile = "tools/codegen/shared-components.yaml"

const (
	beginFmt = "# >>> shared-components: %s — generated from tools/codegen/shared-components.yaml, do not edit"
	endMark  = "# <<< shared-components"
)

// section is one spliced region: a key under `components` and the names it owns.
type section struct {
	key   string   // "schemas" or "responses"
	names []string // in the order they are written
}

// loadSource reads the canonical fragment and renders each section's YAML.
func loadSource() ([]section, map[string]string) {
	source, err := os.ReadFile(sourceFile)
	if err != nil {
		failf("read %s: %v", sourceFile, err)
	}
	var shared struct {
		Schemas   yaml.Node `yaml:"schemas"`
		Responses yaml.Node `yaml:"responses"`
	}
	err = yaml.Unmarshal(source, &shared)
	if err != nil {
		failf("parse %s: %v", sourceFile, err)
	}
	sections := []section{
		{"schemas", mapKeys(&shared.Schemas)},
		{"responses", mapKeys(&shared.Responses)},
	}
	blocks := map[string]string{
		"schemas":   renderMap(&shared.Schemas),
		"responses": renderMap(&shared.Responses),
	}
	return sections, blocks
}

func main() {
	check := flag.Bool("check", false, "fail on drift instead of rewriting")
	flag.Parse()

	sections, blocks := loadSource()

	specs, err := filepath.Glob(filepath.Join("services", "*", "openapi.yaml"))
	if err != nil {
		failf("glob specs: %v", err)
	}
	if len(specs) == 0 {
		failf("no service specs found")
	}

	var drifted, written []string
	for _, spec := range specs {
		changed, err := apply(spec, sections, blocks, *check)
		if err != nil {
			failf("%s: %v", spec, err)
		}
		if !changed {
			continue
		}
		if *check {
			drifted = append(drifted, spec)
		} else {
			written = append(written, spec)
		}
	}

	switch {
	case *check && len(drifted) > 0:
		_, _ = fmt.Fprintln(os.Stderr, "✗ shared components have drifted from "+sourceFile+":")
		for _, s := range drifted {
			_, _ = fmt.Fprintln(os.Stderr, "  "+s)
		}
		_, _ = fmt.Fprintln(os.Stderr, "\n  Run `mise run gen:shared-components` and commit the result.")
		os.Exit(1)
	case *check:
		_, _ = fmt.Fprintf(os.Stdout, "✓ %d specs carry identical shared components\n", len(specs))
	default:
		_, _ = fmt.Fprintf(os.Stdout, "✓ shared components written to %d spec(s)\n", len(written))
	}
}

// apply splices every section into one spec. It reports whether the file changed.
func apply(spec string, sections []section, blocks map[string]string, dryRun bool) (bool, error) {
	original, err := os.ReadFile(spec)
	if err != nil {
		return false, fmt.Errorf("read: %w", err)
	}
	updated := string(original)
	for _, sec := range sections {
		updated, err = splice(updated, sec.key, blocks[sec.key])
		if err != nil {
			return false, fmt.Errorf("%s: %w", sec.key, err)
		}
	}
	if updated == string(original) {
		return false, nil
	}
	if dryRun {
		return true, nil
	}
	// The write target is rebuilt from a validated service name rather than reusing
	// the globbed path, so the only thing this tool can write is a service spec.
	target, err := specPath(spec)
	if err != nil {
		return false, err
	}
	// 0o600 matches the other generators here (tools/adr-rules, tools/admin-gen);
	// git carries the tracked mode, so the bits set on write do not survive a clone.
	// gosec cannot see that specPath rebuilds the target from a validated service
	// name, so the only writable path is services/<name>/openapi.yaml.
	err = os.WriteFile(target, []byte(updated), 0o600) //nolint:gosec // path validated by specPath
	if err != nil {
		return false, fmt.Errorf("write: %w", err)
	}
	return true, nil
}

// specPath rebuilds services/<name>/openapi.yaml from the service name in the
// given path, rejecting anything that is not exactly that shape.
func specPath(spec string) (string, error) {
	cleaned := filepath.Clean(spec)
	dir, file := filepath.Split(cleaned)
	if file != "openapi.yaml" {
		return "", fmt.Errorf("not a service spec: %s", spec)
	}
	parent, name := filepath.Split(filepath.Clean(dir))
	if filepath.Clean(parent) != "services" || name == "" || strings.ContainsAny(name, `/\.`) {
		return "", fmt.Errorf("not a service spec: %s", spec)
	}
	return filepath.Join("services", name, "openapi.yaml"), nil
}

// splice replaces the sentinel-delimited region for one section. A spec with no
// sentinels is an error rather than a silent skip: it means the spec is not wired
// to the shared source at all, which is exactly the state this tool exists to
// prevent from going unnoticed.
func splice(doc, key, block string) (string, error) {
	begin := fmt.Sprintf(beginFmt, key)
	lines := strings.Split(doc, "\n")

	startIdx, endIdx, indent := -1, -1, ""
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if startIdx < 0 && trimmed == begin {
			startIdx = i
			indent = line[:len(line)-len(strings.TrimLeft(line, " "))]
			continue
		}
		if startIdx >= 0 && trimmed == endMark {
			endIdx = i
			break
		}
	}
	if startIdx < 0 {
		return "", fmt.Errorf("no %q sentinel — add it under components.%s", begin, key)
	}
	if endIdx < 0 {
		return "", fmt.Errorf("%q sentinel is not closed by %q", begin, endMark)
	}

	var out []string
	out = append(out, lines[:startIdx+1]...)
	for l := range strings.SplitSeq(strings.TrimRight(block, "\n"), "\n") {
		if l == "" {
			out = append(out, "")
			continue
		}
		out = append(out, indent+l)
	}
	out = append(out, lines[endIdx:]...)
	return strings.Join(out, "\n"), nil
}

// renderMap emits a mapping node's entries as YAML, without the wrapping key.
func renderMap(n *yaml.Node) string {
	if n.Kind != yaml.MappingNode {
		return ""
	}
	var b strings.Builder
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	err := enc.Encode(n)
	if err != nil {
		failf("render: %v", err)
	}
	_ = enc.Close()
	return b.String()
}

func mapKeys(n *yaml.Node) []string {
	var out []string
	if n.Kind != yaml.MappingNode {
		return out
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		out = append(out, n.Content[i].Value)
	}
	return out
}

func failf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "✗ "+format+"\n", args...)
	os.Exit(1)
}
