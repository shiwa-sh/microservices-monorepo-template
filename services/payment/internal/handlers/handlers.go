// Handlers implement the ogen-generated payment.Handler interface (ADR-0008).
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
	payment "github.com/tabmadi/microservices-monorepo-template/libs/go/sdks/payment"
	"github.com/tabmadi/microservices-monorepo-template/services/payment/internal/store"
	"github.com/tabmadi/microservices-monorepo-template/services/payment/internal/workflows"
)

const serviceName = "payment"

type Handlers struct {
	q  *store.Queries
	tc client.Client
}

func New(db *pgxpool.Pool, tc client.Client) *Handlers { return &Handlers{q: store.New(db), tc: tc} }

var _ payment.Handler = (*Handlers)(nil)

func (h *Handlers) CreateCharge(ctx context.Context, req *payment.ChargeInput, params payment.CreateChargeParams) (*payment.WorkflowHandle, error) {
	if params.IdempotencyKey == "" {
		return nil, apierr.BadRequest("Idempotency-Key required")
	}

	// Idempotency lookup before anything else (ADR-0006).
	existing, err := h.q.GetByIdempotencyKey(ctx, params.IdempotencyKey)
	if err == nil {
		return handle(uuid.UUID(existing.ID.Bytes).String()), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, apierr.Internal(err.Error())
	}

	created, err := h.q.CreateCharge(ctx, store.CreateChargeParams{
		OrderID:        pgtype.UUID{Bytes: req.OrderID, Valid: true},
		AmountCents:    int32(req.AmountCents),
		IdempotencyKey: params.IdempotencyKey,
	})
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	id := uuid.UUID(created.ID.Bytes).String()

	_, err = h.tc.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        "charge-" + id,
		TaskQueue: serviceName + "-queue",
	}, workflows.Charge, workflows.ChargeInput{
		ChargeID:    id,
		OrderID:     uuid.UUID(created.OrderID.Bytes).String(),
		AmountCents: created.AmountCents,
	})
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	return handle(id), nil
}

func (h *Handlers) GetCharge(ctx context.Context, params payment.GetChargeParams) (*payment.Charge, error) {
	id, err := uuid.Parse(params.ID)
	if err != nil {
		return nil, apierr.BadRequest("invalid id")
	}
	row, err := h.q.GetCharge(ctx, pgtype.UUID{Bytes: id, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apierr.NotFound("charge")
	}
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	return &payment.Charge{
		ID:          row.ID.Bytes,
		OrderID:     row.OrderID.Bytes,
		AmountCents: int(row.AmountCents),
		Status:      payment.ChargeStatus(row.Status),
	}, nil
}

func handle(id string) *payment.WorkflowHandle {
	return &payment.WorkflowHandle{
		ID:        "charge-" + id,
		RunID:     id,
		Status:    payment.WorkflowHandleStatusRunning,
		ResultURL: payment.NewOptURI(url.URL{Path: "/api/payment/charges/" + id}),
	}
}

// NewError maps a handler error onto the generated RFC 7807 response.
func (h *Handlers) NewError(_ context.Context, err error) *payment.ErrorStatusCode {
	e, ok := apierr.As(err)
	if ok {
		return &payment.ErrorStatusCode{StatusCode: e.Status, Response: payment.Problem{Code: e.Code, Message: e.Message}}
	}
	return &payment.ErrorStatusCode{StatusCode: 500, Response: payment.Problem{Code: "internal", Message: err.Error()}}
}
