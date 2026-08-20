package rollup

import (
	"testing"
	"time"
)

func at(minute int) time.Time {
	return time.Date(2026, 8, 19, 0, minute, 0, 0, time.UTC)
}

// The funnel under test, and its step names as constants: the tests repeat them
// often enough that a typo in one would silently change what is being asserted.
const (
	stepViewed   = "viewed"
	stepAdded    = "added"
	stepCheckout = "checkout"
	stepPlaced   = "placed"

	sessBackwards = "backwards"
)

var steps = []string{stepViewed, stepAdded, stepCheckout, stepPlaced}

func TestCountOrderedTraversal(t *testing.T) {
	t.Parallel()
	rows := []Step{
		{SessionID: "s1", Name: stepViewed, FirstSeen: at(1)},
		{SessionID: "s1", Name: stepAdded, FirstSeen: at(2)},
		{SessionID: "s1", Name: stepCheckout, FirstSeen: at(3)},
		{SessionID: "s1", Name: stepPlaced, FirstSeen: at(4)},
	}
	got := Count(steps, rows)
	want := []int64{1, 1, 1, 1}
	assertCounts(t, got, want)
}

// The case the whole package exists for: a session that reaches a later step
// WITHOUT the earlier ones — a bookmark straight to checkout — must not be counted
// there, or every funnel reports conversion it did not have.
func TestCountRejectsSkippedSteps(t *testing.T) {
	t.Parallel()
	rows := []Step{
		{SessionID: "bookmark", Name: stepCheckout, FirstSeen: at(1)},
		{SessionID: "bookmark", Name: stepPlaced, FirstSeen: at(2)},
	}
	got := Count(steps, rows)
	assertCounts(t, got, []int64{0, 0, 0, 0})
}

// Steps present but in the WRONG ORDER are not a traversal either. This session saw
// every event, so a naive per-name count would report a perfect funnel.
func TestCountRejectsOutOfOrderSteps(t *testing.T) {
	t.Parallel()
	rows := []Step{
		{SessionID: sessBackwards, Name: stepViewed, FirstSeen: at(4)},
		{SessionID: sessBackwards, Name: stepAdded, FirstSeen: at(3)},
		{SessionID: sessBackwards, Name: stepCheckout, FirstSeen: at(2)},
		{SessionID: sessBackwards, Name: stepPlaced, FirstSeen: at(1)},
	}
	got := Count(steps, rows)
	// Only the first step is reached: `added` was first seen BEFORE `viewed`.
	assertCounts(t, got, []int64{1, 0, 0, 0})
}

func TestCountDropOffIsMonotonic(t *testing.T) {
	t.Parallel()
	rows := []Step{
		// Reaches the end.
		{SessionID: "a", Name: stepViewed, FirstSeen: at(1)},
		{SessionID: "a", Name: stepAdded, FirstSeen: at(2)},
		{SessionID: "a", Name: stepCheckout, FirstSeen: at(3)},
		{SessionID: "a", Name: stepPlaced, FirstSeen: at(4)},
		// Leaves after the cart.
		{SessionID: "b", Name: stepViewed, FirstSeen: at(1)},
		{SessionID: "b", Name: stepAdded, FirstSeen: at(2)},
		// Bounces off the product page.
		{SessionID: "c", Name: stepViewed, FirstSeen: at(1)},
	}
	got := Count(steps, rows)
	assertCounts(t, got, []int64{3, 2, 1, 1})

	for i := 1; i < len(got); i++ {
		if got[i] > got[i-1] {
			t.Fatalf("counts must not increase along a funnel: step %d is %d after %d", i, got[i], got[i-1])
		}
	}
}

// Same instant is a traversal, not a rejection: two events in the same millisecond
// are ordered by the funnel's definition, because the clock cannot separate them.
func TestCountAcceptsSimultaneousSteps(t *testing.T) {
	t.Parallel()
	rows := []Step{
		{SessionID: "fast", Name: stepViewed, FirstSeen: at(1)},
		{SessionID: "fast", Name: stepAdded, FirstSeen: at(1)},
	}
	got := Count(steps, rows)
	assertCounts(t, got, []int64{1, 1, 0, 0})
}

// A step nothing reached is a zero, never a missing entry.
func TestCountAlwaysReturnsOnePerStep(t *testing.T) {
	t.Parallel()
	got := Count(steps, nil)
	if len(got) != len(steps) {
		t.Fatalf("got %d counts for %d steps", len(got), len(steps))
	}
	assertCounts(t, got, []int64{0, 0, 0, 0})
}

func TestBucketsAlignToMidnightUTC(t *testing.T) {
	t.Parallel()
	from := time.Date(2026, 8, 19, 3, 30, 0, 0, time.UTC)
	to := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	got := Buckets(from, to)
	if len(got) != 2 {
		t.Fatalf("want 2 buckets, got %d", len(got))
	}
	// The first bucket starts at midnight, NOT at 03:30 — otherwise the same day
	// lands in a different bucket depending on when the pass ran.
	want := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	if !got[0][0].Equal(want) {
		t.Fatalf("first bucket starts at %s, want %s", got[0][0], want)
	}
	if !got[0][1].Equal(got[1][0]) {
		t.Fatalf("buckets are not contiguous: %s then %s", got[0][1], got[1][0])
	}
}

func TestBucketsEmptyWindow(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	if got := Buckets(now, now); len(got) != 0 {
		t.Fatalf("an empty window has no buckets, got %d", len(got))
	}
	if got := Buckets(now.Add(time.Hour), now); len(got) != 0 {
		t.Fatalf("a reversed window has no buckets, got %d", len(got))
	}
}

func assertCounts(t *testing.T, got, want []int64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d counts, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("step %d: got %d, want %d (full: %v)", i, got[i], want[i], got)
		}
	}
}
