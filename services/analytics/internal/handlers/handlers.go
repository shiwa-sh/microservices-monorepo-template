// Package handlers implements the ogen-generated analytics.Handler interface
// (ADR-0303, ADR-0700).
//
// The endpoints are internal: the collector's routing connector writes events
// here, and the consent control writes decisions. No browser calls this service
// directly — the browser emits through Faro, which is what keeps the frontend
// ignorant of the topology and lets this store be replaced without a release.
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/apierr"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/id"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/observability"
	analytics "github.com/tabmadi/microservices-monorepo-template/libs/go/sdks/analytics"
	"github.com/tabmadi/microservices-monorepo-template/services/analytics/internal/store"
)

// stateGranted is the one consent state that permits storing an event. Withdrawn
// and refused both stop emission, and the difference between them is a record of
// what the visitor did rather than a difference in what may be stored.
const stateGranted = "granted"

type Handlers struct {
	q   *store.Queries
	log *slog.Logger
}

func New(db *pgxpool.Pool, log *slog.Logger) *Handlers {
	if log == nil {
		log = slog.Default()
	}
	return &Handlers{q: store.New(db), log: log}
}

var _ analytics.Handler = (*Handlers)(nil)

// RecordEvents stores a batch, and drops it whole if the session has no grant.
//
// This is ADR-0700's SECOND enforcement point, and it exists because the first one
// runs on a client the platform does not control. The browser wrapper refuses to
// emit without a recorded grant; this refuses to store what arrives anyway. Either
// alone is a policy, and both together are a control.
//
// A drop is not an error. The caller is the collector, which cannot fix a missing
// grant and must not retry — so the response reports how many were stored and how
// many were not, and a non-zero `dropped` is a client worth looking at.
func (h *Handlers) RecordEvents(ctx context.Context, req *analytics.EventBatch) (*analytics.RecordResult, error) {
	granted, err := h.hasGrant(ctx, req.SessionID)
	if err != nil {
		observability.RecordError(ctx, err, observability.KindDependency)
		h.log.Error("read consent", "err", err)
		return nil, apierr.Internal("failed to read consent")
	}
	if !granted {
		return &analytics.RecordResult{Stored: 0, Dropped: len(req.Events)}, nil
	}

	stored := 0
	for i := range req.Events {
		err = h.insert(ctx, req, &req.Events[i])
		if err != nil {
			observability.RecordError(ctx, err, observability.KindDependency)
			h.log.Error("insert event", "err", err)
			return nil, apierr.Internal("failed to record events")
		}
		stored++
	}
	return &analytics.RecordResult{Stored: stored, Dropped: 0}, nil
}

// SummariseEvents answers the panel's first question: what is happening at all.
//
// It is read-only and carries no authorization of its own, which is deliberate and
// is why the service is east-west (ADR-0700). The panel that calls it performs the
// authoritative check in its render layer, against `analytics_panel:funnels`; this
// service is not reachable from a browser, so there is no second caller to gate.
func (h *Handlers) SummariseEvents(
	ctx context.Context, params analytics.SummariseEventsParams,
) ([]analytics.EventSummary, error) {
	since := pgtype.Timestamptz{Time: params.Since.UTC(), Valid: true}
	rows, err := h.q.SummariseEvents(ctx, since)
	if err != nil {
		observability.RecordError(ctx, err, observability.KindDependency)
		h.log.Error("summarise events", "err", err)
		return nil, apierr.Internal("failed to summarise events")
	}
	out := make([]analytics.EventSummary, 0, len(rows))
	for _, row := range rows {
		summary := analytics.EventSummary{
			Name:        row.Name,
			Occurrences: int(row.Occurrences),
			Sessions:    int(row.Sessions),
		}
		out = append(out, summary)
	}
	return out, nil
}

// RecordConsent writes the decision and returns it as recorded.
//
// Every field is there to make the consent DEMONSTRABLE later (GDPR Art. 7(1)):
// the purpose text's version, because consent is to a stated purpose and a changed
// purpose is a new consent; and the signal's source, because a `gpc` row is a
// refusal nobody was prompted for and that distinction is the difference between
// honouring a prior signal and ignoring one.
func (h *Handlers) RecordConsent(ctx context.Context, req *analytics.ConsentInput) (*analytics.Consent, error) {
	params := store.UpsertConsentParams{
		SessionID:      req.SessionID,
		IdentityID:     optText(req.IdentityID),
		State:          string(req.State),
		PurposeVersion: req.PurposeVersion,
		Source:         string(req.Source),
	}
	row, err := h.q.UpsertConsent(ctx, params)
	if err != nil {
		observability.RecordError(ctx, err, observability.KindDependency)
		h.log.Error("upsert consent", "err", err)
		return nil, apierr.Internal("failed to record consent")
	}
	return &analytics.Consent{
		SessionID:      row.SessionID,
		IdentityID:     fromText(row.IdentityID),
		State:          analytics.ConsentState(row.State),
		PurposeVersion: row.PurposeVersion,
		Source:         analytics.ConsentSource(row.Source),
		DecidedAt:      row.DecidedAt.Time,
	}, nil
}

// GetConsent reads the decision on file. A session with no row is a session that
// has not answered, which is a 404 rather than an invented refusal: the control
// needs to tell "has not been asked" from "said no", because it prompts for one
// and must never prompt for the other.
func (h *Handlers) GetConsent(ctx context.Context, params analytics.GetConsentParams) (*analytics.Consent, error) {
	row, err := h.q.GetConsent(ctx, params.SessionID)
	if err != nil {
		return nil, apierr.NotFound("no consent decision for that session")
	}
	return &analytics.Consent{
		SessionID:      row.SessionID,
		IdentityID:     fromText(row.IdentityID),
		State:          analytics.ConsentState(row.State),
		PurposeVersion: row.PurposeVersion,
		Source:         analytics.ConsentSource(row.Source),
		DecidedAt:      row.DecidedAt.Time,
	}, nil
}

// NewError renders any handler error as RFC 9457 problem details (ADR-0303).
func (h *Handlers) NewError(ctx context.Context, err error) *analytics.ErrorStatusCode {
	e, ok := apierr.As(err)
	if !ok {
		e = apierr.Internal(err.Error())
	}
	e = e.WithTrace(ctx)

	problem := analytics.Problem{Type: e.Type, Title: e.Title, Status: e.Status}
	if e.Detail != "" {
		problem.Detail = analytics.NewOptString(e.Detail)
	}
	if e.TraceID != "" {
		problem.TraceID = analytics.NewOptString(e.TraceID)
	}
	for _, v := range e.Errors {
		problem.Errors = append(problem.Errors, analytics.ProblemErrorsItem{Pointer: v.Pointer, Message: v.Message})
	}
	return &analytics.ErrorStatusCode{StatusCode: e.Status, Response: problem}
}

// hasGrant answers whether this session's events may be stored.
//
// The two failure modes must not be confused, and confusing them is what a first
// version of this did: a session with NO ROW never answered, and the answer is a
// plain no. Any other error is the database being unreachable, and treating that
// as "no consent" would silently discard every event during an outage — a data
// loss that looks exactly like a working consent gate.
func (h *Handlers) hasGrant(ctx context.Context, sessionID string) (bool, error) {
	row, err := h.q.GetConsent(ctx, sessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read consent: %w", err)
	}
	return row.State == stateGranted, nil
}

func (h *Handlers) insert(ctx context.Context, batch *analytics.EventBatch, event *analytics.Event) error {
	key, err := id.New("evt")
	if err != nil {
		return fmt.Errorf("mint event id: %w", err)
	}
	properties := []byte("{}")
	props, ok := event.Properties.Get()
	if ok {
		properties, err = json.Marshal(props)
		if err != nil {
			return fmt.Errorf("marshal event properties: %w", err)
		}
	}
	deviceClass := "unknown"
	class, ok := batch.DeviceClass.Get()
	if ok {
		deviceClass = string(class)
	}
	params := store.InsertEventParams{
		ID:          pgtype.UUID{Bytes: key.UUID(), Valid: true},
		SessionID:   batch.SessionID,
		IdentityID:  optText(batch.IdentityID),
		Name:        event.Name,
		Properties:  properties,
		DeviceClass: deviceClass,
		OccurredAt:  pgtype.Timestamptz{Time: event.OccurredAt.UTC(), Valid: true},
	}
	err = h.q.InsertEvent(ctx, params)
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	return nil
}

func optText(v analytics.OptString) pgtype.Text {
	s, ok := v.Get()
	if !ok {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func fromText(v pgtype.Text) analytics.OptString {
	if !v.Valid {
		return analytics.OptString{}
	}
	return analytics.NewOptString(v.String)
}
