// Temporal worker for orgs.RegisterUser (ADR-0302): the post-registration
// create-personal-org dual-write (ADR-0304).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/authz"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/dbmw"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/observability"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/temporalmw"
	"github.com/tabmadi/microservices-monorepo-template/services/orgs/internal/activities"
	"github.com/tabmadi/microservices-monorepo-template/services/orgs/internal/workflows"
)

const serviceName = "orgs"

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

	shutdown, err := observability.Init(ctx, observability.Config{ServiceName: serviceName + "-worker"})
	if err != nil {
		return fmt.Errorf("obs init: %w", err)
	}
	defer func() { _ = shutdown(context.Background()) }()

	db := dbmw.MustOpen(ctx, os.Getenv("DATABASE_URL"))
	defer db.Close()

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
	w.RegisterWorkflow(workflows.RegisterUser)

	acts := activities.New(db, granter)
	w.RegisterActivity(acts.CreatePersonalOrgActivity)
	w.RegisterActivity(acts.GrantOrgAdminActivity)
	w.RegisterActivity(acts.SetIdentityOrgActivity)

	interrupt := make(chan any, 1)
	go func() { <-ctx.Done(); interrupt <- nil }()
	err = w.Run(interrupt)
	if err != nil {
		return fmt.Errorf("worker: %w", err)
	}
	return nil
}
