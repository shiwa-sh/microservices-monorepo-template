// Package activities for the Checkout saga (ADR-0302).
// All cross-service calls go through HTTP via the generated client surface
// (here: raw http.Client — replace with the ogen client in libs/go/sdks/<service>
// when `mise run gen` produces the typed clients).
package activities

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/authz"
	"github.com/tabmadi/microservices-monorepo-template/libs/go/id"
	"github.com/tabmadi/microservices-monorepo-template/services/orders/internal/store"
	"github.com/tabmadi/microservices-monorepo-template/services/orders/internal/workflows"
)

type Activities struct {
	DB         *pgxpool.Pool
	Granter    authz.Granter
	HTTP       *http.Client
	CatalogURL string
	PaymentURL string
}

func New(db *pgxpool.Pool, granter authz.Granter) *Activities {
	return &Activities{
		DB:      db,
		Granter: granter,
		// otelhttp transport injects the W3C traceparent on every outbound call, so
		// the catalog/payment server spans stitch under this activity's span (which
		// Temporal's tracing interceptor already links to the checkout workflow).
		// Without it the cross-service hop starts a detached root trace.
		HTTP:       &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)},
		CatalogURL: env("CATALOG_URL", "http://catalog-server.platform.svc.cluster.local"),
		PaymentURL: env("PAYMENT_URL", "http://payment-server.platform.svc.cluster.local"),
	}
}

func env(k, def string) string {
	v := os.Getenv(k)
	if v != "" {
		return v
	}
	return def
}

// CreateOrderActivity is dual-write leg 1 (ADR-0304): the application-DB write.
// The identifier is minted by the handler and passed in, so the insert is keyed on
// a value that does not change between attempts — `on conflict (id) do nothing`
// then makes a retry re-run rather than duplicate, and the empty result it returns
// is the second attempt, not a failure.
func (a *Activities) CreateOrderActivity(ctx context.Context, in workflows.CheckoutInput) error {
	orderID, err := id.Parse("order", in.OrderID)
	if err != nil {
		return fmt.Errorf("create order: parse order id: %w", err)
	}
	productID, err := id.Parse("product", in.ProductID)
	if err != nil {
		return fmt.Errorf("create order: parse product id: %w", err)
	}
	orgID, err := id.Parse("org", in.OrgID)
	if err != nil {
		return fmt.Errorf("create order: parse org id: %w", err)
	}

	_, err = store.New(a.DB).CreateOrder(
		ctx,
		store.CreateOrderParams{
			ID:             pgtype.UUID{Bytes: orderID.UUID(), Valid: true},
			ProductID:      pgtype.UUID{Bytes: productID.UUID(), Valid: true},
			Quantity:       in.Quantity,
			TotalCents:     0,
			OwnerID:        pgtype.Text{String: in.OwnerID, Valid: true},
			OrgID:          pgtype.UUID{Bytes: orgID.UUID(), Valid: true},
			IdempotencyKey: pgtype.Text{String: in.IdempotencyKey, Valid: in.IdempotencyKey != ""},
		},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create order: insert: %w", err)
	}
	return nil
}

// GrantOrderAccessActivity is dual-write leg 2 (ADR-0304): the OpenFGA write that
// makes the row readable. Two tuples, because `order#read` is `owner or write from
// org` — the buyer reads their own order, and the org's admins read every order
// placed through it. Grant treats an already-present tuple as success, so a retry
// of either write is safe.
func (a *Activities) GrantOrderAccessActivity(ctx context.Context, orderID, ownerID, orgID string) error {
	err := a.Granter.Grant(ctx, "user:"+ownerID, "owner", "order:"+orderID)
	if err != nil {
		return fmt.Errorf("grant order owner: %w", err)
	}
	err = a.Granter.Grant(ctx, "org:"+orgID, "org", "order:"+orderID)
	if err != nil {
		return fmt.Errorf("grant order org: %w", err)
	}
	return nil
}

// LookupProductActivity calls catalog. Returns the product's price in cents.
func (a *Activities) LookupProductActivity(ctx context.Context, productID string) (int32, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, a.CatalogURL+"/products/"+productID, nil)
	resp, err := a.HTTP.Do(req)
	if err != nil {
		return 0, fmt.Errorf("lookup product: call catalog: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("catalog status %d", resp.StatusCode)
	}
	var out struct {
		PriceCents int32 `json:"price_cents"`
	}
	err = json.NewDecoder(resp.Body).Decode(&out)
	if err != nil {
		return 0, fmt.Errorf("lookup product: decode response: %w", err)
	}
	return out.PriceCents, nil
}

// ChargeActivity calls payment with an idempotency key derived from the order ID.
// Returns the charge handle ID. The order id is already type-prefixed (ADR-0003),
// so it names its own type in payment's dedup store without a second prefix.
func (a *Activities) ChargeActivity(ctx context.Context, orderID string, totalCents int32) (string, error) {
	body, err := json.Marshal(map[string]any{"order_id": orderID, "amount_cents": totalCents})
	if err != nil {
		return "", fmt.Errorf("charge: marshal request: %w", err)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, a.PaymentURL+"/charges", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", orderID)
	resp, err := a.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("charge: call payment: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusAccepted {
		return "", fmt.Errorf("payment status %d", resp.StatusCode)
	}
	var out struct {
		ID string `json:"id"`
	}
	err = json.NewDecoder(resp.Body).Decode(&out)
	if err != nil {
		return "", fmt.Errorf("charge: decode response: %w", err)
	}
	return out.ID, nil
}

// MarkOrderStatusActivity writes the terminal status of an order. The workflow
// carries the wire form, and the column holds the bare uuid (ADR-0003), so this is
// where the two meet.
func (a *Activities) MarkOrderStatusActivity(ctx context.Context, orderID, status string) error {
	parsed, err := id.Parse("order", orderID)
	if err != nil {
		return fmt.Errorf("mark order status: parse order id: %w", err)
	}
	_, err = a.DB.Exec(ctx, `update orders set status = $2 where id = $1`, parsed.UUID(), status)
	if err != nil {
		return fmt.Errorf("mark order status: update: %w", err)
	}
	return nil
}
