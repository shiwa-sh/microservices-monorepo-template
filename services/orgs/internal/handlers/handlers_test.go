package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/apierr"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/authmw"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/authz"
	orgs "github.com/tabmadi/microservices-monorepo-template/libs/go/sdks/orgs"
)

// fakeChecker stands in for the OpenFGA Checker so the operator gate can be
// exercised without a cluster (ADR-0304).
type fakeChecker struct {
	allowed bool
	err     error
}

func (f fakeChecker) Allowed(context.Context, string, string, string) (bool, error) {
	return f.allowed, f.err
}

// UpdateOrg and DeleteOrg are operator-gated (ADR-0304): the gate rejects before
// any DB access, so a nil store is fine for these cases.
func TestOrgWriteAuthz(t *testing.T) {
	t.Parallel()

	writes := map[string]func(context.Context, *Handlers) error{
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
						ctx = authmw.NewContext(ctx, &authmw.Principal{UserID: "alice"})
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
	ctx := authmw.NewContext(context.Background(), &authmw.Principal{UserID: "alice"})
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
