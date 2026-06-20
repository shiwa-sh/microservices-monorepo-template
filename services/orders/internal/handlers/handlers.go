// Handlers implement the ogen-generated orders.Handler interface (ADR-0008).
// Hand-written code imports the generated schema types and the sqlc store; it
// never shadows them with parallel structs or inline SQL.
package handlers

import (
	"context"
	"errors"
	"net/url"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/sdk/client"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/apierr"
	orders "github.com/tabmadi/microservices-monorepo-template/libs/go/sdks/orders"
	"github.com/tabmadi/microservices-monorepo-template/services/orders/internal/store"
	"github.com/tabmadi/microservices-monorepo-template/services/orders/internal/workflows"
)

const serviceName = "orders"

type Handlers struct {
	q  *store.Queries
	tc client.Client
}

func New(db *pgxpool.Pool, tc client.Client) *Handlers { return &Handlers{q: store.New(db), tc: tc} }

var _ orders.Handler = (*Handlers)(nil)

func (h *Handlers) Checkout(ctx context.Context, req *orders.CheckoutInput) (*orders.WorkflowHandle, error) {
	if req.Quantity <= 0 {
		return nil, apierr.BadRequest("product_id and quantity required")
	}
	row, err := h.q.CreateOrder(ctx, store.CreateOrderParams{
		ProductID:  pgtype.UUID{Bytes: req.ProductID, Valid: true},
		Quantity:   int32(req.Quantity),
		TotalCents: 0,
	})
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	id := uuid.UUID(row.ID.Bytes).String()
	_, err = h.tc.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        "checkout-" + id,
		TaskQueue: serviceName + "-queue",
	}, workflows.Checkout, workflows.CheckoutInput{
		OrderID:   id,
		ProductID: uuid.UUID(row.ProductID.Bytes).String(),
		Quantity:  row.Quantity,
	})
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	return &orders.WorkflowHandle{
		ID:        "checkout-" + id,
		RunID:     id,
		Status:    orders.WorkflowHandleStatusRunning,
		ResultURL: orders.NewOptURI(url.URL{Path: "/api/orders/orders/" + id}),
	}, nil
}

func (h *Handlers) GetOrder(ctx context.Context, params orders.GetOrderParams) (*orders.Order, error) {
	row, err := h.q.GetOrder(ctx, pgtype.UUID{Bytes: params.ID, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apierr.NotFound("order")
	}
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	return &orders.Order{
		ID:         row.ID.Bytes,
		ProductID:  row.ProductID.Bytes,
		Quantity:   int(row.Quantity),
		TotalCents: int(row.TotalCents),
		Status:     orders.OrderStatus(row.Status),
	}, nil
}

// NewError maps a handler error onto the generated RFC 7807 response.
func (h *Handlers) NewError(_ context.Context, err error) *orders.ErrorStatusCode {
	e, ok := apierr.As(err)
	if ok {
		return &orders.ErrorStatusCode{StatusCode: e.Status, Response: orders.Problem{Code: e.Code, Message: e.Message}}
	}
	return &orders.ErrorStatusCode{StatusCode: 500, Response: orders.Problem{Code: "internal", Message: err.Error()}}
}
