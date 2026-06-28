// Package operator exposes POST /admin/operators for the Lowdefy admin console
// (ADR-0012). One call creates a Kratos identity and grants group:operator#member
// in SpiceDB so the new user can reach all ops-tier dashboards immediately.
package operator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/authz"
)

type Handler struct {
	kratosAdmin string
	granter     authz.Granter
	log         *slog.Logger
}

func New(granter authz.Granter, log *slog.Logger) *Handler {
	admin := os.Getenv("KRATOS_ADMIN_URL")
	if admin == "" {
		admin = "http://ory-kratos-admin.platform.svc.cluster.local"
	}
	return &Handler{kratosAdmin: admin, granter: granter, log: log}
}

type createReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type createResp struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req createReq
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.Email == "" || req.Password == "" {
		http.Error(w, "email and password required", http.StatusBadRequest)
		return
	}

	id, err := h.createKratosIdentity(r.Context(), req.Email, req.Password)
	if err != nil {
		h.log.Error("create kratos identity", "err", err)
		http.Error(w, "failed to create identity", http.StatusInternalServerError)
		return
	}

	err = h.granter.Grant(r.Context(), "user:"+id, "member", "group:operator")
	if err != nil {
		h.log.Error("grant operator", "err", err, "id", id)
		http.Error(w, "failed to grant operator role", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	encErr := enc.Encode(createResp{ID: id, Email: req.Email})
	if encErr != nil {
		h.log.Error("encode response", "err", encErr)
	}
}

// kratosIdentityBody is the request body for POST /admin/identities.
type kratosIdentityBody struct {
	SchemaID            string            `json:"schema_id"`
	Traits              kratosTraits      `json:"traits"`
	Credentials         kratosCredentials `json:"credentials"`
	VerifiableAddresses []kratosAddress   `json:"verifiable_addresses"`
}

type kratosTraits struct {
	Email string `json:"email"`
}

type kratosCredentials struct {
	Password kratosPasswordCredential `json:"password"`
}

type kratosPasswordCredential struct {
	Config kratosPasswordConfig `json:"config"`
}

type kratosPasswordConfig struct {
	Password string `json:"password"`
}

type kratosAddress struct {
	Value    string `json:"value"`
	Via      string `json:"via"`
	Verified bool   `json:"verified"`
	Status   string `json:"status"`
}

func (h *Handler) createKratosIdentity(ctx context.Context, email, password string) (string, error) {
	payload := kratosIdentityBody{
		SchemaID: "user_v1",
		Traits:   kratosTraits{Email: email},
		Credentials: kratosCredentials{
			Password: kratosPasswordCredential{
				Config: kratosPasswordConfig{Password: password},
			},
		},
		VerifiableAddresses: []kratosAddress{
			{Value: email, Via: "email", Verified: true, Status: "completed"},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		h.kratosAdmin+"/admin/identities",
		bytes.NewReader(body),
	)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call kratos: %w", err)
	}
	defer func() {
		closeErr := resp.Body.Close()
		if closeErr != nil {
			h.log.Error("close kratos response body", "err", closeErr)
		}
	}()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("kratos %d: %s", resp.StatusCode, b)
	}
	var out struct {
		ID string `json:"id"`
	}
	err = json.NewDecoder(resp.Body).Decode(&out)
	if err != nil {
		return "", fmt.Errorf("decode kratos response: %w", err)
	}
	return out.ID, nil
}
