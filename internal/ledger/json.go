package ledger

import (
	"fmt"
	"unicode/utf8"
)

// Kind is the JSON value kind of a Value.
type Kind int

const (
	Null Kind = iota
	Bool
	Number
	String
	Array
	Object
)

// Value is a parsed JSON value. Object members keep source order in Obj;
// Number keeps the raw literal in Num.
type Value struct {
	Kind Kind
	Num  string
	Str  string
	Bool bool
	Arr  []*Value
	Obj  []Member
}

// Member is one object member (name and value) in source order.
type Member struct {
	Key string
	Val *Value
}

// parseError describes where strict decoding failed.
type parseError struct{ msg string }

func (e *parseError) Error() string { return e.msg }

// envelopeError is a violation of the section 11.1 envelope rules (unknown or
// missing field, malformed values) discovered after the JSON itself parsed.
type envelopeError struct{ msg string }

func (e *envelopeError) Error() string { return e.msg }

func eerr(format string, a ...any) error { return &envelopeError{fmt.Sprintf(format, a...)} }

// dupError is a duplicate-key error; callers map it to the envelope class.
type dupError struct{ key string }

func (e *dupError) Error() string { return fmt.Sprintf("duplicate key %q", e.key) }

// IsDup reports whether err is a duplicate-key error.
func IsDup(err error) bool { _, ok := err.(*dupError); return ok }

func jerr(format string, a ...any) error { return &parseError{fmt.Sprintf(format, a...)} }

// ParseStrict decodes one JSON document from data. It rejects duplicate object
// keys (I-JSON), lone surrogates, invalid UTF-8, raw control characters in
// strings, and trailing data.
func ParseStrict(data []byte) (*Value, error) {
	p := &parser{d: data}
	p.ws()
	v, err := p.value(0)
	if err != nil {
		return nil, err
	}
	p.ws()
	if p.i != len(p.d) {
		return nil, jerr("trailing data at byte %d", p.i)
	}
	return v, nil
}

const maxDepth = 400

type parser struct {
	d []byte
	i int
}

func (p *parser) ws() {
	for p.i < len(p.d) {
		switch p.d[p.i] {
		case ' ', '\t', '\n', '\r':
			p.i++
		default:
			return
		}
	}
}

func (p *parser) value(depth int) (*Value, error) {
	if depth > maxDepth {
		return nil, jerr("nesting deeper than %d", maxDepth)
	}
	if p.i >= len(p.d) {
		return nil, jerr("unexpected end of input")
	}
	switch c := p.d[p.i]; {
	case c == '{':
		return p.object(depth)
	case c == '[':
		return p.array(depth)
	case c == '"':
		s, err := p.str()
		if err != nil {
			return nil, err
		}
		return &Value{Kind: String, Str: s}, nil
	case c == 't':
		if err := p.lit("true"); err != nil {
			return nil, err
		}
		return &Value{Kind: Bool, Bool: true}, nil
	case c == 'f':
		if err := p.lit("false"); err != nil {
			return nil, err
		}
		return &Value{Kind: Bool}, nil
	case c == 'n':
		if err := p.lit("null"); err != nil {
			return nil, err
		}
		return &Value{Kind: Null}, nil
	case c == '-' || (c >= '0' && c <= '9'):
		n, err := p.number()
		if err != nil {
			return nil, err
		}
		return &Value{Kind: Number, Num: n}, nil
	default:
		return nil, jerr("unexpected byte %#x at %d", c, p.i)
	}
}

func (p *parser) lit(s string) error {
	if len(p.d)-p.i < len(s) || string(p.d[p.i:p.i+len(s)]) != s {
		return jerr("invalid literal at byte %d", p.i)
	}
	p.i += len(s)
	return nil
}

func (p *parser) object(depth int) (*Value, error) {
	p.i++ // '{'
	v := &Value{Kind: Object}
	seen := map[string]bool{}
	p.ws()
	if p.i < len(p.d) && p.d[p.i] == '}' {
		p.i++
		return v, nil
	}
	for {
		p.ws()
		if p.i >= len(p.d) || p.d[p.i] != '"' {
			return nil, jerr("object key expected at byte %d", p.i)
		}
		k, err := p.str()
		if err != nil {
			return nil, err
		}
		if seen[k] {
			return nil, &dupError{k}
		}
		seen[k] = true
		p.ws()
		if p.i >= len(p.d) || p.d[p.i] != ':' {
			return nil, jerr("':' expected at byte %d", p.i)
		}
		p.i++
		p.ws()
		m, err := p.value(depth + 1)
		if err != nil {
			return nil, err
		}
		v.Obj = append(v.Obj, Member{Key: k, Val: m})
		p.ws()
		if p.i >= len(p.d) {
			return nil, jerr("unclosed object")
		}
		switch p.d[p.i] {
		case ',':
			p.i++
		case '}':
			p.i++
			return v, nil
		default:
			return nil, jerr("',' or '}' expected at byte %d", p.i)
		}
	}
}

func (p *parser) array(depth int) (*Value, error) {
	p.i++ // '['
	v := &Value{Kind: Array}
	p.ws()
	if p.i < len(p.d) && p.d[p.i] == ']' {
		p.i++
		return v, nil
	}
	for {
		p.ws()
		e, err := p.value(depth + 1)
		if err != nil {
			return nil, err
		}
		v.Arr = append(v.Arr, e)
		p.ws()
		if p.i >= len(p.d) {
			return nil, jerr("unclosed array")
		}
		switch p.d[p.i] {
		case ',':
			p.i++
		case ']':
			p.i++
			return v, nil
		default:
			return nil, jerr("',' or ']' expected at byte %d", p.i)
		}
	}
}

// str parses a JSON string. Raw bytes must be valid UTF-8 and >= U+0020;
// escapes produce valid Unicode only (paired surrogates).
func (p *parser) str() (string, error) {
	p.i++ // '"'
	var out []byte
	start := p.i
	flush := func(to int) {
		if to > start {
			out = append(out, p.d[start:to]...)
		}
	}
	for p.i < len(p.d) {
		c := p.d[p.i]
		switch {
		case c == '"':
			flush(p.i)
			p.i++
			if out != nil {
				return string(out), nil
			}
			return string(p.d[start : p.i-1]), nil
		case c == '\\':
			flush(p.i)
			p.i++
			if p.i >= len(p.d) {
				return "", jerr("truncated escape")
			}
			e := p.d[p.i]
			p.i++
			switch e {
			case '"', '\\', '/':
				out = append(out, e)
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'u':
				r, err := p.u()
				if err != nil {
					return "", err
				}
				out = utf8.AppendRune(out, r)
			default:
				return "", jerr("bad escape \\%c", e)
			}
			start = p.i
		case c < 0x20:
			return "", jerr("raw control character %#x in string", c)
		case c < 0x80:
			p.i++
		default:
			r, n := utf8.DecodeRune(p.d[p.i:])
			if r == utf8.RuneError && n <= 1 {
				return "", jerr("invalid UTF-8 in string")
			}
			p.i += n
		}
	}
	return "", jerr("unterminated string")
}

// u parses one \uXXXX escape (already past 'u'), joining surrogate pairs.
func (p *parser) u() (rune, error) {
	u, err := p.hex4()
	if err != nil {
		return 0, err
	}
	if u >= 0xD800 && u <= 0xDBFF {
		if len(p.d)-p.i < 2 || p.d[p.i] != '\\' || p.d[p.i+1] != 'u' {
			return 0, jerr("lone surrogate")
		}
		p.i += 2
		v, err := p.hex4()
		if err != nil {
			return 0, err
		}
		if v < 0xDC00 || v > 0xDFFF {
			return 0, jerr("high surrogate not followed by low surrogate")
		}
		return 0x10000 + (rune(u-0xD800) << 10) + rune(v-0xDC00), nil
	}
	if u >= 0xDC00 && u <= 0xDFFF {
		return 0, jerr("lone low surrogate")
	}
	return rune(u), nil
}

func (p *parser) hex4() (int, error) {
	if len(p.d)-p.i < 4 {
		return 0, jerr("truncated \\u escape")
	}
	v := 0
	for k := 0; k < 4; k++ {
		c := p.d[p.i+k]
		var d int
		switch {
		case c >= '0' && c <= '9':
			d = int(c - '0')
		case c >= 'a' && c <= 'f':
			d = int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			d = int(c-'A') + 10
		default:
			return 0, jerr("bad hex digit %#x in \\u escape", c)
		}
		v = v<<4 | d
	}
	p.i += 4
	return v, nil
}

func (p *parser) number() (string, error) {
	start := p.i
	if p.i < len(p.d) && p.d[p.i] == '-' {
		p.i++
	}
	// int part: 0 or [1-9][0-9]*
	if p.i >= len(p.d) {
		return "", jerr("truncated number")
	}
	if p.d[p.i] == '0' {
		p.i++
	} else if p.d[p.i] >= '1' && p.d[p.i] <= '9' {
		for p.i < len(p.d) && p.d[p.i] >= '0' && p.d[p.i] <= '9' {
			p.i++
		}
	} else {
		return "", jerr("bad number at byte %d", start)
	}
	if p.i < len(p.d) && p.d[p.i] == '.' {
		p.i++
		if p.i >= len(p.d) || p.d[p.i] < '0' || p.d[p.i] > '9' {
			return "", jerr("bad fraction in number at byte %d", start)
		}
		for p.i < len(p.d) && p.d[p.i] >= '0' && p.d[p.i] <= '9' {
			p.i++
		}
	}
	if p.i < len(p.d) && (p.d[p.i] == 'e' || p.d[p.i] == 'E') {
		p.i++
		if p.i < len(p.d) && (p.d[p.i] == '+' || p.d[p.i] == '-') {
			p.i++
		}
		if p.i >= len(p.d) || p.d[p.i] < '0' || p.d[p.i] > '9' {
			return "", jerr("bad exponent in number at byte %d", start)
		}
		for p.i < len(p.d) && p.d[p.i] >= '0' && p.d[p.i] <= '9' {
			p.i++
		}
	}
	return string(p.d[start:p.i]), nil
}
