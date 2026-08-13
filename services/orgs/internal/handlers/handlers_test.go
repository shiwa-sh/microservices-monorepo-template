package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/apierr"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/authmw"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/authz"
	orgs "github.com/tabmadi/microservices-monorepo-template/libs/go/sdks/orgs"
	"github.com/tabmadi/microservices-monorepo-template/services/orgs/internal/store"
)

// A well-formed wire identifier (ADR-0003), so the cases below exercise the
// handler's gates rather than its identifier decoding.
const testOrgID = orgs.OrgId("org_01kztn9tsrea7b1597q3yjdeav")

// testUser is the principal every authenticated case acts as.
const testUser = "alice"

// fakeQ embeds store.Querier so only the methods a test exercises need stubbing;
// any other call would nil-panic, which is the desired "unexpected query" signal.
type fakeQ struct {
	store.Querier

	org store.GetOrgRow
}

func (f fakeQ) GetOrg(context.Context, pgtype.UUID) (store.GetOrgRow, error) {
	return f.org, nil
}

// opCtx is an authenticated request context — the principal the read gate needs
// before it consults the Checker at all.
func opCtx() context.Context {
	return authmw.NewContext(context.Background(), &authmw.Principal{UserID: testUser})
}

// resourceChecker answers per resource, which is what the read gate needs: a member
// holds `org#read` on their own org and nothing on `group:operator`, and an
// operator is the other way round. A single bool cannot express either.
type resourceChecker map[string]bool

func (c resourceChecker) Allowed(_ context.Context, _, _, resource string) (bool, error) {
	return c[resource], nil
}

// orgObject is the OpenFGA object the read gate checks.
var orgObject = "org:" + string(testOrgID)

// A single org read, exercised through every principal that can ask for it.
func TestGetOrgAuthz(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ctx     func() context.Context
		checker authz.Checker
		want    int
	}{
		{"holding the identifier is not enough", context.Background, resourceChecker{orgObject: true}, 401},
		{"a non-member is forbidden", opCtx, resourceChecker{}, 403},
		{"a member reads their org", opCtx, resourceChecker{orgObject: true}, 0},
		{"an operator reads any org", opCtx, resourceChecker{"group:operator": true}, 0},
	}
	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				t.Parallel()
				h := &Handlers{q: fakeQ{org: store.GetOrgRow{Name: "Northwind"}}, checker: tc.checker}
				_, err := h.GetOrg(tc.ctx(), orgs.GetOrgParams{ID: testOrgID})
				if tc.want == 0 {
					if err != nil {
						t.Fatalf("unexpected error: %v", err)
					}
					return
				}
				assertStatus(t, err, tc.want)
			},
		)
	}
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

// Every org write is operator-gated (ADR-0304): the gate rejects before any DB
// access, so a nil store is fine for these cases.
func TestOrgWriteAuthz(t *testing.T) {
	t.Parallel()

	writes := map[string]func(context.Context, *Handlers) error{
		"CreateOrg": func(ctx context.Context, h *Handlers) error {
			_, err := h.CreateOrg(ctx, &orgs.OrgInput{Name: "acme"})
			return err
		},
		"UpdateOrg": func(ctx context.Context, h *Handlers) error {
			_, err := h.UpdateOrg(ctx, &orgs.OrgInput{Name: "acme"}, orgs.UpdateOrgParams{})
			return err
		},
		"DeleteOrg": func(ctx context.Context, h *Handlers) error {
			return h.DeleteOrg(ctx, orgs.DeleteOrgParams{})
		},
	}
	cases := []struct {
		name    string
		authed  bool
		checker authz.Checker
		want    int
	}{
		{"anonymous is unauthorized", false, fakeChecker{}, 401},
		{"non-operator is forbidden", true, fakeChecker{allowed: false}, 403},
		{"checker failure is internal", true, fakeChecker{err: errors.New("openfga down")}, 500},
	}
	for name, call := range writes {
		for _, tc := range cases {
			t.Run(
				name+"/"+tc.name,
				func(t *testing.T) {
					t.Parallel()
					ctx := context.Background()
					if tc.authed {
						ctx = authmw.NewContext(ctx, &authmw.Principal{UserID: testUser})
					}
					err := call(ctx, &Handlers{checker: tc.checker})
					assertStatus(t, err, tc.want)
				},
			)
		}
	}
}

// UpdateOrg rejects an empty name (ADR-0302) — but only after the operator gate,
// so this exercises the validation path with an authenticated operator and a
// nil store.
func TestUpdateOrgValidation(t *testing.T) {
	t.Parallel()
	ctx := authmw.NewContext(context.Background(), &authmw.Principal{UserID: testUser})
	h := &Handlers{checker: fakeChecker{allowed: true}}
	_, err := h.UpdateOrg(ctx, &orgs.OrgInput{Name: ""}, orgs.UpdateOrgParams{})
	assertStatus(t, err, 400)
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
