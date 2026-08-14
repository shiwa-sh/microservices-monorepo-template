// Package workflows saga (ADR-0302). The owning service is orders, even though the
// data lives in catalog and payment — process-owner rule.
//
// Steps:
//  1. CreateOrderActivity      — local DB write
//  2. GrantOrderAccessActivity — the matching OpenFGA tuples
//  3. LookupProductActivity    — HTTP call to catalog
//  4. ChargeActivity           — HTTP call to payment (starts that service's
//     Charge workflow; we poll the returned handle)
//  5. MarkOrderStatusActivity  — local DB write
//
// 1 and 2 are the authz dual write (ADR-0304): the row and the tuples that say who
// may read it are separate activities of one workflow, so the pair cannot
// half-apply. They are here rather than in the handler for that reason — an order
// nobody can read is the failure this shape exists to prevent. The handler mints
// the identifier before starting the workflow, so a retry re-inserts the same row.
//
// On failure between 4 and 5 there's no compensation — payment owns its own
// retry logic. If charge fails, we mark the order failed.
package workflows

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/money"
)

// statusFailed is the terminal status of an order the saga could not complete.
const statusFailed = "failed"

type CheckoutInput struct {
	OrderID   string
	ProductID string
	Quantity  int32
	// OwnerID is the buyer's Kratos identity; OrgID is the org they act through
	// (ADR-0304). Both are what the order's OpenFGA tuples are written from.
	OwnerID string
	OrgID   string
	// IdempotencyKey is the client's, carried into the row so a retried checkout
	// finds the order the first one created (ADR-0003).
	IdempotencyKey string
}

type CheckoutResult struct {
	Status   string // "confirmed" | "failed"
	Total    money.Amount
	ChargeID string
}

func Checkout(ctx workflow.Context, in CheckoutInput) (CheckoutResult, error) {
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy:         &temporal.RetryPolicy{InitialInterval: time.Second, MaximumAttempts: 3},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	err := workflow.ExecuteActivity(ctx, "CreateOrderActivity", in).Get(ctx, nil)
	if err != nil {
		return CheckoutResult{Status: statusFailed}, fmt.Errorf("checkout: create order: %w", err)
	}
	// Before any step that can fail the order: a row whose status a later activity
	// marks `failed` still has to be readable by the buyer it belongs to.
	err = workflow.ExecuteActivity(ctx, "GrantOrderAccessActivity", in.OrderID, in.OwnerID, in.OrgID).Get(ctx, nil)
	if err != nil {
		return CheckoutResult{Status: statusFailed}, fmt.Errorf("checkout: grant order access: %w", err)
	}

	var price money.Amount
	err = workflow.ExecuteActivity(ctx, "LookupProductActivity", in.ProductID).Get(ctx, &price)
	if err != nil {
		_ = workflow.ExecuteActivity(ctx, "MarkOrderStatusActivity", in.OrderID, statusFailed).Get(ctx, nil)
		return CheckoutResult{Status: statusFailed}, fmt.Errorf("checkout: lookup product: %w", err)
	}
	// Money multiplication, not a bare integer product: the shared type carries the
	// currency and the scale, so the total cannot silently become a different unit
	// (ADR-0300). Deterministic — it is integer arithmetic on big.Int, no clock and
	// no rounding mode in play for a whole-number factor.
	total := price.Mul(int64(in.Quantity))

	err = workflow.ExecuteActivity(ctx, "SetOrderTotalActivity", in.OrderID, total).Get(ctx, nil)
	if err != nil {
		_ = workflow.ExecuteActivity(ctx, "MarkOrderStatusActivity", in.OrderID, statusFailed).Get(ctx, nil)
		return CheckoutResult{Status: statusFailed, Total: total}, fmt.Errorf("checkout: set order total: %w", err)
	}

	var chargeID string
	err = workflow.ExecuteActivity(ctx, "ChargeActivity", in.OrderID, total).Get(ctx, &chargeID)
	if err != nil {
		_ = workflow.ExecuteActivity(ctx, "MarkOrderStatusActivity", in.OrderID, statusFailed).Get(ctx, nil)
		return CheckoutResult{Status: statusFailed, Total: total}, fmt.Errorf("checkout: charge: %w", err)
	}

	err = workflow.ExecuteActivity(ctx, "MarkOrderStatusActivity", in.OrderID, "confirmed").Get(ctx, nil)
	if err != nil {
		return CheckoutResult{
			Status:   "confirmed",
			Total:    total,
			ChargeID: chargeID,
		}, fmt.Errorf("checkout: mark order confirmed: %w", err)
	}
	return CheckoutResult{Status: "confirmed", Total: total, ChargeID: chargeID}, nil
}
