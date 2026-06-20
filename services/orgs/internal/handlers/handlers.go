// Handlers implement the ogen-generated orgs.Handler interface (ADR-0008).
// Hand-written code imports the generated schema types and the sqlc store; it
// never shadows them with parallel structs or inline SQL.
package handlers

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/apierr"
	orgs "github.com/tabmadi/microservices-monorepo-template/libs/go/sdks/orgs"
	"github.com/tabmadi/microservices-monorepo-template/services/orgs/internal/store"
)

type Handlers struct {
	db *pgxpool.Pool
	q  *store.Queries
}

func New(db *pgxpool.Pool) *Handlers { return &Handlers{db: db, q: store.New(db)} }

var _ orgs.Handler = (*Handlers)(nil)

func (h *Handlers) CreateOrg(ctx context.Context, req *orgs.OrgInput) (*orgs.Org, error) {
	if req.Name == "" {
		return nil, apierr.BadRequest("name required")
	}
	row, err := h.q.CreateOrg(ctx, req.Name)
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	return &orgs.Org{ID: row.ID.Bytes, Name: row.Name}, nil
}

func (h *Handlers) GetOrg(ctx context.Context, params orgs.GetOrgParams) (*orgs.Org, error) {
	row, err := h.q.GetOrg(ctx, pgtype.UUID{Bytes: params.ID, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apierr.NotFound("org")
	}
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	return &orgs.Org{ID: row.ID.Bytes, Name: row.Name}, nil
}

// OnIdentityCreated is the Kratos post-registration webhook (ADR-0010): create
// a personal org for the new identity and insert the user as its admin.
func (h *Handlers) OnIdentityCreated(ctx context.Context, req *orgs.OnIdentityCreatedReq) error {
	identityID, ok := req.IdentityID.Get()
	if !ok || identityID == "" {
		return apierr.BadRequest("identity_id required")
	}
	email, _ := req.Email.Get()

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return apierr.Internal(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := h.q.WithTx(tx)
	org, err := qtx.CreateOrg(ctx, email)
	if err != nil {
		return apierr.Internal(err.Error())
	}
	err = qtx.AddMember(ctx, store.AddMemberParams{OrgID: org.ID, UserID: identityID, Role: "admin"})
	if err != nil {
		return apierr.Internal(err.Error())
	}
	err = tx.Commit(ctx)
	if err != nil {
		return apierr.Internal(err.Error())
	}
	return nil
}

// NewError maps a handler error onto the generated RFC 7807 response.
func (h *Handlers) NewError(_ context.Context, err error) *orgs.ErrorStatusCode {
	e, ok := apierr.As(err)
	if ok {
		return &orgs.ErrorStatusCode{StatusCode: e.Status, Response: orgs.Problem{Code: e.Code, Message: e.Message}}
	}
	return &orgs.ErrorStatusCode{StatusCode: 500, Response: orgs.Problem{Code: "internal", Message: err.Error()}}
}
