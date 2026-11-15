package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"time"
)

// ZeroPrev is the `prev` of event 0 (spec/01 section 11.1).
const ZeroPrev = "0000000000000000000000000000000000000000000000000000000000000000"

// Event is one ledger event envelope. CanonicalBytes serializes everything
// except Hash per RFC 8785 (spec/02 section 6); Hash is the SHA-256 of those
// bytes. The stored form of an event adds "hash" as a sibling field.
type Event struct {
	Seq        int64
	Prev       string
	At         string
	Commission string
	Actor      string
	Type       string
	Body       *Value
	Hash       string
}

var (
	reHex64   = regexp.MustCompile(`^[0-9a-f]{64}$`)
	reActor   = regexp.MustCompile(`^(court|human|clerk|agent:[A-Za-z0-9][A-Za-z0-9_.:-]*)$`)
	reAt      = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`)
	reDecisID = regexp.MustCompile(`^D[0-9]+$`)
	atLayout  = "2006-01-02T15:04:05.000Z"
)

// Now returns the envelope timestamp format (UTC, millisecond precision).
func Now() string { return time.Now().UTC().Format(atLayout) }

// ValidAt reports whether s is a well-formed envelope timestamp.
func ValidAt(s string) bool {
	if !reAt.MatchString(s) {
		return false
	}
	_, err := time.Parse(atLayout, s)
	return err == nil
}

// CanonicalBytes returns the RFC 8785 bytes of the envelope without `hash`.
// It always recomputes: callers may mutate fields after decoding.
func (e *Event) CanonicalBytes() ([]byte, error) {
	v := &Value{Kind: Object, Obj: []Member{
		{Key: "seq", Val: &Value{Kind: Number, Num: strconv.FormatInt(e.Seq, 10)}},
		{Key: "prev", Val: &Value{Kind: String, Str: e.Prev}},
		{Key: "at", Val: &Value{Kind: String, Str: e.At}},
		{Key: "commission", Val: &Value{Kind: String, Str: e.Commission}},
		{Key: "actor", Val: &Value{Kind: String, Str: e.Actor}},
		{Key: "type", Val: &Value{Kind: String, Str: e.Type}},
		{Key: "body", Val: e.Body},
	}}
	return Canonical(v)
}

// HashBytes returns the event hash: sha256 of the canonical bytes.
func (e *Event) HashBytes() ([32]byte, error) {
	b, err := e.CanonicalBytes()
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(b), nil
}

// ComputeHash sets e.Hash from the canonical bytes and returns it.
func (e *Event) ComputeHash() (string, error) {
	sum, err := e.HashBytes()
	if err != nil {
		return "", err
	}
	e.Hash = hex.EncodeToString(sum[:])
	return e.Hash, nil
}

// Line is the stored form: the canonical JSON of the envelope including
// `hash`, one line long.
func (e *Event) Line() ([]byte, error) {
	if e.Hash == "" {
		if _, err := e.ComputeHash(); err != nil {
			return nil, err
		}
	}
	v := &Value{Kind: Object, Obj: []Member{
		{Key: "seq", Val: &Value{Kind: Number, Num: strconv.FormatInt(e.Seq, 10)}},
		{Key: "prev", Val: &Value{Kind: String, Str: e.Prev}},
		{Key: "at", Val: &Value{Kind: String, Str: e.At}},
		{Key: "commission", Val: &Value{Kind: String, Str: e.Commission}},
		{Key: "actor", Val: &Value{Kind: String, Str: e.Actor}},
		{Key: "type", Val: &Value{Kind: String, Str: e.Type}},
		{Key: "body", Val: e.Body},
		{Key: "hash", Val: &Value{Kind: String, Str: e.Hash}},
	}}
	return Canonical(v)
}

var envelopeKeys = []string{"seq", "prev", "at", "commission", "actor", "type", "body", "hash"}

// DecodeEnvelope strictly decodes one stored event line. Unknown fields are
// rejected; ParseStrict already rejected duplicate keys. Schema validation of
// the body happens separately (verifySchema) so callers can order errors.
func DecodeEnvelope(line []byte) (*Event, error) {
	v, err := ParseStrict(line)
	if err != nil {
		return nil, err
	}
	if v.Kind != Object {
		return nil, jerr("event is not an object")
	}
	seen := map[string]bool{}
	var e Event
	for _, m := range v.Obj {
		seen[m.Key] = true
		switch m.Key {
		case "seq":
			if m.Val.Kind != Number {
				return nil, jerr("seq is not a number")
			}
			f, err := strconv.ParseFloat(m.Val.Num, 64)
			if err != nil {
				return nil, jerr("seq out of range")
			}
			if f != math.Floor(f) || f < 0 || f >= 9007199254740992 {
				return nil, jerr("seq is not a non-negative integer")
			}
			e.Seq = int64(f)
		case "prev":
			if m.Val.Kind != String || !reHex64.MatchString(m.Val.Str) {
				return nil, jerr("prev is not a sha256 hex string")
			}
			e.Prev = m.Val.Str
		case "at":
			if m.Val.Kind != String || !ValidAt(m.Val.Str) {
				return nil, jerr("at is not a valid timestamp")
			}
			e.At = m.Val.Str
		case "commission":
			if m.Val.Kind != String || len(m.Val.Str) == 0 {
				return nil, jerr("commission is empty or not a string")
			}
			e.Commission = m.Val.Str
		case "actor":
			if m.Val.Kind != String || !reActor.MatchString(m.Val.Str) {
				return nil, jerr("actor has bad shape %q", m.Val.Str)
			}
			e.Actor = m.Val.Str
		case "type":
			if m.Val.Kind != String {
				return nil, jerr("type is not a string")
			}
			if !knownType(m.Val.Str) {
				return nil, jerr("unknown event type %q", m.Val.Str)
			}
			e.Type = m.Val.Str
		case "body":
			if m.Val.Kind != Object {
				return nil, jerr("body is not an object")
			}
			e.Body = m.Val
		case "hash":
			if m.Val.Kind != String || !reHex64.MatchString(m.Val.Str) {
				return nil, jerr("hash is not a sha256 hex string")
			}
			e.Hash = m.Val.Str
		default:
			return nil, jerr("unknown field %q", m.Key)
		}
	}
	for _, k := range envelopeKeys {
		if !seen[k] {
			return nil, jerr("missing field %q", k)
		}
	}
	if _, err := e.CanonicalBytes(); err != nil {
		return nil, err
	}
	return &e, nil
}

// BodyFrom marshals a typed body struct and re-parses it into a Value tree.
// Callers must ensure slices are non-nil (NormalizeBody) so arrays stay arrays.
func BodyFrom(body any) (*Value, error) {
	raw, err := marshalBody(body)
	if err != nil {
		return nil, err
	}
	v, err := ParseStrict(raw)
	if err != nil {
		return nil, fmt.Errorf("internal: typed body does not round-trip: %w", err)
	}
	return v, nil
}
