//go:build _template

// Activities are idempotent and short (ADR-0006). One per file.
package activities

import "context"

func DoStuff(_ context.Context, input string) (string, error) { return "ok:" + input, nil }
