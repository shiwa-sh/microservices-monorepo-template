// Package handlers implements the ogen-generated authz.Handler interface
// (ADR-0303): authz is a spec-first service like every other HTTP service, even
// though it owns no database and sits east-west behind Oathkeeper (ADR-0306).
//
// Two operations:
//
//	Authorize     — the ops-tier decision for Oathkeeper's remote_json authorizer.
//	                200 = allow, 403 = deny. Deny is a valid DECISION, not an error,
//	                so it returns the 403 response variant with nil error; only real
//	                infrastructure failures return an error (→ NewError → 5xx).
//	CreateOperator — starts the operator-registration workflow, which mints a
//	                Kratos identity with the `operator` trait and grants
//	                group:operator#member in OpenFGA (ADR-0401, ADR-0304). The
//	                generated admin page target (x-admin: action).
package handlers

import (
	"context"
	"fmt"
	"log/slog"

	"go.temporal.io/sdk/client"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/apierr"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/authz"
	authzsdk "github.com/tabmadi/microservices-monorepo-template/libs/go/sdks/authz"
	"github.com/tabmadi/microservices-monorepo-template/services/authz/internal/kratos"
	"github.com/tabmadi/microservices-monorepo-template/services/authz/internal/workflows"
)

const (
	aalLevel2         = "aal2" // operator MFA assurance level (ADR-0304)
	operatorTraitTrue = "true" // the `operator` identity trait, when set
	// The task queue this service's worker serves. Named for the service, like
	// every other queue on the platform.
	taskQueue = "authz-queue"
)

type Handlers struct {
	checker     authz.Checker
	granter     authz.Granter
	fineGrained bool // when true, also require dashboard:<tool>#view in OpenFGA
	identities  *kratos.Admin
	tc          client.Client
	log         *slog.Logger
}

func New(
	checker authz.Checker,
	granter authz.Granter,
	fineGrained bool,
	tc client.Client,
	log *slog.Logger,
) *Handlers {
	if log == nil {
		log = slog.Default()
	}
	return &Handlers{
		checker:     checker,
		granter:     granter,
		fineGrained: fineGrained,
		identities:  kratos.New(log),
		tc:          tc,
		log:         log,
	}
}

var _ authzsdk.Handler = (*Handlers)(nil)

// Authorize is the ops-tier edge authorizer (ADR-0306). It answers in two layers:
//
//	coarse (mandatory) — a CLAIM check: the `operator` trait and AAL2. It makes NO
//	    OpenFGA call, so a product-authz outage cannot lock operators out of the
//	    dashboards they need to diagnose it (break-glass independence).
//	fine (optional) — the subject holds dashboard:<tool>#view in OpenFGA, enabled
//	    per-project via OPS_FINE_GRAINED.
//
// A bare authenticated session therefore never grants tool access.
func (h *Handlers) Authorize(ctx context.Context, req *authzsdk.AuthorizeRequest) (authzsdk.AuthorizeRes, error) {
	allowed, reason, err := h.decide(ctx, req)
	if err != nil {
		return nil, apierr.Internal(err.Error())
	}
	// Auth audit event (ADR-0306): who reached which tool, and the outcome.
	h.log.LogAttrs(
		ctx,
		slog.LevelInfo,
		"ops-authz decision",
		slog.String("subject", req.Subject),
		slog.String("tool", req.Tool),
		slog.Bool("allowed", allowed),
		slog.String("reason", reason),
	)
	if !allowed {
		// A deny is the expected answer here, not a failure: Oathkeeper reads the
		// body. It still carries the platform error shape (ADR-0303) so a client
		// parses one thing whatever produced it.
		denied := apierr.Forbidden(reason).WithTrace(ctx)
		return &authzsdk.Problem{
			Type:   denied.Type,
			Title:  denied.Title,
			Status: denied.Status,
			Detail: authzsdk.NewOptString(denied.Detail),
		}, nil
	}
	return &authzsdk.AuthorizeOK{}, nil
}

// CreateOperator starts the operator-registration workflow and returns its handle.
//
// It does not do the work. Minting the Kratos identity and granting
// group:operator#member in OpenFGA is a dual write across two systems with no
// shared transaction (ADR-0304), and doing it inline was this platform's one
// recorded exemption from that rule: a failure between the two left an operator
// who could sign in and was refused by every ops tool, with the request already
// returned and nothing anywhere to say why.
//
// The workflow id is derived from the email, which makes a repeated submission
// idempotent rather than a second identity: Temporal refuses to start a second run
// under an id already running, and this returns the handle of the first.
func (h *Handlers) CreateOperator(
	ctx context.Context, req *authzsdk.OperatorInput,
) (*authzsdk.WorkflowHandle, error) {
	id := "register-operator-" + req.Email
	run, err := h.tc.ExecuteWorkflow(
		ctx,
		client.StartWorkflowOptions{ID: id, TaskQueue: taskQueue},
		workflows.RegisterOperator,
		workflows.RegisterOperatorInput{Email: req.Email, Password: req.Password},
	)
	if err != nil {
		h.log.Error("start register operator", "err", err, "email", req.Email)
		return nil, apierr.Internal("failed to start operator registration")
	}
	return &authzsdk.WorkflowHandle{
		ID:     id,
		RunID:  run.GetRunID(),
		Status: authzsdk.WorkflowHandleStatusRunning,
	}, nil
}

// ListIdentities returns Kratos identities (product users and operators), flattened
// from traits — the console's Users changelist (ADR-0401). Only authz may reach the
// Kratos admin API (network-policies/30-ory.yaml), so the console fetches through
// here rather than talking to Kratos directly. Pagination is forwarded to Kratos.
func (h *Handlers) ListIdentities(
	ctx context.Context, params authzsdk.ListIdentitiesParams,
) ([]authzsdk.Identity, error) {
	ids, err := h.identities.ListIdentities(ctx, params.PerPage.Or(0))
	if err != nil {
		h.log.Error("list kratos identities", "err", err)
		return nil, apierr.Internal("failed to list identities")
	}
	return ids, nil
}

// GetIdentity returns one identity by id — the console's edit-form prefill (ADR-0401).
func (h *Handlers) GetIdentity(ctx context.Context, params authzsdk.GetIdentityParams) (*authzsdk.Identity, error) {
	full, err := h.identities.GetIdentity(ctx, params.ID)
	if err != nil {
		h.log.Error("get kratos identity", "err", err, "id", params.ID)
		return nil, apierr.Internal("failed to get identity")
	}
	id := full.Flatten()
	return &id, nil
}

// UpdateIdentity applies the editable traits (name, operator) to an identity. Kratos
// PUT replaces the whole identity, so it reads the current one first and overlays the
// changed traits, preserving schema_id, state, and the email identifier.
func (h *Handlers) UpdateIdentity(
	ctx context.Context, req *authzsdk.IdentityUpdate, params authzsdk.UpdateIdentityParams,
) (*authzsdk.Identity, error) {
	full, err := h.identities.GetIdentity(ctx, params.ID)
	if err != nil {
		h.log.Error("get kratos identity", "err", err, "id", params.ID)
		return nil, apierr.Internal("failed to load identity")
	}
	name, ok := req.Name.Get()
	if ok {
		full.Traits.Name = name
	}
	operator, ok := req.Operator.Get()
	if ok {
		full.Traits.Operator = operator
	}
	updated, err := h.identities.PutIdentity(ctx, full)
	if err != nil {
		h.log.Error("update kratos identity", "err", err, "id", params.ID)
		return nil, apierr.Internal("failed to update identity")
	}
	id := updated.Flatten()
	return &id, nil
}

// NewError maps a handler error onto the generated RFC 9457 response (ADR-0303).
// The trace-id is stamped here rather than in each handler: a handler that forgets
// it produces an error nobody can correlate, and nothing signals the omission.
func (h *Handlers) NewError(ctx context.Context, err error) *authzsdk.ErrorStatusCode {
	e, ok := apierr.As(err)
	if !ok {
		e = apierr.Internal(err.Error())
	}
	e = e.WithTrace(ctx)

	problem := authzsdk.Problem{Type: e.Type, Title: e.Title, Status: e.Status}
	if e.Detail != "" {
		problem.Detail = authzsdk.NewOptString(e.Detail)
	}
	if e.TraceID != "" {
		problem.TraceID = authzsdk.NewOptString(e.TraceID)
	}
	for _, v := range e.Errors {
		problem.Errors = append(problem.Errors, authzsdk.ProblemErrorsItem{Pointer: v.Pointer, Message: v.Message})
	}
	return &authzsdk.ErrorStatusCode{StatusCode: e.Status, Response: problem}
}

// decide returns the allow/deny decision and its reason. The error is non-nil only
// on an infrastructure failure (a OpenFGA call error), never on a plain deny.
func (h *Handlers) decide(ctx context.Context, req *authzsdk.AuthorizeRequest) (bool, string, error) {
	if req.Subject == "" {
		return false, "no session", nil
	}
	// Coarse gate — a CLAIM, not a Checker call: AAL2 session and the `operator`
	// trait. No OpenFGA is consulted, so a product-authz outage never locks
	// operators out.
	if req.Aal != aalLevel2 {
		return false, "aal2 required", nil
	}
	if req.Operator != operatorTraitTrue {
		return false, "not an operator", nil
	}
	// Fine gate (optional): per-tool grant in OpenFGA. Skipped unless enabled.
	if h.fineGrained {
		ok, err := h.checker.Allowed(ctx, "user:"+req.Subject, "view", "dashboard:"+req.Tool)
		if err != nil {
			return false, "", fmt.Errorf("checker: %w", err)
		}
		if !ok {
			return false, "no grant for " + req.Tool, nil
		}
	}
	return true, "ok", nil
}
