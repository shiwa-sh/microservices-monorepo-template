// Command lint-authorizer enforces the ops-tier authorizer policy (ADR-0306):
// every operator-dashboard access rule must authorize per-tool via the
// remote_json authorizer (→ OpenFGA Checker), never `allow`. A re-introduced
// `"authorizer": {"handler": "allow"}` on an ops route is exactly the gap this
// guard closes, so it fails non-zero on it.
//
// Product /api routes legitimately keep `allow` (services authorize in-process
// via libs/go/authz), so only ops-* rules are checked.
//
//	go run ./tools/lint-authorizer infra/auth/oathkeeper/access-rules.json
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type rule struct {
	ID         string `json:"id"`
	Authorizer struct {
		Handler string `json:"handler"`
	} `json:"authorizer"`
}

func main() {
	if len(os.Args) != 2 {
		failf("usage: lint-authorizer <access-rules.json>")
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
	ops := 0
	for _, r := range rules {
		// Ops dashboards are identified by the `ops-` rule-id prefix (one origin
		// per tool under *.ops.<host>). They must use remote_json, not allow.
		if !strings.HasPrefix(r.ID, "ops-") {
			continue
		}
		ops++
		if r.Authorizer.Handler != "remote_json" {
			bad = append(bad, fmt.Sprintf("%s: authorizer is %q, expected \"remote_json\"", r.ID, r.Authorizer.Handler))
		}
	}
	if len(bad) > 0 {
		_, _ = fmt.Fprintln(os.Stderr, "✗ ops dashboard rules must authorize via remote_json, never allow:")
		for _, b := range bad {
			_, _ = fmt.Fprintln(os.Stderr, "  "+b)
		}
		os.Exit(1)
	}
	_, _ = fmt.Fprintf(os.Stdout, "✓ all ops dashboard rules use remote_json (%d rules)\n", ops)
}

func failf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
