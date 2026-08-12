package workflows

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

type CancelInput struct {
	OrderID string
}

type CancelResult struct {
	Status string // "cancelled"
}

// CancelOrder marks an order cancelled (ADR-0302). Reuses MarkOrderStatusActivity;
// activities are looked up by name so this file has no dependency on the activities
// package. Compensation (releasing the charge) is left to the payment refund path.
func CancelOrder(ctx workflow.Context, in CancelInput) (CancelResult, error) {
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 15 * time.Second,
		RetryPolicy:         &temporal.RetryPolicy{InitialInterval: time.Second, MaximumAttempts: 3},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	err := workflow.ExecuteActivity(ctx, "MarkOrderStatusActivity", in.OrderID, "cancelled").Get(ctx, nil)
	if err != nil {
		return CancelResult{}, fmt.Errorf("cancel: mark cancelled: %w", err)
	}
	return CancelResult{Status: "cancelled"}, nil
}
