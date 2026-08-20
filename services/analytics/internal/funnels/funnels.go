// Package funnels loads the committed funnel definitions (ADR-0700).
//
// A funnel is an ordered list of event names, and the definitions live in
// infra/analytics/funnels.yaml rather than in a table. The reason is the reason
// the access rules and alert rules are committed: a funnel someone defined in a UI
// is a funnel nobody can review, and the definition is what the numbers mean.
package funnels

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Funnel is one definition: an id, a human name, and the ordered steps.
type Funnel struct {
	ID    string   `yaml:"id"`
	Name  string   `yaml:"name"`
	Steps []string `yaml:"steps"`
}

type file struct {
	Funnels []Funnel `yaml:"funnels"`
}

// Set is the loaded definitions, indexed by id.
type Set struct {
	byID map[string]Funnel
}

// Load reads the definitions and validates them.
//
// Validation happens at LOAD rather than at use, so a malformed definition stops
// the service starting instead of producing a rollup that is quietly wrong. A
// funnel with one step is the case worth naming: it is not a funnel, it is a
// count, and it would report a 100% conversion rate forever.
func Load(path string) (*Set, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var f file
	err = yaml.Unmarshal(data, &f)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(f.Funnels) == 0 {
		return nil, fmt.Errorf("%s: no funnels defined", path)
	}

	set := &Set{byID: make(map[string]Funnel, len(f.Funnels))}
	for _, fn := range f.Funnels {
		if fn.ID == "" {
			return nil, fmt.Errorf("%s: a funnel has no id", path)
		}
		if len(fn.Steps) < 2 {
			return nil, fmt.Errorf(
				"%s: funnel %q has %d steps; a funnel needs at least two",
				path,
				fn.ID,
				len(fn.Steps),
			)
		}
		// A repeated step would count the same event twice and make the funnel
		// appear to hold at 100% through the duplicate.
		seen := make(map[string]bool, len(fn.Steps))
		for _, step := range fn.Steps {
			if seen[step] {
				return nil, fmt.Errorf("%s: funnel %q repeats step %q", path, fn.ID, step)
			}
			seen[step] = true
		}
		_, dup := set.byID[fn.ID]
		if dup {
			return nil, fmt.Errorf("%s: funnel %q is defined twice", path, fn.ID)
		}
		set.byID[fn.ID] = fn
	}
	return set, nil
}

// Get returns a funnel by id, and whether it exists.
func (s *Set) Get(id string) (Funnel, bool) {
	fn, ok := s.byID[id]
	return fn, ok
}

// Len reports how many funnels are defined.
func (s *Set) Len() int { return len(s.byID) }
