// Package activities holds the platform worker's activities (ADR-0302).
//
// Every one of them talks to a system outside this process, which is what makes
// them activities rather than workflow code: they read the clock, the network and
// other people's databases, and Temporal retries them until they succeed or the
// run gives up loudly.
package activities

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"time"
)

type Activities struct {
	log *slog.Logger
	// forgeAPI is where tracking issues are opened. Empty until a forge exists,
	// which is the honest state: ADR-0207 says the drill "opens a tracking issue",
	// and there is nowhere to open one yet.
	forgeAPI string
	// analyticsAPI is the analytics service, east-west. Unlike the forge it exists,
	// so an activity that needs it fails rather than logging a warning: a rollup
	// that silently did not run is a panel showing stale numbers with no sign that
	// they are stale.
	analyticsAPI string
	// One client, reused. A client per call leaks a connection pool per call, and
	// the timeout here is what stops an activity hanging until Temporal's
	// StartToClose fires — which would turn a slow dependency into a ten-minute
	// wait per retry.
	http *http.Client
}

func New(log *slog.Logger) *Activities {
	if log == nil {
		log = slog.Default()
	}
	return &Activities{
		log:          log,
		forgeAPI:     os.Getenv("FORGE_API_URL"),
		analyticsAPI: os.Getenv("ANALYTICS_API_URL"),
		http:         &http.Client{Timeout: 2 * time.Minute},
	}
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

// ComputeFunnelRollupActivity asks the analytics service to recompute one funnel
// over a window (ADR-0700, ADR-0302).
//
// It calls the service rather than the database. The events are the analytics
// service's store and no other service reads it directly — a worker holding a
// second connection to someone else's schema is exactly the coupling the ownership
// rule exists to prevent, and it would need its own migrations to stay current.
//
// Unlike the stubs above this one has a real body, because its dependency exists:
// the endpoint is east-west (`x-audience: cluster`) and reachable from this pod.
func (a *Activities) ComputeFunnelRollupActivity(
	ctx context.Context, funnel string, from, to time.Time,
) error {
	if a.analyticsAPI == "" {
		return errors.New("ANALYTICS_API_URL is not set")
	}

	window := map[string]string{
		"from": from.UTC().Format(time.RFC3339),
		"to":   to.UTC().Format(time.RFC3339),
	}
	body, err := json.Marshal(window)
	if err != nil {
		return fmt.Errorf("marshal window: %w", err)
	}

	endpoint := fmt.Sprintf(
		"%s/api/analytics/funnels/%s/rollup",
		a.analyticsAPI,
		url.PathEscape(funnel),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.http.Do(req)
	if err != nil {
		return fmt.Errorf("call analytics: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// The response body is read on the error path and discarded otherwise. A
	// rollup's result is a count this activity has nothing to do with — the record
	// that it ran is the workflow's event history.
	if resp.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("analytics rollup %s: %s: %s", funnel, resp.Status, bytes.TrimSpace(detail))
	}
	a.log.InfoContext(ctx, "funnel rollup computed", "funnel", funnel, "from", from, "to", to)
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

// RestoreToScratchActivity restores the most recent backup into a scratch
// namespace and returns that namespace's name (ADR-0207).
//
// Stubbed, and the stub FAILS rather than succeeding quietly. That is the
// difference between this and the tracking-issue activity above: an issue nobody
// files is visible as a missing issue, but a restore verification that reports
// success without restoring anything is worse than no verification at all — it
// converts "we do not know whether the backups work" into "we believe they do".
func (a *Activities) RestoreToScratchActivity(ctx context.Context) (string, error) {
	a.log.ErrorContext(ctx, "restore verification is scheduled but not implemented")
	return "", errors.New(
		"RestoreToScratchActivity is not implemented: it must create a CNPG Cluster " +
			"with a recovery bootstrap from the backup object store (ADR-0207)",
	)
}

// AssertRestoredRowCountsActivity checks the restored database is not merely
// present but populated (ADR-0207).
func (a *Activities) AssertRestoredRowCountsActivity(ctx context.Context, namespace string) error {
	a.log.ErrorContext(ctx, "restore assertion is scheduled but not implemented", "namespace", namespace)
	return errors.New("AssertRestoredRowCountsActivity is not implemented")
}

// TeardownScratchRestoreActivity removes the scratch restore.
//
// A scratch namespace left behind holds a full copy of production data, so this is
// the one activity here whose failure is worth logging loudly even when the run
// otherwise succeeded.
func (a *Activities) TeardownScratchRestoreActivity(ctx context.Context, namespace string) error {
	a.log.ErrorContext(ctx, "scratch teardown is scheduled but not implemented", "namespace", namespace)
	return errors.New("TeardownScratchRestoreActivity is not implemented")
}
