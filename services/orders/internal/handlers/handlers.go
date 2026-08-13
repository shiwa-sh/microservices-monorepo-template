// Package handlers implement the ogen-generated orders.Handler interface (ADR-0303).
// Hand-written code imports the generated schema types and the sqlc store; it
// never shadows them with parallel structs or inline SQL.
package handlers

import (
	"context"
	"errors"
	"math"
	"net/url"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.temporal.io/sdk/client"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/apierr"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/authmw"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/authz"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/id"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/observability"
	orders "github.com/tabmadi/microservices-monorepo-template/libs/go/sdks/orders"
	"github.com/tabmadi/microservices-monorepo-template/services/orders/internal/store"
	"github.com/tabmadi/microservices-monorepo-template/services/orders/internal/workflows"
)

const serviceName = "orders"

type Handlers struct {
	q                store.Querier
	tc               client.Client
	checker          authz.Checker
	checkoutsStarted metric.Int64Counter
}

func New(db *pgxpool.Pool, tc client.Client, checker authz.Checker) *Handlers {
	return &Handlers{
		q:                store.New(db),
		tc:               tc,
		checker:          checker,
		checkoutsStarted: observability.Counter("orders.checkouts_started"),
	}
}

var _ orders.Handler = (*Handlers)(nil)

// These four are the transport boundary (ADR-0003): the columns hold bare uuids and
// the wire carries `order_`/`product_` and the base32 form. A product id is minted
// by catalog and only carried here, so it is encoded under catalog's prefix.
//
// The prefixes are literals, so encoding cannot fail on real input. Decoding cannot
// either — every identifier reaching a handler has already matched the OrderId or
// ProductId pattern in the generated validator — and it still reports rather than
// panics, because the validator and these calls are two places one spec edit can
// separate.
func orderID(u pgtype.UUID) orders.OrderId {
	return orders.OrderId(id.MustFrom("order", uuid.UUID(u.Bytes)).String())
}

func productID(u pgtype.UUID) orders.ProductId {
	return orders.ProductId(id.MustFrom("product", uuid.UUID(u.Bytes)).String())
}

// mintOrderID is where an identifier enters the system (ADR-0003). The service
// holds it before the insert rather than reading it back from a column default, so
// a write that never lands still has an identifier to log and to name in the
// failure.
func mintOrderID() (pgtype.UUID, error) {
	v, err := id.New("order")
	if err != nil {
		return pgtype.UUID{}, apierr.Internal(err.Error())
	}
	return pgtype.UUID{Bytes: v.UUID(), Valid: true}, nil
}

func storedOrderID(v orders.OrderId) (pgtype.UUID, error) {
	parsed, err := id.Parse("order", string(v))
	if err != nil {
		return pgtype.UUID{}, apierr.BadRequest("malformed order id")
	}
	return pgtype.UUID{Bytes: parsed.UUID(), Valid: true}, nil
}

func storedProductID(v orders.ProductId) (pgtype.UUID, error) {
	parsed, err := id.Parse("product", string(v))
	if err != nil {
		return pgtype.UUID{}, apierr.BadRequest("malformed product id")
	}
	return pgtype.UUID{Bytes: parsed.UUID(), Valid: true}, nil
}

func (h *Handlers) Checkout(ctx context.Context, req *orders.CheckoutInput) (*orders.WorkflowHandle, error) {
	ctx, span := observability.StartSpan(ctx, "orders.Checkout")
	defer span.End()

	if req.Quantity <= 0 || req.Quantity > math.MaxInt32 {
		return nil, apierr.BadRequest("product_id and quantity required")
	}
	product, err := storedProductID(req.ProductID)
	if err != nil {
		return nil, err
	}
	key, err := mintOrderID()
	if err != nil {
		return nil, err
	}
	row, err := h.q.CreateOrder(
		ctx,
		store.CreateOrderParams{
			ID:         key,
			ProductID:  product,
			Quantity:   int32(req.Quantity),
			TotalCents: 0,
		},
	)
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	oid := string(orderID(row.ID))
	// Tag the root span with the order id so a specific checkout is addressable in
	// Tempo (TraceQL `{ .order.id = "<id>" }`) — the anchor the e2e uses to pull the
	// exact end-to-end trace and assert it stitched across catalog + payment. The
	// wire form is what a reader has in hand, so it is what the attribute carries.
	span.SetAttributes(attribute.String("order.id", oid))
	_, err = h.tc.ExecuteWorkflow(
		ctx,
		client.StartWorkflowOptions{
			ID:        "checkout-" + oid,
			TaskQueue: serviceName + "-queue",
		},
		workflows.Checkout,
		workflows.CheckoutInput{
			OrderID:   oid,
			ProductID: string(productID(row.ProductID)),
			Quantity:  row.Quantity,
		},
	)
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	h.checkoutsStarted.Add(ctx, 1)
	return &orders.WorkflowHandle{
		ID:        "checkout-" + oid,
		RunID:     oid,
		Status:    orders.WorkflowHandleStatusRunning,
		ResultURL: orders.NewOptURI(url.URL{Path: "/api/orders/" + oid}),
	}, nil
}

func (h *Handlers) GetOrder(ctx context.Context, params orders.GetOrderParams) (*orders.Order, error) {
	key, err := storedOrderID(params.ID)
	if err != nil {
		return nil, err
	}
	row, err := h.q.GetOrder(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apierr.NotFound("order")
	}
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	return &orders.Order{
		ID:         orderID(row.ID),
		ProductID:  productID(row.ProductID),
		Quantity:   int(row.Quantity),
		TotalCents: int(row.TotalCents),
		Status:     orders.OrderStatus(row.Status),
	}, nil
}

func (h *Handlers) ListOrders(ctx context.Context) ([]orders.Order, error) {
	rows, err := h.q.ListOrders(ctx)
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	out := make([]orders.Order, 0, len(rows))
	for _, r := range rows {
		o := orders.Order{
			ID:         orderID(r.ID),
			ProductID:  productID(r.ProductID),
			Quantity:   int(r.Quantity),
			TotalCents: int(r.TotalCents),
			Status:     orders.OrderStatus(r.Status),
		}
		out = append(out, o)
	}
	return out, nil
}

func (h *Handlers) CancelOrder(ctx context.Context, params orders.CancelOrderParams) (*orders.WorkflowHandle, error) {
	ctx, span := observability.StartSpan(ctx, "orders.CancelOrder")
	defer span.End()

	err := h.requireOperator(ctx, "cancelling orders")
	if err != nil {
		return nil, err
	}
	key, err := storedOrderID(params.ID)
	if err != nil {
		return nil, err
	}
	row, err := h.q.GetOrder(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apierr.NotFound("order")
	}
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	if row.Status == "cancelled" || row.Status == "failed" {
		return nil, apierr.Conflict("order is not cancellable")
	}

	oid := string(params.ID)
	_, err = h.tc.ExecuteWorkflow(
		ctx,
		client.StartWorkflowOptions{
			ID:        "cancel-order-" + oid,
			TaskQueue: serviceName + "-queue",
		},
		workflows.CancelOrder,
		workflows.CancelInput{OrderID: oid},
	)
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	return &orders.WorkflowHandle{
		ID:        "cancel-order-" + oid,
		RunID:     oid,
		Status:    orders.WorkflowHandleStatusRunning,
		ResultURL: orders.NewOptURI(url.URL{Path: "/api/orders/" + oid}),
	}, nil
}

// NewError maps a handler error onto the generated RFC 9457 response (ADR-0303).
// The trace-id is stamped here rather than in each handler: a handler that forgets
// it produces an error nobody can correlate, and nothing signals the omission.
func (h *Handlers) NewError(ctx context.Context, err error) *orders.ErrorStatusCode {
	e, ok := apierr.As(err)
	if !ok {
		e = apierr.Internal(err.Error())
	}
	e = e.WithTrace(ctx)

	problem := orders.Problem{Type: e.Type, Title: e.Title, Status: e.Status}
	if e.Detail != "" {
		problem.Detail = orders.NewOptString(e.Detail)
	}
	if e.TraceID != "" {
		problem.TraceID = orders.NewOptString(e.TraceID)
	}
	for _, v := range e.Errors {
		problem.Errors = append(problem.Errors, orders.ProblemErrorsItem{Pointer: v.Pointer, Message: v.Message})
	}
	return &orders.ErrorStatusCode{StatusCode: e.Status, Response: problem}
}

// requireOperator gates a write on the shared OpenFGA Checker (ADR-0304): the
// caller must be an authenticated operator. Reads (List/Get) and starting a
// checkout stay open; only the destructive cancel is gated, matching catalog's
// operator-write policy.
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
