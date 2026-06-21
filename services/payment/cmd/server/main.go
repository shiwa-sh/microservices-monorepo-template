// payment — charges with idempotency and a Temporal-driven workflow (ADR-0006).
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

	"github.com/tabmadi/microservices-monorepo-template/libs/go/authmw"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/dbmw"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/httpmw"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/observability"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/sdks/payment"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/temporalmw"
	"github.com/tabmadi/microservices-monorepo-template/services/payment/internal/handlers"
)

const serviceName = "payment"

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

	db := dbmw.MustOpen(ctx, os.Getenv("DATABASE_URL"))
	defer db.Close()

	tc, err := temporalmw.NewClient(serviceName)
	if err != nil {
		return fmt.Errorf("temporal: %w", err)
	}
	defer tc.Close()

	api, err := payment.NewServer(handlers.New(db, tc))
	if err != nil {
		return fmt.Errorf("ogen server: %w", err)
	}

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           httpmw.Chain(authmw.Middleware()(api), serviceName),
		ReadHeaderTimeout: 5 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() {
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- fmt.Errorf("http server: %w", err)
		}
	}()
	slog.Info("payment listening", "addr", srv.Addr)

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = srv.Shutdown(shutCtx)
	if err != nil {
		return fmt.Errorf("payment: server shutdown: %w", err)
	}
	return nil
}
