// Package redact turns personal data into something safe to record (ADR-0500).
//
// PII is never written to logs, metrics, traces, or profiles. The rule is
// absolute, and a rule that offers nothing in place of the value it forbids is a
// rule people work around: the field gets logged raw because the alternative was
// losing the ability to tell two records apart. Every formatter here keeps the
// property an operator actually needs — are these two the same subject, which
// domain, roughly how long — and destroys the value itself.
//
// Two shapes, and the difference is the whole design:
//
//   - Token derives a STABLE pseudonym. Two records for one subject carry one
//     token, so a session can be followed end to end without the identity ever
//     being written. It is keyed, because an unkeyed hash of an email address is
//     reversible by anyone holding a list of email addresses.
//   - Mask keeps a SHAPE and no content. Used where correlation is not wanted
//     and the only question is whether the field was populated and plausible.
//
// A token is not anonymous data. It is a pseudonym, still personal data under
// ADR-0301, and it is erased with the subject through the same workflow — which
// is why the key is per-environment and rotating it retires every token derived
// under it.
package redact

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base32"
	"net/netip"
	"os"
	"strings"
	"sync"
	"unicode/utf8"
)

// The pseudonym key. Read once from the environment the sops-operator delivers
// it in (ADR-0202), and empty in local development, where the tokens are
// throwaway like every other local credential (ADR-0205).
//
// Empty is a working key, not a failure: a token derived under an empty key is
// still stable within its environment, and refusing to start over a missing key
// would make an observability helper an availability dependency.
var key = sync.OnceValue(
	func() []byte {
		return []byte(os.Getenv("REDACT_TOKEN_KEY"))
	},
)

// tokenLen is 16 base32 characters — 80 bits of the digest.
//
// Short enough to read in a log line, long enough that collisions are not the
// reason two records look like one subject. The full digest carries no more
// meaning here: nothing verifies a token, so the remaining bits only make the
// line harder to scan.
const tokenLen = 16

var enc = base32.StdEncoding.WithPadding(base32.NoPadding)

// Token derives the stable pseudonym for a value.
//
// The empty string maps to the empty string rather than to a token, so "absent"
// and "present" stay distinguishable — a token for a missing value would read as
// a subject that does not exist.
func Token(v string) string {
	if v == "" {
		return ""
	}
	mac := hmac.New(sha256.New, key())
	// hash.Hash.Write never returns an error; the interface carries one because
	// io.Writer does.
	_, _ = mac.Write([]byte(v))
	return strings.ToLower(enc.EncodeToString(mac.Sum(nil))[:tokenLen])
}

// Email keeps the domain and tokenises the mailbox.
//
// The domain is what an operator reads for — one tenant, one identity provider,
// one bounced destination — and it identifies an organisation rather than a
// person. Anything that does not parse as an address is masked whole rather than
// split on a guess, because a malformed address is often malformed by carrying
// something else entirely.
func Email(v string) string {
	at := strings.LastIndex(v, "@")
	if at <= 0 || at == len(v)-1 {
		return Mask(v)
	}
	return Token(strings.ToLower(v[:at])) + "@" + strings.ToLower(v[at+1:])
}

// IP reduces an address to the network an operator can act on: a /24 for IPv4, a
// /48 for IPv6, which is the smallest block a single subscriber is allocated.
//
// The reduced form is what rate limiting, geography, and abuse investigation
// need. Raw addresses are stored nowhere on the analytics path at all
// (ADR-0700); this is for the operational paths where the network still matters.
func IP(v string) string {
	addr, err := netip.ParseAddr(v)
	if err != nil {
		return Mask(v)
	}
	bits := 24
	if addr.Is6() && !addr.Is4In6() {
		bits = 48
	}
	p, err := addr.Prefix(bits)
	if err != nil {
		return Mask(v)
	}
	return p.String()
}

// Mask replaces a value with its length class.
//
// Not the length: an exact length is a fingerprint, and for short fields it is
// close to the value. The classes are wide enough that they identify nobody and
// narrow enough to answer the question a masked field is read for — whether what
// arrived was the shape the caller was supposed to send.
func Mask(v string) string {
	switch n := utf8.RuneCountInString(v); {
	case n == 0:
		return ""
	case n < 8:
		return "[redacted:short]"
	case n < 64:
		return "[redacted:medium]"
	default:
		return "[redacted:long]"
	}
}
