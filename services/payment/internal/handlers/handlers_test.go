package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"go.temporal.io/sdk/client"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/apierr"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/authmw"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/authz"
	payment "github.com/tabmadi/microservices-monorepo-template/libs/go/sdks/payment"
	"github.com/tabmadi/microservices-monorepo-template/services/payment/internal/store"
)

const statusSettled = "settled"

// fakeQ embeds store.Querier so only the methods a test exercises need stubbing;
// any other call would nil-panic, which is the desired "unexpected query" signal.
type fakeQ struct {
	store.Querier

	charge  store.GetChargeRow
	getErr  error
	list    []store.ListChargesRow
	listErr error
}

func (f fakeQ) GetCharge(context.Context, pgtype.UUID) (store.GetChargeRow, error) {
	return f.charge, f.getErr
}

func (f fakeQ) ListCharges(context.Context) ([]store.ListChargesRow, error) {
	return f.list, f.listErr
}

// fakeTemporal embeds client.Client; only ExecuteWorkflow is reached by the
// handler (its returned run is ignored, so a nil run is fine).
type fakeTemporal struct {
	client.Client

	err error
}

func (f fakeTemporal) ExecuteWorkflow(
	context.Context, client.StartWorkflowOptions, any, ...any,
) (client.WorkflowRun, error) {
	return nil, f.err
}

// fakeChecker stands in for the OpenFGA Checker so the operator gate can be
// exercised without a cluster (ADR-0304).
type fakeChecker struct {
	allowed bool
	err     error
}

func (f fakeChecker) Allowed(context.Context, string, string, string) (bool, error) {
	return f.allowed, f.err
}

// opCtx is an authenticated-operator request context — the happy path past the
// requireOperator gate, so the business-logic cases below reach the store.
func opCtx() context.Context {
	return authmw.NewContext(context.Background(), &authmw.Principal{UserID: "alice"})
}

func okChecker() authz.Checker { return fakeChecker{allowed: true} }

// RefundCharge is operator-gated (ADR-0304): the gate rejects before any DB
// access, so a nil store is fine for these cases.
func TestRefundChargeAuthz(t *testing.T) {
	t.Parallel()
	req := &payment.RefundInput{Reason: "duplicate"}

	tests := []struct {
		name    string
		authed  bool
		checker authz.Checker
		want    int
	}{
		{"anonymous is unauthorized", false, fakeChecker{}, 401},
		{"non-operator is forbidden", true, fakeChecker{allowed: false}, 403},
		{"checker failure is internal", true, fakeChecker{err: errors.New("openfga down")}, 500},
	}
	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				t.Parallel()
				ctx := context.Background()
				if tc.authed {
					ctx = authmw.NewContext(ctx, &authmw.Principal{UserID: "alice"})
				}
				h := &Handlers{checker: tc.checker}
				_, err := h.RefundCharge(ctx, req, payment.RefundChargeParams{})
				assertStatus(t, err, tc.want)
			},
		)
	}
}

func TestRefundCharge(t *testing.T) {
	t.Parallel()
	req := &payment.RefundInput{Reason: "duplicate"}

	t.Run(
		"missing charge is not found",
		func(t *testing.T) {
			t.Parallel()
			h := &Handlers{q: fakeQ{getErr: pgx.ErrNoRows}, tc: fakeTemporal{}, checker: okChecker()}
			_, err := h.RefundCharge(opCtx(), req, payment.RefundChargeParams{})
			assertStatus(t, err, 404)
		},
	)

	t.Run(
		"an unsettled charge is a conflict",
		func(t *testing.T) {
			t.Parallel()
			h := &Handlers{q: fakeQ{charge: store.GetChargeRow{Status: "pending"}}, tc: fakeTemporal{}, checker: okChecker()}
			_, err := h.RefundCharge(opCtx(), req, payment.RefundChargeParams{})
			assertStatus(t, err, 409)
		},
	)

	t.Run(
		"a workflow start failure is internal",
		func(t *testing.T) {
			t.Parallel()
			h := &Handlers{
				q:       fakeQ{charge: store.GetChargeRow{Status: statusSettled}},
				tc:      fakeTemporal{err: errors.New("temporal down")},
				checker: okChecker(),
			}
			_, err := h.RefundCharge(opCtx(), req, payment.RefundChargeParams{})
			assertStatus(t, err, 500)
		},
	)

	t.Run(
		"a settled charge starts the refund workflow",
		func(t *testing.T) {
			t.Parallel()
			h := &Handlers{q: fakeQ{charge: store.GetChargeRow{Status: statusSettled}}, tc: fakeTemporal{}, checker: okChecker()}
			handle, err := h.RefundCharge(opCtx(), req, payment.RefundChargeParams{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if handle.Status != payment.WorkflowHandleStatusRunning {
				t.Fatalf("status = %v, want running", handle.Status)
			}
		},
	)
}

func TestListCharges(t *testing.T) {
	t.Parallel()

	t.Run(
		"a store error is internal",
		func(t *testing.T) {
			t.Parallel()
			h := &Handlers{q: fakeQ{listErr: errors.New("db down")}}
			_, err := h.ListCharges(context.Background())
			assertStatus(t, err, 500)
		},
	)

	t.Run(
		"rows map to the API schema",
		func(t *testing.T) {
			t.Parallel()
			h := &Handlers{q: fakeQ{list: []store.ListChargesRow{
				{AmountCents: 999, Status: statusSettled},
			}}}
			got, err := h.ListCharges(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != 1 || got[0].AmountCents != 999 || got[0].Status != payment.ChargeStatusSettled {
				t.Fatalf("mapping = %+v", got)
			}
		},
	)
}

func assertStatus(t *testing.T, err error, want int) {
	t.Helper()
	e, ok := apierr.As(err)
	if !ok {
		t.Fatalf("want *apierr.Error, got %v", err)
	}
	if e.Status != want {
		t.Fatalf("status = %d, want %d", e.Status, want)
	}
}
