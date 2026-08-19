// Command lint-comments enforces ADR-0001's comment table across the Go and
// TypeScript this repository owns.
//
// ADR-0001 states that the density and banned-construct rules apply unchanged to
// comments, and `lint:prose` reads only Markdown — so the half of the rule that
// governs code has rested on review alone. A house style that only review enforces
// is a house style that drifts, which is the argument every other gate here was
// built on.
//
// # What it checks
//
// The mechanically detectable rows of the table, and deliberately not the others:
// "comments explain why, not what" and "a comment that explains confusing code is a
// defect" are judgements a reader makes, and a linter that guessed at them would
// train everyone to ignore it.
//
//   - commented-out code
//   - changelog, author, and date comments
//   - decorative banners and section dividers
//   - a TODO with no ADR or issue behind it
//   - the intensifiers, hedges, and meta-commentary the prose table bans
//
// # What it skips
//
// Generated files, vendored trees, and this file — which must name the constructs
// it bans. A single line may opt out with a trailing `lint:comments-allow`, for a
// quotation whose wording is not ours to fix.
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type rule struct {
	name    string
	pattern *regexp.Regexp
	reason  string
}

func word(words ...string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)\b(` + strings.Join(words, "|") + `)\b`)
}

var rules = []rule{
	{
		name:    "intensifier",
		pattern: word("very", "really", "quite", "obviously", "of course"),
		reason:  "delete it — a claim needing an intensifier is not established",
	},
	{
		name:    "hedge",
		pattern: word("arguably", "essentially", "basically", "somewhat"),
		reason:  "decide — a hedge is an unfinished decision",
	},
	{
		name:    "meta-commentary",
		pattern: regexp.MustCompile(`(?i)(it is worth (knowing|noting)|it should be noted|note that,)`),
		reason:  "state the thing",
	},
	{
		name:    "changelog comment",
		pattern: regexp.MustCompile(`(?i)(^|\s)(@author|@since|@deprecated\s+\d|author:|created:|last (modified|updated):)`),
		reason:  "git holds authorship and dates",
	},
	{
		name: "chronology",
		pattern: regexp.MustCompile(
			`(?i)\b(used to be|previously (was|did|had)|has since|we then (added|changed)|originally (this|it) )`,
		),
		reason: "state what it is now",
	},
	{
		name: "decorative banner",
		// Four or more repeats of a divider character, which is a banner rather
		// than an em-dash or an arrow in prose.
		pattern: regexp.MustCompile(`([=*#_~-]|─|━|═){8,}`),
		reason:  "the file structure is the structure",
	},
	{
		name:    "uncited TODO",
		pattern: regexp.MustCompile(`\bTODO\b`),
		reason:  "cite an ADR (ADR-0123) or an issue (#123), or do not merge it",
	},
}

// citation is what rescues a TODO: an ADR number or an issue reference.
var citation = regexp.MustCompile(`(ADR-\d{4}|#\d+)`)

// codeish matches a comment whose body is code rather than prose. Conservative on
// purpose: prose that merely ends in a brace is rare, and a false positive here
// costs more than a miss, because it lands on a comment that is doing its job.
var codeish = regexp.MustCompile(
	`^\s*(if|for|while|switch|return|func|function|const|let|var|import|export|class|type|await|go )\b.*[;{}()]\s*$`,
)

// assignmentish catches the other common shape of commented-out code: a bare
// assignment or call ending in a semicolon or a closing brace.
var assignmentish = regexp.MustCompile(`^\s*[\w.\[\]]+\s*(:?=|\+=|\()[^)]*[);]\s*$`)

type finding struct {
	file   string
	line   int
	rule   string
	reason string
	text   string
}

func main() {
	roots := []string{"apps", "libs", "services", "tools"}
	var found []finding
	collect := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if skipDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !scannable(path) {
			return nil
		}
		hits, err := scan(path)
		if err != nil {
			return err
		}
		found = append(found, hits...)
		return nil
	}
	for _, root := range roots {
		err := filepath.Walk(root, collect)
		if err != nil {
			failf("walk %s: %v", root, err)
		}
	}

	if len(found) > 0 {
		for _, f := range found {
			_, _ = fmt.Fprintf(os.Stderr, "✗ %s:%d: %s — %s\n", f.file, f.line, f.rule, f.reason)
			_, _ = fmt.Fprintf(os.Stderr, "    %s\n", strings.TrimSpace(f.text))
		}
		_, _ = fmt.Fprintf(
			os.Stderr,
			"\n  ADR-0001's comment table. A line may opt out with `lint:comments-allow`.\n",
		)
		os.Exit(1)
	}
	_, _ = fmt.Fprintf(os.Stdout, "✓ comments conform to ADR-0001\n")
}

func skipDir(name string) bool {
	switch name {
	case "node_modules", ".next", "dist", "build", "_generated", "sdks", "schemas":
		return true
	default:
		return false
	}
}

func scannable(path string) bool {
	switch filepath.Ext(path) {
	case ".go", ".ts", ".tsx":
	default:
		return false
	}
	base := filepath.Base(path)
	// Generated Go carries the standard marker; generated TypeScript carries this
	// repo's own. Neither is written by hand, so neither is anyone's to fix.
	if strings.HasSuffix(base, "_gen.go") || strings.HasSuffix(base, ".sql.go") {
		return false
	}
	// This file names the constructs it bans, the way ADR-0001 does.
	return !strings.HasSuffix(path, filepath.Join("tools", "lint-comments", "main.go"))
}

// scan reads one file and returns every comment line that breaks a rule.
//
// Line-oriented rather than AST-based, and that is a deliberate limit: a `//`
// inside a string literal is read as a comment here. In this repository that
// shape appears in URLs, which no rule below matches, so the simpler reader buys
// nothing worse than a theoretical false positive.
func scan(path string) ([]finding, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer func() { _ = file.Close() }()

	var out []finding
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	inBlock := false
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		body, ok := commentBody(line, &inBlock)
		if !ok || strings.Contains(line, "lint:comments-allow") {
			continue
		}
		out = append(out, check(path, lineNo, line, body)...)
	}
	err = scanner.Err()
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	return out, nil
}

// commentBody returns the comment text on a line, and whether the line has one.
// It tracks block comments across lines so a `/* … */` span is read as comment
// rather than as code.
func commentBody(line string, inBlock *bool) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if *inBlock {
		if strings.Contains(trimmed, "*/") {
			*inBlock = false
		}
		return strings.TrimPrefix(strings.TrimPrefix(trimmed, "*/"), "*"), true
	}
	if strings.HasPrefix(trimmed, "/*") {
		*inBlock = !strings.Contains(trimmed, "*/")
		return strings.TrimPrefix(trimmed, "/*"), true
	}
	idx := strings.Index(trimmed, "//")
	if idx < 0 {
		return "", false
	}
	// A `//` preceded by a colon is a URL scheme, not a comment.
	if idx > 0 && trimmed[idx-1] == ':' {
		return "", false
	}
	return trimmed[idx+2:], true
}

func check(path string, lineNo int, line, body string) []finding {
	var out []finding
	add := func(name, reason string) {
		out = append(out, finding{file: path, line: lineNo, rule: name, reason: reason, text: line})
	}
	for _, r := range rules {
		if !r.pattern.MatchString(body) {
			continue
		}
		if r.name == "uncited TODO" && citation.MatchString(body) {
			continue
		}
		add(r.name, r.reason)
	}
	// A tab-indented line inside a doc comment is a rendered code BLOCK — the way
	// both gofmt and godoc treat it — so it is documentation showing a call, not a
	// call someone commented out.
	if !strings.HasPrefix(body, "\t") && (codeish.MatchString(body) || assignmentish.MatchString(body)) {
		add("commented-out code", "git holds it")
	}
	return out
}

func failf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "✗ "+format+"\n", args...)
	os.Exit(1)
}
