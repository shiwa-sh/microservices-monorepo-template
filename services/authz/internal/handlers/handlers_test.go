package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	authzsdk "github.com/tabmadi/microservices-monorepo-template/libs/go/sdks/authz"
	"github.com/tabmadi/microservices-monorepo-template/services/authz/internal/kratos"
)

const (
	subjectAlice = "alice"
	subjectBob   = "bob"
	toolGrafana  = "grafana"
	toolHubble   = "hubble"

	traitEmail    = "email"
	traitName     = "name"
	traitOperator = "operator"

	opEmail    = "op@example.com"
	stateValue = "active"
)

// fakeChecker returns canned answers keyed by "permission resource". It also
// records whether it was called, so tests can assert the coarse gate never
// touches OpenFGA.
type fakeChecker struct {
	answers map[string]bool
	called  bool
}

func (f *fakeChecker) Allowed(_ context.Context, _, permission, resource string) (bool, error) {
	f.called = true
	return f.answers[permission+" "+resource], nil
}

func req(subject, tool, aal, operator string) *authzsdk.AuthorizeRequest {
	return &authzsdk.AuthorizeRequest{Subject: subject, Tool: tool, Aal: aal, Operator: operator}
}

// decideStatus runs Authorize and reduces the response variant to its HTTP status:
// 200 for allow (AuthorizeOK), 403 for deny (Problem).
func decideStatus(t *testing.T, h *Handlers, r *authzsdk.AuthorizeRequest) int {
	t.Helper()
	res, err := h.Authorize(context.Background(), r)
	if err != nil {
		t.Fatalf("Authorize returned error: %v", err)
	}
	switch res.(type) {
	case *authzsdk.AuthorizeOK:
		return 200
	case *authzsdk.Problem:
		return 403
	default:
		t.Fatalf("unexpected response type %T", res)
		return 0
	}
}

// The coarse gate is a claim check: operator trait + AAL2, and — critically — no
// OpenFGA call, so a product-authz outage cannot lock operators out (ADR-0306).
func TestCoarseClaimGate(t *testing.T) {
	t.Parallel()
	checker := &fakeChecker{}
	h := New(checker, nil, false, nil, nil) // coarse-only (fine layer off)

	cases := []struct {
		name string
		req  *authzsdk.AuthorizeRequest
		want int
	}{
		{"operator + aal2", req(subjectAlice, toolGrafana, aalLevel2, operatorTraitTrue), 200},
		{"operator + aal2, any tool", req(subjectAlice, toolHubble, aalLevel2, operatorTraitTrue), 200},
		{"operator but aal1", req(subjectAlice, toolGrafana, "aal1", operatorTraitTrue), 403},
		{"aal2 but not operator", req(subjectBob, toolGrafana, aalLevel2, "false"), 403},
		{"operator trait empty", req(subjectBob, toolGrafana, aalLevel2, ""), 403},
		{"anonymous", req("", toolGrafana, aalLevel2, operatorTraitTrue), 403},
	}
	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				t.Parallel()
				got := decideStatus(t, h, tc.req)
				if got != tc.want {
					t.Fatalf("status = %d, want %d", got, tc.want)
				}
			},
		)
	}

	if checker.called {
		t.Fatal("coarse gate called OpenFGA; it must not (break-glass independence, ADR-0306)")
	}
}

// The optional fine gate adds a per-tool OpenFGA check on top of the coarse gate.
func TestFineGrainedGate(t *testing.T) {
	t.Parallel()
	// alice holds o11y but not map.
	h := New(&fakeChecker{answers: map[string]bool{"view dashboard:grafana": true}}, nil, true, nil, nil)

	granted := decideStatus(t, h, req(subjectAlice, toolGrafana, aalLevel2, operatorTraitTrue))
	if granted != 200 {
		t.Fatalf("granted tool status = %d, want 200", granted)
	}
	ungranted := decideStatus(t, h, req(subjectAlice, toolHubble, aalLevel2, operatorTraitTrue))
	if ungranted != 403 {
		t.Fatalf("ungranted tool status = %d, want 403", ungranted)
	}
	// The coarse gate still applies first: a non-operator is denied before the fine check.
	nonOperator := decideStatus(t, h, req(subjectBob, toolGrafana, aalLevel2, "false"))
	if nonOperator != 403 {
		t.Fatalf("non-operator status = %d, want 403", nonOperator)
	}
}

// fakeKratos stands in for the Kratos admin identity API so the identity handlers can
// be tested without a live Kratos: list returns a fixed page, get returns one full
// identity (schema_id/state included), and put echoes back the body it received after
// recording it for the caller to assert on.
func fakeKratos(t *testing.T, gotPut *map[string]any) *httptest.Server {
	t.Helper()
	full := map[string]any{
		"id": "id-1", "schema_id": kratos.SchemaUserV1, "state": stateValue,
		"traits": map[string]any{traitEmail: opEmail, traitName: "Op One", traitOperator: true},
	}
	writeJSON := func(w http.ResponseWriter, v any) {
		err := json.NewEncoder(w).Encode(v)
		if err != nil {
			t.Errorf("encode response: %v", err)
		}
	}
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/admin/identities":
			user := map[string]any{
				"id": "id-2", "schema_id": kratos.SchemaUserV1, "state": stateValue,
				"traits": map[string]any{traitEmail: "user@example.com", traitName: "User Two"},
			}
			writeJSON(w, []any{full, user})
		case r.Method == http.MethodGet && r.URL.Path == "/admin/identities/id-1":
			writeJSON(w, full)
		case r.Method == http.MethodPut && r.URL.Path == "/admin/identities/id-1":
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, gotPut)
			_, _ = w.Write(body) // echo the updated identity back
		default:
			t.Errorf("unexpected kratos call %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}
	return httptest.NewServer(http.HandlerFunc(handler))
}

func newKratosHandlers(url string) *Handlers {
	h := New(nil, nil, false, nil, nil)
	h.identities = kratos.NewAt(url, nil)
	return h
}

// ListIdentities flattens each Kratos identity's traits, defaulting a missing
// operator trait to false.
func TestListIdentities(t *testing.T) {
	t.Parallel()
	srv := fakeKratos(t, &map[string]any{})
	defer srv.Close()
	h := newKratosHandlers(srv.URL)

	ids, err := h.ListIdentities(context.Background(), authzsdk.ListIdentitiesParams{})
	if err != nil {
		t.Fatalf("ListIdentities: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("len = %d, want 2", len(ids))
	}
	if ids[0].Email != opEmail || !ids[0].Operator.Value {
		t.Errorf("identity[0] = %+v, want operator op@example.com", ids[0])
	}
	if ids[1].Operator.Value {
		t.Errorf("identity[1] operator = true, want false (trait absent)")
	}
}

// UpdateIdentity overlays only the changed traits onto the current identity, so a
// Kratos PUT (which replaces the whole record) keeps schema_id, state, and email.
func TestUpdateIdentityPreservesRecord(t *testing.T) {
	t.Parallel()
	got := map[string]any{}
	srv := fakeKratos(t, &got)
	defer srv.Close()
	h := newKratosHandlers(srv.URL)

	body := &authzsdk.IdentityUpdate{Name: authzsdk.NewOptString("Renamed"), Operator: authzsdk.NewOptBool(false)}
	updated, err := h.UpdateIdentity(context.Background(), body, authzsdk.UpdateIdentityParams{ID: "id-1"})
	if err != nil {
		t.Fatalf("UpdateIdentity: %v", err)
	}
	if got["schema_id"] != kratos.SchemaUserV1 || got["state"] != stateValue {
		t.Errorf("PUT dropped schema_id/state: %+v", got)
	}
	traits, _ := got["traits"].(map[string]any)
	if traits[traitEmail] != opEmail {
		t.Errorf("PUT changed email to %v, want preserved op@example.com", traits[traitEmail])
	}
	if traits[traitName] != "Renamed" || traits[traitOperator] != false {
		t.Errorf("PUT traits = %+v, want name=Renamed operator=false", traits)
	}
	if updated.Name.Value != "Renamed" || updated.Operator.Value {
		t.Errorf("returned identity = %+v, want name=Renamed operator=false", updated)
	}
}
