// Command lint-readme-drift is principle 9 applied to the two documents that duplicate by
// design and are not generated. The root README restates selection guidance for a reader
// who has not adopted yet (ADR-0001), and ADR-0000 keeps hand copies of verdicts its own
// downstream ADRs own. A duplicate with no check drifts, and both of these did.
//
// Four checks.
//
// Stack drift: for every row of the README's stack table, the tools named in the decision
// cell are matched against the options the cited ADR rejected. A rejected option presented
// as a current decision is the failure this catches.
//
// Principle blocks: ADR-0000's principles list what each rejected and what each accepted
// on the same rule. A name under *Rejected:* that some ADR chose, or a name under
// *Accepted on the same rule:* that every ADR comparing it refused, is one document
// contradicting another.
//
// Anchors: every principle is anchored to an external standard or marked local, and the
// README states the same anchor at index resolution. The two must agree on which of the
// two it is, and on the sources cited.
//
// Headcount: ADR-0000 states no headcount and none is to be inferred, so no committed
// document states one. The demand side lives in operational-surface.md as obligations
// per component, which the reader sums against their own facts.
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	adrDir         = "docs/adr"
	readmePath     = "README.md"
	foundationsADR = "docs/adr/0000-platform-foundations.md"
)

var (
	adrRef  = regexp.MustCompile(`ADR-(\d{4})|\]\(docs/adr/(\d{4})-`)
	adrFile = regexp.MustCompile(`^(\d{4})-[a-z0-9-]+\.md$`)
	// A headcount claim: a staffing level attached to people. "One person" is not one:
	// the floor's constraints are written as *no component whose only competent operator
	// is one person*, which is a property of the component rather than a team size. What
	// is banned is a number a project could staff to — two engineers, 2–3 platform
	// engineers, four FTEs.
	headcount = regexp.MustCompile(
		`(?i)\b(\d+|two|three|four|five|six|seven|eight|nine|ten)` +
			`\s*([–—-]|\bto\b)?\s*(\d+|two|three|four|five)?\s+(platform\s+)?` +
			`(engineers?|people|persons?|FTEs?|staff)\b`,
	)
	singular  = regexp.MustCompile(`(?i)^(1|one)\b`)
	linkText  = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	inlineTag = regexp.MustCompile("[`*]")

	// A principle heading in ADR-0000, as in "**4. Spend novelty by exit cost**".
	principleHead = regexp.MustCompile(`^\*\*(\d+)\.\s`)
	// A principle row in either README table: the number is the first cell.
	principleRow = regexp.MustCompile(`^\|\s*(\d+)\s*\|`)
	// Link targets, which are what an anchor is compared on. Prose around a citation is
	// written to two different lengths on purpose; the source cited is not.
	linkTarget   = regexp.MustCompile(`\]\(([^)]+)\)`)
	localMarker  = regexp.MustCompile(`(?i)\*\*local\*\*`)
	parenthetics = regexp.MustCompile(`\([^)]*\)`)
)

// principle is one numbered selection or construction principle, as either document
// states it. Only the fields both documents carry are compared.
type principle struct {
	local    bool
	sources  map[string]bool
	rejected []string
	accepted []string
}

// stopWords are option-cell openings that carry no product name. An option is written as
// a phrase, and only some phrases start with the thing being named.
var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "one": true, "two": true, "no": true, "none": true,
	"do": true, "per": true, "build": true, "write": true, "keep": true, "everything": true,
	"paths": true, "anything": true, "each": true, "make": true, "plain": true, "custom": true,
	"rule": true, "requests": true, "vertical": true, "declarative": true, "generated": true,
	"hand": true, "self": true, "email": true, "chat": true, "scripts": true, "separate": true,
	"same": true, "all": true, "in": true, "on": true, "at": true, "with": true, "without": true,
	"second": true, "third": true, "first": true, "flat": true, "shared": true, "manual": true,
}

func main() {
	rejected, err := loadRejected()
	if err != nil {
		failf("%v", err)
	}
	body, err := os.ReadFile(readmePath)
	if err != nil {
		failf("read %s: %v", readmePath, err)
	}
	verdicts, err := loadVerdicts()
	if err != nil {
		failf("%v", err)
	}
	foundations, err := os.ReadFile(foundationsADR)
	if err != nil {
		failf("read %s: %v", foundationsADR, err)
	}
	adrPrinciples := principlesInADR(string(foundations))
	readmePrinciples := principlesInREADME(string(body))

	problems := checkStackTable(string(body), rejected)
	problems = append(problems, checkPrincipleBlocks(adrPrinciples, verdicts)...)
	problems = append(problems, checkAnchors(adrPrinciples, readmePrinciples)...)
	problems = append(problems, checkHeadcount()...)

	if len(problems) > 0 {
		_, _ = fmt.Fprintln(os.Stderr, "✗ the README and ADR-0000 have drifted from the ADR set:")
		sort.Strings(problems)
		for _, p := range problems {
			_, _ = fmt.Fprintln(os.Stderr, "  "+p)
		}
		os.Exit(1)
	}
	_, _ = fmt.Fprintln(os.Stdout, "✓ the README and ADR-0000 agree with the ADR set")
}

// verdictSet is what every ADR's comparison tables concluded, pooled across the set: the
// names that won somewhere, and the names that lost somewhere. ADR-0000 chooses no
// technology, so its principle blocks are claims about these two sets.
type verdictSet struct {
	chosen map[string]string // name → the ADR number that chose it
	lost   map[string]string // name → an ADR number that refused it
}

func loadVerdicts() (verdictSet, error) {
	out := verdictSet{chosen: map[string]string{}, lost: map[string]string{}}
	entries, err := os.ReadDir(adrDir)
	if err != nil {
		return out, fmt.Errorf("read %s: %w", adrDir, err)
	}
	for _, e := range entries {
		m := adrFile.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		body, err := os.ReadFile(filepath.Join(adrDir, e.Name()))
		if err != nil {
			return out, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		chosen, lost := comparedNames(string(body))
		for name := range chosen {
			out.chosen[name] = m[1]
		}
		for name := range lost {
			_, seen := out.lost[name]
			if !seen {
				out.lost[name] = m[1]
			}
		}
	}
	return out, nil
}

// principlesInADR reads ADR-0000's principle blocks. A principle opens with a bold
// numbered heading and carries its anchor, its casualties, and what it admitted on the
// same rule as sibling bullets.
func principlesInADR(body string) map[string]principle {
	out := map[string]principle{}
	current := ""
	for line := range strings.SplitSeq(body, "\n") {
		head := principleHead.FindStringSubmatch(line)
		if head != nil {
			current = head[1]
			out[current] = principle{sources: map[string]bool{}}
			continue
		}
		if current == "" || !strings.HasPrefix(line, "- *") {
			continue
		}
		p := out[current]
		switch {
		case strings.HasPrefix(line, "- *Anchor:*"):
			rest := strings.TrimPrefix(line, "- *Anchor:*")
			p.sources = externalSources(rest)
			p.local = localMarker.MatchString(rest) && len(p.sources) == 0
		case strings.HasPrefix(line, "- *Rejected:*"):
			p.rejected = namesIn(strings.TrimPrefix(line, "- *Rejected:*"))
		case strings.HasPrefix(line, "- *Accepted on the same rule:*"):
			p.accepted = namesIn(strings.TrimPrefix(line, "- *Accepted on the same rule:*"))
		}
		out[current] = p
	}
	return out
}

// principlesInREADME reads the two principle tables. Their rows carry the number, the
// principle, the anchor, and what it rejected, in that order.
func principlesInREADME(body string) map[string]principle {
	out := map[string]principle{}
	for line := range strings.SplitSeq(body, "\n") {
		m := principleRow.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		cells := splitRow(line)
		if len(cells) < 4 {
			continue
		}
		p := principle{sources: externalSources(cells[2])}
		p.local = localMarker.MatchString(cells[2]) && len(p.sources) == 0
		p.rejected = namesIn(cells[3])
		out[m[1]] = p
	}
	return out
}

// checkPrincipleBlocks holds ADR-0000's hand copies against the verdicts their owning
// ADRs reached. This is the check the four stale claims of ADR-0000 needed and did not
// have: the document every other ADR cites was the one document nothing read back.
func checkPrincipleBlocks(principles map[string]principle, v verdictSet) []string {
	var problems []string
	for num, p := range principles {
		for _, name := range p.rejected {
			owner, chosen := v.chosen[name]
			if !chosen {
				continue
			}
			problem := fmt.Sprintf(
				"%s: principle %s rejects %q, and ADR-%s chose it",
				foundationsADR,
				num,
				name,
				owner,
			)
			problems = append(problems, problem)
		}
		for _, name := range p.accepted {
			_, chosen := v.chosen[name]
			if chosen {
				continue
			}
			owner, lost := v.lost[name]
			if !lost {
				continue
			}
			problem := fmt.Sprintf(
				"%s: principle %s accepts %q on the exit-cost rule, and ADR-%s rejected it",
				foundationsADR,
				num,
				name,
				owner,
			)
			problems = append(problems, problem)
		}
	}
	return problems
}

// checkAnchors asserts the two documents agree on what each principle rests on. ADR-0001
// makes the local-or-borrowed distinction load-bearing — a borrowed criterion survives
// re-litigation and a house rule does not — so a reader must not learn a different answer
// depending on which document they opened.
func checkAnchors(adr, readme map[string]principle) []string {
	var problems []string
	for num, r := range readme {
		a, known := adr[num]
		if !known {
			problem := fmt.Sprintf(
				"%s: states principle %s, and %s has no such principle",
				readmePath,
				num,
				foundationsADR,
			)
			problems = append(problems, problem)
			continue
		}
		if a.local != r.local {
			problem := fmt.Sprintf(
				"%s: principle %s is anchored %s, and %s marks it %s",
				readmePath,
				num,
				marking(r.local),
				foundationsADR,
				marking(a.local),
			)
			problems = append(problems, problem)
		}
		for src := range r.sources {
			if a.sources[src] {
				continue
			}
			problem := fmt.Sprintf(
				"%s: principle %s cites %q, which %s's anchor does not",
				readmePath,
				num,
				src,
				foundationsADR,
			)
			problems = append(problems, problem)
		}
	}
	for num := range adr {
		_, carried := readme[num]
		if carried {
			continue
		}
		problem := fmt.Sprintf(
			"%s: states principle %s, and %s does not carry it",
			foundationsADR,
			num,
			readmePath,
		)
		problems = append(problems, problem)
	}
	return problems
}

// externalSources returns the anchor's citations, excluding links into the ADR set. A
// principle marked local may still point at the ADR that elaborates it, and that pointer
// is a cross-reference rather than a borrowed criterion.
func externalSources(s string) map[string]bool {
	out := map[string]bool{}
	for _, m := range linkTarget.FindAllStringSubmatch(s, -1) {
		target := m[1]
		if adrFile.MatchString(target) || strings.HasPrefix(target, "docs/adr/") {
			continue
		}
		out[target] = true
	}
	return out
}

func marking(local bool) string {
	if local {
		return "local"
	}
	return "to an external standard"
}

// namesIn splits a list of tools into the names it holds. The lists are prose — commas,
// "and", a trailing clause after an em dash, and a parenthesised ADR reference per name —
// so each fragment is cleaned rather than reduced.
//
// The whole fragment is kept, not its leading token. "Argo Workflows" and "Argo CD" share
// a first word and are different decisions, and collapsing either to "Argo" makes the
// principle-block check report that principle 5 rejects the deploy engine.
func namesIn(s string) []string {
	s = parenthetics.ReplaceAllString(plain(s), "")
	dash := strings.Index(s, " — ")
	if dash >= 0 {
		s = s[:dash]
	}
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "."))
	var out []string
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' }) {
		part = strings.TrimSpace(part)
		for _, prefix := range []string{"and ", "every ", "all "} {
			part = strings.TrimPrefix(part, prefix)
		}
		name := cleanName(part)
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

// cleanName returns a fragment that could name a product, or empty. A name is short, its
// tokens are name-shaped, and it does not open with an article or a verb.
func cleanName(part string) string {
	fields := strings.Fields(part)
	if len(fields) == 0 || len(fields) > 3 {
		return ""
	}
	for i, f := range fields {
		f = strings.Trim(f, ".,;:()")
		if !isName(f) || (i == 0 && (len(f) < 3 || stopWords[strings.ToLower(f)])) {
			return ""
		}
		fields[i] = f
	}
	return strings.Join(fields, " ")
}

// loadRejected reads every ADR's comparison tables and returns, per ADR number, the
// names of the options that lost. A verdict cell beginning with a bold "Chosen" is the
// winner; everything else in the table is an option this ADR refused.
func loadRejected() (map[string][]string, error) {
	out := map[string][]string{}
	entries, err := os.ReadDir(adrDir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", adrDir, err)
	}
	for _, e := range entries {
		m := adrFile.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		path := filepath.Join(adrDir, e.Name())
		body, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		out[m[1]] = rejectedIn(string(body))
	}
	return out, nil
}

// rejectedIn returns the names one ADR refuses: the options that lost, minus the ones it
// adopts elsewhere, plus the names it states outright are not used.
func rejectedIn(body string) []string {
	chosen, lost := comparedNames(body)
	adopted := adoptedNames(body)

	// A comparison compares variants as well as products, so a losing row often names
	// the winner — "Helm rendered, then Kustomize post-render" loses, and Helm is the
	// decision. A name survives only where no row it heads won and the Decision does
	// not adopt it.
	var out []string
	seen := map[string]bool{}
	for name := range lost {
		if !chosen[name] && !adopted[name] {
			out = append(out, name)
			seen[name] = true
		}
	}
	// A refusal stated in the Decision outranks all of that: "TypeSpec and equivalent
	// authoring layers are not used" is the strongest form the set has, and it needs no
	// comparison row to be binding.
	for name := range refusedNames(body) {
		if !seen[name] {
			out = append(out, name)
		}
	}
	return out
}

// comparedNames reads one ADR's comparison tables and returns the names that won and the
// names that lost, keyed by the leading token of each option cell.
func comparedNames(body string) (map[string]bool, map[string]bool) {
	chosen, lost := map[string]bool{}, map[string]bool{}
	inOptions := false
	for line := range strings.SplitSeq(body, "\n") {
		if strings.HasPrefix(line, "## ") {
			inOptions = strings.HasPrefix(line, "## Considered options")
		}
		if !inOptions || !strings.HasPrefix(line, "| ") || strings.HasPrefix(line, "| ---") {
			continue
		}
		cells := splitRow(line)
		if len(cells) < 2 {
			continue
		}
		name := leadToken(cells[0])
		if name == "" || len(strings.Fields(plain(cells[0]))) > 3 {
			continue
		}
		if strings.Contains(cells[len(cells)-1], "**Chosen") {
			chosen[name] = true
			continue
		}
		lost[name] = true
	}
	return chosen, lost
}

// adoptedNames returns every name the Decision section states positively. A name the
// decision adopts is not a rejection, whatever a comparison row beside it was called —
// and a line that adopts a name while denying it ("Kustomize is not used") states the
// refusal rather than the adoption.
func adoptedNames(body string) map[string]bool {
	out := map[string]bool{}
	inDecision := false
	for line := range strings.SplitSeq(body, "\n") {
		if strings.HasPrefix(line, "## ") {
			inDecision = strings.HasPrefix(line, "## Decision") && !strings.HasPrefix(line, "## Decision drivers")
		}
		if !inDecision || strings.Contains(line, " not ") {
			continue
		}
		for field := range strings.FieldsSeq(plain(line)) {
			field = strings.Trim(field, ".,;:()|")
			if len(field) >= 3 && isName(field) {
				out[field] = true
			}
		}
	}
	return out
}

// refusedNames returns names the ADR states are not used. The phrasing is fixed by
// ADR-0001's declarative rule — "X is not used", "X are not used" — which is what makes
// it greppable.
func refusedNames(body string) map[string]bool {
	out := map[string]bool{}
	for line := range strings.SplitSeq(body, "\n") {
		name := refusalIn(plain(line))
		if name != "" {
			out[name] = true
		}
	}
	return out
}

// refusalIn returns the name a single line refuses, or empty where it refuses nothing.
func refusalIn(line string) string {
	idx, width := strings.Index(line, " not used"), len(" not used")
	if idx < 0 {
		idx, width = strings.Index(line, " not adopted"), len(" not adopted")
	}
	if idx < 0 {
		return ""
	}
	// A qualified refusal is not a refusal of the thing: "Hydra is not used for internal
	// calls" adopts Hydra and scopes it. Only a refusal that ends its clause is blanket.
	rest := strings.TrimSpace(line[idx+width:])
	if rest != "" && rest[0] != '.' && rest[0] != ',' {
		return ""
	}
	// Scope to the clause carrying the refusal: a line may state a decision and then
	// refuse an alternative, and only the second half is the refusal.
	clause := line[:idx]
	for _, sep := range []string{". ", "! ", "? ", "; ", ": ", "| ", "— ", ", and "} {
		i := strings.LastIndex(clause, sep)
		if i >= 0 {
			clause = clause[i+len(sep):]
		}
	}
	var phrase []string
	for field := range strings.FieldsSeq(clause) {
		field = strings.Trim(field, ".,;:()|")
		if len(field) < 3 || !isName(field) || stopWords[strings.ToLower(field)] {
			continue
		}
		if field[0] >= 'A' && field[0] <= 'Z' {
			phrase = append(phrase, field)
		}
	}
	return strings.Join(phrase, " ")
}

func checkStackTable(readme string, rejected map[string][]string) []string {
	var problems []string
	inStack := false
	for line := range strings.SplitSeq(readme, "\n") {
		if strings.HasPrefix(line, "## ") {
			inStack = strings.HasPrefix(line, "## Stack at a glance")
		}
		if !inStack {
			continue
		}
		if !strings.HasPrefix(line, "| ") || strings.HasPrefix(line, "| ---") {
			continue
		}
		cells := splitRow(line)
		if len(cells) < 3 || cells[0] == "Concern" {
			continue
		}
		concern, decision, refs := cells[0], plain(cells[1]), cells[2]
		for _, ref := range adrRef.FindAllStringSubmatch(refs, -1) {
			num := ref[1] + ref[2]
			for _, name := range rejected[num] {
				if containsToken(decision, name) {
					problem := fmt.Sprintf(
						"%s: row %q presents %q as the decision, and ADR-%s rejects it",
						readmePath,
						concern,
						name,
						num,
					)
					problems = append(problems, problem)
				}
			}
		}
	}
	return problems
}

// checkHeadcount asserts the capacity reframe holds everywhere it is stated: the
// obligation columns are the demand side, and no document converts them into a number.
func checkHeadcount() []string {
	var problems []string
	for _, path := range markdownFiles() {
		body, err := os.ReadFile(path)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		for n, line := range strings.Split(string(body), "\n") {
			m := headcount.FindString(line)
			if m == "" || singular.MatchString(strings.TrimSpace(m)) {
				continue
			}
			problem := fmt.Sprintf(
				"%s:%d: states a headcount (%q) — capacity is stated as obligations in docs/operational-surface.md",
				path,
				n+1,
				strings.TrimSpace(m),
			)
			problems = append(problems, problem)
		}
	}
	return problems
}

// markdownFiles lists the committed Markdown a headcount could hide in. Paths are
// collected before anything is read, so no file operation runs inside the walk.
func markdownFiles() []string {
	var out []string
	for _, root := range []string{"docs", readmePath, "AGENTS.md"} {
		err := filepath.WalkDir(
			root,
			func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() || !strings.HasSuffix(path, ".md") || strings.HasSuffix(path, ".local.md") {
					return nil
				}
				out = append(out, path)
				return nil
			},
		)
		if err != nil {
			failf("walk %s: %v", root, err)
		}
	}
	return out
}

// leadToken returns the first word of an option cell that names a thing rather than
// describing one. Options are written as phrases, so the name is the first token that
// is not an article, a quantifier, or a verb.
func leadToken(cell string) string {
	for field := range strings.FieldsSeq(plain(cell)) {
		field = strings.Trim(field, ".,;:()")
		if len(field) < 3 || stopWords[strings.ToLower(field)] {
			continue
		}
		if !isName(field) {
			continue
		}
		return field
	}
	return ""
}

// isName accepts a token that could be a product name: letters, digits, and the
// punctuation product names use.
func isName(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '.', r == '_', r == '/':
		default:
			return false
		}
	}
	return true
}

// containsToken reports whether every word of a name appears in the cell. A refusal may
// name a variant — "Argo CD Image Updater" — and the decision that adopts Argo CD is not
// the decision that adopts the updater.
func containsToken(haystack, name string) bool {
	for token := range strings.FieldsSeq(name) {
		re, err := regexp.Compile(`\b` + regexp.QuoteMeta(token) + `\b`)
		if err != nil || !re.MatchString(haystack) {
			return false
		}
	}
	return true
}

func splitRow(line string) []string {
	trimmed := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(line), "|"), "|")
	parts := strings.Split(trimmed, " | ")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// plain strips link syntax and inline emphasis so a name is compared as a name.
func plain(s string) string {
	s = linkText.ReplaceAllString(s, "$1")
	return inlineTag.ReplaceAllString(s, "")
}

func failf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "✗ "+format+"\n", args...)
	os.Exit(1)
}
