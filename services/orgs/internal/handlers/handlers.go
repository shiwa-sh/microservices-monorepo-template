// Package handlers implement the ogen-generated orgs.Handler interface (ADR-0303).
// Hand-written code imports the generated schema types and the sqlc store; it
// never shadows them with parallel structs or inline SQL.
package handlers

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/sdk/client"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/apierr"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/authmw"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/authz"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/id"
	orgs "github.com/tabmadi/microservices-monorepo-template/libs/go/sdks/orgs"
	"github.com/tabmadi/microservices-monorepo-template/services/orgs/internal/store"
	"github.com/tabmadi/microservices-monorepo-template/services/orgs/internal/workflows"
)

const serviceName = "orgs"

type Handlers struct {
	q       store.Querier
	tc      client.Client
	checker authz.Checker
}

func New(db *pgxpool.Pool, tc client.Client, checker authz.Checker) *Handlers {
	return &Handlers{q: store.New(db), tc: tc, checker: checker}
}

var _ orgs.Handler = (*Handlers)(nil)

// orgID and storedID are the transport boundary (ADR-0003): the column holds a bare
// uuid and the wire carries `org_` and the base32 form. Nothing between the two
// surfaces sees the other's shape.
//
// The prefix is a literal, so encoding cannot fail on real input. Decoding cannot
// either — every identifier reaching a handler has already matched the OrgId pattern
// in the generated validator — and it still reports rather than panics, because the
// validator and this call are two places one spec edit can separate.
func orgID(u pgtype.UUID) orgs.OrgId {
	return orgs.OrgId(id.MustFrom("org", uuid.UUID(u.Bytes)).String())
}

func storedID(v orgs.OrgId) (pgtype.UUID, error) {
	parsed, err := id.Parse("org", string(v))
	if err != nil {
		return pgtype.UUID{}, apierr.BadRequest("malformed org id")
	}
	return pgtype.UUID{Bytes: parsed.UUID(), Valid: true}, nil
}

func (h *Handlers) CreateOrg(ctx context.Context, req *orgs.OrgInput) (*orgs.Org, error) {
	if req.Name == "" {
		return nil, apierr.BadRequest("name required")
	}
	row, err := h.q.CreateOrg(ctx, req.Name)
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	return &orgs.Org{ID: orgID(row.ID), Name: row.Name}, nil
}

func (h *Handlers) GetOrg(ctx context.Context, params orgs.GetOrgParams) (*orgs.Org, error) {
	key, err := storedID(params.ID)
	if err != nil {
		return nil, err
	}
	row, err := h.q.GetOrg(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apierr.NotFound("org")
	}
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	return &orgs.Org{ID: orgID(row.ID), Name: row.Name}, nil
}

// OnIdentityCreated is the Kratos post-registration webhook (ADR-0304/0006). It
// starts the RegisterUser workflow rather than writing directly: creating the
// personal org spans the orgs DB and the OpenFGA owner tuple (an authz-relevant
// mutation), so it must run as a Temporal dual-write, never a bare DB write. The
// workflow ID is derived from the identity, so a duplicate webhook delivery is a
// no-op (Temporal rejects the duplicate ID).
func (h *Handlers) OnIdentityCreated(ctx context.Context, req *orgs.OnIdentityCreatedReq) error {
	identityID, ok := req.IdentityID.Get()
	if !ok || identityID == "" {
		return apierr.BadRequest("identity_id required")
	}

	_, err := h.tc.ExecuteWorkflow(
		ctx,
		client.StartWorkflowOptions{
			ID:        "register-user-" + identityID,
			TaskQueue: serviceName + "-queue",
		},
		workflows.RegisterUser,
		workflows.RegisterInput{IdentityID: identityID},
	)
	if err != nil {
		return apierr.Internal(err.Error())
	}
	return nil
}

func (h *Handlers) ListOrgs(ctx context.Context) ([]orgs.Org, error) {
	rows, err := h.q.ListOrgs(ctx)
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	out := make([]orgs.Org, 0, len(rows))
	for _, r := range rows {
		out = append(out, orgs.Org{ID: orgID(r.ID), Name: r.Name})
	}
	return out, nil
}

func (h *Handlers) UpdateOrg(ctx context.Context, req *orgs.OrgInput, params orgs.UpdateOrgParams) (*orgs.Org, error) {
	err := h.requireOperator(ctx, "updating orgs")
	if err != nil {
		return nil, err
	}
	if req.Name == "" {
		return nil, apierr.BadRequest("name required")
	}
	key, err := storedID(params.ID)
	if err != nil {
		return nil, err
	}
	row, err := h.q.UpdateOrg(
		ctx,
		store.UpdateOrgParams{
			ID:   key,
			Name: req.Name,
		},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apierr.NotFound("org")
	}
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	return &orgs.Org{ID: orgID(row.ID), Name: row.Name}, nil
}

func (h *Handlers) DeleteOrg(ctx context.Context, params orgs.DeleteOrgParams) error {
	err := h.requireOperator(ctx, "deleting orgs")
	if err != nil {
		return err
	}
	key, err := storedID(params.ID)
	if err != nil {
		return err
	}
	err = h.q.DeleteOrg(ctx, key)
	if err != nil {
		return apierr.Internal(err.Error())
	}
	return nil
}

// NewError maps a handler error onto the generated RFC 9457 response (ADR-0303).
// The trace-id is stamped here rather than in each handler: a handler that forgets
// it produces an error nobody can correlate, and nothing signals the omission.
func (h *Handlers) NewError(ctx context.Context, err error) *orgs.ErrorStatusCode {
	e, ok := apierr.As(err)
	if !ok {
		e = apierr.Internal(err.Error())
	}
	e = e.WithTrace(ctx)

	problem := orgs.Problem{Type: e.Type, Title: e.Title, Status: e.Status}
	if e.Detail != "" {
		problem.Detail = orgs.NewOptString(e.Detail)
	}
	if e.TraceID != "" {
		problem.TraceID = orgs.NewOptString(e.TraceID)
	}
	for _, v := range e.Errors {
		problem.Errors = append(problem.Errors, orgs.ProblemErrorsItem{Pointer: v.Pointer, Message: v.Message})
	}
	return &orgs.ErrorStatusCode{StatusCode: e.Status, Response: problem}
}

// requireOperator gates a write on the shared OpenFGA Checker (ADR-0304): the
// caller must be an authenticated operator. Reads (List/Get) and the
// registration webhook stay open; only the operator-facing org mutations are
// gated, matching catalog's operator-write policy.
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
