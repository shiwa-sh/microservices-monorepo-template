// Package rollup turns raw event rows into ordered funnel counts (ADR-0700).
//
// The ordering is the whole point and it is the part that is easy to get wrong. A
// session that visits the checkout page from a bookmark has not traversed the
// funnel, and counting it would make every funnel look flat — so a step counts a
// session only when every EARLIER step happened first.
//
// This is a pure function over first-seen timestamps, deliberately separated from
// both the query and the handler: it is the only part with a rule worth testing,
// and it is testable without a database.
package rollup

import "time"

// Step is one session's first occurrence of one event name.
type Step struct {
	SessionID string
	Name      string
	FirstSeen time.Time
}

// Count returns, for each step in `steps`, the number of sessions that reached it
// IN ORDER — having reached every earlier step at or before it.
//
// The result is always the same length as `steps`, so a step nothing reached is a
// zero rather than a missing row. A funnel with a gap in it is a finding; a funnel
// with a hole in its output is a rendering bug waiting to happen.
//
// Counts are monotonically non-increasing by construction: a session counted at
// step n was counted at every step before it.
func Count(steps []string, rows []Step) []int64 {
	counts := make([]int64, len(steps))
	if len(steps) == 0 {
		return counts
	}

	// Group first-seen times by session. A session's rows arrive in no particular
	// order, and a step's position in `steps` is what orders them.
	bySession := make(map[string]map[string]time.Time)
	for _, r := range rows {
		seen, ok := bySession[r.SessionID]
		if !ok {
			seen = make(map[string]time.Time, len(steps))
			bySession[r.SessionID] = seen
		}
		// The query returns one row per session and name, but a caller could pass
		// more; keeping the earliest is what "first seen" means either way.
		prev, exists := seen[r.Name]
		if !exists || r.FirstSeen.Before(prev) {
			seen[r.Name] = r.FirstSeen
		}
	}

	for _, seen := range bySession {
		// `prev` is the time the previous step was reached. A step counts only if
		// it was first seen at or after it — `!Before` rather than `After`, because
		// two events in the same millisecond are ordered by the funnel's definition
		// rather than by a clock that cannot separate them.
		var prev time.Time
		for i, name := range steps {
			at, ok := seen[name]
			if !ok || at.Before(prev) {
				// The session left the funnel here. Every later step is unreached
				// BY THIS PATH even if the event exists, which is the difference
				// between a funnel and a set of independent counts.
				break
			}
			counts[i]++
			prev = at
		}
	}
	return counts
}

// Buckets splits a half-open window into day-aligned buckets.
//
// Aligned to midnight UTC rather than to the window's start, so the same day is
// the same bucket on every run — a pass over a window starting at 03:00 must not
// produce buckets offset by three hours from yesterday's.
//
// The last bucket may extend past `to`; the query that fills it is bounded by the
// bucket, so a partially elapsed day is counted as far as it has gone and replaced
// on the next pass.
func Buckets(from, to time.Time) [][2]time.Time {
	var out [][2]time.Time
	if !from.Before(to) {
		return out
	}
	start := from.UTC().Truncate(24 * time.Hour)
	for start.Before(to) {
		end := start.Add(24 * time.Hour)
		out = append(out, [2]time.Time{start, end})
		start = end
	}
	return out
}
