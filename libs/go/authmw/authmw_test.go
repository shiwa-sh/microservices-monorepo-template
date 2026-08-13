package authmw_test

import (
	"net/http"
	"testing"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/authmw"
)

// The edge admits guests through its anonymous authenticator, which puts a real
// value in X-User-Id — `guest`. A service that reads "the header is set" as "a
// user is signed in" therefore treats every anonymous caller as a principal, and
// answers 403 where it means 401. Both halves are pinned here because the value
// lives in infra/auth/oathkeeper/values.yaml and nothing else connects the two.
func TestGuestIsNotAuthenticated(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		userID string
		want   bool
	}{
		"a real identity":  {"30b478e3-24c9-48b6-911b-3520bfdeeced", true},
		"the edge's guest": {"guest", false},
		"no header at all": {"", false},
	}
	for name, tc := range tests {
		t.Run(
			name,
			func(t *testing.T) {
				t.Parallel()
				h := http.Header{}
				if tc.userID != "" {
					h.Set(authmw.HeaderUserID, tc.userID)
				}
				p := authmw.Read(h)
				if got := p.Authenticated(); got != tc.want {
					t.Errorf("Authenticated() = %v, want %v", got, tc.want)
				}
				if !tc.want && p.Subject() != "" {
					t.Errorf("Subject() = %q, want empty for a caller who is not signed in", p.Subject())
				}
			},
		)
	}
}

// The zero Principal is what a handler holds when the middleware never ran.
func TestNilPrincipal(t *testing.T) {
	t.Parallel()

	var p *authmw.Principal
	if p.Authenticated() {
		t.Error("a nil principal reports as authenticated")
	}
	if p.HasRole("operator") {
		t.Error("a nil principal reports a role")
	}
}
