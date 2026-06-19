//go:build ruleguard

// Package gorules holds custom gocritic/ruleguard lint rules for this repo.
// It is compiled only under the `ruleguard` build tag (used by golangci-lint),
// never by the normal Go build.
package gorules

import "github.com/quasilyte/go-ruleguard/dsl"

// noIfInitAssign forbids the init-statement (assignment) form of `if`,
// e.g. `if err := f(); err != nil {`. Declare the variable on its own line.
//
// The `$*_ := $*_` head matches any number of assignees, so single- and
// multi-value inits (`x := f()`, `v, ok := m[k]`) are both caught. The three
// else-shapes (no else / else-block / else-if) are spelled out because gogrep
// has no wildcard for an else clause. Inits inside a branch are still caught
// independently — the AST walker visits nested `if`s on their own — so the only
// thing this misses is an *outer* init followed by a multi-branch else-if chain,
// which is essentially never written (and is exactly the unreadable shape the
// ban discourages).
func noIfInitAssign(m dsl.Matcher) {
	m.Match(
		`if $*_ := $*_; $_ { $*_ }`,
		`if $*_ := $*_; $_ { $*_ } else { $*_ }`,
		`if $*_ := $*_; $_ { $*_ } else if $_ { $*_ }`,
		`if $*_ = $*_; $_ { $*_ }`,
		`if $*_ = $*_; $_ { $*_ } else { $*_ }`,
		`if $*_ = $*_; $_ { $*_ } else if $_ { $*_ }`,
	).Report(`no assignment in if-init: declare the variable on its own line`)
}
