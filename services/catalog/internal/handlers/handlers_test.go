package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/apierr"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/authmw"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/authz"
	catalog "github.com/tabmadi/microservices-monorepo-template/libs/go/sdks/catalog"
)

// fakeChecker stands in for the OpenFGA Checker so the authz gate can be
// exercised without a cluster (ADR-0304).
type fakeChecker struct {
	allowed bool
	err     error
}

func (f fakeChecker) Allowed(context.Context, string, string, string) (bool, error) {
	return f.allowed, f.err
}

// CreateProduct is operator-gated (x-audience: internal, ADR-0303). The gate
// rejects before any DB access, so a nil store is fine for these cases.
func TestCreateProductAuthz(t *testing.T) {
	t.Parallel()
	req := &catalog.ProductInput{Name: "widget", PriceCents: 100}

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
				_, err := h.CreateProduct(ctx, req)
				e, ok := apierr.As(err)
				if !ok {
					t.Fatalf("want *apierr.Error, got %v", err)
				}
				if e.Status != tc.want {
					t.Fatalf("status = %d, want %d", e.Status, tc.want)
				}
			},
		)
	}
}

// UpdateProduct and DeleteProduct (ADR-0302) run the same operator gate as
// CreateProduct, before any DB access — so a nil store is fine for these cases.
func TestWriteAuthz(t *testing.T) {
	t.Parallel()
	input := &catalog.ProductInput{Name: "widget", PriceCents: 100}

	writes := map[string]func(context.Context, *Handlers) error{
		"UpdateProduct": func(ctx context.Context, h *Handlers) error {
			_, err := h.UpdateProduct(ctx, input, catalog.UpdateProductParams{})
			return err
		},
		"DeleteProduct": func(ctx context.Context, h *Handlers) error {
			return h.DeleteProduct(ctx, catalog.DeleteProductParams{})
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
					e, ok := apierr.As(err)
					if !ok {
						t.Fatalf("want *apierr.Error, got %v", err)
					}
					if e.Status != tc.want {
						t.Fatalf("status = %d, want %d", e.Status, tc.want)
					}
				},
			)
		}
	}
}
