// Package dbmw wires pgx with OTel tracing + per-query metrics (ADR-0011).
// Services pass the returned tracer to pgxpool.Config.ConnConfig.Tracer.
package dbmw

import (
	"context"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MustOpen opens a pgxpool with the platform-default tracer.
// dsn typically comes from an envFrom-mounted Secret (ADR-0005).
func MustOpen(ctx context.Context, dsn string) *pgxpool.Pool {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		panic(err)
	}
	// PgBouncer transaction-mode compatibility (ADR-0007).
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeDescribeExec
	cfg.ConnConfig.Tracer = otelpgx.NewTracer()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		panic(err)
	}
	if err := pool.Ping(ctx); err != nil {
		panic(err)
	}
	return pool
}
