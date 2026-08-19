// Package dbmw wires pgx with OTel tracing + per-query metrics (ADR-0500).
// Services pass the returned tracer to pgxpool.Config.ConnConfig.Tracer.
package dbmw

import (
	"context"
	"fmt"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/observability"
)

// MustOpen opens a pgxpool with the platform-default tracer.
// dsn typically comes from an envFrom-mounted Secret (ADR-0202).
func MustOpen(ctx context.Context, dsn string) *pgxpool.Pool {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		panic(err)
	}
	// PgBouncer transaction-mode compatibility (ADR-0300). `DescribeExec` describes
	// a statement without creating a server-side prepared statement, which is what
	// makes a transaction-mode pooler safe.
	//
	// Still required, and the version number is not the reason to stop. PgBouncer
	// has supported prepared statements in transaction mode since 1.21 and the
	// deployed pooler is well past that — but only when `max_prepared_statements`
	// is greater than zero, and it defaults to zero. The CNPG `Pooler` in
	// infra/helm/platform/postgres sets `max_client_conn` and `default_pool_size`
	// and not that one, so the capability is off however new the binary is.
	//
	// Turning it on is an ADR-0300 amendment rather than a values edit: that rule
	// forbids server-side prepared statements "without compatibility flags", and
	// the flag has a per-connection cache to size.
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeDescribeExec
	cfg.ConnConfig.Tracer = otelpgx.NewTracer()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		panic(err)
	}
	// Bounded startup retry: on a cold cluster dependencies come up in parallel, so
	// Postgres may not accept connections yet. Retry instead of crashing on the first
	// miss — a panic here yields CrashLoopBackOff with a growing (up to 5-minute)
	// delay. Runtime blips need no handling here: pgxpool reconnects on demand and
	// the /readyz gate parks the pod out of rotation meanwhile (see observability).
	err = retry(ctx, pool.Ping)
	if err != nil {
		panic(err)
	}
	// Auto-register the /readyz check for this dependency (ADR-0500).
	observability.RegisterReadinessCheck("postgres", pool.Ping)
	return pool
}

// retry calls fn until it succeeds, a ~60s budget elapses, or ctx is cancelled,
// backing off 500ms→5s between attempts. Returns fn's last error on give-up.
func retry(ctx context.Context, fn func(context.Context) error) error {
	const budget = 60 * time.Second
	deadline := time.Now().Add(budget)
	delay := 500 * time.Millisecond
	for {
		err := fn(ctx)
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("dbmw: startup retry cancelled: %w", ctx.Err())
		case <-time.After(delay):
		}
		if delay < 5*time.Second {
			delay *= 2
		}
	}
}
