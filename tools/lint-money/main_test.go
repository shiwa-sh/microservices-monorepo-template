package main

import "testing"

// The vocabulary match is on words, not substrings. Both directions matter: a
// substring match makes `pricing_enabled` a monetary field and the gate gets
// bypassed; too strict a match misses `unitPrice` and the gate does nothing.
func TestIsMoneyName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want bool
	}{
		{wordPrice, true},
		{"unitPrice", true},
		{"total_amount", true},
		{"AmountCents", true},
		{"refund_total", true},
		{"totals", false},
		{"pricing_enabled", false},
		{"total_count", false},
		{"quantity", false},
		{"name", false},
	}
	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				t.Parallel()
				if got := isMoneyName(tc.name); got != tc.want {
					t.Fatalf("isMoneyName(%q) = %v, want %v", tc.name, got, tc.want)
				}
			},
		)
	}
}

func TestSplitIdentifier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want []string
	}{
		{"price_cents", []string{wordPrice, "cents"}},
		{"unitPrice", []string{"unit", wordPrice}},
		{"AmountCents", []string{wordAmount, "cents"}},
		{"price.amount", []string{wordPrice, wordAmount}},
	}
	for _, tc := range tests {
		t.Run(
			tc.in,
			func(t *testing.T) {
				t.Parallel()
				got := splitIdentifier(tc.in)
				if len(got) != len(tc.want) {
					t.Fatalf("splitIdentifier(%q) = %v, want %v", tc.in, got, tc.want)
				}
				for i := range got {
					if got[i] != tc.want[i] {
						t.Fatalf("splitIdentifier(%q) = %v, want %v", tc.in, got, tc.want)
					}
				}
			},
		)
	}
}
