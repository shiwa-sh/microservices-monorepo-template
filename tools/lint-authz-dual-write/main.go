// Command lint-authz-dual-write checks that authorization tuples are written from
// a workflow's activities, never from a request handler (ADR-0304, ADR-0302).
//
// ADR-0304's rule: an authz-relevant mutation runs inside a Temporal workflow with
// the database write and the OpenFGA write as SEPARATE activities. The reason is
// the one property neither store can give on its own — there is no transaction
// across Postgres and OpenFGA, so the only thing that can make "the row exists and
// the tuple exists" true is a saga that retries each half until it is.
//
// A handler that writes both is the failure this prevents: it succeeds at the first
// write, fails at the second, returns an error, and leaves a resource that exists
// and cannot be read — or worse, one that exists and is readable by the wrong
// people. Nothing surfaces it. The row is there, the check says no, and the report
// is "permissions are broken" weeks later.
//
// # What it looks for
//
// A call to the granter — `Grant`, `Revoke`, or anything else on `authz.Granter` —
// from a package that is not `internal/activities`. The Granter is the only way to
// write a tuple (ADR-0304 confines the OpenFGA SDK to libs/go/authz, which
// depguard enforces), so the seam is exactly one type and this can be exact rather
// than heuristic.
//
// # What it does NOT look for
//
// Reads. `Checker.Allowed` from a handler is the correct shape and is what every
// protected route does.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// granterMethods are the mutating half of the authz seam (libs/go/authz).
var granterMethods = []string{"Grant", "Revoke"}

// activitiesDir is the one package a tuple write belongs in: an activity is what
// Temporal can retry until it succeeds, which is what makes the pair eventually
// consistent rather than occasionally wrong.
const activitiesDir = "internal/activities"

// exempt lists the call sites that are known violations, each with the reason it is
// still here. An exemption is a debt with a name — the alternative is a gate that
// does not exist because the first violation was reason enough to delete it.
var exempt = map[string]string{
	// The authz service has no Temporal worker at all, so there is no workflow to
	// move this into: `CreateOperator` writes a Kratos identity and then a
	// `group:operator` tuple, and a failure between them leaves an operator who
	// cannot use the ops tier. Closing it means giving authz a worker, which is a
	// service change rather than a lint fix. See the plan's F-row.
	"services/authz/internal/handlers/handlers.go": "authz has no worker yet; CreateOperator's dual write is tracked",
}

type finding struct {
	file string
	line int
	call string
}

func main() {
	var found []finding

	err := filepath.WalkDir(
		"services",
		func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			clean := filepath.ToSlash(path)
			// The scaffold is built under its own tag and deploys nowhere.
			if strings.HasPrefix(clean, "services/_") {
				return nil
			}
			if strings.Contains(clean, activitiesDir) {
				return nil
			}
			hits, parseErr := grantCalls(clean)
			if parseErr != nil {
				return parseErr
			}
			found = append(found, hits...)
			return nil
		},
	)
	if err != nil {
		failf(err)
	}

	var problems []finding
	for _, f := range found {
		_, exempted := exempt[f.file]
		if exempted {
			continue
		}
		problems = append(problems, f)
	}

	if len(problems) > 0 {
		sort.Slice(problems, func(i, j int) bool { return problems[i].line < problems[j].line })
		for _, p := range problems {
			_, _ = fmt.Fprintf(os.Stderr, "✗ %s:%d: %s writes an authz tuple outside an activity\n", p.file, p.line, p.call)
		}
		_, _ = fmt.Fprintf(os.Stderr, "\n  There is no transaction across Postgres and OpenFGA (ADR-0304). The row\n")
		_, _ = fmt.Fprintf(os.Stderr, "  write and the tuple write belong in one workflow, as two activities it can\n")
		_, _ = fmt.Fprintf(os.Stderr, "  retry — a handler that does both leaves a resource nobody can read.\n")
		os.Exit(1)
	}

	_, _ = fmt.Fprintf(os.Stdout, "✓ authz tuples are written from activities (%d exemption(s) recorded)\n", len(exempt))
}

// grantCalls returns every granter mutation called in a file.
func grantCalls(path string) ([]finding, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	var out []finding
	ast.Inspect(
		file,
		func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !slices.Contains(granterMethods, sel.Sel.Name) {
				return true
			}
			// `x.Grant(...)` where x is a granter. The receiver's TYPE is not
			// resolved here — that would mean type-checking the package for a
			// method name that belongs to one interface in this repo. A false
			// positive is a call named Grant on something else, which is a name
			// worth questioning anyway.
			hit := finding{
				file: path,
				line: fset.Position(call.Pos()).Line,
				call: render(sel),
			}
			out = append(out, hit)
			return true
		},
	)
	return out, nil
}

func render(sel *ast.SelectorExpr) string {
	ident, ok := sel.X.(*ast.Ident)
	if ok {
		return ident.Name + "." + sel.Sel.Name
	}
	return sel.Sel.Name
}

func failf(err error) {
	_, _ = fmt.Fprintf(os.Stderr, "✗ %v\n", err)
	os.Exit(1)
}
