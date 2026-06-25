// Package decision is the ops-tier edge authorizer (ADR-0017). Oathkeeper's
// remote_json authorizer POSTs {subject, tool, aal} for each request to an
// operator dashboard; this handler returns 200 (allow) or 403 (deny) after
// checking, through libs/go/authz's Checker:
//
//	coarse — the subject is in group:operator (the whole ops tier), and
//	fine   — the subject has dashboard:<tool>#view (the specific tool), and
//	AAL2   — the session carries a second factor (operator MFA, ADR-0010).
//
// A bare authenticated session therefore never grants tool access; that is the
// gap this closes versus the previous `authorizer: { handler: allow }`.
package decision

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/authz"
)

// Request is the remote_json payload (see infra/auth/oathkeeper/access-rules.json).
type Request struct {
	Subject string `json:"subject"` // Kratos identity id; "" for anonymous
	Tool    string `json:"tool"`    // ops dashboard slug: hubble, grafana, …
	AAL     string `json:"aal"`     // authenticator_assurance_level from whoami
}

// Handler authorizes ops-tier dashboard access against SpiceDB.
type Handler struct {
	checker authz.Checker
	log     *slog.Logger
}

// New returns a Handler backed by the given Checker.
func New(checker authz.Checker, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{checker: checker, log: log}
}

// ServeHTTP handles POST /internal/authorize.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req Request
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	allowed, reason := h.authorize(r.Context(), req)
	// Auth audit event (ADR-0017, Phase 7): who reached which tool, and the
	// outcome — queryable by actor, resource, outcome.
	h.log.LogAttrs(
		r.Context(),
		slog.LevelInfo,
		"ops-authz decision",
		slog.String("subject", req.Subject),
		slog.String("tool", req.Tool),
		slog.Bool("allowed", allowed),
		slog.String("reason", reason),
	)
	if !allowed {
		http.Error(w, reason, http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) authorize(ctx context.Context, req Request) (bool, string) {
	if req.Subject == "" {
		return false, "no session"
	}
	// Operator MFA: the ops tier requires an AAL2 session even though the cookie
	// is shared with the (AAL1) product tier (ADR-0017 default model).
	if req.AAL != "aal2" {
		return false, "aal2 required"
	}
	subject := "user:" + req.Subject
	// Coarse gate: must be an operator at all.
	ok, err := h.checker.Allowed(ctx, subject, "member", "group:operator")
	if err != nil {
		return false, "authz error"
	}
	if !ok {
		return false, "not an operator"
	}
	// Fine gate: must hold this specific dashboard.
	ok, err = h.checker.Allowed(ctx, subject, "view", "dashboard:"+req.Tool)
	if err != nil {
		return false, "authz error"
	}
	if !ok {
		return false, "no grant for " + req.Tool
	}
	return true, "ok"
}
