// Package activities holds the authz workflow activities (ADR-0302): the two legs
// of the operator-registration dual write (ADR-0304).
package activities

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/authz"
	"github.com/tabmadi/microservices-monorepo-template/services/authz/internal/kratos"
)

type Activities struct {
	identities *kratos.Admin
	granter    authz.Granter
}

func New(granter authz.Granter, log *slog.Logger) *Activities {
	return &Activities{identities: kratos.New(log), granter: granter}
}

// CreateOperatorIdentityActivity is dual-write leg 1: the Kratos identity, carrying
// the `operator` trait that is the coarse ops-tier claim gate (ADR-0306). Returns
// the identity id, which leg 2 needs to address the subject.
//
// Not idempotent on its own, and it does not need to be: Kratos refuses a second
// identity with the same email address, so a retry after a partial failure either
// creates the one identity or fails with a conflict the workflow surfaces.
func (a *Activities) CreateOperatorIdentityActivity(ctx context.Context, email, password string) (string, error) {
	id, err := a.identities.CreateOperatorIdentity(ctx, email, password)
	if err != nil {
		return "", fmt.Errorf("create operator identity: %w", err)
	}
	return id, nil
}

// GrantOperatorRoleActivity is dual-write leg 2: `group:operator#member` in
// OpenFGA, which seeds the optional fine per-tool dashboard layer (ADR-0401).
//
// This is the leg whose silent failure the workflow exists to prevent. Written
// from a handler, a failure here left an identity that can sign in and is refused
// by every ops tool. A tuple write is idempotent, so the retry is free.
func (a *Activities) GrantOperatorRoleActivity(ctx context.Context, identityID string) error {
	err := a.granter.Grant(ctx, "user:"+identityID, "member", "group:operator")
	if err != nil {
		return fmt.Errorf("grant operator role: %w", err)
	}
	return nil
}
