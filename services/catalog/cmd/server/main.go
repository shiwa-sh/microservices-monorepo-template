// catalog — product CRUD. The simplest shop service: pure HTTP + Postgres,
// no workflows. Demonstrates the OpenAPI → handler → sqlc → migrations path.
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

	"github.com/tabmadi/microservices-monorepo-template/libs/go/apierr"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/dbmw"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/httpmw"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/observability"
)

const serviceName = "catalog"

type Product struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	PriceCents int32     `json:"price_cents"`
}

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

	mux := http.NewServeMux()
	mux.HandleFunc("GET /products", listProducts(db))
	mux.HandleFunc("GET /products/{id}", getProduct(db))
	mux.HandleFunc("POST /products", createProduct(db))

	srv := &http.Server{Addr: ":8080", Handler: httpmw.Chain(mux, serviceName), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.ListenAndServe() }()
	slog.Info("catalog listening", "addr", srv.Addr)
	<-ctx.Done()
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
}

func listProducts(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query(r.Context(), `select id, name, price_cents from products order by created_at desc limit 100`)
		if err != nil {
			apierr.Internal(err.Error()).Write(w)
			return
		}
		defer rows.Close()
		out := []Product{}
		for rows.Next() {
			var p Product
			if err := rows.Scan(&p.ID, &p.Name, &p.PriceCents); err != nil {
				apierr.Internal(err.Error()).Write(w)
				return
			}
			out = append(out, p)
		}
		_ = json.NewEncoder(w).Encode(out)
	}
}

func getProduct(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			apierr.BadRequest("invalid id").Write(w)
			return
		}
		var p Product
		err = db.QueryRow(r.Context(), `select id, name, price_cents from products where id = $1`, id).
			Scan(&p.ID, &p.Name, &p.PriceCents)
		if errors.Is(err, pgx.ErrNoRows) {
			apierr.NotFound("product").Write(w)
			return
		}
		if err != nil {
			apierr.Internal(err.Error()).Write(w)
			return
		}
		_ = json.NewEncoder(w).Encode(p)
	}
}

func createProduct(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name       string `json:"name"`
			PriceCents int32  `json:"price_cents"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			apierr.BadRequest(err.Error()).Write(w)
			return
		}
		if in.Name == "" || in.PriceCents < 0 {
			apierr.BadRequest("name and price_cents required").Write(w)
			return
		}
		var p Product
		err := db.QueryRow(r.Context(),
			`insert into products (name, price_cents) values ($1, $2) returning id, name, price_cents`,
			in.Name, in.PriceCents).Scan(&p.ID, &p.Name, &p.PriceCents)
		if err != nil {
			apierr.Internal(err.Error()).Write(w)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(p)
	}
}
