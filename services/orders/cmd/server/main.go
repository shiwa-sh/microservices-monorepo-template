// orders — checkout saga over catalog + payment (ADR-0006).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/sdk/client"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/apierr"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/dbmw"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/httpmw"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/observability"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/temporalmw"
	"github.com/tabmadi/microservices-monorepo-template/services/orders/internal/workflows"
)

const serviceName = "orders"

type Order struct {
	ID         uuid.UUID `json:"id"`
	ProductID  uuid.UUID `json:"product_id"`
	Quantity   int32     `json:"quantity"`
	TotalCents int32     `json:"total_cents"`
	Status     string    `json:"status"`
}

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

	mux := http.NewServeMux()
	mux.HandleFunc("POST /orders", checkout(db, tc))
	mux.HandleFunc("GET /orders/{id}", getOrder(db))

	srv := &http.Server{Addr: ":8080", Handler: httpmw.Chain(mux, serviceName), ReadHeaderTimeout: 5 * time.Second}
	serveErr := make(chan error, 1)
	go func() {
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- fmt.Errorf("http server: %w", err)
		}
	}()
	slog.Info("orders listening", "addr", srv.Addr)

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(c)
}

func checkout(db *pgxpool.Pool, tc client.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ProductID uuid.UUID `json:"product_id"`
			Quantity  int32     `json:"quantity"`
		}
		err := json.NewDecoder(r.Body).Decode(&in)
		if err != nil || in.Quantity <= 0 {
			apierr.BadRequest("product_id and quantity required").Write(w)
			return
		}
		var o Order
		err = db.QueryRow(r.Context(),
			`insert into orders (product_id, quantity, total_cents, status)
			 values ($1, $2, 0, 'pending')
			 returning id, product_id, quantity, total_cents, status`,
			in.ProductID, in.Quantity).
			Scan(&o.ID, &o.ProductID, &o.Quantity, &o.TotalCents, &o.Status)
		if err != nil {
			apierr.Internal(err.Error()).Write(w)
			return
		}

		_, err = tc.ExecuteWorkflow(r.Context(), client.StartWorkflowOptions{
			ID:        "checkout-" + o.ID.String(),
			TaskQueue: serviceName + "-queue",
		}, workflows.Checkout, workflows.CheckoutInput{
			OrderID:   o.ID.String(),
			ProductID: o.ProductID.String(),
			Quantity:  o.Quantity,
		})
		if err != nil {
			apierr.Internal(err.Error()).Write(w)
			return
		}

		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":         "checkout-" + o.ID.String(),
			"run_id":     o.ID.String(),
			"status":     "running",
			"result_url": "/api/orders/orders/" + o.ID.String(),
		})
	}
}

func getOrder(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			apierr.BadRequest("invalid id").Write(w)
			return
		}
		var o Order
		err = db.QueryRow(r.Context(),
			`select id, product_id, quantity, total_cents, status from orders where id = $1`, id).
			Scan(&o.ID, &o.ProductID, &o.Quantity, &o.TotalCents, &o.Status)
		if errors.Is(err, pgx.ErrNoRows) {
			apierr.NotFound("order").Write(w)
			return
		}
		if err != nil {
			apierr.Internal(err.Error()).Write(w)
			return
		}
		_ = json.NewEncoder(w).Encode(o)
	}
}
