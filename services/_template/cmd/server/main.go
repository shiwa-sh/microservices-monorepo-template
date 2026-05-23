//go:build _template

// Server entry point for the template service. Copy this file when scaffolding
// a new service (see scripts/new-service.sh) and strip the build tag.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/authmw"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/dbmw"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/httpmw"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/observability"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/temporalmw"
)

const serviceName = "_template"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdown, err := observability.Init(ctx, observability.Config{ServiceName: serviceName})
	if err != nil {
		slog.Error("obs init", "err", err)
		os.Exit(1)
	}
	defer shutdown(context.Background())

	db := dbmw.MustOpen(ctx, os.Getenv("DATABASE_URL"))
	defer db.Close()

	tc, err := temporalmw.NewClient(serviceName)
	if err != nil {
		slog.Error("temporal", "err", err)
		os.Exit(1)
	}
	defer tc.Close()

	mux := http.NewServeMux()
	// Wire generated server stubs here:
	// _template.RegisterHandlers(mux, handlers.New(db, tc))
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })

	verify := authmw.Verify()
	srv := &http.Server{
		Addr:              ":8080",
		Handler:           httpmw.Chain(verify(mux), serviceName),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() { _ = srv.ListenAndServe() }()
	slog.Info("server listening", "addr", srv.Addr)
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
