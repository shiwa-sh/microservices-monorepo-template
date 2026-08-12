package observability

import (
	"context"
	"sync"
	"time"
)

// Readiness checks answer "can this pod serve right now?" — the /readyz probe
// (ADR-0500). They are DEEP: each pings a live dependency (Postgres, Temporal).
// The shared dependency wiring registers its own check automatically — dbmw.Open
// registers "postgres", temporalmw.NewClient registers "temporal" — so a service
// gets exactly the checks for the dependencies it actually opens, with no per-
// service code. Liveness (/livez) stays shallow and MUST NOT consult these.
type readyCheck struct {
	name string
	ping func(context.Context) error
}

var (
	readyMu     sync.RWMutex
	readyChecks []readyCheck
)

// RegisterReadinessCheck adds a dependency probe to /readyz. Safe for concurrent
// use; the admin server (started in Init) reads the set on every /readyz request.
func RegisterReadinessCheck(name string, ping func(context.Context) error) {
	readyMu.Lock()
	defer readyMu.Unlock()
	readyChecks = append(readyChecks, readyCheck{name: name, ping: ping})
}

// checkReadiness runs every registered check under a bounded context. It returns
// the name of the first failing dependency and its error, or "" and nil if all
// pass (or none are registered — a pod with no dependencies is always ready).
func checkReadiness(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	readyMu.RLock()
	checks := readyChecks
	readyMu.RUnlock()
	for _, c := range checks {
		err := c.ping(ctx)
		if err != nil {
			return c.name, err
		}
	}
	return "", nil
}
