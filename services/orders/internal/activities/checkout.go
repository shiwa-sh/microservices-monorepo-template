// Activities for the Checkout saga (ADR-0006).
// All cross-service calls go through HTTP via the generated client surface
// (here: raw http.Client — replace with libs/sdks/go/<service>/client when
// `mise run gen:all` produces the typed clients).
package activities

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Activities struct {
	DB         *pgxpool.Pool
	HTTP       *http.Client
	CatalogURL string
	PaymentURL string
}

func New(db *pgxpool.Pool) *Activities {
	return &Activities{
		DB:         db,
		HTTP:       &http.Client{},
		CatalogURL: env("CATALOG_URL", "http://catalog-server.platform.svc.cluster.local"),
		PaymentURL: env("PAYMENT_URL", "http://payment-server.platform.svc.cluster.local"),
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// LookupProductActivity calls catalog. Returns the product's price in cents.
func (a *Activities) LookupProductActivity(ctx context.Context, productID string) (int32, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", a.CatalogURL+"/products/"+productID, nil)
	resp, err := a.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("catalog status %d", resp.StatusCode)
	}
	var out struct {
		PriceCents int32 `json:"price_cents"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	return out.PriceCents, nil
}

// ChargeActivity calls payment with an idempotency key derived from the order ID.
// Returns the charge handle ID.
func (a *Activities) ChargeActivity(ctx context.Context, orderID string, totalCents int32) (string, error) {
	body, _ := json.Marshal(map[string]any{"order_id": orderID, "amount_cents": totalCents})
	req, _ := http.NewRequestWithContext(ctx, "POST", a.PaymentURL+"/charges", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "order-"+orderID)
	resp, err := a.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 202 {
		return "", fmt.Errorf("payment status %d", resp.StatusCode)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.ID, nil
}

// MarkOrderStatusActivity writes the terminal status of an order.
func (a *Activities) MarkOrderStatusActivity(ctx context.Context, orderID, status string) error {
	id, err := uuid.Parse(orderID)
	if err != nil {
		return err
	}
	_, err = a.DB.Exec(ctx, `update orders set status = $2 where id = $1`, id, status)
	return err
}
