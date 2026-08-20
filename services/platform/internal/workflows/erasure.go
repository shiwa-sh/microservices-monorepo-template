package workflows

import (
	"fmt"

	"go.temporal.io/sdk/workflow"
)

// EraseSubjectInput identifies the person whose data is being erased.
type EraseSubjectInput struct {
	// IdentityID is the Kratos identity — the `user:<id>` subject every store keys
	// personal data by.
	IdentityID string
}

// EraseSubject runs a right-to-erasure request across every store that holds the
// subject's data (GDPR Art. 17, ADR-0301).
//
// The deadline is one month from the request (Art. 12(3)), and a half-completed
// erasure is worse than an unstarted one — a subject deleted from the application
// database while still present in authz tuples is a subject the system believes in
// and cannot show you. So the process must survive a partial failure and RESUME,
// which is the property that made a workflow the answer rather than a script.
//
// Order is deliberate. The authz tuples go LAST: while they exist, the services can
// still answer questions about the subject, which is what lets a failed run be
// retried safely. Removing them first would leave rows nothing can reach — erased
// in effect, and invisible to the retry that was supposed to erase them.
//
// Each store is its own activity so a failure names the store that failed, and the
// event history is the audit trail that the erasure happened and to what.
func EraseSubject(ctx workflow.Context, in EraseSubjectInput) error {
	ctx = activityOptions(ctx)

	// Application data, per owning service. Each activity is responsible for
	// deciding delete-versus-anonymise from the column's declared class
	// (docs/reference/data-classes.md) — that decision is recorded per category and
	// is never made per request, because a request that could choose could erase an
	// audit obligation.
	for _, service := range []string{"orgs", "orders", "payment", "catalog", "analytics"} {
		err := workflow.ExecuteActivity(ctx, "EraseServiceDataActivity", service, in.IdentityID).
			Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("erase subject: %s: %w", service, err)
		}
	}

	// The identity itself, which is what makes the subject unable to sign in again.
	err := workflow.ExecuteActivity(ctx, "EraseIdentityActivity", in.IdentityID).Get(ctx, nil)
	if err != nil {
		return fmt.Errorf("erase subject: identity: %w", err)
	}

	// The authz tuples last, for the reason above.
	err = workflow.ExecuteActivity(ctx, "EraseAuthzTuplesActivity", in.IdentityID).Get(ctx, nil)
	if err != nil {
		return fmt.Errorf("erase subject: authz tuples: %w", err)
	}
	return nil
}

// ExportSubject answers a subject-access request (GDPR Art. 15 and Art. 20,
// ADR-0301).
//
// It reads the same stores erasure writes, and reads them in the same order for the
// same reason: the authz tuples describe what the subject could reach, and reading
// them first would describe a state the rest of the export does not match.
func ExportSubject(ctx workflow.Context, in EraseSubjectInput) (string, error) {
	ctx = activityOptions(ctx)
	var location string
	err := workflow.ExecuteActivity(ctx, "ExportSubjectDataActivity", in.IdentityID).
		Get(ctx, &location)
	if err != nil {
		return "", fmt.Errorf("export subject: %w", err)
	}
	return location, nil
}

// RetentionPass prunes or anonymises data past its class's declared retention
// (ADR-0301).
//
// Driven by the registry rather than by a list here: `docs/reference/data-classes.md`
// is generated from the `pii:<class>` tags in the migrations plus the periods in
// tools/codegen/retention.yaml, so a new tagged column is covered by this pass
// without anyone remembering to add it.
func RetentionPass(ctx workflow.Context) error {
	ctx = activityOptions(ctx)
	err := workflow.ExecuteActivity(ctx, "ApplyRetentionActivity").Get(ctx, nil)
	if err != nil {
		return fmt.Errorf("retention pass: %w", err)
	}
	return nil
}
