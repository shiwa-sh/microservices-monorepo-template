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
	"strconv"

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

	// OTEL_SDK_DISABLED=true (OTel spec env) makes Init a no-op for exporters:
	// no OTLP connections are opened, so a service can run locally without the
	// Collector stack. Logs still go to stdout and pprof is still served.
	disabled, _ := strconv.ParseBool(os.Getenv("OTEL_SDK_DISABLED"))
	if disabled {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
		go serveAdmin(cfg.AdminAddr)
		return func(context.Context) error { return nil }, nil
	}

	// In local dev the OTel Collector is reached over plaintext (no TLS), so the
	// OTLP exporters must opt out of their default https/TLS behavior.
	var local bool
	switch os.Getenv("DEPLOY_ENV") {
	case "", "dev", "local":
		local = true
	}

	res, err := resource.New(
		ctx,
		resource.WithAttributes(semconv.ServiceName(cfg.ServiceName)),
		resource.WithFromEnv(),
		resource.WithProcessRuntimeName(),
	)
	if err != nil {
		return nil, fmt.Errorf("resource: %w", err)
	}

	var traceOpts []otlptracegrpc.Option
	var metricOpts []otlpmetricgrpc.Option
	var logOpts []otlploghttp.Option
	if local {
		traceOpts = append(traceOpts, otlptracegrpc.WithInsecure())
		metricOpts = append(metricOpts, otlpmetricgrpc.WithInsecure())
		logOpts = append(logOpts, otlploghttp.WithInsecure())
	}

	traceExp, err := otlptracegrpc.New(ctx, traceOpts...)
	if err != nil {
		return nil, fmt.Errorf("trace exporter: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	metricExp, err := otlpmetricgrpc.New(ctx, metricOpts...)
	if err != nil {
		return nil, fmt.Errorf("metric exporter: %w", err)
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp)),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	logExp, err := otlploghttp.New(ctx, logOpts...)
	if err != nil {
		return nil, fmt.Errorf("log exporter: %w", err)
	}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp)),
		sdklog.WithResource(res),
	)
	handlers := fanout{otelslog.NewHandler(cfg.ServiceName, otelslog.WithLoggerProvider(lp))}
	if local {
		handlers = append(fanout{slog.NewTextHandler(os.Stdout, nil)}, handlers...)
	}
	slog.SetDefault(slog.New(handlers))

	go serveAdmin(cfg.AdminAddr)

	shutdown := func(ctx context.Context) error {
		_ = tp.Shutdown(ctx)
		_ = mp.Shutdown(ctx)
		_ = lp.Shutdown(ctx)
		return nil
	}
	return shutdown, nil
}

// fanout is a slog.Handler that dispatches every record to all wrapped handlers,
// so logs reach both stdout (visible in local runs) and the OTLP pipeline.
type fanout []slog.Handler

func (f fanout) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range f {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (f fanout) Handle(ctx context.Context, r slog.Record) error {
	var err error
	for _, h := range f {
		if h.Enabled(ctx, r.Level) {
			e := h.Handle(ctx, r.Clone())
			if e != nil {
				err = e
			}
		}
	}
	return err
}

func (f fanout) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make(fanout, len(f))
	for i, h := range f {
		next[i] = h.WithAttrs(attrs)
	}
	return next
}

func (f fanout) WithGroup(name string) slog.Handler {
	next := make(fanout, len(f))
	for i, h := range f {
		next[i] = h.WithGroup(name)
	}
	return next
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
	err := srv.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, "admin server:", err)
	}
}
