package redact_test

import (
	"strings"
	"testing"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/observability/redact"
)

func TestTokenIsStableAndDoesNotCarryTheValue(t *testing.T) {
	t.Parallel()
	a := redact.Token("user@example.com")
	if a != redact.Token("user@example.com") {
		t.Fatal("token is not stable: two records for one subject would not correlate")
	}
	if a == redact.Token("other@example.com") {
		t.Fatal("two subjects share a token")
	}
	if strings.Contains(a, "user") || strings.Contains(a, "example") {
		t.Fatalf("token carries the value: %q", a)
	}
	if redact.Token("") != "" {
		t.Fatal("an absent value tokenised into a subject that does not exist")
	}
}

func TestEmailKeepsTheDomainOnly(t *testing.T) {
	t.Parallel()
	got := redact.Email("Someone@Example.com")
	if !strings.HasSuffix(got, "@example.com") {
		t.Fatalf("domain not preserved: %q", got)
	}
	if strings.Contains(got, "omeone") {
		t.Fatalf("mailbox survived: %q", got)
	}
	if got != redact.Email("someone@example.com") {
		t.Fatal("case decided the token, so one subject produces two")
	}
	for _, bad := range []string{"not-an-address", "@example.com", "trailing@"} {
		if !strings.HasPrefix(redact.Email(bad), "[redacted:") {
			t.Fatalf("malformed address was split rather than masked: %q", bad)
		}
	}
}

func TestIPReducesToTheActionableNetwork(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"203.0.113.42":    "203.0.113.0/24",
		"2001:db8:1:2::1": "2001:db8:1::/48",
		"not-an-ip":       "[redacted:medium]",
	}
	for in, want := range cases {
		if got := redact.IP(in); got != want {
			t.Fatalf("IP(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMaskKeepsNoContent(t *testing.T) {
	t.Parallel()
	if redact.Mask("") != "" {
		t.Fatal("an empty field became a populated one")
	}
	if redact.Mask("short") == redact.Mask(strings.Repeat("x", 100)) {
		t.Fatal("every length collapsed to one class, so the shape check answers nothing")
	}
	if strings.Contains(redact.Mask("4111111111111111"), "4111") {
		t.Fatal("mask leaked the value")
	}
}
