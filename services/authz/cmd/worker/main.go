// Temporal worker for authz.RegisterOperator (ADR-0302): the operator-creation
// dual write (ADR-0304).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/authz"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/observability"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/temporalmw"
	"github.com/tabmadi/microservices-monorepo-template/services/authz/internal/activities"
	"github.com/tabmadi/microservices-monorepo-template/services/authz/internal/workflows"
)

const serviceName = "authz"

func main() {
	err := run()
	if err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

// No database. authz owns no schema — its state is Kratos identities and OpenFGA
// tuples — so this worker is the one on the platform that opens no pool.
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdown, err := observability.Init(ctx, observability.Config{ServiceName: serviceName + "-worker"})
	if err != nil {
		return fmt.Errorf("obs init: %w", err)
	}
	defer func() { _ = shutdown(context.Background()) }()

	granter, err := authz.NewGranter()
	if err != nil {
		return fmt.Errorf("openfga: %w", err)
	}

	tc, err := temporalmw.NewClient(serviceName + "-worker")
	if err != nil {
		return fmt.Errorf("temporal: %w", err)
	}
	defer tc.Close()

	w := temporalmw.NewWorker(tc, serviceName+"-queue")
	w.RegisterWorkflow(workflows.RegisterOperator)

	acts := activities.New(granter, slog.Default())
	w.RegisterActivity(acts.CreateOperatorIdentityActivity)
	w.RegisterActivity(acts.GrantOperatorRoleActivity)

	interrupt := make(chan any, 1)
	go func() { <-ctx.Done(); interrupt <- nil }()
	err = w.Run(interrupt)
	if err != nil {
		return fmt.Errorf("worker: %w", err)
	}
	return nil
}
