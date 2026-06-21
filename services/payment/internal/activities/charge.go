// Package activities holds the Charge workflow's activities.
package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tabmadi/microservices-monorepo-template/services/payment/internal/workflows"
)

type Activities struct{ DB *pgxpool.Pool }

func New(db *pgxpool.Pool) *Activities { return &Activities{DB: db} }

// SettleActivity is the placeholder PSP call. Replace with the real integration.
func (a *Activities) SettleActivity(_ context.Context, _ workflows.ChargeInput) error { return nil }

// MarkChargeStatusActivity writes the terminal status of a charge.
func (a *Activities) MarkChargeStatusActivity(ctx context.Context, chargeID, status string) error {
	_, err := a.DB.Exec(ctx, `update charges set status = $2 where id = $1`, chargeID, status)
	if err != nil {
		return fmt.Errorf("mark charge status: update: %w", err)
	}
	return nil
}
