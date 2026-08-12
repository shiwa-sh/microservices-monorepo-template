// Package handlers implement the ogen-generated payment.Handler interface (ADR-0303).
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
	"go.opentelemetry.io/otel/metric"
	"go.temporal.io/sdk/client"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/apierr"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/authmw"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/authz"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/observability"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/sdks/payment"
	"github.com/tabmadi/microservices-monorepo-template/services/payment/internal/store"
	"github.com/tabmadi/microservices-monorepo-template/services/payment/internal/workflows"
)

const serviceName = "payment"

type Handlers struct {
	q              store.Querier
	tc             client.Client
	checker        authz.Checker
	chargesCreated metric.Int64Counter
}

func New(db *pgxpool.Pool, tc client.Client, checker authz.Checker) *Handlers {
	return &Handlers{
		q:              store.New(db),
		tc:             tc,
		checker:        checker,
		chargesCreated: observability.Counter("payment.charges_created"),
	}
}

var _ payment.Handler = (*Handlers)(nil)

func (h *Handlers) CreateCharge(
	ctx context.Context,
	req *payment.ChargeInput,
	params payment.CreateChargeParams,
) (*payment.WorkflowHandle, error) {
	ctx, span := observability.StartSpan(ctx, "payment.CreateCharge")
	defer span.End()

	if params.IdempotencyKey == "" {
		return nil, apierr.BadRequest("Idempotency-Key required")
	}
	if req.AmountCents < 0 || req.AmountCents > math.MaxInt32 {
		return nil, apierr.BadRequest("amount_cents out of range")
	}

	// Idempotency lookup before anything else (ADR-0302).
	existing, err := h.q.GetByIdempotencyKey(ctx, params.IdempotencyKey)
	if err == nil {
		id := uuid.UUID(existing.ID.Bytes).String()
		return handle("charge-"+id, id), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, apierr.Internal(err.Error())
	}

	created, err := h.q.CreateCharge(
		ctx,
		store.CreateChargeParams{
			OrderID:        pgtype.UUID{Bytes: req.OrderID, Valid: true},
			AmountCents:    int32(req.AmountCents),
			IdempotencyKey: params.IdempotencyKey,
		},
	)
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	id := uuid.UUID(created.ID.Bytes).String()

	_, err = h.tc.ExecuteWorkflow(
		ctx,
		client.StartWorkflowOptions{
			ID:        "charge-" + id,
			TaskQueue: serviceName + "-queue",
		},
		workflows.Charge,
		workflows.ChargeInput{
			ChargeID:    id,
			OrderID:     uuid.UUID(created.OrderID.Bytes).String(),
			AmountCents: created.AmountCents,
		},
	)
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	h.chargesCreated.Add(ctx, 1)
	return handle("charge-"+id, id), nil
}

func (h *Handlers) RefundCharge(
	ctx context.Context,
	req *payment.RefundInput,
	params payment.RefundChargeParams,
) (*payment.WorkflowHandle, error) {
	ctx, span := observability.StartSpan(ctx, "payment.RefundCharge")
	defer span.End()

	err := h.requireOperator(ctx, "refunding charges")
	if err != nil {
		return nil, err
	}
	row, err := h.q.GetCharge(ctx, pgtype.UUID{Bytes: params.ID, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apierr.NotFound("charge")
	}
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	if row.Status != "settled" {
		return nil, apierr.Conflict("only settled charges can be refunded")
	}

	id := params.ID.String()
	_, err = h.tc.ExecuteWorkflow(
		ctx,
		client.StartWorkflowOptions{
			ID:        "refund-" + id,
			TaskQueue: serviceName + "-queue",
		},
		workflows.Refund,
		workflows.RefundInput{ChargeID: id, Reason: req.Reason},
	)
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	return handle("refund-"+id, id), nil
}

func (h *Handlers) ListCharges(ctx context.Context) ([]payment.Charge, error) {
	rows, err := h.q.ListCharges(ctx)
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	out := make([]payment.Charge, 0, len(rows))
	for _, r := range rows {
		c := payment.Charge{
			ID:          r.ID.Bytes,
			OrderID:     r.OrderID.Bytes,
			AmountCents: int(r.AmountCents),
			Status:      payment.ChargeStatus(r.Status),
		}
		out = append(out, c)
	}
	return out, nil
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

func handle(workflowID, chargeID string) *payment.WorkflowHandle {
	return &payment.WorkflowHandle{
		ID:        workflowID,
		RunID:     chargeID,
		Status:    payment.WorkflowHandleStatusRunning,
		ResultURL: payment.NewOptURI(url.URL{Path: "/api/charges/" + chargeID}),
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

// requireOperator gates a write on the shared OpenFGA Checker (ADR-0304): the
// caller must be an authenticated operator. Reads (List/Get) and creating a
// charge stay open; only the destructive refund is gated, matching catalog's
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
