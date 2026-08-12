// Command lint-api-prefixes enforces resource-prefix ownership across the API
// namespace (ADR-0303, ADR-0306).
//
// All edge-exposed specs share one flat `/api` namespace, so which service owns a
// resource is a hidden routing detail: two services cannot both claim `/orders`.
// ADR-0306 names the edge route table — `ingress.resources` in the committed dev
// values — as the ownership registry, so that is what this reads. Dev is the
// canonical copy; the route table is identical across environments.
//
// Two failures are reported, and they are different defects:
//
//  1. Collision — two services declare the same resource. A name collision is a
//     genuine domain-modelling conflict, which is why it is a hard failure rather
//     than a first-writer-wins rule.
//  2. Drift between the registry and the spec — a service routes a resource its
//     spec has no path for, or serves a top-level path it never routed. Either way
//     the published contract and the reachable surface disagree, which is the same
//     class of defect lint-api-audience catches at the service level.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

func main() {
	specs, err := filepath.Glob(filepath.Join("services", "*", "openapi.yaml"))
	if err != nil {
		failf("glob specs: %v", err)
	}
	sort.Strings(specs)

	// owner maps a resource to every service declaring it, so a collision reports
	// all claimants rather than just the second one.
	owner := map[string][]string{}
	var problems []string

	for _, spec := range specs {
		svc := filepath.Base(filepath.Dir(spec))

		routed, err := routedResources(svc)
		if err != nil {
			failf("%s: %v", svc, err)
		}
		if len(routed) == 0 {
			continue // east-west service: it claims nothing in the /api namespace
		}

		served, err := servedPrefixes(spec)
		if err != nil {
			failf("%s: %v", spec, err)
		}

		for _, r := range routed {
			owner[r] = append(owner[r], svc)
			if !served[r] {
				msg := fmt.Sprintf("%s: routes %q at the edge but its spec has no /%s path", svc, r, r)
				problems = append(problems, msg)
			}
		}
		for p := range served {
			if slices.Contains(routed, p) {
				continue
			}
			msg := fmt.Sprintf("%s: spec serves /%s but the edge route table does not claim %q", svc, p, p)
			problems = append(problems, msg)
		}
	}

	for _, r := range sortedKeys(owner) {
		claimants := owner[r]
		if len(claimants) < 2 {
			continue
		}
		claimed := strings.Join(claimants, ", ")
		msg := fmt.Sprintf("resource %q is claimed by %s — one flat /api namespace admits one owner", r, claimed)
		problems = append(problems, msg)
	}

	if len(problems) > 0 {
		_, _ = fmt.Fprintln(os.Stderr, "✗ resource-prefix ownership (ADR-0303, ADR-0306):")
		sort.Strings(problems)
		for _, p := range problems {
			_, _ = fmt.Fprintln(os.Stderr, "  "+p)
		}
		os.Exit(1)
	}
	_, _ = fmt.Fprintf(os.Stdout, "✓ %d resource prefixes, each owned by one service\n", len(owner))
}

// routedResources reads the service's edge route table from its canonical dev
// values. A service with no values file (the _template scaffold) is not deployed
// and routes nothing.
func routedResources(svc string) ([]string, error) {
	path := filepath.Join("infra", "gitops", "services", "dev", "values", svc+".yaml")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var v struct {
		Ingress struct {
			Enabled   *bool    `yaml:"enabled"`
			Resources []string `yaml:"resources"`
		} `yaml:"ingress"`
	}
	err = yaml.Unmarshal(data, &v)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if v.Ingress.Enabled != nil && !*v.Ingress.Enabled { // chart default is true
		return nil, nil
	}
	return v.Ingress.Resources, nil
}

// httpMethods are the OpenAPI operation keys under a path item; other keys
// (parameters, servers, summary) are not operations and carry no audience.
var httpMethods = map[string]bool{
	"get": true, "put": true, "post": true, "delete": true,
	"patch": true, "options": true, "head": true, "trace": true,
}

// servedPrefixes is the set of top-level path segments the spec exposes AT THE
// EDGE. A path whose every operation resolves to `x-audience: cluster` is
// east-west — the Kratos identity webhook into orgs is the standing example — so
// it is deliberately unrouted and must not read as drift. Audience resolves per
// operation: its own x-audience, else the service default, else `cluster`.
func servedPrefixes(spec string) (map[string]bool, error) {
	data, err := os.ReadFile(spec)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", spec, err)
	}
	var s struct {
		Info struct {
			XAudience string `yaml:"x-audience"`
		} `yaml:"info"`
		Paths map[string]map[string]yaml.Node `yaml:"paths"`
	}
	err = yaml.Unmarshal(data, &s)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", spec, err)
	}
	def := s.Info.XAudience
	if def == "" {
		def = "cluster"
	}

	out := map[string]bool{}
	for p, methods := range s.Paths {
		seg := strings.Split(strings.TrimPrefix(p, "/"), "/")[0]
		if seg == "" {
			continue
		}
		for method, node := range methods {
			if !httpMethods[method] {
				continue
			}
			var op struct {
				XAudience string `yaml:"x-audience"`
			}
			err = node.Decode(&op)
			if err != nil {
				return nil, fmt.Errorf("parse %s %s %s: %w", spec, method, p, err)
			}
			aud := op.XAudience
			if aud == "" {
				aud = def
			}
			if aud != "cluster" {
				out[seg] = true
			}
		}
	}
	return out, nil
}

func sortedKeys(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func failf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "✗ "+format+"\n", args...)
	os.Exit(1)
}
