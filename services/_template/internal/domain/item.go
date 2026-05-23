//go:build _template

// Pure domain types — no DB, no HTTP, no Temporal imports.
package domain

import "github.com/google/uuid"

type Item struct {
	ID   uuid.UUID
	Name string
}
