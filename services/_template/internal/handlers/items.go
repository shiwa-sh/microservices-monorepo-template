//go:build _template

// Handlers implement the ogen-generated server Handler interface from
// libs/go/sdks/<svc> (ADR-0303). Copy this file when scaffolding a new service;
// new-service.sh strips the build tag and rewrites _template → <svc>.
// Hand-written code imports the generated schema types and the sqlc store; it
// never shadows them with parallel structs or inline SQL.
package handlers

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/apierr"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/id"
	tmpl "github.com/tabmadi/microservices-monorepo-template/libs/go/sdks/_template"
	"github.com/tabmadi/microservices-monorepo-template/services/_template/internal/store"
)

type Handlers struct {
	q *store.Queries
}

func New(db *pgxpool.Pool) *Handlers { return &Handlers{q: store.New(db)} }

var _ tmpl.Handler = (*Handlers)(nil)

// itemID and mintID are the transport boundary (ADR-0003): the column holds a bare
// uuid and the wire carries `item_` and the base32 form. Rename the prefix with the
// resource — it is the singular of the collection noun, so /orders yields "order".
//
// Minting here rather than in a column default is what lets a handler log the
// identifier of a write that never lands. The prefix is a literal, so encoding
// cannot fail on real input.
func itemID(u pgtype.UUID) tmpl.ItemId {
	return tmpl.ItemId(id.MustFrom("item", uuid.UUID(u.Bytes)).String())
}

func mintID() (pgtype.UUID, error) {
	v, err := id.New("item")
	if err != nil {
		return pgtype.UUID{}, apierr.Internal(err.Error())
	}
	return pgtype.UUID{Bytes: v.UUID(), Valid: true}, nil
}

func (h *Handlers) ListItems(ctx context.Context) ([]tmpl.Item, error) {
	rows, err := h.q.ListItems(ctx)
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	out := make([]tmpl.Item, 0, len(rows))
	for _, r := range rows {
		out = append(out, tmpl.Item{ID: itemID(r.ID), Name: r.Name})
	}
	return out, nil
}

func (h *Handlers) CreateItem(ctx context.Context, req *tmpl.ItemInput) (*tmpl.Item, error) {
	if req.Name == "" {
		return nil, apierr.BadRequest("name required")
	}
	key, err := mintID()
	if err != nil {
		return nil, err
	}
	row, err := h.q.CreateItem(ctx, store.CreateItemParams{ID: key, Name: req.Name})
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	return &tmpl.Item{ID: itemID(row.ID), Name: row.Name}, nil
}

// NewError maps a handler error onto the generated RFC 9457 response (ADR-0303).
// The trace-id is stamped here rather than in each handler: a handler that forgets
// it produces an error nobody can correlate, and nothing signals the omission.
func (h *Handlers) NewError(ctx context.Context, err error) *tmpl.ErrorStatusCode {
	e, ok := apierr.As(err)
	if !ok {
		e = apierr.Internal(err.Error())
	}
	e = e.WithTrace(ctx)

	problem := tmpl.Problem{Type: e.Type, Title: e.Title, Status: e.Status}
	if e.Detail != "" {
		problem.Detail = tmpl.NewOptString(e.Detail)
	}
	if e.TraceID != "" {
		problem.TraceID = tmpl.NewOptString(e.TraceID)
	}
	for _, v := range e.Errors {
		problem.Errors = append(problem.Errors, tmpl.ProblemErrorsItem{Pointer: v.Pointer, Message: v.Message})
	}
	return &tmpl.ErrorStatusCode{StatusCode: e.Status, Response: problem}
}
