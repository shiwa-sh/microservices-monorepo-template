// Package authmw reads the trusted identity headers injected by the edge
// (Oathkeeper — ADR-0009, ADR-0010): X-User-Id, X-Org-Id, X-Roles. Token
// validation happens once at the edge; services never parse a JWT. authmw turns
// those headers into a typed Principal on the request context. Service-to-service
// calls forward the same headers, so handlers read identity one way regardless
// of origin. Authorisation is a separate concern (libs/go/authz Checker).
package authmw

import (
	"context"
	"net/http"
	"strings"
)

// Canonical identity header names. The edge is the only authority that sets
// them; any client-supplied copies are stripped before the request arrives.
const (
	HeaderUserID = "X-User-Id"
	HeaderOrgID  = "X-Org-Id"
	HeaderRoles  = "X-Roles"
)

type ctxKey int

const principalKey ctxKey = 1

// Principal is the identity of the caller, as forwarded by the edge.
type Principal struct {
	UserID string
	OrgID  string
	Roles  []string
}

// Authenticated reports whether the edge resolved a real user (vs. anonymous).
func (p *Principal) Authenticated() bool { return p != nil && p.UserID != "" }

// HasRole reports whether the principal carries role.
func (p *Principal) HasRole(role string) bool {
	if p == nil {
		return false
	}
	for _, r := range p.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// Subject renders the principal as a SpiceDB subject ("user:<id>") for the
// authz Checker (ADR-0010).
func (p *Principal) Subject() string {
	if !p.Authenticated() {
		return ""
	}
	return "user:" + p.UserID
}

// Read parses the trusted identity headers from h into a Principal. An absent
// user id yields an unauthenticated (guest) principal — the edge admits guests
// and each service decides per route whether a real principal is required.
func Read(h http.Header) *Principal {
	return &Principal{
		UserID: h.Get(HeaderUserID),
		OrgID:  h.Get(HeaderOrgID),
		Roles:  ParseRoles(h.Get(HeaderRoles)),
	}
}

// FromContext returns the principal attached by Middleware, or nil if absent.
func FromContext(ctx context.Context) (*Principal, bool) {
	p, ok := ctx.Value(principalKey).(*Principal)
	return p, ok
}

// Middleware attaches the parsed principal to the request context. It never
// rejects: validation already happened at the edge, and authorisation is the
// handler's job via the authz Checker.
func Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				ctx := context.WithValue(r.Context(), principalKey, Read(r.Header))
				next.ServeHTTP(w, r.WithContext(ctx))
			},
		)
	}
}

// ParseRoles splits the X-Roles header into roles. It tolerates both the
// comma-separated form and Go's bracketed slice rendering ("[admin member]"),
// so it survives however the edge mutator stringifies the roles claim.
func ParseRoles(raw string) []string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "[")
	raw = strings.TrimSuffix(raw, "]")
	if raw == "" {
		return nil
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' })
	roles := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f != "" {
			roles = append(roles, f)
		}
	}
	if len(roles) == 0 {
		return nil
	}
	return roles
}
