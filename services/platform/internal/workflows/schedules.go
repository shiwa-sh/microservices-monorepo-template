// Package workflows holds the platform's periodic obligations (ADR-0302).
//
// These belong to no single service, and that is why this deployable exists. A DR
// drill, a retention pass, a cardinality audit and a quarterly review are each
// decided by an ADR that names no owner, and hanging them off catalog or orders
// would put a service in charge of work outside its domain — the thing the
// process-owner rule exists to prevent.
//
// ADR-0301 rejects "a dedicated erasure service calling each owning service's API",
// and this is not that. The objection there was to a service reimplementing
// retries, timers and state; everything here gets all three from Temporal, which
// is the option that ADR chose. What lives here is the WORKFLOW, not a second
// orchestrator.
package workflows

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// activityOptions is shared by every workflow here. Periodic work is not latency
// sensitive and its activities talk to systems that are occasionally slow, so the
// timeout is generous and the retry count is high: a run that fails because a
// dependency was restarting has told us nothing.
func activityOptions(ctx workflow.Context) workflow.Context {
	return workflow.WithActivityOptions(
		ctx,
		workflow.ActivityOptions{
			StartToCloseTimeout: 10 * time.Minute,
			RetryPolicy: &temporal.RetryPolicy{
				InitialInterval: 30 * time.Second,
				MaximumAttempts: 10,
			},
		},
	)
}

// DisasterRecoveryDrill opens the quarterly tracking issue ADR-0207 requires.
//
// It opens an issue rather than performing a restore, and that is the ADR's own
// wording: the drill is a rehearsal people carry out, and what a schedule can
// guarantee is that nobody forgets it is due. A workflow that claimed to have
// tested a restore without a human reading the result would be worse than none.
func DisasterRecoveryDrill(ctx workflow.Context) error {
	ctx = activityOptions(ctx)
	title := "Quarterly restore rehearsal (ADR-0200, ADR-0207)"
	body := "The backup restore is rehearsed quarterly, and the rehearsal is what " +
		"makes the recovery objectives measurements rather than intentions. " +
		"Restore the most recent base backup into a scratch namespace, confirm it " +
		"is byte-exact, and record the elapsed time against the stated RTO."
	err := workflow.ExecuteActivity(ctx, "OpenTrackingIssueActivity", title, body).Get(ctx, nil)
	if err != nil {
		return fmt.Errorf("dr drill: open issue: %w", err)
	}
	return nil
}

// TriggerReview opens the quarterly deferral-and-verification review (ADR-0000).
//
// It covers the deferral register's `query` rows — the ones whose data exists and
// which nothing is asking — and the ASVS table's cadence. Both are obligations
// that decay silently: nothing breaks when a review is skipped, which is exactly
// why the reminder has to be mechanical.
func TriggerReview(ctx workflow.Context) error {
	ctx = activityOptions(ctx)
	title := "Quarterly deferral and verification review (ADR-0000, ADR-0203)"
	body := "Walk the `query` rows in docs/reference/deferral-register.md and the " +
		"cadence rows in docs/reference/asvs-verification.md. A query row walked " +
		"without its answer being recorded has not been walked."
	err := workflow.ExecuteActivity(ctx, "OpenTrackingIssueActivity", title, body).Get(ctx, nil)
	if err != nil {
		return fmt.Errorf("trigger review: open issue: %w", err)
	}
	return nil
}

// CardinalityAudit reports the metric label cardinality ADR-0500 asks to be
// audited, and opens an issue when it is close to the ceiling.
//
// The audit is a query rather than a rule because the interesting answer is a
// ranking — which labels are growing — and an alert can only say that a total
// crossed a line. `ActiveSeriesNearCeiling` already covers the line.
func CardinalityAudit(ctx workflow.Context) error {
	ctx = activityOptions(ctx)
	err := workflow.ExecuteActivity(ctx, "AuditCardinalityActivity").Get(ctx, nil)
	if err != nil {
		return fmt.Errorf("cardinality audit: %w", err)
	}
	return nil
}
