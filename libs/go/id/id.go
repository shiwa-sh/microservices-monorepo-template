// Package id generates and encodes entity identifiers (ADR-0003).
//
// The value is a UUIDv7 ([RFC 9562]) and the surface form is the [TypeID]
// convention: a type prefix, an underscore, and the UUID in 26 characters of
// Crockford base32.
//
//	order_01j8xk7m3q0000000000000000
//	{prefix}_{UUIDv7 in 26 characters of base32}
//
// Two properties are being bought at once, and they pull in opposite directions:
//
//   - UUIDv7 is time-ordered, so a primary-key index appends rather than fragments.
//     That is why storage keeps the bare uuid column.
//   - The prefix names the type at every boundary, so a value in a support ticket
//     says what it is, a log line is greppable by type, and a function signature
//     cannot accept an order_ where a product_ belongs.
//
// The conversion happens at the TRANSPORT boundary and nowhere else. Storage sees
// [ID.UUID]; the wire sees [ID.String]. Generation is here rather than a Postgres
// default so the service holds the identifier before the insert — an identifier the
// database mints is one the caller cannot log, trace, or return until the write
// succeeds.
//
// An identifier is opaque to a consumer ([AIP-122]): nothing outside this package
// parses, orders, or constructs one. And an unguessable identifier is not an
// authorisation control — every read and mutation is authorised as though the
// identifier were public, because it is (ADR-0304).
//
// [RFC 9562]: https://www.rfc-editor.org/rfc/rfc9562
// [TypeID]: https://github.com/jetify-com/typeid
// [AIP-122]: https://google.aip.dev/122
package id

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// alphabet is Crockford base32 in TypeID's ordering: lowercase, and without i, l,
// o, or u, so a transcribed identifier cannot be misread as a digit or an obscenity.
const alphabet = "0123456789abcdefghjkmnpqrstvwxyz"

// encodedLen is 26: 128 bits at 5 bits per character needs 25.6 characters, and the
// leading one carries the 3 spare bits.
const encodedLen = 26

// maxPrefixLen bounds the prefix at the same 63 characters a DNS label allows,
// which is the tightest bound any downstream consumer of these strings imposes.
const maxPrefixLen = 63

var (
	ErrEmptyPrefix    = errors.New("id: empty prefix")
	ErrPrefixTooLong  = fmt.Errorf("id: prefix longer than %d characters", maxPrefixLen)
	ErrPrefixCharset  = errors.New("id: prefix must be lowercase a-z and underscores")
	ErrMissingPrefix  = errors.New("id: no prefix separator")
	ErrSuffixLength   = fmt.Errorf("id: suffix must be %d characters", encodedLen)
	ErrSuffixCharset  = errors.New("id: suffix is not Crockford base32")
	ErrSuffixRange    = errors.New("id: suffix overflows 128 bits")
	ErrPrefixMismatch = errors.New("id: prefix does not match the expected type")
)

// decodeTable maps a base32 character to its value; 0xff marks an invalid one.
var decodeTable = func() [256]byte {
	var t [256]byte
	for i := range t {
		t[i] = 0xff
	}
	for i, c := range alphabet {
		t[c] = byte(i)
	}
	return t
}()

// ID is a type-prefixed UUIDv7. The zero value is not a valid identifier.
//
// The receivers are mixed deliberately, which recvcheck flags: UnmarshalText must
// take a pointer to satisfy encoding.TextUnmarshaler, and everything else takes a
// value so that id.MustNew("order").String() works on a non-addressable result.
// Making them uniformly pointers would break that call at every use site.
//
//nolint:recvcheck // encoding.TextUnmarshaler requires the pointer receiver
type ID struct {
	prefix string
	uuid   uuid.UUID
}

// New mints a new identifier of the given type. The prefix is the singular
// snake_case form of the resource's collection noun — /orders yields "order".
func New(prefix string) (ID, error) {
	err := ValidatePrefix(prefix)
	if err != nil {
		return ID{}, err
	}
	u, err := uuid.NewV7()
	if err != nil {
		return ID{}, fmt.Errorf("id: generate uuidv7: %w", err)
	}
	return ID{prefix: prefix, uuid: u}, nil
}

// MustNew is New for package-level initialisation and tests, where a failure is a
// programming error rather than a runtime condition.
func MustNew(prefix string) ID {
	v, err := New(prefix)
	if err != nil {
		panic(err)
	}
	return v
}

// From wraps an existing UUID — the read path, where storage returns the bare
// column and the transport boundary adds the prefix back.
func From(prefix string, u uuid.UUID) (ID, error) {
	err := ValidatePrefix(prefix)
	if err != nil {
		return ID{}, err
	}
	return ID{prefix: prefix, uuid: u}, nil
}

// Parse decodes a wire-form identifier and checks it carries the expected prefix.
// Passing the expected type is the point: it is what stops an order_ being accepted
// where a product_ belongs, which a bare UUID cannot express.
func Parse(prefix, s string) (ID, error) {
	got, err := ParseAny(s)
	if err != nil {
		return ID{}, err
	}
	if got.prefix != prefix {
		return ID{}, fmt.Errorf("%w: want %q, got %q", ErrPrefixMismatch, prefix, got.prefix)
	}
	return got, nil
}

// ParseAny decodes a wire-form identifier without checking its type. Use it only
// where the type genuinely is not known ahead of time, such as a log processor.
func ParseAny(s string) (ID, error) {
	sep := strings.LastIndex(s, "_")
	if sep < 0 {
		return ID{}, ErrMissingPrefix
	}
	prefix, suffix := s[:sep], s[sep+1:]

	err := ValidatePrefix(prefix)
	if err != nil {
		return ID{}, err
	}
	u, err := decode(suffix)
	if err != nil {
		return ID{}, err
	}
	return ID{prefix: prefix, uuid: u}, nil
}

// ValidatePrefix reports whether a prefix is well-formed.
func ValidatePrefix(prefix string) error {
	switch {
	case prefix == "":
		return ErrEmptyPrefix
	case len(prefix) > maxPrefixLen:
		return ErrPrefixTooLong
	}
	// An underscore is allowed inside the prefix but not at either end: the LAST
	// underscore separates prefix from suffix, so a trailing one would make the
	// prefix ambiguous.
	if strings.HasPrefix(prefix, "_") || strings.HasSuffix(prefix, "_") {
		return ErrPrefixCharset
	}
	for _, r := range prefix {
		if (r < 'a' || r > 'z') && r != '_' {
			return ErrPrefixCharset
		}
	}
	return nil
}

// Prefix is the type this identifier names.
func (i ID) Prefix() string { return i.prefix }

// UUID is the stored value: what goes in the uuid column, and nowhere else.
func (i ID) UUID() uuid.UUID { return i.uuid }

// IsZero reports whether this is the zero value rather than a real identifier.
func (i ID) IsZero() bool { return i.prefix == "" && i.uuid == uuid.Nil }

// String is the wire form.
func (i ID) String() string {
	if i.prefix == "" {
		return ""
	}
	return i.prefix + "_" + encode(i.uuid)
}

// MarshalText makes ID work anywhere encoding/json reaches, so a handler never
// converts by hand.
func (i ID) MarshalText() ([]byte, error) { return []byte(i.String()), nil }

// UnmarshalText accepts any well-formed identifier. The TYPE check lives in Parse,
// because a struct field's expected prefix is not visible from here.
func (i *ID) UnmarshalText(b []byte) error {
	v, err := ParseAny(string(b))
	if err != nil {
		return err
	}
	*i = v
	return nil
}

// encode renders the 128-bit value as 26 base32 characters, most significant first.
func encode(u uuid.UUID) string {
	var out [encodedLen]byte
	// The first character carries the top 3 bits; the remaining 25 carry 5 each.
	out[0] = alphabet[(u[0]&0xe0)>>5]

	// Walk the 128 bits as a bit offset so the 5-bit groups cross byte boundaries
	// without a special case per group.
	bit := 3
	for i := 1; i < encodedLen; i++ {
		var v byte
		for range 5 {
			byteIdx, bitIdx := bit/8, uint(bit%8)
			v = v<<1 | (u[byteIdx]>>(7-bitIdx))&1
			bit++
		}
		out[i] = alphabet[v]
	}
	return string(out[:])
}

// decode is encode's inverse.
func decode(s string) (uuid.UUID, error) {
	if len(s) != encodedLen {
		return uuid.Nil, ErrSuffixLength
	}
	// 26 characters hold 130 bits and a UUID is 128, so the first character cannot
	// exceed 7 — anything larger is a value that does not fit and would silently
	// wrap if the bits were simply shifted in.
	first := decodeTable[s[0]]
	if first == 0xff {
		return uuid.Nil, ErrSuffixCharset
	}
	if first > 7 {
		return uuid.Nil, ErrSuffixRange
	}

	var u uuid.UUID
	u[0] = first << 5

	bit := 3
	for i := 1; i < encodedLen; i++ {
		v := decodeTable[s[i]]
		if v == 0xff {
			return uuid.Nil, ErrSuffixCharset
		}
		for shift := 4; shift >= 0; shift-- {
			byteIdx, bitIdx := bit/8, uint(bit%8)
			u[byteIdx] |= ((v >> uint(shift)) & 1) << (7 - bitIdx)
			bit++
		}
	}
	return u, nil
}
