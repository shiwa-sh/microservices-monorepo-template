//go:build _template

// Handlers implement the ogen-generated server Handler interface from
// libs/go/sdks/<svc> (ADR-0008). Copy this file when scaffolding a new service;
// new-service.sh strips the build tag and rewrites _template → <svc>.
// Hand-written code imports the generated schema types and the sqlc store; it
// never shadows them with parallel structs or inline SQL.
package handlers

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/apierr"
	tmpl "github.com/tabmadi/microservices-monorepo-template/libs/go/sdks/_template"
	"github.com/tabmadi/microservices-monorepo-template/services/_template/internal/store"
)

type Handlers struct {
	q *store.Queries
}

func New(db *pgxpool.Pool) *Handlers { return &Handlers{q: store.New(db)} }

var _ tmpl.Handler = (*Handlers)(nil)

func (h *Handlers) ListItems(ctx context.Context) ([]tmpl.Item, error) {
	rows, err := h.q.ListItems(ctx)
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	out := make([]tmpl.Item, 0, len(rows))
	for _, r := range rows {
		out = append(out, tmpl.Item{ID: r.ID.Bytes, Name: r.Name})
	}
	return out, nil
}

func (h *Handlers) CreateItem(ctx context.Context, req *tmpl.ItemInput) (*tmpl.Item, error) {
	if req.Name == "" {
		return nil, apierr.BadRequest("name required")
	}
	row, err := h.q.CreateItem(ctx, req.Name)
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	return &tmpl.Item{ID: row.ID.Bytes, Name: row.Name}, nil
}

// NewError maps a handler error onto the generated RFC 7807 response.
func (h *Handlers) NewError(_ context.Context, err error) *tmpl.ErrorStatusCode {
	e, ok := apierr.As(err)
	if ok {
		return &tmpl.ErrorStatusCode{StatusCode: e.Status, Response: tmpl.Problem{Code: e.Code, Message: e.Message}}
	}
	return &tmpl.ErrorStatusCode{StatusCode: 500, Response: tmpl.Problem{Code: "internal", Message: err.Error()}}
}
