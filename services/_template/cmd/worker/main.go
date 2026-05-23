//go:build _template

// Worker entry point for the template service. Copy + strip the build tag.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

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

	tc, err := temporalmw.NewClient(serviceName)
	if err != nil {
		slog.Error("temporal", "err", err)
		os.Exit(1)
	}
	defer tc.Close()

	w := temporalmw.NewWorker(tc, serviceName+"-queue")
	// Register workflows + activities here:
	// w.RegisterWorkflow(workflows.Example)
	// w.RegisterActivity(activities.DoStuff)

	if err := w.Run(worker_interrupt(ctx)); err != nil {
		slog.Error("worker", "err", err)
		os.Exit(1)
	}
}

func worker_interrupt(ctx context.Context) <-chan interface{} {
	ch := make(chan interface{}, 1)
	go func() { <-ctx.Done(); ch <- nil }()
	return ch
}
