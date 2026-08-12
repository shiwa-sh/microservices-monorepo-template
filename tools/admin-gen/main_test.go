package main

import (
	"reflect"
	"testing"
)

func TestPathToURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		path string
		want any
	}{
		{"/products", "/products"},
		{"/products/{id}", map[string]any{"_string.concat": []any{
			"/products/", map[string]any{"_payload": "id"},
		}}},
		{"/charges/{id}/refund", map[string]any{"_string.concat": []any{
			"/charges/", map[string]any{"_payload": "id"}, "/refund",
		}}},
	}
	for _, c := range cases {
		got := pathToURL(c.path)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("pathToURL(%q) = %#v, want %#v", c.path, got, c.want)
		}
	}
}

func TestLabels(t *testing.T) {
	t.Parallel()
	if got := title("products"); got != "Products" {
		t.Errorf("title = %q", got)
	}
	if got := headerName("price_cents"); got != "Price cents" {
		t.Errorf("headerName = %q", got)
	}
	if got := humanize("refundCharge"); got != "Refund charge" {
		t.Errorf("humanize = %q", got)
	}
	if got := humanize("createOperator"); got != "Create operator" {
		t.Errorf("humanize = %q", got)
	}
	if got := singular("products"); got != "product" {
		t.Errorf("singular = %q", got)
	}
	if got := singular("orgs"); got != "org" {
		t.Errorf("singular = %q", got)
	}
	if got := singular("identities"); got != "identity" {
		t.Errorf("singular(identities) = %q, want identity", got)
	}
}

func newOp(method, path string, respCodes []string, pathParams ...string) op {
	o := op{method: method, path: path, Responses: map[string]body{}}
	for _, c := range respCodes {
		o.Responses[c] = body{}
	}
	for _, p := range pathParams {
		o.Parameters = append(o.Parameters, param{Name: p, In: "path"})
	}
	return o
}

// TestClassify pins the role assignment, including the 201-vs-202 heuristic: a
// synchronous create (201) becomes a form; an async workflow create (202) is left
// unassigned so the resource stays list-only (ADR-0401).
func TestClassify(t *testing.T) {
	t.Parallel()

	r := &resource{name: "products"}
	r.classify(newOp("get", "/products", nil))
	r.classify(newOp("get", "/products/{id}", nil, "id"))
	r.classify(newOp("post", "/products", []string{"201"}))
	r.classify(newOp("put", "/products/{id}", nil, "id"))
	r.classify(newOp("delete", "/products/{id}", nil, "id"))
	if r.list == nil || r.get == nil || r.create == nil || r.update == nil || r.remove == nil {
		t.Fatalf("full CRUD not classified: %+v", r)
	}

	async := &resource{name: "orders"}
	async.classify(newOp("get", "/orders", nil))
	async.classify(newOp("post", "/orders", []string{"202"}))
	if async.list == nil {
		t.Fatal("list not classified")
	}
	if async.create != nil {
		t.Fatal("async 202 create should be left unassigned (list-only)")
	}
}
