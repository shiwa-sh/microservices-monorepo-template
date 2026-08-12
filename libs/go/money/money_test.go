package money_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/money"
)

const (
	smallestUnit = "0.0001" // one unit at the internal scale
	quarter      = "0.25"
	halfOfUnit   = "0.00005" // exactly half a unit at the internal scale: the tie case
	one          = "1.00"
)

func TestParseAndString(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"0":                "0",
		"1299.00":          "1299",
		"1299.5":           "1299.5",
		smallestUnit:       smallestUnit,
		"-42.75":           "-42.75",
		"-" + smallestUnit: "-" + smallestUnit,
		"999999999":        "999999999",
	}
	for input, want := range cases {
		t.Run(
			input,
			func(t *testing.T) {
				t.Parallel()
				a := money.MustParse(input, "EUR")
				if a.String() != want {
					t.Errorf("String() = %q, want %q", a.String(), want)
				}
			},
		)
	}
}

// Precision the caller supplied must not be silently dropped: a rate with five
// decimal places is a rate someone meant, and truncating it is a wrong total.
func TestParseRejectsExcessPrecision(t *testing.T) {
	t.Parallel()

	_, err := money.Parse("1.000005", "EUR")
	if err == nil {
		t.Fatal("Parse accepted more precision than it can hold")
	}
}

func TestParseRejectsMalformed(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"", "abc", "1,299.00", "1.2.3", "1e5", "+5", " 5", "5 "} {
		_, err := money.Parse(input, "EUR")
		if err == nil {
			t.Errorf("Parse(%q) accepted a non-amount", input)
		}
	}
}

func TestCurrencyValidation(t *testing.T) {
	t.Parallel()

	for _, c := range []string{"", "eur", "EURO", "EU", "E1R"} {
		_, err := money.Parse(one, c)
		if err == nil {
			t.Errorf("Parse accepted currency %q", c)
		}
	}
}

// The property the type exists for. A mismatch is reachable from request data, so
// it is an error rather than a panic.
func TestArithmeticRejectsCurrencyMismatch(t *testing.T) {
	t.Parallel()

	eur := money.MustParse("10.00", "EUR")
	usd := money.MustParse("10.00", "USD")

	_, err := eur.Add(usd)
	if !errors.Is(err, money.ErrCurrencyMismatch) {
		t.Errorf("Add across currencies = %v, want ErrCurrencyMismatch", err)
	}
	_, err = eur.Sub(usd)
	if !errors.Is(err, money.ErrCurrencyMismatch) {
		t.Errorf("Sub across currencies = %v, want ErrCurrencyMismatch", err)
	}
	_, err = eur.Cmp(usd)
	if !errors.Is(err, money.ErrCurrencyMismatch) {
		t.Errorf("Cmp across currencies = %v, want ErrCurrencyMismatch", err)
	}
	// Equal is the exception: the answer is simply false.
	if eur.Equal(usd) {
		t.Error("Equal returned true across currencies")
	}
}

func TestAddAndSub(t *testing.T) {
	t.Parallel()

	a := money.MustParse("10.50", "EUR")
	b := money.MustParse("4.25", "EUR")

	sum, err := a.Add(b)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if sum.String() != "14.75" {
		t.Errorf("Add = %q, want 14.75", sum)
	}

	diff, err := a.Sub(b)
	if err != nil {
		t.Fatalf("Sub: %v", err)
	}
	if diff.String() != "6.25" {
		t.Errorf("Sub = %q, want 6.25", diff)
	}
}

// The operation a naive division gets wrong: the parts must sum back exactly, and
// the indivisible remainder is money that has to land somewhere.
func TestSplitSumsBackExactly(t *testing.T) {
	t.Parallel()

	cases := []struct {
		amount string
		n      int
		want   []string
	}{
		{"10.00", 3, []string{"3.3334", "3.3333", "3.3333"}},
		{"0.03", 2, []string{"0.015", "0.015"}},
		{one, 4, []string{quarter, quarter, quarter, quarter}},
		{"-10.00", 3, []string{"-3.3334", "-3.3333", "-3.3333"}},
	}
	for _, c := range cases {
		t.Run(
			c.amount,
			func(t *testing.T) {
				t.Parallel()
				original := money.MustParse(c.amount, "EUR")
				parts, err := original.Split(c.n)
				if err != nil {
					t.Fatalf("Split: %v", err)
				}

				total := money.MustParse("0", "EUR")
				for i, p := range parts {
					if p.String() != c.want[i] {
						t.Errorf("part %d = %q, want %q", i, p, c.want[i])
					}
					total, err = total.Add(p)
					if err != nil {
						t.Fatalf("Add: %v", err)
					}
				}
				if !total.Equal(original) {
					t.Errorf("parts sum to %q, want %q — the remainder evaporated", total, original)
				}
			},
		)
	}
}

// HalfEven does not drift; HalfUp biases every tie away from zero, which across
// many rows is a real sum rather than a rounding artefact.
func TestRoundingModes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		amount   string
		rate     string
		mode     money.Rounding
		want     string
		modeName string
	}{
		{one, halfOfUnit, money.HalfEven, "0", "HalfEven ties to even (0)"},
		{"3.00", halfOfUnit, money.HalfEven, "0.0002", "HalfEven ties to even (2)"},
		{one, halfOfUnit, money.HalfUp, "0.0001", "HalfUp ties away"},
		{one, "0.00009", money.Down, "0", "Down truncates"},
	}
	for _, c := range cases {
		t.Run(
			c.modeName,
			func(t *testing.T) {
				t.Parallel()
				got, err := money.MustParse(c.amount, "EUR").MulRate(c.rate, c.mode)
				if err != nil {
					t.Fatalf("MulRate: %v", err)
				}
				if got.String() != c.want {
					t.Errorf("= %q, want %q", got, c.want)
				}
			},
		)
	}
}

func TestFromMinorUnits(t *testing.T) {
	t.Parallel()

	eur, err := money.FromMinorUnits(129900, 2, "EUR")
	if err != nil {
		t.Fatalf("FromMinorUnits: %v", err)
	}
	if eur.String() != "1299" {
		t.Errorf("EUR = %q, want 1299", eur)
	}

	// JPY has no minor unit, so the same integer is a different amount.
	jpy, err := money.FromMinorUnits(129900, 0, "JPY")
	if err != nil {
		t.Fatalf("FromMinorUnits: %v", err)
	}
	if jpy.String() != "129900" {
		t.Errorf("JPY = %q, want 129900", jpy)
	}
}

// The wire form is a string. A JSON number would be a double by the time a
// TypeScript client read it, whatever the Go type is.
func TestJSONUsesAStringAmount(t *testing.T) {
	t.Parallel()

	encoded, err := json.Marshal(money.MustParse("1299.00", "EUR"))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	const want = `{"amount":"1299","currency":"EUR"}`
	if string(encoded) != want {
		t.Errorf("json = %s, want %s", encoded, want)
	}

	var decoded money.Amount
	err = json.Unmarshal(encoded, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !decoded.Equal(money.MustParse("1299", "EUR")) {
		t.Errorf("round trip = %q", decoded)
	}
}

func TestJSONRejectsANumericAmount(t *testing.T) {
	t.Parallel()

	var a money.Amount
	err := json.Unmarshal([]byte(`{"amount":1299.00,"currency":"EUR"}`), &a)
	if err == nil {
		t.Fatal("Unmarshal accepted a JSON number as an amount")
	}
}

func TestSQLRoundTrip(t *testing.T) {
	t.Parallel()

	original := money.MustParse("1299.75", "EUR")
	value, err := original.Value()
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	if value != "1299.75" {
		t.Errorf("Value = %v, want the decimal string", value)
	}

	var scanned money.Amount
	err = scanned.Scan(value)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	withCurrency, err := scanned.WithCurrency("EUR")
	if err != nil {
		t.Fatalf("WithCurrency: %v", err)
	}
	if !withCurrency.Equal(original) {
		t.Errorf("round trip = %q, want %q", withCurrency, original)
	}
}

// A float8 column is the schema defect ADR-0100 forbids. Accepting the scan would
// make it invisible until a total came out wrong.
func TestScanRefusesAFloat(t *testing.T) {
	t.Parallel()

	var a money.Amount
	err := a.Scan(1299.75)
	if err == nil {
		t.Fatal("Scan accepted a float64; the column must be numeric")
	}
}

func TestZeroValueIsNotUsable(t *testing.T) {
	t.Parallel()

	var a money.Amount
	if a.Valid() {
		t.Error("the zero value reports Valid")
	}
	if a.String() != "" {
		t.Errorf("zero value String = %q, want empty", a)
	}
}
