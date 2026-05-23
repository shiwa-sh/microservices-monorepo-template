// payment — charges with idempotency and a Temporal-driven workflow (ADR-0006).
package main

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/tabmadi/microservices-monorepo-template/services/payment/internal/workflows"
)

const serviceName = "payment"

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
	mux.HandleFunc("POST /charges", createCharge(db, tc))
	mux.HandleFunc("GET /charges/{id}", getCharge(db))

	srv := &http.Server{Addr: ":8080", Handler: httpmw.Chain(mux, serviceName), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.ListenAndServe() }()
	slog.Info("payment listening", "addr", srv.Addr)
	<-ctx.Done()
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(c)
}

type chargeRow struct {
	ID          uuid.UUID `json:"id"`
	OrderID     uuid.UUID `json:"order_id"`
	AmountCents int32     `json:"amount_cents"`
	Status      string    `json:"status"`
}

func createCharge(db *pgxpool.Pool, tc client.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idemKey := r.Header.Get("Idempotency-Key")
		if idemKey == "" {
			apierr.BadRequest("Idempotency-Key required").Write(w)
			return
		}

		var in struct {
			OrderID     uuid.UUID `json:"order_id"`
			AmountCents int32     `json:"amount_cents"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			apierr.BadRequest(err.Error()).Write(w)
			return
		}

		// Idempotency lookup before anything else.
		var existing chargeRow
		err := db.QueryRow(r.Context(),
			`select id, order_id, amount_cents, status from charges where idempotency_key = $1`, idemKey).
			Scan(&existing.ID, &existing.OrderID, &existing.AmountCents, &existing.Status)
		if err == nil {
			respondHandle(w, existing.ID.String())
			return
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			apierr.Internal(err.Error()).Write(w)
			return
		}

		var created chargeRow
		err = db.QueryRow(r.Context(),
			`insert into charges (order_id, amount_cents, status, idempotency_key)
			 values ($1, $2, 'pending', $3)
			 returning id, order_id, amount_cents, status`,
			in.OrderID, in.AmountCents, idemKey).
			Scan(&created.ID, &created.OrderID, &created.AmountCents, &created.Status)
		if err != nil {
			apierr.Internal(err.Error()).Write(w)
			return
		}

		_, err = tc.ExecuteWorkflow(r.Context(), client.StartWorkflowOptions{
			ID:        "charge-" + created.ID.String(),
			TaskQueue: serviceName + "-queue",
		}, workflows.Charge, workflows.ChargeInput{
			ChargeID:    created.ID.String(),
			OrderID:     created.OrderID.String(),
			AmountCents: created.AmountCents,
		})
		if err != nil {
			apierr.Internal(err.Error()).Write(w)
			return
		}

		respondHandle(w, created.ID.String())
	}
}

func respondHandle(w http.ResponseWriter, id string) {
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":         "charge-" + id,
		"run_id":     id,
		"status":     "running",
		"result_url": "/api/payment/charges/" + id,
	})
}

func getCharge(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			apierr.BadRequest("invalid id").Write(w)
			return
		}
		var c chargeRow
		err = db.QueryRow(r.Context(),
			`select id, order_id, amount_cents, status from charges where id = $1`, id).
			Scan(&c.ID, &c.OrderID, &c.AmountCents, &c.Status)
		if errors.Is(err, pgx.ErrNoRows) {
			apierr.NotFound("charge").Write(w)
			return
		}
		if err != nil {
			apierr.Internal(err.Error()).Write(w)
			return
		}
		_ = json.NewEncoder(w).Encode(c)
	}
}
