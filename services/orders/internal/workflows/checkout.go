// Package workflows saga (ADR-0006). The owning service is orders, even though the
// data lives in catalog and payment — process-owner rule.
//
// Steps:
//  1. LookupProductActivity — HTTP call to catalog
//  2. ChargeActivity        — HTTP call to payment (starts that service's
//     Charge workflow; we poll the returned handle)
//  3. ConfirmOrderActivity  — local DB write
//
// On failure between 2 and 3 there's no compensation — payment owns its own
// retry logic. If charge fails, we mark the order failed.
package workflows

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

type CheckoutInput struct {
	OrderID   string
	ProductID string
	Quantity  int32
}

type CheckoutResult struct {
	Status     string // "confirmed" | "failed"
	TotalCents int32
	ChargeID   string
}

func Checkout(ctx workflow.Context, in CheckoutInput) (CheckoutResult, error) {
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy:         &temporal.RetryPolicy{InitialInterval: time.Second, MaximumAttempts: 3},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	var price int32
	err := workflow.ExecuteActivity(ctx, "LookupProductActivity", in.ProductID).Get(ctx, &price)
	if err != nil {
		_ = workflow.ExecuteActivity(ctx, "MarkOrderStatusActivity", in.OrderID, "failed").Get(ctx, nil)
		return CheckoutResult{Status: "failed"}, fmt.Errorf("checkout: lookup product: %w", err)
	}
	total := price * in.Quantity

	var chargeID string
	err = workflow.ExecuteActivity(ctx, "ChargeActivity", in.OrderID, total).Get(ctx, &chargeID)
	if err != nil {
		_ = workflow.ExecuteActivity(ctx, "MarkOrderStatusActivity", in.OrderID, "failed").Get(ctx, nil)
		return CheckoutResult{Status: "failed", TotalCents: total}, fmt.Errorf("checkout: charge: %w", err)
	}

	err = workflow.ExecuteActivity(ctx, "MarkOrderStatusActivity", in.OrderID, "confirmed").Get(ctx, nil)
	if err != nil {
		return CheckoutResult{
			Status:     "confirmed",
			TotalCents: total,
			ChargeID:   chargeID,
		}, fmt.Errorf("checkout: mark order confirmed: %w", err)
	}
	return CheckoutResult{Status: "confirmed", TotalCents: total, ChargeID: chargeID}, nil
}
