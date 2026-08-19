// Package workflows holds the authz Temporal workflows (ADR-0302). One process
// lives here: creating an operator, which is a dual write (ADR-0304) across two
// systems that share no transaction.
package workflows

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// RegisterOperatorInput is the createOperator request body, forwarded by the
// handler that accepted it.
type RegisterOperatorInput struct {
	Email    string
	Password string
}

// RegisterOperator runs the operator dual write: mint the Kratos identity, then
// grant `group:operator#member` in OpenFGA.
//
// It was a handler doing both in sequence with no transaction and no retry
// between them, which is the one exemption `lint:authz-dual-write` shipped with.
// The failure it allowed is quiet and permanent: the identity is created, the
// grant fails, and the result is an operator who can sign in and is refused by
// every ops tool — with nothing anywhere saying why, because the request already
// returned. Here the grant is retried, and a run that exhausts its attempts is a
// failed workflow someone can find.
//
// Order matters and is not arbitrary. The identity is minted first because the
// grant needs its id, which means the reverse failure — identity created, grant
// pending — is the state the retry closes. The opposite order would leave a tuple
// naming a user that does not exist.
func RegisterOperator(ctx workflow.Context, in RegisterOperatorInput) error {
	ctx = workflow.WithActivityOptions(
		ctx,
		workflow.ActivityOptions{
			StartToCloseTimeout: 30 * time.Second,
			RetryPolicy:         &temporal.RetryPolicy{InitialInterval: time.Second, MaximumAttempts: 5},
		},
	)

	var identityID string
	err := workflow.ExecuteActivity(ctx, "CreateOperatorIdentityActivity", in.Email, in.Password).Get(ctx, &identityID)
	if err != nil {
		return fmt.Errorf("register operator: create identity: %w", err)
	}
	err = workflow.ExecuteActivity(ctx, "GrantOperatorRoleActivity", identityID).Get(ctx, nil)
	if err != nil {
		return fmt.Errorf("register operator: grant operator role: %w", err)
	}
	return nil
}
