// Package observability is the single entry point for logs, metrics, traces,
// and continuous profiles (ADR-0011). Service code calls obs.Init in main and
// gets every signal wired through the OTel Collector.
package observability

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"os"

	otelslog "go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// Config is read from environment variables when omitted. Service code provides
// only the service name; everything else defaults.
type Config struct {
	ServiceName  string
	OTLPEndpoint string // default: $OTEL_EXPORTER_OTLP_ENDPOINT
	AdminAddr    string // default: :9090
}

// Init wires up tracing, metrics, logs, pprof, and slog. The returned shutdown
// function must be called from main to flush all signals.
func Init(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	if cfg.ServiceName == "" {
		return nil, fmt.Errorf("observability.Init: ServiceName is required")
	}
	if cfg.AdminAddr == "" {
		cfg.AdminAddr = ":9090"
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(cfg.ServiceName)),
		resource.WithFromEnv(),
		resource.WithProcessRuntimeName(),
	)
	if err != nil {
		return nil, fmt.Errorf("resource: %w", err)
	}

	traceExp, err := otlptracegrpc.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("trace exporter: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	metricExp, err := otlpmetricgrpc.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("metric exporter: %w", err)
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp)),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	logExp, err := otlploghttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("log exporter: %w", err)
	}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp)),
		sdklog.WithResource(res),
	)
	slog.SetDefault(slog.New(otelslog.NewHandler(cfg.ServiceName, otelslog.WithLoggerProvider(lp))))

	go serveAdmin(cfg.AdminAddr)

	shutdown := func(ctx context.Context) error {
		_ = tp.Shutdown(ctx)
		_ = mp.Shutdown(ctx)
		_ = lp.Shutdown(ctx)
		return nil
	}
	return shutdown, nil
}

// StartSpan wraps otel.Tracer().Start so service code never imports otel directly.
func StartSpan(ctx context.Context, name string) (context.Context, trace.Span) {
	return otel.Tracer("service").Start(ctx, name)
}

// Counter returns (or creates) a named counter on the service meter.
func Counter(name string, opts ...metric.Int64CounterOption) metric.Int64Counter {
	c, err := otel.Meter("service").Int64Counter(name, opts...)
	if err != nil {
		// Counter creation is a programmer error; surface it loudly during dev.
		panic("observability.Counter(" + name + "): " + err.Error())
	}
	return c
}

func serveAdmin(addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	srv := &http.Server{Addr: addr, Handler: mux}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, "admin server:", err)
	}
}
