// Command lint-api-audience is the audience↔exposure gate (ADR-0303). The audience
// ladder — cluster → internal → public — is a per-operation label (resolved from the
// operation's own x-audience, else the service default info.x-audience, else the
// fail-closed `cluster`). A service is edge-exposed iff it has at least one operation
// at `internal` or `public` (the edge surface); a `cluster`-only service is east-west
// and must NOT have an /api route. This keeps the documented audience and the real
// exposure boundary in lockstep — an edge contract is never published for an
// unreachable service, and an edge service is never silently left all-cluster.
//
// Edge exposure is read from the canonical dev gitops values (ingress.resources is
// identical across envs). A control-plane/decision service with no spec (e.g. authz,
// ADR-0303) has no audience and is exempt: it simply has no openapi.yaml to glob.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	audienceCluster  = "cluster"
	audienceInternal = "internal"
	audiencePublic   = "public"
)

// The OpenAPI operation keys under a path item; other keys (parameters, servers,
// summary) are not operations and carry no audience.
var httpMethods = map[string]bool{
	"get": true, "put": true, "post": true, "delete": true,
	"patch": true, "options": true, "head": true, "trace": true,
}

func main() {
	specs, err := filepath.Glob(filepath.Join("services", "*", "openapi.yaml"))
	if err != nil {
		failf("glob specs: %v", err)
	}
	problems := make([]string, 0, len(specs))
	for _, path := range specs {
		svc := filepath.Base(filepath.Dir(path))
		auds, err := effectiveAudiences(path)
		if err != nil {
			failf("%s: %v", path, err)
		}
		exposed, err := edgeExposed(svc)
		if err != nil {
			failf("%s: %v", svc, err)
		}
		problems = append(problems, check(svc, auds, exposed)...)
	}
	if len(problems) > 0 {
		_, _ = fmt.Fprintln(os.Stderr, "✗ API audience does not match edge exposure (ADR-0303):")
		for _, p := range problems {
			_, _ = fmt.Fprintln(os.Stderr, "  "+p)
		}
		os.Exit(1)
	}
	_, _ = fmt.Fprintf(os.Stdout, "✓ %d API specs: x-audience matches edge exposure\n", len(specs))
}

// check reports problems when the audience ladder and real edge exposure disagree.
// auds are the resolved effective audiences of every operation. A service is
// edge-exposed iff any operation is internal or public; a cluster-only service is
// east-west and must not have an /api route. Unknown audiences are also reported.
func check(svc string, auds []string, exposed bool) []string {
	problems := make([]string, 0, len(auds))
	hasEdge := false
	for _, a := range auds {
		switch a {
		case audienceCluster:
		case audienceInternal, audiencePublic:
			hasEdge = true
		default:
			msg := fmt.Sprintf("%s: unknown x-audience %q (expected cluster, internal, or public)", svc, a)
			problems = append(problems, msg)
		}
	}
	switch {
	case hasEdge && !exposed:
		msg := svc + ": has edge operations (x-audience internal/public) but no /api route (ingress.resources empty)"
		problems = append(problems, msg)
	case !hasEdge && exposed:
		msg := svc + ": edge-exposed (ingress.resources set) but every operation is x-audience: cluster (east-west)"
		problems = append(problems, msg)
	}
	return problems
}

// effectiveAudiences resolves each operation's audience: its own x-audience, else the
// service default (info.x-audience), else the fail-closed `cluster`.
func effectiveAudiences(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var s struct {
		Info struct {
			XAudience string `yaml:"x-audience"`
		} `yaml:"info"`
		Paths map[string]map[string]yaml.Node `yaml:"paths"`
	}
	err = yaml.Unmarshal(data, &s)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	def := s.Info.XAudience
	if def == "" {
		def = audienceCluster
	}
	var auds []string
	for _, methods := range s.Paths {
		for method, node := range methods {
			if !httpMethods[method] {
				continue
			}
			var op struct {
				XAudience string `yaml:"x-audience"`
			}
			err := node.Decode(&op)
			if err != nil {
				return nil, fmt.Errorf("parse %s operation %q: %w", path, method, err)
			}
			a := op.XAudience
			if a == "" {
				a = def
			}
			auds = append(auds, a)
		}
	}
	return auds, nil
}

// edgeExposed reports whether the service has an /api route, read from its canonical
// dev gitops values. A service with no such file (e.g. the _template scaffold) is
// treated as not deployed, hence not edge-exposed.
func edgeExposed(svc string) (bool, error) {
	path := filepath.Join("infra", "gitops", "services", "dev", "values", svc+".yaml")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	var v struct {
		Ingress struct {
			Enabled   *bool    `yaml:"enabled"`
			Resources []string `yaml:"resources"`
		} `yaml:"ingress"`
	}
	err = yaml.Unmarshal(data, &v)
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", path, err)
	}
	enabled := v.Ingress.Enabled == nil || *v.Ingress.Enabled // chart default is true
	return enabled && len(v.Ingress.Resources) > 0, nil
}

func failf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
