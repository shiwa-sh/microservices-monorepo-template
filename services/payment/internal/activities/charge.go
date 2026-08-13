// Package activities holds the Charge workflow's activities.
package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/authz"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/id"
	"github.com/tabmadi/microservices-monorepo-template/services/payment/internal/workflows"
)

type Activities struct {
	DB      *pgxpool.Pool
	Granter authz.Granter
}

func New(db *pgxpool.Pool, granter authz.Granter) *Activities {
	return &Activities{DB: db, Granter: granter}
}

// GrantChargeAccessActivity is the OpenFGA leg of the charge write (ADR-0304).
// One tuple: `charge#read` is defined as `read from order`, so whoever may read the
// order may read its charge, and a change to who may read an order cannot leave the
// charge behind. Grant treats an already-present tuple as success, so a retry is
// safe.
func (a *Activities) GrantChargeAccessActivity(ctx context.Context, chargeID, orderID string) error {
	err := a.Granter.Grant(ctx, "order:"+orderID, "order", "charge:"+chargeID)
	if err != nil {
		return fmt.Errorf("grant charge access: %w", err)
	}
	return nil
}

// SettleActivity is the placeholder PSP call. Replace with the real integration.
func (a *Activities) SettleActivity(_ context.Context, _ workflows.ChargeInput) error { return nil }

// RefundActivity is the placeholder PSP refund call. Replace with the real integration.
func (a *Activities) RefundActivity(_ context.Context, _ workflows.RefundInput) error { return nil }

// MarkChargeStatusActivity writes the terminal status of a charge. The workflow
// carries the wire form, and the column holds the bare uuid (ADR-0003), so this is
// where the two meet.
func (a *Activities) MarkChargeStatusActivity(ctx context.Context, chargeID, status string) error {
	parsed, err := id.Parse("charge", chargeID)
	if err != nil {
		return fmt.Errorf("mark charge status: parse charge id: %w", err)
	}
	_, err = a.DB.Exec(ctx, `update charges set status = $2 where id = $1`, parsed.UUID(), status)
	if err != nil {
		return fmt.Errorf("mark charge status: update: %w", err)
	}
	return nil
}
