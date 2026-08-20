package funnels_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tabmadi/microservices-monorepo-template/services/analytics/internal/funnels"
)

func write(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "funnels.yaml")
	err := os.WriteFile(path, []byte(body), 0o600)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func TestLoadsTheCommittedDefinitions(t *testing.T) {
	t.Parallel()
	// The real file, so a change to it that breaks the rules fails here rather
	// than at the first rollup in a deployed environment.
	set, err := funnels.Load(filepath.Join("..", "..", "..", "..", "infra", "analytics", "funnels.yaml"))
	if err != nil {
		t.Fatalf("the committed definitions do not load: %v", err)
	}
	if set.Len() == 0 {
		t.Fatal("the committed definitions are empty")
	}
	fn, ok := set.Get("checkout")
	if !ok {
		t.Fatal("the checkout funnel is missing")
	}
	if len(fn.Steps) < 2 {
		t.Fatalf("checkout has %d steps", len(fn.Steps))
	}
}

// A one-step funnel is not a funnel, it is a count — and it would report a 100%
// conversion rate forever, which is worse than reporting nothing.
func TestRejectsFunnelWithOneStep(t *testing.T) {
	t.Parallel()
	path := write(t, "funnels:\n  - id: thin\n    steps: [only_one]\n")
	_, err := funnels.Load(path)
	if err == nil {
		t.Fatal("a one-step funnel was accepted")
	}
	if !strings.Contains(err.Error(), "at least two") {
		t.Fatalf("unhelpful error: %v", err)
	}
}

// A repeated step counts the same event twice and makes the funnel appear to hold
// at 100% through the duplicate.
func TestRejectsRepeatedStep(t *testing.T) {
	t.Parallel()
	path := write(t, "funnels:\n  - id: dup\n    steps: [a, b, a]\n")
	_, err := funnels.Load(path)
	if err == nil {
		t.Fatal("a repeated step was accepted")
	}
	if !strings.Contains(err.Error(), "repeats step") {
		t.Fatalf("unhelpful error: %v", err)
	}
}

func TestRejectsDuplicateFunnelID(t *testing.T) {
	t.Parallel()
	path := write(t, "funnels:\n  - id: x\n    steps: [a, b]\n  - id: x\n    steps: [c, d]\n")
	_, err := funnels.Load(path)
	if err == nil {
		t.Fatal("a duplicate funnel id was accepted")
	}
}

func TestRejectsEmptyFile(t *testing.T) {
	t.Parallel()
	path := write(t, "funnels: []\n")
	_, err := funnels.Load(path)
	if err == nil {
		t.Fatal("an empty definition set was accepted")
	}
}

func TestRejectsMissingFile(t *testing.T) {
	t.Parallel()
	_, err := funnels.Load(filepath.Join(t.TempDir(), "absent.yaml"))
	if err == nil {
		t.Fatal("a missing file was accepted")
	}
}

func TestGetUnknownFunnel(t *testing.T) {
	t.Parallel()
	path := write(t, "funnels:\n  - id: known\n    steps: [a, b]\n")
	set, err := funnels.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, ok := set.Get("unknown"); ok {
		t.Fatal("an undefined funnel resolved")
	}
}
