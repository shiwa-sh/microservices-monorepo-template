// orgs — B2B multi-tenancy on top of Kratos identities (ADR-0010).
// Owns: organisations, memberships, and the post-registration "create personal
// org" webhook called by Kratos.
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

const serviceName = "orgs"

type Org struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
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
	mux.HandleFunc("POST /orgs", createOrg(db))
	mux.HandleFunc("GET /orgs/{id}", getOrg(db))
	mux.HandleFunc("POST /internal/identity-created", onIdentityCreated(db))

	srv := &http.Server{Addr: ":8080", Handler: httpmw.Chain(mux, serviceName), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.ListenAndServe() }()
	slog.Info("orgs listening", "addr", srv.Addr)
	<-ctx.Done()
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(c)
}

func createOrg(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Name == "" {
			apierr.BadRequest("name required").Write(w)
			return
		}
		var o Org
		if err := db.QueryRow(r.Context(), `insert into orgs (name) values ($1) returning id, name`, in.Name).
			Scan(&o.ID, &o.Name); err != nil {
			apierr.Internal(err.Error()).Write(w)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(o)
	}
}

func getOrg(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			apierr.BadRequest("invalid id").Write(w)
			return
		}
		var o Org
		err = db.QueryRow(r.Context(), `select id, name from orgs where id = $1`, id).Scan(&o.ID, &o.Name)
		if errors.Is(err, pgx.ErrNoRows) {
			apierr.NotFound("org").Write(w)
			return
		}
		if err != nil {
			apierr.Internal(err.Error()).Write(w)
			return
		}
		_ = json.NewEncoder(w).Encode(o)
	}
}

// Kratos post-registration webhook (ADR-0010 §B2B Organisations).
// Creates a personal org for the new identity and inserts the user as admin.
func onIdentityCreated(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct{ IdentityID, Email string }
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.IdentityID == "" {
			apierr.BadRequest("identity_id required").Write(w)
			return
		}
		tx, err := db.Begin(r.Context())
		if err != nil {
			apierr.Internal(err.Error()).Write(w)
			return
		}
		defer func() { _ = tx.Rollback(r.Context()) }()
		var orgID uuid.UUID
		if err := tx.QueryRow(r.Context(), `insert into orgs (name) values ($1) returning id`, in.Email).Scan(&orgID); err != nil {
			apierr.Internal(err.Error()).Write(w)
			return
		}
		if _, err := tx.Exec(r.Context(),
			`insert into org_members (org_id, user_id, role) values ($1, $2, 'admin')`, orgID, in.IdentityID); err != nil {
			apierr.Internal(err.Error()).Write(w)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			apierr.Internal(err.Error()).Write(w)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
