// Command gostyle runs this repo's custom style analyzers — checks that
// gofmt/gofumpt/golangci-lint can't express because they're layout-based
// rather than purely structural. Add more analyzers to the Analyzers slice
// as they come up.
package main

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/multichecker"
)

// callArgsAnalyzer checks that every call's argument list is laid out as
// either entirely single-line (foo(x, y)) or fully exploded with one argument
// per line (foo(\n\tx,\n\ty,\n)) — never a partial mix of the two. gofmt
// preserves whatever line breaks the source already had for a call's
// arguments, so nothing in gofmt/gofumpt/golangci-lint catches an
// inconsistent in-between layout on its own.
var callArgsAnalyzer = &analysis.Analyzer{
	Name: "callargs",
	Doc:  "a call's arguments must be either all on one line or fully exploded one per line",
	Run:  runCallArgs,
}

func main() {
	multichecker.Main(callArgsAnalyzer)
}

func runCallArgs(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		if isGenerated(file) {
			continue
		}

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}

			if !isSingleLine(call, pass) && !isFullyExploded(call, pass) {
				pass.Reportf(
					call.Lparen,
					"call arguments are partially broken across lines; use either a single line or one argument per line",
				)
			}

			return true
		})
	}
	return nil, nil //nolint:nilnil // Run's (result, error) signature; nil result is normal
}

// isGenerated reports whether the file carries the standard
// "// Code generated ... DO NOT EDIT." marker (same convention golangci-lint
// uses for its own `generated: lax` exclusion).
func isGenerated(file *ast.File) bool {
	for _, c := range file.Comments {
		for _, line := range c.List {
			if strings.Contains(line.Text, "Code generated") && strings.Contains(line.Text, "DO NOT EDIT") {
				return true
			}
		}
	}
	return false
}

// isSingleLine reports whether the call's parens and every argument sit on
// the same source line.
func isSingleLine(call *ast.CallExpr, pass *analysis.Pass) bool {
	lparenLine := pass.Fset.Position(call.Lparen).Line
	rparenLine := pass.Fset.Position(call.Rparen).Line
	return lparenLine == rparenLine
}

// isFullyExploded reports whether the call has a newline right after the
// opening paren, a newline right before the closing paren, and each argument
// starts on a line of its own (none sharing a line with its neighbor).
func isFullyExploded(call *ast.CallExpr, pass *analysis.Pass) bool {
	lparenLine := pass.Fset.Position(call.Lparen).Line
	firstArgLine := pass.Fset.Position(call.Args[0].Pos()).Line
	if lparenLine == firstArgLine {
		return false
	}

	rparenLine := pass.Fset.Position(call.Rparen).Line
	lastArgEndLine := pass.Fset.Position(call.Args[len(call.Args)-1].End()).Line
	if rparenLine == lastArgEndLine {
		return false
	}

	for i := 1; i < len(call.Args); i++ {
		prevEndLine := pass.Fset.Position(call.Args[i-1].End()).Line
		curLine := pass.Fset.Position(call.Args[i].Pos()).Line
		if prevEndLine == curLine {
			return false
		}
	}

	return true
}
