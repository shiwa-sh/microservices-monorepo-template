// Package handlers implement the ogen-generated catalog.Handler interface (ADR-0303).
// Hand-written code imports the generated schema types and the sqlc store; it
// never shadows them with parallel structs or inline SQL.
package handlers

import (
	"context"
	"errors"
	"math"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/metric"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/apierr"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/authmw"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/authz"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/observability"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/sdks/catalog"
	"github.com/tabmadi/microservices-monorepo-template/services/catalog/internal/store"
)

type Handlers struct {
	q               store.Querier
	checker         authz.Checker
	productsCreated metric.Int64Counter
}

func New(db *pgxpool.Pool, checker authz.Checker) *Handlers {
	return &Handlers{
		q:               store.New(db),
		checker:         checker,
		productsCreated: observability.Counter("catalog.products_created"),
	}
}

var _ catalog.Handler = (*Handlers)(nil)

func (h *Handlers) ListProducts(ctx context.Context) ([]catalog.Product, error) {
	rows, err := h.q.ListProducts(ctx)
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	out := make([]catalog.Product, 0, len(rows))
	for _, r := range rows {
		out = append(out, catalog.Product{ID: r.ID.Bytes, Name: r.Name, PriceCents: int(r.PriceCents)})
	}
	return out, nil
}

func (h *Handlers) GetProduct(ctx context.Context, params catalog.GetProductParams) (*catalog.Product, error) {
	row, err := h.q.GetProduct(ctx, pgtype.UUID{Bytes: params.ID, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apierr.NotFound("product")
	}
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	return &catalog.Product{ID: row.ID.Bytes, Name: row.Name, PriceCents: int(row.PriceCents)}, nil
}

func (h *Handlers) CreateProduct(ctx context.Context, req *catalog.ProductInput) (*catalog.Product, error) {
	ctx, span := observability.StartSpan(ctx, "catalog.CreateProduct")
	defer span.End()

	err := h.requireOperator(ctx, "creating products")
	if err != nil {
		return nil, err
	}
	if req.Name == "" || req.PriceCents < 0 || req.PriceCents > math.MaxInt32 {
		return nil, apierr.BadRequest("name and price_cents required")
	}
	row, err := h.q.CreateProduct(ctx, store.CreateProductParams{Name: req.Name, PriceCents: int32(req.PriceCents)})
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	h.productsCreated.Add(ctx, 1)
	return &catalog.Product{ID: row.ID.Bytes, Name: row.Name, PriceCents: int(row.PriceCents)}, nil
}

func (h *Handlers) UpdateProduct(
	ctx context.Context,
	req *catalog.ProductInput,
	params catalog.UpdateProductParams,
) (*catalog.Product, error) {
	ctx, span := observability.StartSpan(ctx, "catalog.UpdateProduct")
	defer span.End()

	err := h.requireOperator(ctx, "updating products")
	if err != nil {
		return nil, err
	}
	if req.Name == "" || req.PriceCents < 0 || req.PriceCents > math.MaxInt32 {
		return nil, apierr.BadRequest("name and price_cents required")
	}
	row, err := h.q.UpdateProduct(
		ctx,
		store.UpdateProductParams{
			ID:         pgtype.UUID{Bytes: params.ID, Valid: true},
			Name:       req.Name,
			PriceCents: int32(req.PriceCents),
		},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apierr.NotFound("product")
	}
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	return &catalog.Product{ID: row.ID.Bytes, Name: row.Name, PriceCents: int(row.PriceCents)}, nil
}

func (h *Handlers) DeleteProduct(ctx context.Context, params catalog.DeleteProductParams) error {
	ctx, span := observability.StartSpan(ctx, "catalog.DeleteProduct")
	defer span.End()

	err := h.requireOperator(ctx, "deleting products")
	if err != nil {
		return err
	}
	err = h.q.DeleteProduct(ctx, pgtype.UUID{Bytes: params.ID, Valid: true})
	if err != nil {
		return apierr.Internal(err.Error())
	}
	return nil
}

// NewError maps a handler error onto the generated RFC 9457 response (ADR-0303).
// The trace-id is stamped here rather than in each handler: a handler that forgets
// it produces an error nobody can correlate, and nothing signals the omission.
func (h *Handlers) NewError(ctx context.Context, err error) *catalog.ErrorStatusCode {
	e, ok := apierr.As(err)
	if !ok {
		e = apierr.Internal(err.Error())
	}
	e = e.WithTrace(ctx)

	problem := catalog.Problem{Type: e.Type, Title: e.Title, Status: e.Status}
	if e.Detail != "" {
		problem.Detail = catalog.NewOptString(e.Detail)
	}
	if e.TraceID != "" {
		problem.TraceID = catalog.NewOptString(e.TraceID)
	}
	for _, v := range e.Errors {
		problem.Errors = append(problem.Errors, catalog.ProblemErrorsItem{Pointer: v.Pointer, Message: v.Message})
	}
	return &catalog.ErrorStatusCode{StatusCode: e.Status, Response: problem}
}

// requireOperator gates a write on the shared OpenFGA Checker (ADR-0304): the
// caller must be an authenticated operator. Reads (List/Get) stay open; only
// writes to the global catalog are gated (x-audience: internal, ADR-0303).
func (h *Handlers) requireOperator(ctx context.Context, action string) error {
	principal, _ := authmw.FromContext(ctx)
	if !principal.Authenticated() {
		return apierr.Unauthorized()
	}
	allowed, err := h.checker.Allowed(ctx, principal.Subject(), "member", "group:operator")
	if err != nil {
		return apierr.Internal(err.Error())
	}
	if !allowed {
		return apierr.Forbidden(action + " requires operator")
	}
	return nil
}
