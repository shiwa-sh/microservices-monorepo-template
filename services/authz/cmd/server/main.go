// authz — the ops-tier edge authorizer (ADR-0306). A tiny internal HTTP service
// (no DB, no edge route) that Oathkeeper's remote_json authorizer calls to decide
// per-tool operator dashboard access via libs/go/authz's OpenFGA Checker.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/authz"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/httpmw"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/observability"
	authzsdk "github.com/tabmadi/microservices-monorepo-template/libs/go/sdks/authz"
	"github.com/tabmadi/microservices-monorepo-template/services/authz/internal/handlers"
)

const serviceName = "authz"

func main() {
	err := run()
	if err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdown, err := observability.Init(ctx, observability.Config{ServiceName: serviceName})
	if err != nil {
		return fmt.Errorf("obs init: %w", err)
	}
	defer func() { _ = shutdown(context.Background()) }()

	checker, err := authz.New()
	if err != nil {
		return fmt.Errorf("authz client: %w", err)
	}
	granter, err := authz.NewGranter()
	if err != nil {
		return fmt.Errorf("authz granter: %w", err)
	}

	// Coarse claim gate is always on; the optional fine per-tool OpenFGA layer is
	// enabled per-project (ADR-0306). Default off keeps the coarse gate free of any
	// OpenFGA dependency.
	fineGrained := os.Getenv("OPS_FINE_GRAINED") == "true"

	// authz is spec-first like every HTTP service (ADR-0303): the ogen server routes
	// and validates; the handlers implement the generated interface. No authmw — the
	// caller is Oathkeeper (remote_json), not a user session.
	api, err := authzsdk.NewServer(handlers.New(checker, granter, fineGrained, slog.Default()))
	if err != nil {
		return fmt.Errorf("ogen server: %w", err)
	}

	srv := &http.Server{
		Addr:              httpmw.ListenAddr(),
		Handler:           httpmw.Chain(api, serviceName),
		ReadHeaderTimeout: 5 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() {
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- fmt.Errorf("http server: %w", err)
		}
	}()
	slog.Info("authz listening", "addr", srv.Addr)

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = srv.Shutdown(shutCtx)
	if err != nil {
		return fmt.Errorf("authz: server shutdown: %w", err)
	}
	return nil
}
