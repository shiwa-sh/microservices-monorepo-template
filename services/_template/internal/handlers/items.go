//go:build _template

// Handlers implement the ogen-generated server Handler interface from libs/sdks/go/<svc>.
// Copy this file when scaffolding a new service; the codegen + this glue
// is what services own per-route.
package handlers

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/sdk/client"
)

// Handlers is the receiver for every generated operation.
type Handlers struct {
	DB *pgxpool.Pool
	TC client.Client
}

func New(db *pgxpool.Pool, tc client.Client) *Handlers { return &Handlers{DB: db, TC: tc} }

// Example ListItems handler — real signature comes from the ogen-generated server.
func (h *Handlers) ListItems(ctx context.Context) ([]map[string]any, error) {
	return []map[string]any{}, nil
}
