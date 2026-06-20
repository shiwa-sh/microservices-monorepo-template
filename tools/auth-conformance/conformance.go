// Package authconformance exercises the platform's edge→service identity
// contract (ADR-0009, ADR-0010): given the identity headers the edge injects,
// authmw must parse a known Principal, and a role-gated authorisation must
// resolve the expected way. The fixtures are identity-header inputs with
// expected principals + authz outcomes; the test in conformance_test.go runs
// them against the real libs/go/authmw reader.
package authconformance

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/authmw"
)

//go:embed fixtures.json
var fixturesJSON []byte

// Fixture is one identity-header input with its expected outcome.
type Fixture struct {
	Name        string            `json:"name"`
	Headers     map[string]string `json:"headers"`
	Want        WantPrincipal     `json:"want"`
	Subject     string            `json:"subject"`
	RequireRole string            `json:"require_role"`
	WantAllowed bool              `json:"want_allowed"`
}

// WantPrincipal is the expected parsed principal for a fixture.
type WantPrincipal struct {
	UserID string   `json:"user_id"`
	OrgID  string   `json:"org_id"`
	Roles  []string `json:"roles"`
}

// Fixtures loads the committed identity-header fixtures.
func Fixtures() ([]Fixture, error) {
	var fs []Fixture
	err := json.Unmarshal(fixturesJSON, &fs)
	if err != nil {
		return nil, fmt.Errorf("decode fixtures: %w", err)
	}
	return fs, nil
}

// RoleAllowed is the sample authorisation contract used by conformance: a
// request is allowed when the edge-resolved principal carries the required
// role. Real services delegate to the SpiceDB Checker (libs/go/authz); this
// hermetic stand-in keeps the conformance suite free of external services while
// still asserting "principal in → decision out".
func RoleAllowed(p *authmw.Principal, requiredRole string) bool {
	return p.Authenticated() && p.HasRole(requiredRole)
}
