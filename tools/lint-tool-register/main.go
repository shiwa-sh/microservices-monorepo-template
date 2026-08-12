// Command lint-tool-register asserts that the tool register and the ADR set agree
// (ADR-0002).
//
// ADR-0002 makes docs/tool-register.md the canonical inventory and sets what each
// exit-cost tier owes its owning ADR. Four of its Rules name this task:
//
//  1. Every Tier 1 and Tier 2 row names an owning ADR, and that ADR exists.
//  2. A Tier 1 tool has a full comparison table in that ADR and a named runner-up,
//     or says outright that no option survived the hard constraints.
//  3. A Tier 2 tool has a short comparison table in that ADR.
//  4. Every alternative a row names is visible somewhere in that ADR. A rejection
//     nobody can see is indistinguishable from an option nobody considered, which
//     is the failure this check exists to prevent.
//
// Tier 3 rows own no ADR table — removal is a mechanical edit inside the packages
// that import the library — so they are checked for shape alone.
//
// What this does NOT check: whether a licence or governing-body cell is TRUE. Those
// are third-party facts, re-verified on the cadence in reference/upstream-status.md.
// A linter that cannot read upstream cannot assert them, and pretending otherwise
// would make the gate a rubber stamp on the register's least reliable columns.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const registerFile = "docs/tool-register.md"

const adrDir = "docs/adr"

// A row is one parsed table line from the register.
type row struct {
	tier         int
	tool         string
	owningADR    string
	licence      string
	governance   string
	runnerUp     string
	alternatives []string
	line         int
}

var (
	// [0100](adr/0100-language-and-runtime.md) — the register links relative to docs/.
	adrLinkRe = regexp.MustCompile(`\[(\d{4})\]\(adr/(\d{4}-[a-z0-9-]+)\.md\)`)
	// Cells carry markdown; the comparison is on the visible text.
	markupRe = regexp.MustCompile("[`*_]|\\[([^\\]]*)\\]\\([^)]*\\)")
)

func main() {
	data, err := os.ReadFile(registerFile)
	if err != nil {
		failf("read %s: %v", registerFile, err)
	}
	rows := parseRegister(string(data))
	if len(rows) == 0 {
		failf("%s: no tool rows found", registerFile)
	}

	adrs, err := loadADRs()
	if err != nil {
		failf("%v", err)
	}

	problems := make([]string, 0, len(rows))
	for _, r := range rows {
		problems = append(problems, checkRow(r, adrs)...)
	}

	if len(problems) > 0 {
		_, _ = fmt.Fprintf(os.Stderr, "✗ tool register does not agree with the ADR set (ADR-0002):\n")
		for _, p := range problems {
			_, _ = fmt.Fprintln(os.Stderr, "  "+p)
		}
		os.Exit(1)
	}
	_, _ = fmt.Fprintf(os.Stdout, "✓ %d tool-register rows agree with their owning ADRs\n", len(rows))
}

// checkRow applies the four Rules to one row.
func checkRow(r row, adrs map[string]string) []string {
	var problems []string
	where := fmt.Sprintf("%s:%d %s", registerFile, r.line, r.tool)

	if r.tier == 3 {
		if len(r.alternatives) == 0 {
			problems = append(problems, where+": Tier 3 row names nothing it was picked over")
		}
		return problems
	}

	// Rule 1 — the owning ADR exists.
	if r.owningADR == "" {
		problems = append(problems, where+": no owning ADR")
		return problems
	}
	body, ok := adrs[r.owningADR]
	if !ok {
		problems = append(problems, fmt.Sprintf("%s: owning ADR %s does not exist", where, r.owningADR))
		return problems
	}

	// Rules 2 and 3 — the ADR carries the comparison its tier owes.
	options := consideredOptions(body)
	if options == "" {
		problems = append(problems, fmt.Sprintf("%s: ADR %s has no Considered options section", where, r.owningADR))
		return problems
	}
	if !strings.Contains(options, "| ---") {
		problems = append(problems, fmt.Sprintf("%s: ADR %s Considered options has no comparison table", where, r.owningADR))
	}

	// Rule 2 — Tier 1 names a runner-up, or says no option survived.
	if r.tier == 1 && r.runnerUp == "" {
		problems = append(problems, where+": Tier 1 row has an empty Runner-up cell")
	}

	// Rule 4 — every named alternative is visible somewhere in the owning ADR.
	// The Rule names Considered options, and the ADRs put sub-decisions and their
	// comparison tables in Decision: ADR-0300 weighs pgcat against PgBouncer there,
	// under the pooler question. Scoping to one section would demand restructuring
	// those ADRs rather than finding what the Rule is about, which is an option the
	// ADR never shows the reader at all.
	haystack := normalise(strip(body))
	for _, alt := range r.alternatives {
		if alt == "" || isEscapeHatch(alt) {
			continue
		}
		if !mentions(haystack, alt) {
			const form = "%s: %q is recorded against it but appears nowhere in ADR %s"
			problems = append(problems, fmt.Sprintf(form, where, alt, r.owningADR))
		}
	}
	return problems
}

// mentions reports whether the ADR's Considered options names this alternative.
//
// The comparison is on significant tokens rather than the whole cell, because the
// register and the ADR legitimately word the same option differently: the register
// says "C#/.NET" where ADR-0100 says ".NET (C#)", and "TypeScript on the backend"
// where it says "TypeScript on Bun". Requiring the exact phrase would fail on
// wording, which is not what the Rule is about. The Rule is about an option the ADR
// never mentions at all, so one significant token is the right bar.
func mentions(haystack, alt string) bool {
	for _, tok := range tokens(alt) {
		if strings.Contains(haystack, tok) {
			return true
		}
	}
	return false
}

// tokens are the words in a cell that carry identity — the stopwords a phrase like
// "Traefik ForwardAuth to a first-party service" wraps around "traefik" do not.
func tokens(s string) []string {
	var out []string
	for w := range strings.FieldsSeq(normalise(s)) {
		if len(w) < 3 || stopwords[w] {
			continue
		}
		out = append(out, w)
	}
	return out
}

var stopwords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "into": true,
	"its": true, "own": true, "per": true, "alone": true, "then": true,
	"service": true, "services": true, "backend": true, "plain": true,
}

// normalise lowercases and turns separators into spaces, so "C#/.NET" and
// ".NET (C#)" tokenise the same way. `#` and `.` survive because they are part of
// a name here.
func normalise(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '#', r == '.', r == '+':
			_, _ = b.WriteRune(r)
		default:
			_, _ = b.WriteRune(' ')
		}
	}
	return b.String()
}

// isEscapeHatch reports whether a cell is prose standing in for an option rather
// than naming one — "none — the browser sets it", "review alone". These are honest
// answers to "what else was considered", and there is nothing to find in a table.
func isEscapeHatch(alt string) bool {
	lower := strings.ToLower(alt)
	for _, prefix := range []string{"none", "review alone", "build it", "nothing", "any "} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// consideredOptions returns the ADR's Considered options section, up to the next
// section at the same level.
func consideredOptions(body string) string {
	_, rest, found := strings.Cut(body, "## Considered options")
	if !found {
		return ""
	}
	section, _, split := strings.Cut(rest, "\n## ")
	if split {
		return section
	}
	return rest
}

// parseRegister reads every table row under a tier heading.
func parseRegister(md string) []row {
	var rows []row
	tier := 0
	for i, line := range strings.Split(md, "\n") {
		switch {
		case strings.HasPrefix(line, "## Tier 1"):
			tier = 1
			continue
		case strings.HasPrefix(line, "## Tier 2"):
			tier = 2
			continue
		case strings.HasPrefix(line, "## Tier 3"):
			tier = 3
			continue
		case strings.HasPrefix(line, "## "):
			tier = 0
			continue
		}
		if tier == 0 || !strings.HasPrefix(line, "| ") {
			continue
		}
		cells := splitRow(line)
		// Header and separator rows.
		if len(cells) < 3 || cells[0] == "Tool" || cells[0] == "Library" || strings.HasPrefix(cells[0], "---") {
			continue
		}
		rows = append(rows, buildRow(tier, cells, i+1))
	}
	return rows
}

func buildRow(tier int, cells []string, line int) row {
	r := row{tier: tier, tool: strip(cells[0]), line: line}
	if tier == 3 {
		// | Library | Concern | Picked over |
		if len(cells) >= 3 {
			r.alternatives = splitList(cells[2])
		}
		return r
	}
	if len(cells) >= 3 {
		m := adrLinkRe.FindStringSubmatch(cells[2])
		if m != nil {
			r.owningADR = m[2]
		}
	}
	if len(cells) >= 5 {
		r.licence, r.governance = strip(cells[3]), strip(cells[4])
	}
	if tier == 1 && len(cells) >= 7 {
		// | Tool | Concern | ADR | Licence | Governance | Runner-up | Recorded against |
		r.runnerUp = strip(cells[5])
		r.alternatives = splitList(cells[6])
	}
	if tier == 2 && len(cells) >= 6 {
		// | Tool | Concern | ADR | Licence | Governance | Recorded against |
		r.alternatives = splitList(cells[5])
	}
	return r
}

func splitRow(line string) []string {
	trimmed := strings.Trim(strings.TrimSpace(line), "|")
	parts := strings.Split(trimmed, "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}

// splitList splits a "Recorded against" cell into the alternatives it names.
func splitList(cell string) []string {
	var out []string
	for part := range strings.SplitSeq(cell, ",") {
		part = strip(strings.TrimSpace(part))
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// strip removes markdown emphasis, code ticks, and link targets, leaving the text a
// reader sees. Comparing rendered text is what makes a `sqlc` cell match a plain
// sqlc mention in an ADR table.
func strip(s string) string {
	return strings.TrimSpace(markupRe.ReplaceAllString(s, "$1"))
}

func loadADRs() (map[string]string, error) {
	paths, err := filepath.Glob(filepath.Join(adrDir, "*.md"))
	if err != nil {
		return nil, fmt.Errorf("glob %s: %w", adrDir, err)
	}
	out := make(map[string]string, len(paths))
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", p, err)
		}
		out[strings.TrimSuffix(filepath.Base(p), ".md")] = string(data)
	}
	return out, nil
}

func failf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "✗ "+format+"\n", args...)
	os.Exit(1)
}
