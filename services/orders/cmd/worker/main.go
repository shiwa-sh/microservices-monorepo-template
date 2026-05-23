// Temporal worker for orders.Checkout (ADR-0006).
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/dbmw"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/observability"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/temporalmw"
	"github.com/tabmadi/microservices-monorepo-template/services/orders/internal/activities"
	"github.com/tabmadi/microservices-monorepo-template/services/orders/internal/workflows"
)

const serviceName = "orders"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdown, err := observability.Init(ctx, observability.Config{ServiceName: serviceName + "-worker"})
	if err != nil {
		slog.Error("obs init", "err", err)
		os.Exit(1)
	}
	defer shutdown(context.Background())

	db := dbmw.MustOpen(ctx, os.Getenv("DATABASE_URL"))
	defer db.Close()

	tc, err := temporalmw.NewClient(serviceName + "-worker")
	if err != nil {
		slog.Error("temporal", "err", err)
		os.Exit(1)
	}
	defer tc.Close()

	w := temporalmw.NewWorker(tc, serviceName+"-queue")
	w.RegisterWorkflow(workflows.Checkout)

	acts := activities.New(db)
	w.RegisterActivity(acts.LookupProductActivity)
	w.RegisterActivity(acts.ChargeActivity)
	w.RegisterActivity(acts.MarkOrderStatusActivity)

	interrupt := make(chan interface{}, 1)
	go func() { <-ctx.Done(); interrupt <- nil }()
	if err := w.Run(interrupt); err != nil {
		slog.Error("worker", "err", err)
		os.Exit(1)
	}
}
