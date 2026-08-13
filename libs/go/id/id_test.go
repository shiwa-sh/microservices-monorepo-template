package id_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/id"
)

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	original := id.MustNew("order")
	parsed, err := id.Parse("order", original.String())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if parsed.UUID() != original.UUID() {
		t.Errorf("uuid = %v, want %v", parsed.UUID(), original.UUID())
	}
	if parsed.String() != original.String() {
		t.Errorf("string = %q, want %q", parsed, original)
	}
}

// The encoding must cover the whole 128-bit space, not just the values a v7
// generator happens to produce — a read path parses whatever is in the column.
func TestRoundTripAcrossTheValueSpace(t *testing.T) {
	t.Parallel()

	cases := map[string]uuid.UUID{
		"zero":     {},
		"max":      {0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		"low bit":  {0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
		"high bit": {0x80, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		"walking":  {0x01, 0x02, 0x04, 0x08, 0x10, 0x20, 0x40, 0x80, 0x81, 0x42, 0x24, 0x18, 0x99, 0x5a, 0xa5, 0x3c},
	}
	for name, u := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				t.Parallel()
				original, err := id.From("thing", u)
				if err != nil {
					t.Fatalf("From: %v", err)
				}
				parsed, err := id.Parse("thing", original.String())
				if err != nil {
					t.Fatalf("Parse(%q): %v", original, err)
				}
				if parsed.UUID() != u {
					t.Errorf("uuid = %v, want %v", parsed.UUID(), u)
				}
			},
		)
	}
}

func TestWireForm(t *testing.T) {
	t.Parallel()

	v, err := id.From("order", uuid.UUID{})
	if err != nil {
		t.Fatalf("From: %v", err)
	}
	const want = "order_00000000000000000000000000"
	if v.String() != want {
		t.Errorf("string = %q, want %q", v, want)
	}
	if len(strings.SplitN(v.String(), "_", 2)[1]) != 26 {
		t.Errorf("suffix is not 26 characters: %q", v)
	}
}

// The whole reason the prefix exists: a generated client cannot pass an order_
// where a product_ belongs.
func TestParseRejectsTheWrongType(t *testing.T) {
	t.Parallel()

	order := id.MustNew("order")
	_, err := id.Parse("product", order.String())
	if err == nil {
		t.Fatal("Parse accepted an order_ as a product_")
	}
}

func TestParseRejectsMalformed(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"no separator":     "order01j8xk7m3q0000000000000000",
		"empty prefix":     "_01j8xk7m3q0000000000000000",
		"short suffix":     "order_01j8xk7m3q",
		"long suffix":      "order_01j8xk7m3q00000000000000000",
		"excluded letter":  "order_01j8xk7m3q000000000000000i",
		"uppercase suffix": "order_01J8XK7M3Q0000000000000000",
		"uppercase prefix": "Order_01j8xk7m3q0000000000000000",
		"digit in prefix":  "order2_01j8xk7m3q0000000000000000",
		// 26 characters hold 130 bits; a leading character above 7 does not fit in
		// 128 and must be rejected rather than silently truncated.
		"overflows 128 bits": "order_81j8xk7m3q0000000000000000",
	}
	for name, s := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				t.Parallel()
				_, err := id.ParseAny(s)
				if err == nil {
					t.Errorf("ParseAny(%q) accepted a malformed identifier", s)
				}
			},
		)
	}
}

// UUIDv7 is time-ordered, and the encoding must preserve that ordering or the
// reason for choosing v7 is lost at the boundary.
func TestEncodingPreservesTimeOrdering(t *testing.T) {
	t.Parallel()

	previous := id.MustNew("order").String()
	for range 100 {
		next := id.MustNew("order").String()
		if next < previous {
			t.Fatalf("ordering broken: %q sorts before %q", next, previous)
		}
		previous = next
	}
}

func TestJSONUsesTheWireForm(t *testing.T) {
	t.Parallel()

	type payload struct {
		ID id.ID `json:"id"`
	}
	original := id.MustNew("product")

	encoded, err := json.Marshal(payload{ID: original})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"`+original.String()+`"`) {
		t.Fatalf("json = %s, want the wire form", encoded)
	}

	var decoded payload
	err = json.Unmarshal(encoded, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.ID.UUID() != original.UUID() {
		t.Errorf("uuid = %v, want %v", decoded.ID.UUID(), original.UUID())
	}
}

// Vectors from the TypeID specification. Checking the encoding against the standard
// rather than only against itself is what makes the surface form interoperable: a
// round-trip test passes just as happily on a private encoding.
func TestTypeIDSpecVectors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		uuid string
		want string
	}{
		{"00000000-0000-0000-0000-000000000000", "00000000000000000000000000"},
		{"00000000-0000-0000-0000-000000000001", "00000000000000000000000001"},
		{"01890a5d-ac96-774b-bcce-b302099a8057", "01h455vb4pex5vsknk084sn02q"},
		{"ffffffff-ffff-ffff-ffff-ffffffffffff", "7zzzzzzzzzzzzzzzzzzzzzzzzz"},
	}
	for _, c := range cases {
		t.Run(
			c.uuid,
			func(t *testing.T) {
				t.Parallel()
				v, err := id.From("thing", uuid.MustParse(c.uuid))
				if err != nil {
					t.Fatalf("From: %v", err)
				}
				want := "thing_" + c.want
				if v.String() != want {
					t.Errorf("string = %q, want %q", v, want)
				}
			},
		)
	}
}

func TestPrefixValidation(t *testing.T) {
	t.Parallel()

	valid := []string{"order", "product", "payment_method", "a"}
	for _, p := range valid {
		err := id.ValidatePrefix(p)
		if err != nil {
			t.Errorf("ValidatePrefix(%q) = %v, want nil", p, err)
		}
	}
	invalid := []string{"", "Order", "order-item", "order1", "_order", "order_", strings.Repeat("a", 64)}
	for _, p := range invalid {
		err := id.ValidatePrefix(p)
		if err == nil {
			t.Errorf("ValidatePrefix(%q) = nil, want an error", p)
		}
	}
}

// MustFrom is what every transport read path calls, so both its outcomes are
// pinned: the value it returns must match From's, and a bad prefix must panic
// rather than yield an identifier whose type nothing checked.
func TestMustFrom(t *testing.T) {
	t.Parallel()

	u := uuid.MustParse("019ff54e-a5c0-7dc3-808f-856854ec5f86")
	want, err := id.From("product", u)
	if err != nil {
		t.Fatalf("From: %v", err)
	}
	if got := id.MustFrom("product", u); got.String() != want.String() {
		t.Errorf("MustFrom = %q, want %q", got, want)
	}

	defer func() {
		if recover() == nil {
			t.Error("MustFrom with an invalid prefix did not panic")
		}
	}()
	id.MustFrom("Product", u)
}

func TestZeroValue(t *testing.T) {
	t.Parallel()

	var v id.ID
	if !v.IsZero() {
		t.Error("zero value does not report IsZero")
	}
	if v.String() != "" {
		t.Errorf("zero value string = %q, want empty", v)
	}
}
