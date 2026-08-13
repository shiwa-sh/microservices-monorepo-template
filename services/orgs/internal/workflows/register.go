// Package workflows holds the orgs Temporal workflows (ADR-0302). The orgs
// service owns the post-registration "create personal org" process even though
// the OpenFGA write targets the authz store — process-owner rule.
package workflows

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// RegisterInput is the Kratos post-registration payload, forwarded by the
// /identity-created webhook handler.
type RegisterInput struct {
	IdentityID string
}

// RegisterUser runs the dual-write (ADR-0304) for a new identity: create the
// personal org + admin membership in the orgs DB, then write the matching
// OpenFGA owner tuple. Both are activities so the pair cannot half-apply — a
// failed OpenFGA write is retried, and an exhausted workflow surfaces rather
// than silently leaving the app DB and the authz store divergent.
//
// The third activity records the org on the Kratos identity, which is what makes
// it visible to everything downstream: the edge builds X-Org-Id out of that field,
// so without it the identity reaches every service belonging to no org, and an
// order — which must belong to one — cannot be placed at all.
func RegisterUser(ctx workflow.Context, in RegisterInput) error {
	ctx = workflow.WithActivityOptions(
		ctx,
		workflow.ActivityOptions{
			StartToCloseTimeout: 30 * time.Second,
			RetryPolicy:         &temporal.RetryPolicy{InitialInterval: time.Second, MaximumAttempts: 5},
		},
	)

	var orgID string
	err := workflow.ExecuteActivity(ctx, "CreatePersonalOrgActivity", in.IdentityID).Get(ctx, &orgID)
	if err != nil {
		return fmt.Errorf("register user: create personal org: %w", err)
	}
	err = workflow.ExecuteActivity(ctx, "GrantOrgAdminActivity", orgID, in.IdentityID).Get(ctx, nil)
	if err != nil {
		return fmt.Errorf("register user: grant org admin: %w", err)
	}
	err = workflow.ExecuteActivity(ctx, "SetIdentityOrgActivity", in.IdentityID, orgID).Get(ctx, nil)
	if err != nil {
		return fmt.Errorf("register user: set identity org: %w", err)
	}
	return nil
}
