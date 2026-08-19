// Command data-classes generates the data-class and retention registry
// (ADR-0301).
//
// ADR-0301 calls the registry "the checklist a new service is reviewed against",
// and the risk it names is precise: a new service can silently omit its
// obligations, and the omission surfaces only on the first request that needs it.
// A checklist someone maintains by hand is a checklist that is wrong the first
// time nobody notices a table.
//
// So it is a VIEW over two sources and owns neither. The tags come from the
// migrations, where `COMMENT ON COLUMN … IS 'pii:<class>'` already lives in the
// DDL and travels with the schema; the retention and disposal come from
// tools/codegen/retention.yaml, because how long to keep a class and whether to
// delete or anonymise it are decisions, and a column tag cannot carry a decision.
//
// The registry is therefore never edited. Adding a tagged column to a migration
// adds a row here, and the drift check is what makes that automatic rather than
// remembered.
package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// The registry's prose, kept beside this file rather than inside it. Markdown
// paragraphs are long lines by nature and Go source lines are not, so embedding is
// what lets both read the way they should.
//
// `.tmpl` rather than `.md`: their relative links resolve from the OUTPUT's
// directory, not from this one, so the markdown linter would report every one of
// them as broken.
//
//go:embed header.tmpl
var header string

//go:embed footer.tmpl
var footer string

const (
	outPath       = "docs/reference/data-classes.md"
	retentionPath = "tools/codegen/retention.yaml"
)

// tagRe matches the tag ADR-0300 puts in the DDL. It is the same expression
// lint-pii uses, and deliberately so: a registry that recognised a different set
// of tags from the gate would disagree with it about what is classified.
var tagRe = regexp.MustCompile(
	`(?i)comment\s+on\s+column\s+([a-z0-9_."]+)\.([a-z0-9_"]+)\s+is\s+'pii:([a-z_]+)'`,
)

type classPolicy struct {
	Period   string `yaml:"period"`
	Disposal string `yaml:"disposal"`
	Note     string `yaml:"note"`
}

type retention struct {
	Classes map[string]classPolicy `yaml:"classes"`
}

type column struct {
	service string
	table   string
	name    string
	class   string
}

func main() {
	check := len(os.Args) > 1 && os.Args[1] == "--check"

	policies, err := loadRetention()
	if err != nil {
		failf("%v", err)
	}
	columns, err := scanColumns()
	if err != nil {
		failf("%v", err)
	}
	if len(columns) == 0 {
		failf("no tagged columns found — the registry would be empty, which is never right")
	}
	for _, c := range columns {
		_, known := policies.Classes[c.class]
		if !known {
			failf("%s.%s.%s is tagged pii:%s, which retention.yaml does not define", c.service, c.table, c.name, c.class)
		}
	}

	want := render(columns, policies)
	if check {
		got, readErr := os.ReadFile(outPath)
		if readErr != nil || string(got) != want {
			_, _ = fmt.Fprintf(os.Stderr, "✗ %s is stale — run `mise run gen:data-classes`\n", outPath)
			os.Exit(1)
		}
		_, _ = fmt.Fprintf(os.Stdout, "✓ %s matches the migrations and retention.yaml\n", outPath)
		return
	}
	err = os.WriteFile(outPath, []byte(want), 0o600)
	if err != nil {
		failf("write %s: %v", outPath, err)
	}
	_, _ = fmt.Fprintf(os.Stdout, "✓ %d classified columns in %s\n", len(columns), outPath)
}

func loadRetention() (retention, error) {
	var out retention
	raw, err := os.ReadFile(retentionPath)
	if err != nil {
		return out, fmt.Errorf("read %s: %w", retentionPath, err)
	}
	err = yaml.Unmarshal(raw, &out)
	if err != nil {
		return out, fmt.Errorf("parse %s: %w", retentionPath, err)
	}
	return out, nil
}

// scanColumns reads every service's migrations and returns the tagged columns.
//
// A later migration's tag wins, which is what makes re-classifying a column
// possible: a dbmate migration is immutable once applied, so a column changes
// class by a new `COMMENT ON`, and the registry must reflect the last word rather
// than the first.
func scanColumns() ([]column, error) {
	paths, err := filepath.Glob(filepath.Join("services", "*", "migrations", "*.sql"))
	if err != nil {
		return nil, fmt.Errorf("glob migrations: %w", err)
	}
	sort.Strings(paths)

	latest := map[string]column{}
	for _, path := range paths {
		service := filepath.Base(filepath.Dir(filepath.Dir(path)))
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, fmt.Errorf("read %s: %w", path, readErr)
		}
		// The `migrate:down` half undoes the tags, so reading it would record every
		// column as untagged by the end of the file.
		body := upSection(string(raw))
		for _, m := range tagRe.FindAllStringSubmatch(body, -1) {
			table, name := unquote(m[1]), unquote(m[2])
			latest[service+"."+table+"."+name] = column{
				service: service,
				table:   table,
				name:    name,
				class:   strings.ToLower(m[3]),
			}
		}
	}

	out := make([]column, 0, len(latest))
	for _, c := range latest {
		out = append(out, c)
	}
	sort.Slice(out, byServiceThenColumn(out))
	return out, nil
}

// byServiceThenColumn orders the registry the way a reader scans it: by service,
// then by table, then by column.
func byServiceThenColumn(out []column) func(i, j int) bool {
	return func(i, j int) bool {
		if out[i].service != out[j].service {
			return out[i].service < out[j].service
		}
		if out[i].table != out[j].table {
			return out[i].table < out[j].table
		}
		return out[i].name < out[j].name
	}
}

func upSection(body string) string {
	up, _, found := strings.Cut(body, "-- migrate:down")
	if !found {
		return body
	}
	return up
}

func unquote(s string) string {
	s = strings.ReplaceAll(s, `"`, "")
	idx := strings.LastIndex(s, ".")
	if idx >= 0 {
		s = s[idx+1:]
	}
	return s
}

func render(columns []column, policies retention) string {
	var b strings.Builder
	_, _ = b.WriteString(header)
	_, _ = b.WriteString("\n## Policy per class\n\n")
	_, _ = b.WriteString("| Class | Retention | At the end | Why |\n| --- | --- | --- | --- |\n")
	for _, name := range sortedPolicies(policies) {
		p := policies.Classes[name]
		_, _ = fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n", name, p.Period, p.Disposal, p.Note)
	}

	_, _ = b.WriteString("\n## Classified columns\n\n")
	_, _ = fmt.Fprintf(&b, "%d columns across %d services.\n\n", len(columns), countServices(columns))
	_, _ = b.WriteString("| Service | Column | Class | Retention | At the end |\n| --- | --- | --- | --- | --- |\n")
	for _, c := range columns {
		p := policies.Classes[c.class]
		row := fmt.Sprintf(
			"| %s | `%s.%s` | `%s` | %s | %s |\n",
			c.service,
			c.table,
			c.name,
			c.class,
			p.Period,
			p.Disposal,
		)
		_, _ = b.WriteString(row)
	}
	_, _ = b.WriteString(footer)
	return b.String()
}

func sortedPolicies(policies retention) []string {
	out := make([]string, 0, len(policies.Classes))
	for name := range policies.Classes {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func countServices(columns []column) int {
	seen := map[string]bool{}
	for _, c := range columns {
		seen[c.service] = true
	}
	return len(seen)
}

func failf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "✗ "+format+"\n", args...)
	os.Exit(1)
}
