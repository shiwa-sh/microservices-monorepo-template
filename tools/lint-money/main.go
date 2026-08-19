// Command lint-money enforces the monetary-value rule (ADR-0100, ADR-0300).
//
// The rule is one sentence: a monetary amount is the shared money type, never a
// floating-point number — `numeric` in the database, a string in the OpenAPI
// schema, and a string on the wire. What makes it worth a gate rather than a review
// note is that every violation of it COMPILES, passes its tests, and produces a
// number that is correct in the examples anyone writes by hand. `price float64`
// round-trips 19.99 through a unit test and loses a cent on the ninety-third order.
//
// So this reads the three surfaces the rule names, plus TypeScript:
//
//	OpenAPI     a money-ish property is `$ref: Money`, not a number and not an
//	            integer of minor units.
//	Go          no float32/float64 field or variable with a money-ish name, and no
//	            `Cents` identifier outside the money package itself.
//	TypeScript  no money-ish `number`, and no `_cents` field.
//
// # What is a "money-ish name"
//
// A vocabulary, not a type system. The gate cannot know that `weight` is not money
// and `principal` is, so it works from the words this domain actually uses for
// money and accepts that a new one has to be added here. That is the correct
// failure mode: adding a word is a one-line edit in a review that is already about
// money, and the alternative — inferring it — is a gate that fires on `total_count`
// and gets bypassed within a month.
//
// # What this does NOT check, and why
//
// The database columns. `services/*/migrations/` still holds the `_cents` integer
// columns the money conversion left behind: ADR-0300 requires expand, then the code
// switch, then contract, with a deploy boundary in between, so the old columns
// outlive the code that read them by design. Flagging them would make this gate red
// for the entire correct execution of the migration it exists to support.
//
// Nothing is lost by the omission. A column is only reachable through a Go struct
// field or an OpenAPI property, and both of those are checked here — a new integer
// money column cannot be introduced without also introducing something this gate
// rejects.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// moneyWords is the vocabulary. A property, field, or variable whose name contains
// one of these as a whole word is treated as monetary.
// wordAmount and wordPrice are named because they appear in the vocabulary, in the
// Money component's own members, and in the report.
const (
	wordAmount = "amount"
	wordPrice  = "price"
)

var moneyWords = []string{
	wordAmount,
	"balance",
	"charge",
	"cost",
	"discount",
	"fee",
	wordPrice,
	"refund",
	"subtotal",
	"total",
}

// countWords end an identifier that counts something rather than valuing it.
// `total_count`, `total_items` and `amount_of_rows` all contain a money word and
// none of them are money — this is the exception that keeps the vocabulary usable
// on a codebase that also has quantities.
var countWords = []string{
	"bytes",
	"count",
	"items",
	"length",
	"quantity",
	"qty",
	"rows",
	"seconds",
	"size",
}

// moneyComponent is the shared OpenAPI schema every monetary property must point at
// (tools/codegen/shared-components.yaml).
const moneyComponent = "Money"

// A finding is one violation, rendered as `file:line: message`.
type finding struct {
	file string
	line int
	msg  string
}

func main() {
	var found []finding

	specs, err := filepath.Glob(filepath.Join("services", "*", "openapi.yaml"))
	if err != nil {
		failf("glob specs: %v", err)
	}
	sort.Strings(specs)
	for _, spec := range specs {
		hits, err := checkSpec(spec)
		if err != nil {
			failf("%v", err)
		}
		found = append(found, hits...)
	}

	goHits, err := checkGo()
	if err != nil {
		failf("%v", err)
	}
	found = append(found, goHits...)

	tsHits, err := checkTypeScript()
	if err != nil {
		failf("%v", err)
	}
	found = append(found, tsHits...)

	if len(found) > 0 {
		sort.Slice(
			found,
			func(i, j int) bool {
				if found[i].file != found[j].file {
					return found[i].file < found[j].file
				}
				return found[i].line < found[j].line
			},
		)
		for _, f := range found {
			_, _ = fmt.Fprintf(os.Stderr, "✗ %s:%d: %s\n", f.file, f.line, f.msg)
		}
		_, _ = fmt.Fprintf(os.Stderr, "\n  A monetary amount is the shared money type (ADR-0100):\n")
		_, _ = fmt.Fprintf(
			os.Stderr,
			"  numeric in the column, %s in the spec, a decimal string on the wire.\n",
			moneyComponent,
		)
		os.Exit(1)
	}

	_, _ = fmt.Fprintln(os.Stdout, "✓ every monetary value is the shared money type")
}

// checkSpec walks every schema's `properties` mapping and reports a monetary
// property that is not the shared component.
func checkSpec(path string) ([]finding, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var root yaml.Node
	err = yaml.Unmarshal(data, &root)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	var found []finding
	walkYAML(
		&root,
		func(key string, value *yaml.Node) {
			if key != "properties" || value.Kind != yaml.MappingNode {
				return
			}
			// The Money component's own members. It is the definition of the shape, so
			// its `amount` is a string by construction rather than in violation.
			if isMoneyDefinition(value) {
				return
			}
			for i := 0; i+1 < len(value.Content); i += 2 {
				name, schema := value.Content[i], value.Content[i+1]
				if !isMoneyName(name.Value) {
					continue
				}
				var s struct {
					Ref  string `yaml:"$ref"`
					Type string `yaml:"type"`
				}
				_ = schema.Decode(&s)
				if filepath.Base(s.Ref) == moneyComponent {
					continue
				}
				// A monetary property that carries no type at all is a composed schema
				// (allOf, oneOf) or a free-form object, neither of which this can read as
				// a money shape. Reporting it is right: the reader has to say which.
				msg := fmt.Sprintf(
					"%q is a monetary property but is %s, not $ref: %s",
					name.Value,
					describeType(s.Type),
					moneyComponent,
				)
				found = append(found, finding{file: path, line: name.Line, msg: msg})
			}
		},
	)
	return found, nil
}

// isMoneyDefinition reports whether a `properties` mapping is the Money component
// itself, recognised by its members rather than by the schema's name — a spec is
// free to call it something else, and what makes it Money is amount + currency.
func isMoneyDefinition(properties *yaml.Node) bool {
	var amount, currency bool
	for i := 0; i < len(properties.Content); i += 2 {
		switch properties.Content[i].Value {
		case wordAmount:
			amount = true
		case "currency":
			currency = true
		}
	}
	return amount && currency
}

func describeType(t string) string {
	if t == "" {
		return "an unrecognised shape"
	}
	return "type: " + t
}

// walkYAML calls fn for every key/value pair in every mapping in the document.
func walkYAML(n *yaml.Node, fn func(key string, value *yaml.Node)) {
	if n == nil {
		return
	}
	if n.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(n.Content); i += 2 {
			fn(n.Content[i].Value, n.Content[i+1])
		}
	}
	for _, c := range n.Content {
		walkYAML(c, fn)
	}
}

// goSkip are the trees this cannot say anything useful about: the money package
// implements the type (its own fields are `big.Int`, and FromMinorUnits exists
// precisely to read a legacy cents column), generated SDKs and stores mirror a spec
// and a schema that are checked at their source, and vendored code is not ours.
var goSkip = []string{
	"libs/go/money",
	// This file names the thing it forbids, in the pattern and in the explanation
	// of why the pattern exists.
	"tools/lint-money",
	"libs/go/sdks",
	"internal/store",
	"node_modules",
	"vendor",
	".next",
}

// floatTypes are the two types the rule names outright.
var floatTypes = map[string]bool{"float32": true, "float64": true}

// centsPattern matches an identifier that names minor units — the representation
// the shared type replaced.
var centsPattern = regexp.MustCompile(`(?i)cents`)

func checkGo() ([]finding, error) {
	var found []finding
	fset := token.NewFileSet()

	for _, path := range sourceFiles([]string{".go"}, goSkip) {
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		found = append(found, checkGoFile(fset, path, file)...)
	}
	return found, nil
}

// checkGoFile reports the two Go-side shapes: a float typed money value, and an
// identifier that names minor units.
func checkGoFile(fset *token.FileSet, path string, file *ast.File) []finding {
	var found []finding

	report := func(pos token.Pos, msg string) {
		found = append(found, finding{file: path, line: fset.Position(pos).Line, msg: msg})
	}

	ast.Inspect(
		file,
		func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.Field:
				for _, name := range v.Names {
					checkGoName(name.Name, v.Type, name.Pos(), report)
				}
			case *ast.ValueSpec:
				for _, name := range v.Names {
					checkGoName(name.Name, v.Type, name.Pos(), report)
				}
			}
			return true
		},
	)
	return found
}

func checkGoName(name string, typ ast.Expr, pos token.Pos, report func(token.Pos, string)) {
	if centsPattern.MatchString(name) {
		report(pos, fmt.Sprintf("%q names minor units; a monetary value is money.Amount", name))
		return
	}
	if !isMoneyName(name) {
		return
	}
	ident, ok := typ.(*ast.Ident)
	if ok && floatTypes[ident.Name] {
		report(pos, fmt.Sprintf("%q is monetary and %s; use money.Amount", name, ident.Name))
	}
}

// tsSkip mirrors goSkip: generated clients restate a spec that is checked at its
// source, and build output is not source at all.
var tsSkip = []string{
	// The counterpart of the money package exclusion: libs/ts/money IMPLEMENTS the
	// type, so its own members are the shape rather than a violation of it.
	"libs/ts/money",
	"libs/ts/sdks",
	"node_modules",
	".next",
	"dist",
	"test-results",
	"playwright-report",
}

// tsMoneyField matches an object-type member or literal whose name is monetary and
// whose value is a number: `price: number`, `total_cents: 1299`.
var tsMoneyField = regexp.MustCompile(`(?i)\b([a-z_][a-z0-9_]*)\s*\??\s*:\s*(number\b|-?[0-9])`)

func checkTypeScript() ([]finding, error) {
	var found []finding

	for _, path := range sourceFiles([]string{".ts", ".tsx", ".js"}, tsSkip) {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		found = append(found, checkTypeScriptFile(path, string(data))...)
	}
	return found, nil
}

// checkTypeScriptFile is textual where the Go side is syntactic. TypeScript has no
// standard-library parser to reach for, and the shape being looked for — a name and
// a number on the same line — survives the loss of structure.
func checkTypeScriptFile(path, source string) []finding {
	var found []finding

	for i, line := range strings.Split(source, "\n") {
		// A comment is prose about money, not money. The check is textual here, so
		// the exclusion has to be textual too.
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") {
			continue
		}
		for _, m := range tsMoneyField.FindAllStringSubmatch(line, -1) {
			name := m[1]
			if !isMoneyName(name) && !centsPattern.MatchString(name) {
				continue
			}
			hit := finding{
				file: path,
				line: i + 1,
				msg:  fmt.Sprintf("%q is a monetary value as a number; use Money from @libs/money", name),
			}
			found = append(found, hit)
		}
	}
	return found
}

// sourceFiles lists the committed files with one of these extensions, outside the
// skipped trees. Paths are collected before anything is read, so no file operation
// runs inside the walk.
func sourceFiles(exts, skip []string) []string {
	var out []string
	err := filepath.WalkDir(
		".",
		func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				// A dot-directory is .git, .next, or a tool cache: none of it is source.
				if strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
					return fs.SkipDir
				}
				return nil
			}
			if !slices.Contains(exts, filepath.Ext(path)) || skipped(path, skip) {
				return nil
			}
			out = append(out, path)
			return nil
		},
	)
	if err != nil {
		failf("walk: %v", err)
	}
	return out
}

// isMoneyName reports whether an identifier names a monetary value. The match is on
// WORDS, so `total_amount` and `unitPrice` match while `totals` and `pricing` do
// not — a substring match makes `pricing_enabled` a monetary field.
func isMoneyName(name string) bool {
	words := splitIdentifier(name)
	if len(words) == 0 {
		return false
	}
	// The last word decides what the identifier IS; the earlier ones qualify it.
	// `total_count` is a count of something, `order_total` is a total.
	if slices.Contains(countWords, words[len(words)-1]) {
		return false
	}
	for _, word := range words {
		if slices.Contains(moneyWords, word) {
			return true
		}
	}
	return false
}

// splitIdentifier breaks snake_case, camelCase, and PascalCase into lowercase words.
func splitIdentifier(name string) []string {
	var words []string
	var current strings.Builder
	flush := func() {
		if current.Len() > 0 {
			words = append(words, strings.ToLower(current.String()))
			current.Reset()
		}
	}
	for _, r := range name {
		switch {
		case r == '_' || r == '-' || r == '.':
			flush()
		case r >= 'A' && r <= 'Z':
			flush()
			_, _ = current.WriteRune(r)
		default:
			_, _ = current.WriteRune(r)
		}
	}
	flush()
	return words
}

func skipped(path string, prefixes []string) bool {
	clean := filepath.ToSlash(path)
	for _, p := range prefixes {
		if strings.Contains(clean, p) {
			return true
		}
	}
	return false
}

func failf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "✗ "+format+"\n", args...)
	os.Exit(1)
}
