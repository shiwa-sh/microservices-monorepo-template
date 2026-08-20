// The platform worker (ADR-0302, ADR-0301).
//
// It owns the work that belongs to the platform rather than to any one service:
// the periodic obligations several ADRs decide, and the erasure, export and
// retention processes that cross every store.
//
// # Why this deployable exists
//
// A DR drill, a retention pass, a cardinality audit and a quarterly review are each
// decided by an ADR that names no owner. Hanging them off catalog or orders would
// put a service in charge of work outside its domain — what the process-owner rule
// exists to prevent — and a Kubernetes CronJob is forbidden for business-meaningful
// work by ADR-0302. So they get a worker, and it is worker-only: no HTTP surface,
// because it answers no requests.
//
// ADR-0301 rejects "a dedicated erasure service calling each owning service's API".
// This is not that. The objection there was to a service reimplementing retries,
// timers and state against a Temporal that is already Core; everything here gets
// all three from Temporal. What lives in this process is the workflow, not a second
// orchestrator.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/observability"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/temporalmw"
	"github.com/tabmadi/microservices-monorepo-template/services/platform/internal/activities"
	"github.com/tabmadi/microservices-monorepo-template/services/platform/internal/workflows"
)

const serviceName = "platform"

func main() {
	err := run()
	if err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

// No database and no OpenFGA client. This worker owns no schema — every store it
// touches belongs to someone else, and it reaches them through activities.
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdown, err := observability.Init(ctx, observability.Config{ServiceName: serviceName + "-worker"})
	if err != nil {
		return fmt.Errorf("obs init: %w", err)
	}
	defer func() { _ = shutdown(context.Background()) }()

	tc, err := temporalmw.NewClient(serviceName + "-worker")
	if err != nil {
		return fmt.Errorf("temporal: %w", err)
	}
	defer tc.Close()

	w := temporalmw.NewWorker(tc, serviceName+"-queue")
	w.RegisterWorkflow(workflows.DisasterRecoveryDrill)
	w.RegisterWorkflow(workflows.TriggerReview)
	w.RegisterWorkflow(workflows.CardinalityAudit)
	w.RegisterWorkflow(workflows.EraseSubject)
	w.RegisterWorkflow(workflows.ExportSubject)
	w.RegisterWorkflow(workflows.RetentionPass)

	acts := activities.New(slog.Default())
	w.RegisterActivity(acts.OpenTrackingIssueActivity)
	w.RegisterActivity(acts.AuditCardinalityActivity)
	w.RegisterActivity(acts.EraseServiceDataActivity)
	w.RegisterActivity(acts.EraseIdentityActivity)
	w.RegisterActivity(acts.EraseAuthzTuplesActivity)
	w.RegisterActivity(acts.ExportSubjectDataActivity)
	w.RegisterActivity(acts.ApplyRetentionActivity)

	interrupt := make(chan any, 1)
	go func() { <-ctx.Done(); interrupt <- nil }()
	err = w.Run(interrupt)
	if err != nil {
		return fmt.Errorf("worker: %w", err)
	}
	return nil
}
