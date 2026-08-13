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
	"github.com/tabmadi/microservices-monorepo-template/libs/go/id"
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

// These four are the transport boundary (ADR-0003): the columns hold bare uuids and
// the wire carries `charge_`/`order_` and the base32 form. An order id is minted by
// orders and only carried here, so it is encoded under that service's prefix.
//
// The prefixes are literals, so encoding cannot fail on real input. Decoding cannot
// either — every identifier reaching a handler has already matched the ChargeId or
// OrderId pattern in the generated validator — and it still reports rather than
// panics, because the validator and these calls are two places one spec edit can
// separate.
func chargeID(u pgtype.UUID) payment.ChargeId {
	return payment.ChargeId(id.MustFrom("charge", uuid.UUID(u.Bytes)).String())
}

func orderID(u pgtype.UUID) payment.OrderId {
	return payment.OrderId(id.MustFrom("order", uuid.UUID(u.Bytes)).String())
}

// mintChargeID is where an identifier enters the system (ADR-0003). The service
// holds it before the insert rather than reading it back from a column default, so
// a write that never lands still has an identifier to log and to name in the
// failure.
func mintChargeID() (pgtype.UUID, error) {
	v, err := id.New("charge")
	if err != nil {
		return pgtype.UUID{}, apierr.Internal(err.Error())
	}
	return pgtype.UUID{Bytes: v.UUID(), Valid: true}, nil
}

func storedChargeID(v payment.ChargeId) (pgtype.UUID, error) {
	parsed, err := id.Parse("charge", string(v))
	if err != nil {
		return pgtype.UUID{}, apierr.BadRequest("malformed charge id")
	}
	return pgtype.UUID{Bytes: parsed.UUID(), Valid: true}, nil
}

func storedOrderID(v payment.OrderId) (pgtype.UUID, error) {
	parsed, err := id.Parse("order", string(v))
	if err != nil {
		return pgtype.UUID{}, apierr.BadRequest("malformed order id")
	}
	return pgtype.UUID{Bytes: parsed.UUID(), Valid: true}, nil
}

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
		cid := string(chargeID(existing.ID))
		return handle("charge-"+cid, cid), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, apierr.Internal(err.Error())
	}

	created, err := h.insertCharge(ctx, req.OrderID, int32(req.AmountCents), params.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	cid := string(chargeID(created.ID))

	_, err = h.tc.ExecuteWorkflow(
		ctx,
		client.StartWorkflowOptions{
			ID:        "charge-" + cid,
			TaskQueue: serviceName + "-queue",
		},
		workflows.Charge,
		workflows.ChargeInput{
			ChargeID:    cid,
			OrderID:     string(orderID(created.OrderID)),
			AmountCents: created.AmountCents,
		},
	)
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	h.chargesCreated.Add(ctx, 1)
	return handle("charge-"+cid, cid), nil
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
	key, err := storedChargeID(params.ID)
	if err != nil {
		return nil, err
	}
	row, err := h.q.GetCharge(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apierr.NotFound("charge")
	}
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	if row.Status != "settled" {
		return nil, apierr.Conflict("only settled charges can be refunded")
	}

	cid := string(params.ID)
	_, err = h.tc.ExecuteWorkflow(
		ctx,
		client.StartWorkflowOptions{
			ID:        "refund-" + cid,
			TaskQueue: serviceName + "-queue",
		},
		workflows.Refund,
		workflows.RefundInput{ChargeID: cid, Reason: req.Reason},
	)
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	return handle("refund-"+cid, cid), nil
}

func (h *Handlers) ListCharges(ctx context.Context) ([]payment.Charge, error) {
	// Every charge, not the caller's — a back-office view (`x-audience: internal`),
	// so it is operator-gated rather than scoped.
	err := h.requireOperator(ctx, "listing every charge")
	if err != nil {
		return nil, err
	}
	rows, err := h.q.ListCharges(ctx)
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	out := make([]payment.Charge, 0, len(rows))
	for _, r := range rows {
		c := payment.Charge{
			ID:          chargeID(r.ID),
			OrderID:     orderID(r.OrderID),
			AmountCents: int(r.AmountCents),
			Status:      payment.ChargeStatus(r.Status),
		}
		out = append(out, c)
	}
	return out, nil
}

func (h *Handlers) GetCharge(ctx context.Context, params payment.GetChargeParams) (*payment.Charge, error) {
	key, err := storedChargeID(params.ID)
	if err != nil {
		return nil, err
	}
	err = h.requireReader(ctx, "charge:"+string(params.ID))
	if err != nil {
		return nil, err
	}
	row, err := h.q.GetCharge(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apierr.NotFound("charge")
	}
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	return &payment.Charge{
		ID:          chargeID(row.ID),
		OrderID:     orderID(row.OrderID),
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

// NewError maps a handler error onto the generated RFC 9457 response (ADR-0303).
// The trace-id is stamped here rather than in each handler: a handler that forgets
// it produces an error nobody can correlate, and nothing signals the omission.
func (h *Handlers) NewError(ctx context.Context, err error) *payment.ErrorStatusCode {
	e, ok := apierr.As(err)
	if !ok {
		e = apierr.Internal(err.Error())
	}
	e = e.WithTrace(ctx)

	problem := payment.Problem{Type: e.Type, Title: e.Title, Status: e.Status}
	if e.Detail != "" {
		problem.Detail = payment.NewOptString(e.Detail)
	}
	if e.TraceID != "" {
		problem.TraceID = payment.NewOptString(e.TraceID)
	}
	for _, v := range e.Errors {
		problem.Errors = append(problem.Errors, payment.ProblemErrorsItem{Pointer: v.Pointer, Message: v.Message})
	}
	return &payment.ErrorStatusCode{StatusCode: e.Status, Response: problem}
}

// insertCharge puts the row on the two identifiers it needs: the order's, decoded
// from the wire, and its own, minted here (ADR-0003). The amount arrives already
// narrowed, so the range check stays with the request it validates.
func (h *Handlers) insertCharge(
	ctx context.Context,
	order payment.OrderId,
	amountCents int32,
	idempotencyKey string,
) (store.CreateChargeRow, error) {
	orderKey, err := storedOrderID(order)
	if err != nil {
		return store.CreateChargeRow{}, err
	}
	key, err := mintChargeID()
	if err != nil {
		return store.CreateChargeRow{}, err
	}
	created, err := h.q.CreateCharge(
		ctx,
		store.CreateChargeParams{
			ID:             key,
			OrderID:        orderKey,
			AmountCents:    amountCents,
			IdempotencyKey: idempotencyKey,
		},
	)
	if err != nil {
		return store.CreateChargeRow{}, apierr.Internal(err.Error())
	}
	return created, nil
}

// requireReader authorises a single-charge read (ADR-0003): an unguessable
// identifier is not an access control, so holding one grants nothing. `charge#read`
// in model.fga resolves through the charge's order, so the buyer and the owning
// org's admins reach it; an operator reaches every charge through the same
// back-office grant the list uses. Both are Checker calls.
func (h *Handlers) requireReader(ctx context.Context, object string) error {
	principal, _ := authmw.FromContext(ctx)
	if !principal.Authenticated() {
		return apierr.Unauthorized()
	}
	allowed, err := h.checker.Allowed(ctx, principal.Subject(), "read", object)
	if err != nil {
		return apierr.Internal(err.Error())
	}
	if allowed {
		return nil
	}
	return h.requireOperator(ctx, "reading a charge against someone else's order")
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
