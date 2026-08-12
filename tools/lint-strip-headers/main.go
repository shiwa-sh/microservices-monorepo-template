// Command lint-strip-headers is the anti-spoofing gate (ADR-0305). Every
// IngressRoute that authenticates via the Oathkeeper forwardAuth middleware MUST
// also apply strip-identity-headers BEFORE it, so a client cannot inject
// X-User-* / X-Org-Id / X-Roles on any route (anonymous routes especially). It
// fails non-zero if a forward-auth route is missing the strip, or applies it
// after forwardAuth.
//
// It reads the rendered manifests (kubectl kustomize + helm template) from
// stdin; the caller is scripts/lint-strip-headers.sh, which does the rendering.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

type ingressRoute struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		Routes []struct {
			Middlewares []struct {
				Name string `yaml:"name"`
			} `yaml:"middlewares"`
		} `yaml:"routes"`
	} `yaml:"spec"`
}

func main() {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		failf("read stdin: %v", err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))

	var bad []string
	checked := 0
	for {
		var ir ingressRoute
		err := dec.Decode(&ir)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			failf("parse yaml: %v", err)
		}
		if ir.Kind != "IngressRoute" {
			continue
		}
		for _, route := range ir.Spec.Routes {
			var mws []string
			for _, m := range route.Middlewares {
				mws = append(mws, m.Name)
			}
			fwd := indexOf(mws, "oathkeeper-forward-auth")
			if fwd < 0 {
				continue
			}
			checked++
			strip := indexOf(mws, "strip-identity-headers")
			switch {
			case strip < 0:
				bad = append(bad, ir.Metadata.Name+": forward-auth route without strip-identity-headers")
			case strip > fwd:
				bad = append(bad, ir.Metadata.Name+": strip-identity-headers must come BEFORE forward-auth")
			}
		}
	}
	if len(bad) > 0 {
		_, _ = fmt.Fprintln(os.Stderr, "✗ anti-spoofing gate failed:")
		for _, b := range bad {
			_, _ = fmt.Fprintln(os.Stderr, "  "+b)
		}
		os.Exit(1)
	}
	_, _ = fmt.Fprintf(os.Stdout, "✓ all %d forward-auth routes strip identity headers first\n", checked)
}

func indexOf(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

func failf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
