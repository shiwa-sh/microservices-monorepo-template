// Charge workflow (ADR-0006). Demonstrates an idempotent activity sequence
// with compensation on failure. Real payment processors are mocked by the
// SettleActivity for template purposes.
package workflows

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

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

	if err := workflow.ExecuteActivity(ctx, "SettleActivity", in).Get(ctx, nil); err != nil {
		_ = workflow.ExecuteActivity(ctx, "MarkChargeStatusActivity", in.ChargeID, "failed").Get(ctx, nil)
		return ChargeResult{Status: "failed"}, err
	}
	if err := workflow.ExecuteActivity(ctx, "MarkChargeStatusActivity", in.ChargeID, "settled").Get(ctx, nil); err != nil {
		return ChargeResult{Status: "settled"}, err
	}
	return ChargeResult{Status: "settled"}, nil
}
