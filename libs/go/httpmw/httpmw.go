// Package httpmw provides default HTTP middleware (ADR-0500): a tracing span,
// RED metrics, and a structured access log. Services compose them via Chain.
package httpmw

import (
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/buildinfo"
)

// Chain wraps h with tracing, RED metrics, and access logging. RED is owned by
// otelhttp: its stable http.server.request.duration histogram (correct second-scale
// buckets) plus request counters are the RED signal the dashboards read. This
// package used to emit hand-rolled http.server.requests / http.server.duration_seconds
// instruments too, but they duplicated otelhttp and — worse — recorded seconds into
// OTel's default millisecond-scale bucket boundaries, so every latency percentile
// collapsed into the first bucket and read as nonsense. They were removed; the
// dashboards and the availability alert now key off the stable otelhttp metric.
//
// otelhttp must wrap access, not the other way round: it creates the server span and
// injects it into the request context it passes inward. The access log reads that
// context, so keeping it INSIDE the span is what stamps trace_id/span_id onto every
// access log line (→ Loki structured metadata), giving logs↔traces correlation. With
// access outside otelhttp the log had no span and every line landed in Loki untraceable.
func Chain(h http.Handler, serviceName string) http.Handler {
	traced := otelhttp.NewHandler(access(h), "http", otelhttp.WithServerName(serviceName))
	return version(traced)
}

// version stamps the running binary's identity on every response (ADR-0103), so a
// client — the frontend, a curl, DevOps — can confirm which build answered without
// trusting the deploy pipeline. Set outermost so the headers land before any write.
func version(next http.Handler) http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-App-Version", buildinfo.Version)
			w.Header().Set("X-App-Revision", buildinfo.SHA)
			next.ServeHTTP(w, r)
		},
	)
}

type statusWriter struct {
	http.ResponseWriter

	status int
}

func (s *statusWriter) WriteHeader(c int) { s.status = c; s.ResponseWriter.WriteHeader(c) }

func access(next http.Handler) http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()
			next.ServeHTTP(sw, r)
			slog.LogAttrs(
				r.Context(),
				slog.LevelInfo,
				"http",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", sw.status),
				slog.String("duration", strconv.FormatFloat(time.Since(start).Seconds(), 'f', 6, 64)),
			)
		},
	)
}

// ListenAddr is the address a service's HTTP server binds. In-cluster it is always
// ":8080" — the chart's containerPort, the edge IngressRoute and the NetworkPolicies
// all assume it, and the chart sets no PORT.
//
// PORT overrides it for host-native runs, where every service instead binds the
// stable port assigned to it in scripts/lib/ports.sh (ADR-0205). Without that they
// would all bind :8080 and only one could run at a time, which made reproducing a
// cross-service bug — the orders checkout saga calls catalog and payment — mean
// deploying the callees and giving up breakpoints in them. The registry also lets
// each caller's <CALLEE>_URL ship a working default instead of a port the engineer
// has to invent.
func ListenAddr() string {
	p := os.Getenv("PORT")
	if p != "" {
		return ":" + p
	}
	return ":8080"
}
