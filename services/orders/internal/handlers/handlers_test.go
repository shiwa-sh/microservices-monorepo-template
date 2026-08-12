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
	orders "github.com/tabmadi/microservices-monorepo-template/libs/go/sdks/orders"
	"github.com/tabmadi/microservices-monorepo-template/services/orders/internal/store"
)

const statusPending = "pending"

// fakeQ embeds store.Querier so only the methods a test exercises need stubbing;
// any other call would nil-panic, which is the desired "unexpected query" signal.
type fakeQ struct {
	store.Querier

	order   store.GetOrderRow
	getErr  error
	list    []store.ListOrdersRow
	listErr error
}

func (f fakeQ) GetOrder(context.Context, pgtype.UUID) (store.GetOrderRow, error) {
	return f.order, f.getErr
}

func (f fakeQ) ListOrders(context.Context) ([]store.ListOrdersRow, error) {
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

// CancelOrder is operator-gated (ADR-0304): the gate rejects before any DB
// access, so a nil store is fine for these cases.
func TestCancelOrderAuthz(t *testing.T) {
	t.Parallel()

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
				_, err := h.CancelOrder(ctx, orders.CancelOrderParams{})
				assertStatus(t, err, tc.want)
			},
		)
	}
}

func TestCancelOrder(t *testing.T) {
	t.Parallel()

	t.Run(
		"missing order is not found",
		func(t *testing.T) {
			t.Parallel()
			h := &Handlers{q: fakeQ{getErr: pgx.ErrNoRows}, tc: fakeTemporal{}, checker: okChecker()}
			_, err := h.CancelOrder(opCtx(), orders.CancelOrderParams{})
			assertStatus(t, err, 404)
		},
	)

	for _, status := range []string{"cancelled", "failed"} {
		t.Run(
			status+" order is a conflict",
			func(t *testing.T) {
				t.Parallel()
				h := &Handlers{q: fakeQ{order: store.GetOrderRow{Status: status}}, tc: fakeTemporal{}, checker: okChecker()}
				_, err := h.CancelOrder(opCtx(), orders.CancelOrderParams{})
				assertStatus(t, err, 409)
			},
		)
	}

	t.Run(
		"a workflow start failure is internal",
		func(t *testing.T) {
			t.Parallel()
			h := &Handlers{
				q:       fakeQ{order: store.GetOrderRow{Status: statusPending}},
				tc:      fakeTemporal{err: errors.New("temporal down")},
				checker: okChecker(),
			}
			_, err := h.CancelOrder(opCtx(), orders.CancelOrderParams{})
			assertStatus(t, err, 500)
		},
	)

	t.Run(
		"a cancellable order starts the cancel workflow",
		func(t *testing.T) {
			t.Parallel()
			h := &Handlers{q: fakeQ{order: store.GetOrderRow{Status: statusPending}}, tc: fakeTemporal{}, checker: okChecker()}
			handle, err := h.CancelOrder(opCtx(), orders.CancelOrderParams{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if handle.Status != orders.WorkflowHandleStatusRunning {
				t.Fatalf("status = %v, want running", handle.Status)
			}
		},
	)
}

func TestListOrders(t *testing.T) {
	t.Parallel()

	t.Run(
		"a store error is internal",
		func(t *testing.T) {
			t.Parallel()
			h := &Handlers{q: fakeQ{listErr: errors.New("db down")}}
			_, err := h.ListOrders(context.Background())
			assertStatus(t, err, 500)
		},
	)

	t.Run(
		"rows map to the API schema",
		func(t *testing.T) {
			t.Parallel()
			h := &Handlers{q: fakeQ{list: []store.ListOrdersRow{
				{Quantity: 2, TotalCents: 500, Status: statusPending},
			}}}
			got, err := h.ListOrders(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != 1 || got[0].Quantity != 2 || got[0].TotalCents != 500 ||
				got[0].Status != orders.OrderStatusPending {
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
