package workflows

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

type RefundInput struct {
	ChargeID string
	Reason   string
}

type RefundResult struct {
	Status string // "refunded" | "failed"
}

// Refund reverses a settled charge (ADR-0302). Mirrors Charge: a mock PSP
// activity followed by the shared status write. Activities are looked up by name
// so this file has no dependency on the activities package.
func Refund(ctx workflow.Context, in RefundInput) (RefundResult, error) {
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 15 * time.Second,
		RetryPolicy:         &temporal.RetryPolicy{InitialInterval: time.Second, MaximumAttempts: 3},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	err := workflow.ExecuteActivity(ctx, "RefundActivity", in).Get(ctx, nil)
	if err != nil {
		return RefundResult{Status: statusFailed}, fmt.Errorf("refund: psp: %w", err)
	}
	err = workflow.ExecuteActivity(ctx, "MarkChargeStatusActivity", in.ChargeID, "refunded").Get(ctx, nil)
	if err != nil {
		return RefundResult{Status: "refunded"}, fmt.Errorf("refund: mark refunded: %w", err)
	}
	return RefundResult{Status: "refunded"}, nil
}
