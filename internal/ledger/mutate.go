package ledger

import (
	"bytes"
	"fmt"
	"math/rand"
	"strconv"
)

// Mutation kinds for the C1.3.1/C1.3.5 suite. Every mutant must be detected
// by the verifier; Mutate guarantees detectability where a random choice
// could accidentally be a no-op.
var MutantKinds = []string{
	"byte-flip", "remove", "swap", "edit-seq", "edit-prev", "edit-at",
	"edit-commission", "edit-actor", "edit-type", "edit-body",
	"dup", "truncate", "edit-body-fixchain",
	// Strict-decoding rules (spec/01 section 11): each kind exists so that
	// disabling the rule in either verifier produces a class disagreement.
	"unknown-field-envelope", "dup-key-envelope", "dup-key-body",
	"non-utf8", "bad-escape", "range-number", "trailing-data",
	// Each invalid-surrogate form is its own kind: disabling one side of
	// the check (e.g. lone lows) must produce a disagreement.
	"lone-surrogate-high", "lone-surrogate-low", "surrogate-low-high",
	"surrogate-high-nonlow",
	// B3-11: stored bytes are the canonical bytes. Each kind decodes to
	// the same event but rewrites the stored line, so only the
	// stored-bytes check catches it — a verifier that re-canonicalizes
	// instead of comparing reports ok and disagrees.
	"whitespace", "crlf-inside", "key-order", "number-spelling",
	"alt-escape",
	// B3-02: a stored hash must be given, never recomputed. blank-hash
	// fails the hex check; missing-hash drops the field — both envelope.
	"blank-hash", "missing-hash",
}

// Mutate returns mutated copies of a verified ledger's canonical lines.
// kind must be one of MutantKinds. The result is guaranteed detectable
// (VerifyLineBytes returns a class other than "ok").
func Mutate(lines [][]byte, kind string, seed int64) [][]byte {
	r := rand.New(rand.NewSource(seed))
	n := len(lines)
	for tries := 0; tries < 4000; tries++ {
		out := dupAll(lines)
		switch kind {
		case "byte-flip":
			i := r.Intn(n)
			p := 1 + r.Intn(len(out[i])-2)
			out[i][p] ^= byte(1 + r.Intn(255))
		case "remove":
			i := r.Intn(n)
			out = append(out[:i], out[i+1:]...)
		case "swap":
			if n < 3 {
				continue
			}
			i := 1 + r.Intn(n-1)
			j := 1 + r.Intn(n-1)
			if i == j {
				j = (j + 1) % n
				if j == 0 {
					j = 1
				}
			}
			out[i], out[j] = out[j], out[i]
		case "edit-seq":
			i := r.Intn(n)
			out[i] = editField(out[i], "seq", numVal(int64(1+r.Intn(n+5))))
		case "edit-prev":
			i := r.Intn(n)
			out[i] = editField(out[i], "prev", strVal(randHex(r, 64)))
		case "edit-at":
			i := r.Intn(n)
			out[i] = editField(out[i], "at",
				strVal("2027-01-01T00:00:00.000Z"))
		case "edit-commission":
			i := r.Intn(n)
			out[i] = editField(out[i], "commission", strVal("CX"))
		case "edit-actor":
			i := r.Intn(n)
			out[i] = editField(out[i], "actor", strVal("agent:evil"))
		case "edit-type":
			i := r.Intn(n)
			out[i] = editField(out[i], "type", strVal("world.created"))
		case "edit-body":
			i := r.Intn(n)
			out[i] = editBodyField(out[i], r)
		case "edit-body-fixchain":
			i := r.Intn(n)
			out[i] = editBodyField(out[i], r)
			fixChain(out, i)
		case "dup":
			i := r.Intn(n - 1)
			out = insertAt(out, i, out[i])
		case "truncate":
			i := r.Intn(n)
			cut := 1 + r.Intn(len(out[i])-1)
			out = append(out[:i:i], out[i][:cut])
		case "unknown-field-envelope":
			i := r.Intn(n)
			out[i] = injectAfter(out[i], "{", `"zebra":1,`)
		case "dup-key-envelope":
			i := r.Intn(n)
			out[i] = injectAfter(out[i], "{", `"actor":"agent:dup",`)
		case "dup-key-body":
			i := r.Intn(n)
			out[i] = injectBefore(out[i], `},"commission"`, `,"_dup":1,"_dup":2`)
		case "non-utf8":
			i := r.Intn(n)
			out[i] = injectAfter(out[i], `"type":"`, "\xff")
		case "bad-escape":
			i := r.Intn(n)
			out[i] = injectAfter(out[i], `"type":"`, `\q`)
		case "lone-surrogate-high":
			i := r.Intn(n)
			out[i] = injectAfter(out[i], `"type":"`, `\ud800`)
		case "lone-surrogate-low":
			i := r.Intn(n)
			out[i] = injectAfter(out[i], `"type":"`, `\udc00`)
		case "surrogate-low-high":
			i := r.Intn(n)
			out[i] = injectAfter(out[i], `"type":"`, `\udc00\ud800`)
		case "surrogate-high-nonlow":
			i := r.Intn(n)
			out[i] = injectAfter(out[i], `"type":"`, `\ud800\u0041`)
		case "range-number":
			i := r.Intn(n)
			out[i] = injectAfter(out[i], `"body":{`, `"_n":1e999,`)
		case "trailing-data":
			i := r.Intn(n)
			out[i] = append(append([]byte{}, out[i]...), ' ', 'x')
		case "whitespace":
			i := r.Intn(n)
			out[i] = injectAfter(out[i], `{"actor"`, " ")
		case "crlf-inside":
			i := r.Intn(n)
			out[i] = injectAfter(out[i], `{"actor"`, "\r")
		case "key-order":
			i := r.Intn(n)
			out[i] = reorderEnvelope(out[i])
		case "number-spelling":
			i := r.Intn(n)
			out[i] = respellNumber(out[i])
		case "alt-escape":
			i := r.Intn(n)
			out[i] = bytes.Replace(out[i], []byte(`"actor"`), []byte(`"\u0061ctor"`), 1)
		case "blank-hash":
			i := r.Intn(n)
			out[i] = editField(out[i], "hash", strVal(""))
		case "missing-hash":
			i := r.Intn(n)
			out[i] = dropField(out[i], "hash")
		default:
			panic("unknown mutant kind " + kind)
		}
		if VerifyLineBytes(out).Class != ClassOK {
			return out
		}
	}
	panic("could not produce a detectable " + kind + " mutant")
}

func dupAll(lines [][]byte) [][]byte {
	out := make([][]byte, len(lines))
	for i, l := range lines {
		c := make([]byte, len(l))
		copy(c, l)
		out[i] = c
	}
	return out
}

// injectAfter inserts payload immediately after the first occurrence of
// marker in line. Panics when the marker is absent.
func injectAfter(line []byte, marker, payload string) []byte {
	i := bytes.Index(line, []byte(marker))
	if i < 0 {
		panic("marker not found: " + marker)
	}
	at := i + len(marker)
	out := make([]byte, 0, len(line)+len(payload))
	out = append(out, line[:at]...)
	out = append(out, payload...)
	return append(out, line[at:]...)
}

// injectBefore inserts payload immediately before the first occurrence of
// marker in line. Panics when the marker is absent.
func injectBefore(line []byte, marker, payload string) []byte {
	i := bytes.Index(line, []byte(marker))
	if i < 0 {
		panic("marker not found: " + marker)
	}
	out := make([]byte, 0, len(line)+len(payload))
	out = append(out, line[:i]...)
	out = append(out, payload...)
	return append(out, line[i:]...)
}

func insertAt(ls [][]byte, i int, l []byte) [][]byte {
	out := make([][]byte, 0, len(ls)+1)
	out = append(out, ls[:i]...)
	out = append(out, l)
	return append(out, ls[i:]...)
}

func numVal(x int64) *Value {
	return &Value{Kind: Number, Num: strconv.FormatInt(x, 10)}
}
func strVal(s string) *Value { return &Value{Kind: String, Str: s} }

func randHex(r *rand.Rand, n int) string {
	const d = "0123456789abcdef"
	b := make([]byte, n)
	for i := range b {
		b[i] = d[r.Intn(16)]
	}
	return string(b)
}

// editField replaces one top-level envelope field, keeping the stored hash,
// then re-serializes.
func editField(line []byte, field string, v *Value) []byte {
	ev, err := DecodeEnvelope(line)
	if err != nil {
		panic(err)
	}
	obj := []*Member{
		{Key: "seq", Val: numVal(ev.Seq)},
		{Key: "prev", Val: strVal(ev.Prev)},
		{Key: "at", Val: strVal(ev.At)},
		{Key: "commission", Val: strVal(ev.Commission)},
		{Key: "actor", Val: strVal(ev.Actor)},
		{Key: "type", Val: strVal(ev.Type)},
		{Key: "body", Val: ev.Body},
		{Key: "hash", Val: strVal(ev.Hash)},
	}
	for _, m := range obj {
		if m.Key == field {
			m.Val = v
		}
	}
	members := make([]Member, len(obj))
	for i, m := range obj {
		members[i] = *m
	}
	return MustCanonical(&Value{Kind: Object, Obj: members})
}

// editBodyField mutates the event body: renames a scalar field, flips a
// boolean, or changes a string/number — whatever the shape allows.
func editBodyField(line []byte, r *rand.Rand) []byte {
	ev, err := DecodeEnvelope(line)
	if err != nil {
		panic(err)
	}
	b := ev.Body
	if b == nil || b.Kind != Object || len(b.Obj) == 0 {
		// Body is {} (decision.none): replace with a nonempty bogus member.
		ev.Body = &Value{Kind: Object, Obj: []Member{
			{Key: "bogus", Val: numVal(1)}}}
		return reemit(ev)
	}
	i := r.Intn(len(b.Obj))
	m := &b.Obj[i]
	switch m.Val.Kind {
	case Bool:
		m.Val = &Value{Kind: Bool, Bool: !m.Val.Bool}
	case Number:
		f, _ := numOf(m.Val)
		m.Val = numVal(int64(f) + 1)
	case String:
		m.Val = strVal(m.Val.Str + "X")
	case Null:
		m.Val = strVal("x")
	default: // array/object: corrupt a scalar inside, or rename the key
		if scalar := firstScalar(m.Val, r); scalar != nil {
			if scalar.Val.Kind == String {
				*scalar.Val = *strVal(scalar.Val.Str + "X")
			} else {
				*scalar.Val = *numVal(777)
			}
		} else {
			m.Key = m.Key + "_z"
		}
	}
	return reemit(ev)
}

// scalarRef is a mutable reference to a Value inside a tree.
type scalarRef struct{ Val *Value }

func firstScalar(v *Value, r *rand.Rand) *scalarRef {
	switch v.Kind {
	case String, Number, Bool:
		return &scalarRef{Val: v}
	case Array:
		for _, e := range v.Arr {
			if s := firstScalar(e, r); s != nil {
				return s
			}
		}
	case Object:
		for i := range v.Obj {
			if s := firstScalar(v.Obj[i].Val, r); s != nil {
				return s
			}
		}
	}
	return nil
}

// reemit writes the event back out keeping its stored hash (a corrupt line).
func reemit(ev *Event) []byte {
	return MustCanonical(eventValue(ev))
}

// reorderEnvelope re-emits the event with its keys in a non-canonical
// order — the same event, stored in bytes the canonical form would never
// produce (B3-11).
func reorderEnvelope(line []byte) []byte {
	ev, err := DecodeEnvelope(line)
	if err != nil {
		panic(err)
	}
	pairs := [][2]string{
		{"prev", ev.Prev},
		{"seq", fmt.Sprint(ev.Seq)},
		{"at", ev.At},
		{"commission", ev.Commission},
		{"actor", ev.Actor},
		{"type", ev.Type},
		{"hash", ev.Hash},
	}
	out := []byte("{")
	for i, kv := range pairs {
		if i > 0 {
			out = append(out, ',')
		}
		out = append(out, MustCanonical(strVal(kv[0]))...)
		out = append(out, ':')
		if kv[0] == "seq" {
			out = append(out, kv[1]...)
		} else {
			out = append(out, MustCanonical(strVal(kv[1]))...)
		}
	}
	out = append(out, ',')
	out = append(out, MustCanonical(strVal("body"))...)
	out = append(out, ':')
	out = append(out, MustCanonical(ev.Body)...)
	return append(out, '}')
}

// respellNumber rewrites the envelope's seq literal with an equivalent
// but non-canonical spelling (5 -> 5.0): the same event in different
// stored bytes (B3-11).
func respellNumber(line []byte) []byte {
	i := bytes.Index(line, []byte(`"seq":`))
	if i < 0 {
		panic("no seq literal")
	}
	j := i + len(`"seq":`)
	k := j
	for k < len(line) && line[k] >= '0' && line[k] <= '9' {
		k++
	}
	out := append([]byte{}, line[:k]...)
	out = append(out, '.', '0')
	return append(out, line[k:]...)
}

// dropField removes one "key":"value" member (string value, no escapes)
// from the stored line — used to drop the hash member (B3-02).
func dropField(line []byte, key string) []byte {
	marker := []byte(`,"` + key + `":"`)
	i := bytes.Index(line, marker)
	if i < 0 {
		panic("field not found: " + key)
	}
	val := i + len(marker)
	end := bytes.IndexByte(line[val:], '"')
	if end < 0 {
		panic("unterminated value for " + key)
	}
	end += val + 1 // past the closing quote
	return append(append([]byte{}, line[:i]...), line[end:]...)
}

// PatchBody rewrites event i's body via fn, then recomputes the chain from
// i onward. Used to build golden vectors; mutation testing uses Mutate.
func PatchBody(lines [][]byte, i int, fn func(*Value)) [][]byte {
	out := dupAll(lines)
	ev, err := DecodeEnvelope(out[i])
	if err != nil {
		panic(err)
	}
	fn(ev.Body)
	ev.ComputeHash()
	line, _ := ev.Line()
	out[i] = line
	fixChain(out, i+1)
	return out
}

// ObjGetPub is objGet exported for cmd/court's vector builder.
func ObjGetPub(v *Value, key string) (*Value, bool) { return objGet(v, key) }

// fixChain recomputes hashes and prev links from index i onward so the only
// remaining corruption is inside event i's body (schema layer).
func fixChain(out [][]byte, i int) {
	for j := i; j < len(out); j++ {
		ev, err := DecodeEnvelope(out[j])
		if err != nil {
			panic(err)
		}
		if j > 0 {
			prevEv, _ := DecodeEnvelope(out[j-1])
			ev.Prev = prevEv.Hash
		}
		if _, err := ev.ComputeHash(); err != nil {
			panic(err)
		}
		line, err := ev.Line()
		if err != nil {
			panic(err)
		}
		out[j] = line
	}
}
