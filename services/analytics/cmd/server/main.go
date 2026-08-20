// analytics — the marketing event store and the consent record (ADR-0700).
//
// An ordinary first-party service, which is the decision rather than an accident:
// a platform component pays the full resource, supply-chain, network-policy and
// backup tax, and a service inherits all of it from the template. Putting the
// store here is also what makes an erasure workflow able to reach the rows, which
// it could not do inside a vendored product.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel/metric"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/apierr"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/authmw"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/dbmw"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/httpmw"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/observability"
	analytics "github.com/tabmadi/microservices-monorepo-template/libs/go/sdks/analytics"
	"github.com/tabmadi/microservices-monorepo-template/services/analytics/internal/funnels"
	"github.com/tabmadi/microservices-monorepo-template/services/analytics/internal/handlers"
	"github.com/tabmadi/microservices-monorepo-template/services/analytics/internal/store"
)

const serviceName = "analytics"

func main() {
	err := run()
	if err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdown, err := observability.Init(ctx, observability.Config{ServiceName: serviceName})
	if err != nil {
		return fmt.Errorf("obs init: %w", err)
	}
	defer func() { _ = shutdown(context.Background()) }()

	db := dbmw.MustOpen(ctx, os.Getenv("DATABASE_URL"))
	defer db.Close()

	// The funnel definitions are committed configuration, loaded at START rather
	// than per request: a malformed definition should stop the service coming up,
	// not surface as a rollup that is quietly wrong. Delivered to the pod as a
	// ConfigMap mount; the default is the in-repo path, for running natively.
	defs, err := funnels.Load(envOr("FUNNELS_PATH", defaultFunnelsPath))
	if err != nil {
		return fmt.Errorf("funnels: %w", err)
	}
	slog.Info("funnel definitions loaded", "count", defs.Len())

	// The store's growth, as a gauge (ADR-0700). This is the series the deferral
	// register's ClickHouse row was waiting on: the trigger is "the events table
	// sustaining more than about 10M rows per month", and until something emitted
	// it the row read `uncollected` — measurable in principle, gathered by nothing.
	//
	// Scoped to the CURRENT MONTH, which is both what the trigger asks and what
	// keeps the query cheap: `events` is partitioned by month, so this touches one
	// partition rather than scanning every month ever written.
	registerStorageGauge(store.New(db))

	// No OpenFGA client. The panel that reads this store is authorised in the render
	// layer against the marketing group (ADR-0700); the write path here is reached
	// only by the collector and the consent control, both east-west, and its own
	// gate is the consent record rather than a relationship.
	api, err := analytics.NewServer(
		handlers.New(db, defs, slog.Default()),
		// A request the generated server rejects before a handler runs — a malformed
		// body, a bad parameter, a missing required header — still gets an RFC 9457
		// problem with a 4xx rather than ogen's bare 500 (ADR-0303).
		analytics.WithErrorHandler(apierr.ServeError),
	)
	if err != nil {
		return fmt.Errorf("ogen server: %w", err)
	}

	srv := &http.Server{
		Addr:              httpmw.ListenAddr(),
		Handler:           httpmw.Chain(authmw.Middleware()(api), serviceName),
		ReadHeaderTimeout: 5 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() {
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- fmt.Errorf("http server: %w", err)
		}
	}()
	slog.Info("analytics listening", "addr", srv.Addr)

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = srv.Shutdown(shutCtx)
	if err != nil {
		return fmt.Errorf("analytics: server shutdown: %w", err)
	}
	return nil
}

// defaultFunnelsPath is where the Dockerfile bakes the definitions. Running
// natively overrides it with FUNNELS_PATH, because the repo path is not this one.
const defaultFunnelsPath = "/etc/analytics/funnels.yaml"

// envOr reads an environment variable, falling back to a default.
func envOr(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

// registerStorageGauge publishes the current month's event-row count.
//
// Scoped to the current MONTH, which is both what ADR-0700's trigger asks and what
// keeps the query cheap: `events` is partitioned by month, so this reads one
// partition rather than scanning every month ever written.
func registerStorageGauge(q *store.Queries) {
	observability.ObservableGauge(
		"analytics_events_rows_current_month",
		func(ctx context.Context) (int64, error) {
			now := time.Now().UTC()
			start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
			return q.CountEventsSince(ctx, pgtype.Timestamptz{Time: start, Valid: true})
		},
		metric.WithDescription(
			"Rows in the events table for the current month, for ADR-0700's storage-growth trigger.",
		),
	)
}
