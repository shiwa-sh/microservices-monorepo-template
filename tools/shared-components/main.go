// Command shared-components gives every service spec the canonical definition of
// each shared component that spec uses (ADR-0303).
//
//	shared-components          rewrite each spec's shared region from the source
//	shared-components -check   fail if any spec's region has drifted
//
// Why the components are COPIED into each spec rather than $ref'd across files:
// a spec stays self-contained, so ogen and vacuum each read one document with no
// external resolution. The cost is duplication, and this tool is the reason the
// duplication is safe — "identical wherever it appears" is a fact the check
// establishes rather than a habit reviewers maintain.
//
// A spec carries only what it references, transitively: `Error` reaches `Problem`,
// and a spec with no monetary field does not carry `Money`. Copying the whole
// catalogue into every spec instead would put a schema nothing points at into five
// documents, which is what vacuum's oas3-unused-component reports — and a rule
// switched off to accommodate generated noise stops catching the orphan it exists
// to catch. A field added to a spec pulls its component in on the next generate.
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
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const sourceFile = "tools/codegen/shared-components.yaml"

const (
	beginFmt = "# >>> shared-components: %s — generated from tools/codegen/shared-components.yaml, do not edit"
	endMark  = "# <<< shared-components"
)

// refPattern matches a local component reference. Local is the whole story here:
// a spec has no external refs, which is the property the copying buys.
var refPattern = regexp.MustCompile(`#/components/(schemas|responses)/([A-Za-z0-9_.-]+)`)

// section is one spliced region: a key under `components` and the source mapping
// holding every component that region can carry.
type section struct {
	key  string // "schemas" or "responses"
	node *yaml.Node
}

// loadSource reads the canonical fragment.
func loadSource() []section {
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
	return []section{
		{"schemas", &shared.Schemas},
		{"responses", &shared.Responses},
	}
}

// blocksFor renders the region each section contributes to one spec: the shared
// components that spec reaches, in source order.
func blocksFor(sections []section, spec string) map[string]string {
	keep := reachable(sections, spec)
	blocks := make(map[string]string, len(sections))
	for _, sec := range sections {
		blocks[sec.key] = renderMap(filterMap(sec.node, sec.key, keep))
	}
	return blocks
}

// reachable is the set of shared components a spec points at, closed over the
// references the shared components make among themselves. The seed deliberately
// ignores the spliced regions: a component is carried because the SPEC needs it,
// never because last generate happened to put it there.
func reachable(sections []section, spec string) map[string]bool {
	byName := make(map[string]*yaml.Node)
	for _, sec := range sections {
		for name, node := range mapEntries(sec.node) {
			byName[sec.key+"/"+name] = node
		}
	}

	pending := refsIn(stripRegions(readSpec(spec)))
	seen := make(map[string]bool, len(pending))
	for len(pending) > 0 {
		ref := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		node, shared := byName[ref]
		if seen[ref] || !shared {
			continue
		}
		seen[ref] = true
		pending = append(pending, refsIn(renderNode(node))...)
	}
	return seen
}

// refsIn collects the component references in a chunk of YAML text.
func refsIn(doc string) []string {
	matches := refPattern.FindAllStringSubmatch(doc, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m[1]+"/"+m[2])
	}
	return out
}

// stripRegions removes the spliced regions, leaving the spec's own content.
func stripRegions(doc string) string {
	var out []string
	inRegion := false
	for line := range strings.SplitSeq(doc, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "# >>> shared-components:"):
			inRegion = true
		case inRegion && trimmed == endMark:
			inRegion = false
		case !inRegion:
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func main() {
	check := flag.Bool("check", false, "fail on drift instead of rewriting")
	flag.Parse()

	sections := loadSource()

	specs, err := filepath.Glob(filepath.Join("services", "*", "openapi.yaml"))
	if err != nil {
		failf("glob specs: %v", err)
	}
	if len(specs) == 0 {
		failf("no service specs found")
	}

	var drifted, written []string
	for _, spec := range specs {
		changed, err := apply(spec, sections, blocksFor(sections, spec), *check)
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
		_, _ = fmt.Fprintf(os.Stdout, "✓ %d specs carry the canonical form of every shared component they use\n", len(specs))
	default:
		_, _ = fmt.Fprintf(os.Stdout, "✓ shared components written to %d spec(s)\n", len(written))
	}
}

// apply splices every section into one spec. It reports whether the file changed.
func apply(spec string, sections []section, blocks map[string]string, dryRun bool) (bool, error) {
	original := readSpec(spec)
	updated := original
	var err error
	for _, sec := range sections {
		updated, err = splice(updated, sec.key, blocks[sec.key])
		if err != nil {
			return false, fmt.Errorf("%s: %w", sec.key, err)
		}
	}
	if updated == original {
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
	err = os.WriteFile(target, []byte(updated), 0o600)
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
	if n == nil || n.Kind != yaml.MappingNode || len(n.Content) == 0 {
		return ""
	}
	return renderNode(n)
}

func renderNode(n *yaml.Node) string {
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

// filterMap copies a mapping node down to the kept entries, in source order. The
// key node carries the leading comment, so a kept component keeps its rationale.
func filterMap(n *yaml.Node, key string, keep map[string]bool) *yaml.Node {
	if n.Kind != yaml.MappingNode {
		return nil
	}
	out := *n
	out.Content = nil
	for i := 0; i+1 < len(n.Content); i += 2 {
		if keep[key+"/"+n.Content[i].Value] {
			out.Content = append(out.Content, n.Content[i], n.Content[i+1])
		}
	}
	return &out
}

// mapEntries yields a mapping node's entries by name.
func mapEntries(n *yaml.Node) map[string]*yaml.Node {
	out := make(map[string]*yaml.Node)
	if n.Kind != yaml.MappingNode {
		return out
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		out[n.Content[i].Value] = n.Content[i+1]
	}
	return out
}

func readSpec(spec string) string {
	b, err := os.ReadFile(spec)
	if err != nil {
		failf("read %s: %v", spec, err)
	}
	return string(b)
}

func failf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "✗ "+format+"\n", args...)
	os.Exit(1)
}
