package authconformance

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/authmw"
)

func TestConformance(t *testing.T) {
	fixtures, err := Fixtures()
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) == 0 {
		t.Fatal("no fixtures loaded")
	}

	for _, f := range fixtures {
		t.Run(f.Name, func(t *testing.T) {
			h := http.Header{}
			for k, v := range f.Headers {
				h.Set(k, v)
			}

			p := authmw.Read(h)

			if p.UserID != f.Want.UserID {
				t.Errorf("UserID = %q, want %q", p.UserID, f.Want.UserID)
			}
			if p.OrgID != f.Want.OrgID {
				t.Errorf("OrgID = %q, want %q", p.OrgID, f.Want.OrgID)
			}
			if !reflect.DeepEqual(p.Roles, f.Want.Roles) {
				t.Errorf("Roles = %#v, want %#v", p.Roles, f.Want.Roles)
			}
			if got := p.Subject(); got != f.Subject {
				t.Errorf("Subject = %q, want %q", got, f.Subject)
			}
			if got := RoleAllowed(p, f.RequireRole); got != f.WantAllowed {
				t.Errorf("RoleAllowed(%q) = %v, want %v", f.RequireRole, got, f.WantAllowed)
			}
		})
	}
}
