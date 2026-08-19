// Package kratos is this service's client for the Kratos admin API.
//
// It exists as a package rather than as methods on the handlers because two
// callers need it and they are not in the same process: the HTTP handlers read
// and update identities for the console (ADR-0401), and the operator-registration
// activity creates one (ADR-0304). Only authz may reach the admin API at all
// (network-policies/30-ory.yaml), which is why every other component asks this
// service rather than Kratos.
package kratos

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"

	authzsdk "github.com/tabmadi/microservices-monorepo-template/libs/go/sdks/authz"
)

// SchemaUserV1 is the Kratos identity schema id (user.v1.json).
const SchemaUserV1 = "user_v1"

// Admin is the admin-API client. The zero value is not usable; call New.
type Admin struct {
	baseURL string
	log     *slog.Logger
}

// New reads the admin URL from the environment, falling back to the in-cluster
// Service. The fallback is the deployed case, so a missing variable is a local
// run rather than a misconfiguration.
func New(log *slog.Logger) *Admin {
	base := os.Getenv("KRATOS_ADMIN_URL")
	if base == "" {
		base = "http://ory-kratos-admin.platform.svc.cluster.local"
	}
	return NewAt(base, log)
}

// NewAt builds a client against an explicit base URL. It exists so a test can
// point at an httptest server without setting a process-wide environment
// variable, which parallel tests cannot share.
func NewAt(baseURL string, log *slog.Logger) *Admin {
	if log == nil {
		log = slog.Default()
	}
	return &Admin{baseURL: baseURL, log: log}
}

// identityBody is the request body for POST /admin/identities.
type identityBody struct {
	SchemaID            string      `json:"schema_id"`
	Traits              Traits      `json:"traits"`
	Credentials         credentials `json:"credentials"`
	VerifiableAddresses []address   `json:"verifiable_addresses"`
}

type Traits struct {
	Email    string `json:"email"`
	Operator bool   `json:"operator"`
}

type credentials struct {
	Password passwordCredential `json:"password"`
}

type passwordCredential struct {
	Config passwordConfig `json:"config"`
}

type passwordConfig struct {
	Password string `json:"password"`
}

type address struct {
	Value    string `json:"value"`
	Via      string `json:"via"`
	Verified bool   `json:"verified"`
	Status   string `json:"status"`
}

// Identity is the subset of a Kratos admin identity this service reads and
// writes. schema_id and state are carried through unmodified on update — Kratos PUT
// replaces the whole record, so dropping them would reset the identity. So is
// metadata_public, which this service never edits and must not erase: the edge
// builds X-Org-Id and X-Roles out of it (ADR-0304), so writing the record back
// without it would silently unassign an operator's org on the next name change.
type Identity struct {
	ID             string          `json:"id,omitempty"`
	SchemaID       string          `json:"schema_id,omitempty"`
	State          string          `json:"state,omitempty"`
	MetadataPublic json.RawMessage `json:"metadata_public,omitempty"`
	Traits         struct {
		Email    string `json:"email"`
		Name     string `json:"name,omitempty"`
		Operator bool   `json:"operator"`
	} `json:"traits"`
}

// Flatten projects the Kratos identity onto the admin-facing Identity shape.
func (k *Identity) Flatten() authzsdk.Identity {
	id := authzsdk.Identity{ID: k.ID, Email: k.Traits.Email, Operator: authzsdk.NewOptBool(k.Traits.Operator)}
	if k.Traits.Name != "" {
		id.Name = authzsdk.NewOptString(k.Traits.Name)
	}
	return id
}

// ListIdentities reads GET /admin/identities and flattens each identity's
// traits. Only per_page (page_size) is forwarded — this Kratos paginates by keyset,
// where `page` is an opaque token, not a 1-based offset; a numeric page returns an
// empty set. Zero perPage lets Kratos apply its own default.
func (a *Admin) ListIdentities(ctx context.Context, perPage int) ([]authzsdk.Identity, error) {
	u := a.baseURL + "/admin/identities"
	q := url.Values{}
	if perPage > 0 {
		q.Set("per_page", strconv.Itoa(perPage))
	}
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var raw []Identity
	err := a.do(ctx, http.MethodGet, u, nil, http.StatusOK, &raw)
	if err != nil {
		return nil, err
	}
	out := make([]authzsdk.Identity, 0, len(raw))
	for i := range raw {
		out = append(out, raw[i].Flatten())
	}
	return out, nil
}

// GetIdentity fetches one full identity by id.
func (a *Admin) GetIdentity(ctx context.Context, id string) (*Identity, error) {
	var out Identity
	err := a.do(ctx, http.MethodGet, a.identityURL(id), nil, http.StatusOK, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// PutIdentity writes a full identity back (Kratos PUT replaces the record).
func (a *Admin) PutIdentity(ctx context.Context, ident *Identity) (*Identity, error) {
	body := *ident
	body.ID = "" // id is the path, not part of the update body
	var out Identity
	err := a.do(ctx, http.MethodPut, a.identityURL(ident.ID), body, http.StatusOK, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (a *Admin) CreateOperatorIdentity(ctx context.Context, email, password string) (string, error) {
	payload := identityBody{
		SchemaID: SchemaUserV1,
		Traits:   Traits{Email: email, Operator: true},
		Credentials: credentials{
			Password: passwordCredential{
				Config: passwordConfig{Password: password},
			},
		},
		VerifiableAddresses: []address{
			{Value: email, Via: "email", Verified: true, Status: "completed"},
		},
	}
	var out struct {
		ID string `json:"id"`
	}
	err := a.do(ctx, http.MethodPost, a.baseURL+"/admin/identities", payload, http.StatusCreated, &out)
	if err != nil {
		return "", err
	}
	return out.ID, nil
}

// identityURL is the Kratos admin URL for one identity.
func (a *Admin) identityURL(id string) string {
	return a.baseURL + "/admin/identities/" + url.PathEscape(id)
}

// kratosJSON performs a JSON request to the Kratos admin API and decodes a JSON
// response, asserting the expected status. reqBody nil sends no body; out nil skips
// decoding. It is the shared transport for the identity read/write helpers.
func (a *Admin) do(ctx context.Context, method, u string, reqBody any, wantStatus int, out any) error {
	var reader io.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("call kratos: %w", err)
	}
	defer func() {
		closeErr := resp.Body.Close()
		if closeErr != nil {
			a.log.Error("close kratos response body", "err", closeErr)
		}
	}()
	if resp.StatusCode != wantStatus {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("kratos %d: %s", resp.StatusCode, b)
	}
	if out == nil {
		return nil
	}
	err = json.NewDecoder(resp.Body).Decode(out)
	if err != nil {
		return fmt.Errorf("decode kratos response: %w", err)
	}
	return nil
}
