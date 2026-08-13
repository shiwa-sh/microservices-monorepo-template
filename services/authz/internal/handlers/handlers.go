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
//	CreateOperator — mints a Kratos identity with the `operator` trait and grants
//	                group:operator#member in OpenFGA (ADR-0401), the generated admin
//	                page target (x-admin: action).
package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/apierr"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/authz"
	authzsdk "github.com/tabmadi/microservices-monorepo-template/libs/go/sdks/authz"
)

const (
	aalLevel2         = "aal2"    // operator MFA assurance level (ADR-0304)
	operatorTraitTrue = "true"    // the `operator` identity trait, when set
	schemaUserV1      = "user_v1" // the Kratos identity schema id (user.v1.json)
)

type Handlers struct {
	checker     authz.Checker
	granter     authz.Granter
	fineGrained bool // when true, also require dashboard:<tool>#view in OpenFGA
	kratosAdmin string
	log         *slog.Logger
}

func New(checker authz.Checker, granter authz.Granter, fineGrained bool, log *slog.Logger) *Handlers {
	if log == nil {
		log = slog.Default()
	}
	admin := os.Getenv("KRATOS_ADMIN_URL")
	if admin == "" {
		admin = "http://ory-kratos-admin.platform.svc.cluster.local"
	}
	return &Handlers{checker: checker, granter: granter, fineGrained: fineGrained, kratosAdmin: admin, log: log}
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

// CreateOperator mints a Kratos identity carrying the `operator` trait — the coarse
// ops-tier claim gate — and grants group:operator#member in OpenFGA to seed the
// optional fine per-tool layer (ADR-0401).
func (h *Handlers) CreateOperator(ctx context.Context, req *authzsdk.OperatorInput) (*authzsdk.Operator, error) {
	id, err := h.createKratosIdentity(ctx, req.Email, req.Password)
	if err != nil {
		h.log.Error("create kratos identity", "err", err)
		return nil, apierr.Internal("failed to create identity")
	}
	err = h.granter.Grant(ctx, "user:"+id, "member", "group:operator")
	if err != nil {
		h.log.Error("grant operator", "err", err, "id", id)
		return nil, apierr.Internal("failed to grant operator role")
	}
	return &authzsdk.Operator{ID: id, Email: req.Email}, nil
}

// ListIdentities returns Kratos identities (product users and operators), flattened
// from traits — the console's Users changelist (ADR-0401). Only authz may reach the
// Kratos admin API (network-policies/30-ory.yaml), so the console fetches through
// here rather than talking to Kratos directly. Pagination is forwarded to Kratos.
func (h *Handlers) ListIdentities(
	ctx context.Context, params authzsdk.ListIdentitiesParams,
) ([]authzsdk.Identity, error) {
	ids, err := h.listKratosIdentities(ctx, params.PerPage.Or(0))
	if err != nil {
		h.log.Error("list kratos identities", "err", err)
		return nil, apierr.Internal("failed to list identities")
	}
	return ids, nil
}

// GetIdentity returns one identity by id — the console's edit-form prefill (ADR-0401).
func (h *Handlers) GetIdentity(ctx context.Context, params authzsdk.GetIdentityParams) (*authzsdk.Identity, error) {
	full, err := h.getKratosIdentity(ctx, params.ID)
	if err != nil {
		h.log.Error("get kratos identity", "err", err, "id", params.ID)
		return nil, apierr.Internal("failed to get identity")
	}
	id := full.flatten()
	return &id, nil
}

// UpdateIdentity applies the editable traits (name, operator) to an identity. Kratos
// PUT replaces the whole identity, so it reads the current one first and overlays the
// changed traits, preserving schema_id, state, and the email identifier.
func (h *Handlers) UpdateIdentity(
	ctx context.Context, req *authzsdk.IdentityUpdate, params authzsdk.UpdateIdentityParams,
) (*authzsdk.Identity, error) {
	full, err := h.getKratosIdentity(ctx, params.ID)
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
	updated, err := h.putKratosIdentity(ctx, full)
	if err != nil {
		h.log.Error("update kratos identity", "err", err, "id", params.ID)
		return nil, apierr.Internal("failed to update identity")
	}
	id := updated.flatten()
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

// kratosIdentityBody is the request body for POST /admin/identities.
type kratosIdentityBody struct {
	SchemaID            string            `json:"schema_id"`
	Traits              kratosTraits      `json:"traits"`
	Credentials         kratosCredentials `json:"credentials"`
	VerifiableAddresses []kratosAddress   `json:"verifiable_addresses"`
}

type kratosTraits struct {
	Email    string `json:"email"`
	Operator bool   `json:"operator"`
}

type kratosCredentials struct {
	Password kratosPasswordCredential `json:"password"`
}

type kratosPasswordCredential struct {
	Config kratosPasswordConfig `json:"config"`
}

type kratosPasswordConfig struct {
	Password string `json:"password"`
}

type kratosAddress struct {
	Value    string `json:"value"`
	Via      string `json:"via"`
	Verified bool   `json:"verified"`
	Status   string `json:"status"`
}

// kratosIdentity is the subset of a Kratos admin identity this service reads and
// writes. schema_id and state are carried through unmodified on update — Kratos PUT
// replaces the whole record, so dropping them would reset the identity. So is
// metadata_public, which this service never edits and must not erase: the edge
// builds X-Org-Id and X-Roles out of it (ADR-0304), so writing the record back
// without it would silently unassign an operator's org on the next name change.
type kratosIdentity struct {
	ID             string          `json:"id,omitempty"`
	SchemaID       string          `json:"schema_id,omitempty"`
	State          string          `json:"state,omitempty"`
	MetadataPublic json.RawMessage `json:"metadata_public,omitempty"`
	Traits         struct {
		Email    string `json:"email"`
		Name     string `json:"name,omitempty"`
		Operator bool   `json:"operator"`
	} `json:"traits"`
}

// flatten projects the Kratos identity onto the admin-facing Identity shape.
func (k *kratosIdentity) flatten() authzsdk.Identity {
	id := authzsdk.Identity{ID: k.ID, Email: k.Traits.Email, Operator: authzsdk.NewOptBool(k.Traits.Operator)}
	if k.Traits.Name != "" {
		id.Name = authzsdk.NewOptString(k.Traits.Name)
	}
	return id
}

// listKratosIdentities reads GET /admin/identities and flattens each identity's
// traits. Only per_page (page_size) is forwarded — this Kratos paginates by keyset,
// where `page` is an opaque token, not a 1-based offset; a numeric page returns an
// empty set. Zero perPage lets Kratos apply its own default.
func (h *Handlers) listKratosIdentities(ctx context.Context, perPage int) ([]authzsdk.Identity, error) {
	u := h.kratosAdmin + "/admin/identities"
	q := url.Values{}
	if perPage > 0 {
		q.Set("per_page", strconv.Itoa(perPage))
	}
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var raw []kratosIdentity
	err := h.kratosJSON(ctx, http.MethodGet, u, nil, http.StatusOK, &raw)
	if err != nil {
		return nil, err
	}
	out := make([]authzsdk.Identity, 0, len(raw))
	for i := range raw {
		out = append(out, raw[i].flatten())
	}
	return out, nil
}

// identityURL is the Kratos admin URL for one identity.
func (h *Handlers) identityURL(id string) string {
	return h.kratosAdmin + "/admin/identities/" + url.PathEscape(id)
}

// getKratosIdentity fetches one full identity by id.
func (h *Handlers) getKratosIdentity(ctx context.Context, id string) (*kratosIdentity, error) {
	var out kratosIdentity
	err := h.kratosJSON(ctx, http.MethodGet, h.identityURL(id), nil, http.StatusOK, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// putKratosIdentity writes a full identity back (Kratos PUT replaces the record).
func (h *Handlers) putKratosIdentity(ctx context.Context, ident *kratosIdentity) (*kratosIdentity, error) {
	body := *ident
	body.ID = "" // id is the path, not part of the update body
	var out kratosIdentity
	err := h.kratosJSON(ctx, http.MethodPut, h.identityURL(ident.ID), body, http.StatusOK, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// kratosJSON performs a JSON request to the Kratos admin API and decodes a JSON
// response, asserting the expected status. reqBody nil sends no body; out nil skips
// decoding. It is the shared transport for the identity read/write helpers.
func (h *Handlers) kratosJSON(ctx context.Context, method, u string, reqBody any, wantStatus int, out any) error {
	var reader io.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("call kratos: %w", err)
	}
	defer func() {
		closeErr := resp.Body.Close()
		if closeErr != nil {
			h.log.Error("close kratos response body", "err", closeErr)
		}
	}()
	if resp.StatusCode != wantStatus {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("kratos %d: %s", resp.StatusCode, b)
	}
	if out == nil {
		return nil
	}
	err = json.NewDecoder(resp.Body).Decode(out)
	if err != nil {
		return fmt.Errorf("decode kratos response: %w", err)
	}
	return nil
}

func (h *Handlers) createKratosIdentity(ctx context.Context, email, password string) (string, error) {
	payload := kratosIdentityBody{
		SchemaID: schemaUserV1,
		Traits:   kratosTraits{Email: email, Operator: true},
		Credentials: kratosCredentials{
			Password: kratosPasswordCredential{
				Config: kratosPasswordConfig{Password: password},
			},
		},
		VerifiableAddresses: []kratosAddress{
			{Value: email, Via: "email", Verified: true, Status: "completed"},
		},
	}
	var out struct {
		ID string `json:"id"`
	}
	err := h.kratosJSON(ctx, http.MethodPost, h.kratosAdmin+"/admin/identities", payload, http.StatusCreated, &out)
	if err != nil {
		return "", err
	}
	return out.ID, nil
}
