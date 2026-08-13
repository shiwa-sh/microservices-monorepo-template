// Package workflows holds the Charge workflow (ADR-0302). It demonstrates an
// idempotent activity sequence with compensation on failure. Real payment
// processors are mocked by the SettleActivity for template purposes.
package workflows

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// statusFailed is the terminal status of a charge the workflow could not complete.
const statusFailed = "failed"

type ChargeInput struct {
	ChargeID    string
	OrderID     string
	AmountCents int32
}

type ChargeResult struct {
	Status string // "settled" | "failed"
}

// Charge is the workflow body. Activities are looked up by name string so this
// file can compile without depending on the activities package.
func Charge(ctx workflow.Context, in ChargeInput) (ChargeResult, error) {
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 15 * time.Second,
		RetryPolicy:         &temporal.RetryPolicy{InitialInterval: time.Second, MaximumAttempts: 3},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// The tuple that makes the charge readable comes first, before anything that can
	// fail it: `charge#read` resolves through the order, so this is what lets the
	// buyer see a charge that settled and a charge that did not.
	//
	// The row itself is inserted by the handler rather than by an activity here,
	// unlike orders (ADR-0304 wants both writes in the workflow). Payment's
	// deduplication key is a client-supplied header enforced by a unique constraint,
	// so the insert has to happen synchronously for the handler to know which charge
	// id it is answering with; orders keys its insert on an id it mints itself and
	// therefore has no such constraint.
	err := workflow.ExecuteActivity(ctx, "GrantChargeAccessActivity", in.ChargeID, in.OrderID).Get(ctx, nil)
	if err != nil {
		return ChargeResult{Status: statusFailed}, fmt.Errorf("charge: grant charge access: %w", err)
	}

	err = workflow.ExecuteActivity(ctx, "SettleActivity", in).Get(ctx, nil)
	if err != nil {
		_ = workflow.ExecuteActivity(ctx, "MarkChargeStatusActivity", in.ChargeID, statusFailed).Get(ctx, nil)
		return ChargeResult{Status: statusFailed}, fmt.Errorf("charge: settle: %w", err)
	}
	err = workflow.ExecuteActivity(ctx, "MarkChargeStatusActivity", in.ChargeID, "settled").Get(ctx, nil)
	if err != nil {
		return ChargeResult{Status: "settled"}, fmt.Errorf("charge: mark settled: %w", err)
	}
	return ChargeResult{Status: "settled"}, nil
}
