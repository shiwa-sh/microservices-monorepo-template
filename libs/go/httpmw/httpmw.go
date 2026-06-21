// Package httpmw provides default HTTP middleware (ADR-0011): a tracing span,
// RED metrics, and a structured access log. Services compose them via Chain.
package httpmw

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	requestCount metric.Int64Counter
	requestDur   metric.Float64Histogram
)

func init() {
	m := otel.Meter("http.server")
	var err error
	requestCount, err = m.Int64Counter("http.server.requests")
	if err != nil {
		panic(err)
	}
	requestDur, err = m.Float64Histogram("http.server.duration_seconds")
	if err != nil {
		panic(err)
	}
}

// Chain wraps h with tracing, RED metrics, and access logging.
func Chain(h http.Handler, serviceName string) http.Handler {
	traced := otelhttp.NewHandler(h, "http", otelhttp.WithServerName(serviceName))
	return red(access(traced))
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(c int) { s.status = c; s.ResponseWriter.WriteHeader(c) }

func red(next http.Handler) http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			sw := &statusWriter{ResponseWriter: w, status: 200}
			start := time.Now()
			next.ServeHTTP(sw, r)
			dur := time.Since(start).Seconds()
			attrs := metric.WithAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.route", r.URL.Path),
				attribute.Int("http.status_code", sw.status),
			)
			requestCount.Add(r.Context(), 1, attrs)
			requestDur.Record(r.Context(), dur, attrs)
		},
	)
}

func access(next http.Handler) http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			sw := &statusWriter{ResponseWriter: w, status: 200}
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
