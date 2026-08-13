// Package activities holds the orgs workflow activities (ADR-0302): the two legs
// of the register-user dual-write (ADR-0304).
package activities

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/authz"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/id"
	"github.com/tabmadi/microservices-monorepo-template/services/orgs/internal/store"
)

type Activities struct {
	db      *pgxpool.Pool
	q       *store.Queries
	granter authz.Granter
}

func New(db *pgxpool.Pool, granter authz.Granter) *Activities {
	return &Activities{db: db, q: store.New(db), granter: granter}
}

// personalOrgName is the default display name for the org auto-created at
// registration. It is deliberately generic, not the user's email: an org is a
// tenant that may later hold a whole team, its `name` is shown to every member
// the user invites (so an email here would leak PII, ADR-0301), and email is
// mutable. The user renames it from the console; the stable identity is the id.
const personalOrgName = "Personal workspace"

// CreatePersonalOrgActivity is dual-write leg 1 (ADR-0304): the application-DB
// write. Creates the identity's personal org (generic default name) and records
// them as its admin member in one transaction. Returns the new org id.
func (a *Activities) CreatePersonalOrgActivity(ctx context.Context, identityID string) (string, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("create personal org: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// The identifier is minted here rather than by a column default (ADR-0003), and
	// here rather than in the workflow: `id.New` reads the clock and the entropy
	// pool, which a workflow function may not do. An activity retry therefore mints a
	// fresh one, the same as the default it replaces.
	key, err := id.New("org")
	if err != nil {
		return "", fmt.Errorf("create personal org: mint id: %w", err)
	}

	qtx := a.q.WithTx(tx)
	org, err := qtx.CreateOrg(
		ctx,
		store.CreateOrgParams{
			ID:   pgtype.UUID{Bytes: key.UUID(), Valid: true},
			Name: personalOrgName,
		},
	)
	if err != nil {
		return "", fmt.Errorf("create personal org: insert org: %w", err)
	}
	err = qtx.AddMember(ctx, store.AddMemberParams{OrgID: org.ID, UserID: identityID, Role: "admin"})
	if err != nil {
		return "", fmt.Errorf("create personal org: add member: %w", err)
	}
	err = tx.Commit(ctx)
	if err != nil {
		return "", fmt.Errorf("create personal org: commit: %w", err)
	}
	return uuid.UUID(org.ID.Bytes).String(), nil
}

// GrantOrgAdminActivity is dual-write leg 2 (ADR-0304): the OpenFGA write. Grants
// the identity the `admin` relation on their personal org (org:<id>#admin@user:<id>,
// model.fga) so ReBAC ownership matches the app-DB membership.
func (a *Activities) GrantOrgAdminActivity(ctx context.Context, orgID, identityID string) error {
	err := a.granter.Grant(ctx, "user:"+identityID, "admin", "org:"+orgID)
	if err != nil {
		return fmt.Errorf("grant org admin: %w", err)
	}
	return nil
}
