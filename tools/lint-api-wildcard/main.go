// Command lint-api-wildcard enforces that the product edge rule (ADR-0306) never
// matches `/api/` with an open path wildcard. The panel-breaking regression was a
// bare `<**>/api/<**>` match: it collides with every ops dashboard's own route, so
// an ops `/api/*` request matched both `api-services` and its `ops-<tool>` rule and
// Oathkeeper returned 500 on the ambiguity. The safe form enumerates the resources
// it fronts (`/api/<{products,orders,...}><**>`), so this guard fails non-zero on any
// rule whose match URL has a wildcard immediately after `/api/`.
//
//	go run ./tools/lint-api-wildcard infra/auth/oathkeeper/access-rules.json
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
)

type rule struct {
	ID    string `json:"id"`
	Match struct {
		URL string `json:"url"`
	} `json:"match"`
}

// A wildcard token (`<*>`, `<**>`, or a bare `*`) directly after `/api/` — the
// collision-prone form. `/api/<{products,...}>` (an enumerated alternation) and
// literal segments are fine.
var bareAPIWildcard = regexp.MustCompile(`/api/(<\*|\*)`)

func main() {
	if len(os.Args) != 2 {
		failf("usage: lint-api-wildcard <access-rules.json>")
	}
	// #nosec G304 G703 -- path is an operator-supplied repo file; this is a local lint helper, not a server.
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		failf("read %s: %v", os.Args[1], err)
	}
	var rules []rule
	err = json.Unmarshal(data, &rules)
	if err != nil {
		failf("parse %s: %v", os.Args[1], err)
	}

	var bad []string
	for _, r := range rules {
		if bareAPIWildcard.MatchString(r.Match.URL) {
			msg := fmt.Sprintf(
				"%s: match url %q has a bare wildcard after /api/ — enumerate resources instead (/api/<{a,b,c}>)",
				r.ID,
				r.Match.URL,
			)
			bad = append(bad, msg)
		}
	}
	if len(bad) > 0 {
		_, _ = fmt.Fprintln(os.Stderr, "✗ product /api rules must enumerate resources, never a bare /api wildcard:")
		for _, b := range bad {
			_, _ = fmt.Fprintln(os.Stderr, "  "+b)
		}
		os.Exit(1)
	}
	_, _ = fmt.Fprintf(os.Stdout, "✓ no bare /api wildcard in %d rules\n", len(rules))
}

func failf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
