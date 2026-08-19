package observability

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"runtime"
	"strings"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// The error signal (ADR-0503). Errors are OpenTelemetry data grouped by a
// computed fingerprint; no error-tracking product is on the floor, so the
// grouping is a property of the data rather than of a backend.
//
// Two things travel with an error, and they are deliberately different shapes:
//
//   - error.fingerprint — high cardinality, and therefore a SPAN ATTRIBUTE and log
//     structured metadata only. Never a Loki stream label, never a metric label
//     (ADR-0500). It answers "which fault is this".
//   - errors_total{service, kind} — one bounded metric, with `kind` from the closed
//     enumeration below. It answers "how much", and it is what alerts read.
//
// The fingerprint is ours to get right: too coarse merges distinct faults, too fine
// mints a new fault on every release. It is computed here, in one place, so the
// frame-selection rule is tuned once and carries the tests that pin it.

// Kind is the closed enumeration `errors_total` is labelled by.
//
// Closed, and small, because it is a metric label: an open string here is an
// unbounded label, which is the failure mode ADR-0500's cardinality discipline
// exists to prevent. A caller that needs more detail puts it on the span, where
// detail is free.
type Kind string

const (
	// KindInternal is a fault in this service — the default when nothing more
	// specific is true.
	KindInternal Kind = "internal"
	// KindDependency is a failure reaching something this service depends on: a
	// database, a sibling service, an external API.
	KindDependency Kind = "dependency"
	// KindValidation is a request this service refused as malformed. It is counted
	// because a spike of it is a caller that has broken, which is worth seeing.
	KindValidation Kind = "validation"
	// KindConflict is a request refused because the world moved: a version clash,
	// a duplicate, a state that no longer permits the operation.
	KindConflict Kind = "conflict"
	// KindTimeout is a deadline this service ran out of, whether its own or one
	// inherited from its caller.
	KindTimeout Kind = "timeout"
)

// valid is the allow-list, enforced rather than documented. An unknown kind
// becomes `internal` rather than being passed through: a typo must not be able to
// mint a new label value, because a metric label is the one place a typo is
// permanent.
var valid = map[Kind]bool{
	KindInternal:   true,
	KindDependency: true,
	KindValidation: true,
	KindConflict:   true,
	KindTimeout:    true,
}

var (
	errorsOnce  sync.Once
	errorsTotal metric.Int64Counter
)

// initErrorsTotal creates the counter on first use rather than at package init,
// because a meter created before obs.Init has no provider behind it and records
// into nothing.
func initErrorsTotal() {
	errorsTotal = Counter(
		"errors_total",
		metric.WithDescription("Errors by kind (ADR-0503)."),
	)
}

// RecordError counts the error and attaches its fingerprint to the active span.
//
// One call does both, because the two halves are only useful together: a count
// with no fingerprint says something broke and not what, and a fingerprint with no
// count cannot be alerted on. Splitting them into two helpers is how one of them
// stops being called.
func RecordError(ctx context.Context, err error, kind Kind) {
	if err == nil {
		return
	}
	if !valid[kind] {
		kind = KindInternal
	}
	errorsOnce.Do(initErrorsTotal)
	errorsTotal.Add(
		ctx,
		1,
		metric.WithAttributes(attribute.String("kind", string(kind))),
	)

	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}
	// RecordError puts the exception on the span as an event; the fingerprint rides
	// beside it as an attribute so a trace search can group by fault.
	span.RecordError(err)
	span.SetAttributes(
		attribute.String("error.fingerprint", Fingerprint(err)),
		attribute.String("error.kind", string(kind)),
	)
}

// Fingerprint returns a stable identifier for the FAULT rather than for the
// occurrence: the same bug in the same place yields the same value on every
// occurrence and across releases, and two different bugs do not collide.
//
// It hashes the error's type together with the application call frames that
// produced it, and deliberately excludes three things:
//
//   - LINE NUMBERS. Editing a comment above a function shifts every line below it.
//     Including them mints a new fault for a cosmetic edit, which is the failure
//     ADR-0503 names first.
//   - VENDOR FRAMES. The frames inside a driver or a framework are the same for
//     every caller, so including them merges unrelated faults that happen to fail
//     in the same library.
//   - THE VARYING PART OF THE MESSAGE. Ids, hostnames and counts differ per
//     occurrence, so hashing them raw mints a fault per request. The message is
//     used, with those runs normalised away — see normalise.
//
// # Why the message and not only the stack
//
// Go's standard errors carry no stack from where they were created. By the time a
// handler records one, the call stack is the handler's, which is identical for
// every fault it catches — so frames alone cannot tell two faults in one handler
// apart. That was measured rather than assumed: the test that separates two call
// sites failed against a frames-only hash. The wrap chain is what Go actually
// preserves about an error's origin, so it is what identifies the fault, with the
// record-site frames narrowing it further.
func Fingerprint(err error) string {
	if err == nil {
		return ""
	}
	sum := sha256.New()
	_, _ = sum.Write([]byte(errorType(err)))
	_, _ = sum.Write([]byte{0})
	_, _ = sum.Write([]byte(normalise(err.Error())))
	for _, frame := range appFrames() {
		_, _ = sum.Write([]byte{0})
		_, _ = sum.Write([]byte(frame))
	}
	// 16 hex characters, not 64. It is an identifier a human reads off a dashboard
	// and types into a query; the collision probability over the number of distinct
	// faults a platform has is not the binding constraint, legibility is.
	return hex.EncodeToString(sum.Sum(nil))[:16]
}

// normalise removes the parts of a message that vary per occurrence, leaving the
// skeleton the code wrote.
//
// Two classes cover what this platform puts in messages, and both were derived
// from what the identifiers here actually look like rather than guessed:
//
//   - a run of digits becomes <n> — counts, sizes, ports, status codes;
//   - a run of letters and digits together, four characters or longer, becomes <x>
//     — entity ids, uuids, digests, hostnames with an ordinal.
//
// The second rule is why this is not a hex-digit rule. The wire form of an id is
// Crockford base32 (`order_01kztmx9e0fq1r13w5d1aerqw6`), whose alphabet includes
// letters no hex run matches, so a hex rule shreds one id into fragments that
// still differ between occurrences — which is the bug this replaced.
//
// It is deliberately blunt. A normaliser that tried to understand the message
// would be a parser for a language nobody defined; the cost of blunt is merging
// two faults whose messages differ only in a number, which is nearly always the
// right answer anyway.
func normalise(msg string) string {
	var b strings.Builder
	b.Grow(len(msg))
	for i := 0; i < len(msg); {
		if !isAlnum(msg[i]) {
			_ = b.WriteByte(msg[i])
			i++
			continue
		}
		j := i
		digits, letters := 0, 0
		for j < len(msg) && isAlnum(msg[j]) {
			if msg[j] >= '0' && msg[j] <= '9' {
				digits++
			} else {
				letters++
			}
			j++
		}
		run := msg[i:j]
		switch {
		case letters == 0:
			_, _ = b.WriteString("<n>")
		case digits > 0 && len(run) >= 4:
			_, _ = b.WriteString("<x>")
		default:
			_, _ = b.WriteString(run)
		}
		i = j
	}
	return b.String()
}

func isAlnum(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// errorType is the concrete type name of the error, unwrapped to its root.
//
// The root rather than the wrapper: every layer wraps with fmt.Errorf, so the
// outermost type is `*fmt.wrapError` for almost everything and would group the
// whole platform into one fault.
func errorType(err error) string {
	for {
		unwrapped, ok := err.(interface{ Unwrap() error })
		if !ok {
			break
		}
		next := unwrapped.Unwrap()
		if next == nil {
			break
		}
		err = next
	}
	return strings.TrimPrefix(typeName(err), "*")
}

// appFrames walks the caller stack and keeps the first-party frames.
//
// The skip count starts above this package: the caller of RecordError or
// Fingerprint is the first frame that identifies the fault. `runtime` and this
// package's own frames would be identical for every error on the platform.
func appFrames() []string {
	const (
		skip      = 3
		maxFrames = 8
	)
	pcs := make([]uintptr, maxFrames+skip)
	n := runtime.Callers(skip, pcs)
	if n == 0 {
		return nil
	}
	frames := runtime.CallersFrames(pcs[:n])
	var out []string
	for {
		frame, more := frames.Next()
		if frame.Function != "" && isFirstParty(frame.Function) {
			out = append(out, frame.Function)
		}
		if !more || len(out) == maxFrames {
			break
		}
	}
	return out
}

// modulePrefix is this platform's module path. A frame outside it is a vendor
// frame: the same code for every caller, so it identifies a library rather than a
// fault.
const modulePrefix = "github.com/tabmadi/microservices-monorepo-template/"

func isFirstParty(fn string) bool {
	if !strings.HasPrefix(fn, modulePrefix) {
		return false
	}
	// This package's own frames are first-party and are still not the fault: they
	// appear identically in every error the platform records.
	return !strings.HasPrefix(fn, modulePrefix+"libs/go/observability")
}

// typeName renders an error's dynamic type.
//
// The TYPE, not the message: a message carries the varying part — an id, a
// hostname, a count — so hashing it would mint a fault per occurrence. Sentinel
// errors created by errors.New all share one unexported type, which is why the
// call frames below do the discriminating rather than this alone.
func typeName(err error) string {
	return reflect.TypeOf(err).String()
}

// NormaliseForTest exposes the message normaliser to this package's tests.
//
// The rules it applies decide what merges with what, so they are pinned directly
// rather than only through the fingerprints they feed — a change that merged two
// distinct faults would otherwise show up as a passing test and a quieter
// dashboard.
func NormaliseForTest(msg string) string { return normalise(msg) }
