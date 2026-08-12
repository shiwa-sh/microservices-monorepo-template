package money

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// The database and wire surfaces (ADR-0100, ADR-0303).
//
// Both carry the amount as a STRING and the currency as a separate column or
// member. Neither carries a float at any point: a numeric column read into a
// float64 has already lost the property the numeric column existed to provide.
//
// The currency is NOT part of the scanned value, because a Postgres numeric holds
// only the amount. A table storing money carries two columns — amount numeric(19,4)
// and currency char(3) — and the store composes them. [Amount.Scan] therefore
// produces an amount whose currency the caller must supply through [Amount.WithCurrency].

// Value implements driver.Valuer: the amount as a decimal string, which pgx sends
// to a numeric column without an intermediate float.
func (a Amount) Value() (driver.Value, error) {
	if !a.Valid() {
		return nil, nil //nolint:nilnil // a NULL numeric column is the zero Amount
	}
	return a.String(), nil
}

// Scan implements sql.Scanner for a numeric column. The result carries no currency
// — see [Amount.WithCurrency].
func (a *Amount) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*a = Amount{}
		return nil
	case string:
		return a.scanString(v)
	case []byte:
		return a.scanString(string(v))
	case float64:
		// Reachable only if a column is float8 rather than numeric, which ADR-0100
		// forbids. Failing loudly here is the point: silently accepting it would make
		// the schema defect invisible until a total came out wrong.
		return fmt.Errorf("money: refusing to scan a float64 (%v) — the column must be numeric, not float8", v)
	default:
		return fmt.Errorf("money: cannot scan %T", src)
	}
}

func (a *Amount) scanString(s string) error {
	// A placeholder currency is used because Parse requires one and the column does
	// not carry it; WithCurrency replaces it before the value is used.
	parsed, err := Parse(s, "XXX")
	if err != nil {
		return fmt.Errorf("money: scan %q: %w", s, err)
	}
	*a = parsed
	return nil
}

// WithCurrency returns the amount with its currency set, which is how a store
// reassembles a value from the amount and currency columns.
func (a Amount) WithCurrency(currency string) (Amount, error) {
	err := ValidateCurrency(currency)
	if err != nil {
		return Amount{}, err
	}
	return Amount{units: a.units, currency: currency}, nil
}

// wire is the JSON shape, matching the Money component in
// tools/codegen/shared-components.yaml.
type wire struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

// MarshalJSON writes {"amount": "1299.00", "currency": "EUR"}.
//
// The amount is a string on purpose. A JSON number is an IEEE-754 double by the
// time a TypeScript client reads it, and no Go-side type prevents that — the wire
// form is what decides it.
func (a Amount) MarshalJSON() ([]byte, error) {
	if !a.Valid() {
		return []byte("null"), nil
	}
	data, err := json.Marshal(wire{Amount: a.String(), Currency: a.currency})
	if err != nil {
		return nil, fmt.Errorf("money: marshal: %w", err)
	}
	return data, nil
}

// UnmarshalJSON reads the same shape, rejecting a JSON number outright: accepting
// one would silently take the precision loss the string form exists to prevent.
func (a *Amount) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*a = Amount{}
		return nil
	}
	var w wire
	err := json.Unmarshal(data, &w)
	if err != nil {
		return fmt.Errorf("money: unmarshal: %w", err)
	}
	parsed, err := Parse(w.Amount, w.Currency)
	if err != nil {
		return err
	}
	*a = parsed
	return nil
}
