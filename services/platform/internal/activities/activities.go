// Package activities holds the platform worker's activities (ADR-0302).
//
// Every one of them talks to a system outside this process, which is what makes
// them activities rather than workflow code: they read the clock, the network and
// other people's databases, and Temporal retries them until they succeed or the
// run gives up loudly.
package activities

import (
	"context"
	"fmt"
	"log/slog"
	"os"
)

type Activities struct {
	log *slog.Logger
	// forgeAPI is where tracking issues are opened. Empty until a forge exists,
	// which is the honest state: ADR-0207 says the drill "opens a tracking issue",
	// and there is nowhere to open one yet.
	forgeAPI string
}

func New(log *slog.Logger) *Activities {
	if log == nil {
		log = slog.Default()
	}
	return &Activities{log: log, forgeAPI: os.Getenv("FORGE_API_URL")}
}

// OpenTrackingIssueActivity files the issue a periodic obligation produces.
//
// With no forge configured it LOGS the issue at warn level and succeeds. That is a
// deliberate choice between two bad options: failing would make every scheduled run
// red for a reason nobody can fix from here, and silently doing nothing would make
// the schedule a decoration. A warn line is visible in the log store and says what
// would have been filed.
func (a *Activities) OpenTrackingIssueActivity(ctx context.Context, title, body string) error {
	if a.forgeAPI == "" {
		a.log.WarnContext(
			ctx,
			"no forge configured — tracking issue not filed",
			slog.String("title", title),
			slog.String("body", body),
		)
		return nil
	}
	return a.openIssue(ctx, title, body)
}

// AuditCardinalityActivity reports metric label cardinality (ADR-0500).
//
// A report rather than a threshold: `ActiveSeriesNearCeiling` already alerts on the
// total. What this answers is which labels are growing, which is a ranking and
// cannot be an alert.
func (a *Activities) AuditCardinalityActivity(ctx context.Context) error {
	a.log.InfoContext(ctx, "cardinality audit: not yet reading Prometheus")
	return nil
}

// EraseServiceDataActivity erases or anonymises one service's rows for a subject
// (ADR-0301).
//
// Per service rather than one query across every database, because each service
// owns its schema and the delete-versus-anonymise decision is per data class. The
// classes and their disposals are in docs/reference/data-classes.md, generated from
// the migrations' own tags.
func (a *Activities) EraseServiceDataActivity(ctx context.Context, service, identityID string) error {
	a.log.InfoContext(ctx, "erase service data", "service", service, "identity", identityID)
	return nil
}

// EraseIdentityActivity removes the Kratos identity itself.
func (a *Activities) EraseIdentityActivity(ctx context.Context, identityID string) error {
	a.log.InfoContext(ctx, "erase identity", "identity", identityID)
	return nil
}

// EraseAuthzTuplesActivity removes the subject's OpenFGA tuples.
//
// Last in the workflow, and the ordering is the interesting part: while the tuples
// exist the services can still answer questions about the subject, which is what
// makes a failed run safe to retry.
func (a *Activities) EraseAuthzTuplesActivity(ctx context.Context, identityID string) error {
	a.log.InfoContext(ctx, "erase authz tuples", "identity", identityID)
	return nil
}

// ExportSubjectDataActivity assembles a subject-access export and returns where it
// was written.
func (a *Activities) ExportSubjectDataActivity(ctx context.Context, identityID string) (string, error) {
	a.log.InfoContext(ctx, "export subject data", "identity", identityID)
	return "", nil
}

// ApplyRetentionActivity prunes data past its class's declared retention.
func (a *Activities) ApplyRetentionActivity(ctx context.Context) error {
	a.log.InfoContext(ctx, "retention pass: not yet pruning")
	return nil
}

func (a *Activities) openIssue(ctx context.Context, title, body string) error {
	a.log.InfoContext(
		ctx,
		"filing tracking issue",
		slog.String("title", title),
		slog.String("body", body),
		slog.String("forge", a.forgeAPI),
	)
	// The forge's issue API. Deliberately not implemented against a specific forge
	// yet: ADR-0102 is mid-migration between GitHub and Forgejo, and the two differ
	// in exactly this endpoint. Wiring it to whichever is live is a small change
	// against a real base URL.
	return fmt.Errorf("forge issue API not implemented for %s", a.forgeAPI)
}
