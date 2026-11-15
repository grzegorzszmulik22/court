package ledger

import (
	"math"
	"sort"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
)

// Canonical serializes v as RFC 8785 (JCS) bytes: no whitespace, properties
// sorted by UTF-16 code units, minimal string escaping, ES6 numbers.
// Numbers are parsed as IEEE-754 doubles; non-finite values are an error.
func Canonical(v *Value) ([]byte, error) {
	b := make([]byte, 0, 256)
	if err := canon(v, &b); err != nil {
		return nil, err
	}
	return b, nil
}

// MustCanonical is Canonical for values known to be safe (e.g. built by us).
func MustCanonical(v *Value) []byte {
	b, err := Canonical(v)
	if err != nil {
		panic(err)
	}
	return b
}

func canon(v *Value, b *[]byte) error {
	switch v.Kind {
	case Null:
		*b = append(*b, "null"...)
	case Bool:
		if v.Bool {
			*b = append(*b, "true"...)
		} else {
			*b = append(*b, "false"...)
		}
	case Number:
		f, err := strconv.ParseFloat(v.Num, 64)
		if err != nil {
			return jerr("number %q out of IEEE-754 range", v.Num)
		}
		*b = appendNumber(*b, f)
	case String:
		*b = appendString(*b, v.Str)
	case Array:
		*b = append(*b, '[')
		for i, e := range v.Arr {
			if i > 0 {
				*b = append(*b, ',')
			}
			if err := canon(e, b); err != nil {
				return err
			}
		}
		*b = append(*b, ']')
	case Object:
		members := make([]Member, len(v.Obj))
		copy(members, v.Obj)
		sort.Slice(members, func(i, j int) bool {
			return keyLess(members[i].Key, members[j].Key)
		})
		*b = append(*b, '{')
		for i, m := range members {
			if i > 0 {
				*b = append(*b, ',')
			}
			*b = appendString(*b, m.Key)
			*b = append(*b, ':')
			if err := canon(m.Val, b); err != nil {
				return err
			}
		}
		*b = append(*b, '}')
	}
	return nil
}

// keyLess compares two property names as arrays of UTF-16 code units
// (RFC 8785 section 3.2.3).
func keyLess(a, b string) bool {
	aa := utf16.Encode([]rune(a))
	bb := utf16.Encode([]rune(b))
	for i := 0; i < len(aa) && i < len(bb); i++ {
		if aa[i] != bb[i] {
			return aa[i] < bb[i]
		}
	}
	return len(aa) < len(bb)
}

var hexLower = "0123456789abcdef"

// appendString serializes a string per RFC 8785 section 3.2.2.2:
// only '"', '\\' and U+0000..U+001F are escaped (\b \t \n \f \r, else \uhhhh
// lowercase). Everything else is emitted as raw UTF-8.
func appendString(b []byte, s string) []byte {
	b = append(b, '"')
	for _, r := range s {
		switch r {
		case '"':
			b = append(b, '\\', '"')
		case '\\':
			b = append(b, '\\', '\\')
		case '\b':
			b = append(b, '\\', 'b')
		case '\t':
			b = append(b, '\\', 't')
		case '\n':
			b = append(b, '\\', 'n')
		case '\f':
			b = append(b, '\\', 'f')
		case '\r':
			b = append(b, '\\', 'r')
		default:
			if r < 0x20 {
				b = append(b, '\\', 'u', '0', '0', hexLower[r>>4], hexLower[r&15])
			} else {
				b = utf8.AppendRune(b, r)
			}
		}
	}
	return append(b, '"')
}

// appendNumber serializes a double per RFC 8785 section 3.2.2.3 (the
// ECMAScript Number-to-String algorithm). Go's shortest round-trip digits are
// re-laid-out under the ES6 rule: exponential form iff |f| < 1e-6 or >= 1e21.
func appendNumber(b []byte, f float64) []byte {
	if f == 0 { // covers -0.0
		return append(b, '0')
	}
	a := math.Abs(f)
	if a < 1e-6 || a >= 1e21 {
		n := len(b)
		b = strconv.AppendFloat(b, f, 'e', -1, 64)
		// Go emits two-digit exponents ("e-07"); JCS wants "e-7".
		m := len(b)
		if m-n >= 4 && b[m-4] == 'e' && b[m-3] == '-' && b[m-2] == '0' {
			b[m-2] = b[m-1]
			b = b[:m-1]
		}
		return b
	}
	return strconv.AppendFloat(b, f, 'f', -1, 64)
}
