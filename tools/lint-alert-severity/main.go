// Command lint-alert-severity asserts that every committed alert rule carries a
// severity ADR-0502 admits, and that the Watchdog exists.
//
// ADR-0502 gives the vocabulary two values and one meaning each: `page` asserts a
// human acts within minutes, `ticket` is everything else. A third value is not a
// finer gradation — Alertmanager's routing tree matches on these two, so a rule
// carrying anything else reaches a receiver by falling through rather than by
// decision, and reads as routed while being routed by accident. Three rules in
// capacity.yaml carried `warn` for the life of the repo and nothing said so.
//
// The Watchdog check is here rather than in a runtime probe because absence is
// the signal: a repo with no Watchdog rule cannot detect a dead alerting
// pipeline, and that is a property of the committed files.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

const rulesDir = "infra/observability/alerts"

// watchdogAlert is the dead man's switch ADR-0502 names.
const watchdogAlert = "Watchdog"

// The vocabulary. Not configurable: it is the routing tree's matcher set.
var allowed = map[string]bool{"page": true, "ticket": true}

type ruleFile struct {
	Groups []struct {
		Name  string `yaml:"name"`
		Rules []struct {
			Alert  string            `yaml:"alert"`
			Record string            `yaml:"record"`
			Labels map[string]string `yaml:"labels"`
		} `yaml:"rules"`
	} `yaml:"groups"`
}

func main() {
	paths, err := filepath.Glob(filepath.Join(rulesDir, "*.yaml"))
	if err != nil {
		failf("glob %s: %v", rulesDir, err)
	}
	sort.Strings(paths)

	var problems []string
	alerts, watchdog := 0, false

	for _, path := range paths {
		// Chart.yaml and any other non-rule YAML that lands here parses cleanly
		// into an empty Groups, so it is skipped rather than reported.
		data, err := os.ReadFile(path)
		if err != nil {
			failf("read %s: %v", path, err)
		}
		var rf ruleFile
		err = yaml.Unmarshal(data, &rf)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: parse: %v", path, err))
			continue
		}
		for _, g := range rf.Groups {
			for _, r := range g.Rules {
				if r.Alert == "" {
					continue // a recording rule carries no severity
				}
				alerts++
				if r.Alert == watchdogAlert {
					watchdog = true
				}
				sev, ok := r.Labels["severity"]
				switch {
				case !ok:
					problems = append(problems, fmt.Sprintf("%s: %s carries no severity", path, r.Alert))
				case !allowed[sev]:
					const form = "%s: %s carries severity %q, which is not page or ticket"
					problems = append(problems, fmt.Sprintf(form, path, r.Alert, sev))
				}
			}
		}
	}

	if !watchdog {
		const form = "no %s rule in %s — nothing detects a dead alerting pipeline (ADR-0502)"
		problems = append(problems, fmt.Sprintf(form, watchdogAlert, rulesDir))
	}

	if len(problems) > 0 {
		_, _ = fmt.Fprintln(os.Stderr, "✗ alert severities do not match ADR-0502:")
		for _, p := range problems {
			_, _ = fmt.Fprintln(os.Stderr, "  "+p)
		}
		os.Exit(1)
	}
	_, _ = fmt.Fprintf(os.Stdout, "✓ %d alert rules carry page or ticket, and the Watchdog exists\n", alerts)
}

func failf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
