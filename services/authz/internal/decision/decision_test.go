package decision

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeChecker returns canned answers keyed by "permission resource".
type fakeChecker map[string]bool

func (f fakeChecker) Allowed(_ context.Context, _, permission, resource string) (bool, error) {
	return f[permission+" "+resource], nil
}

func post(t *testing.T, h *Handler, body string) int {
	t.Helper()
	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"/internal/authorize",
		strings.NewReader(body),
	)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	_, _ = io.Copy(io.Discard, rec.Body)
	return rec.Code
}

func TestAuthorize(t *testing.T) {
	t.Parallel()
	// alice: operator + grafana grant, AAL2. bob: not an operator.
	checker := fakeChecker{
		"member group:operator":  true,
		"view dashboard:grafana": true,
	}
	h := New(checker, nil)

	cases := []struct {
		name string
		body string
		want int
	}{
		{"operator with grant", `{"subject":"alice","tool":"grafana","aal":"aal2"}`, http.StatusOK},
		{"operator without this grant", `{"subject":"alice","tool":"hubble","aal":"aal2"}`, http.StatusForbidden},
		{"operator but only aal1", `{"subject":"alice","tool":"grafana","aal":"aal1"}`, http.StatusForbidden},
		{"anonymous", `{"subject":"","tool":"grafana","aal":"aal2"}`, http.StatusForbidden},
		{"malformed json", `not json`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				t.Parallel()
				got := post(t, h, tc.body)
				if got != tc.want {
					t.Fatalf("status = %d, want %d", got, tc.want)
				}
			},
		)
	}
}

func TestNonOperatorDenied(t *testing.T) {
	t.Parallel()
	h := New(fakeChecker{"view dashboard:grafana": true}, nil) // grant but not operator
	got := post(t, h, `{"subject":"bob","tool":"grafana","aal":"aal2"}`)
	if got != http.StatusForbidden {
		t.Fatalf("non-operator status = %d, want 403", got)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	t.Parallel()
	h := New(fakeChecker{}, nil)
	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"/internal/authorize",
		nil,
	)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status = %d, want 405", rec.Code)
	}
}
