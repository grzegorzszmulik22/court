package ledger

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
)

// Verification error classes (docs/M1.3.md lists the contract; refledger
// reports the same classes).
const (
	ClassOK        = "ok"
	ClassIO        = "io"
	ClassEmpty     = "empty"
	ClassTruncated = "truncated"
	ClassJSON      = "json"
	ClassEnvelope  = "envelope"
	ClassSeq       = "seq"
	ClassPrev      = "prev"
	ClassHash      = "hash"
	ClassSchema    = "schema"
)

// Verdict is one file's verification outcome.
type Verdict struct {
	Class  string // one of the Class* constants
	Seq    int64  // index of the offending event (-1 when none)
	Detail string
	Events int // events verified when Class == ClassOK
	// HeadSeq and HeadHash are the chain head — (seq, hash) of the newest
	// verified event — set when Class == ClassOK. The head is the value a
	// reader keeps so a later --head check covers the newest event and any
	// tail appended after it (CR-004).
	HeadSeq  int64
	HeadHash string
}

// VerifyLineBytes verifies a ledger given as raw stored lines.
// Order: JSON well-formedness, envelope strictness, stored-bytes equality
// with the canonical form, seq continuity, prev linkage, hash
// recomputation, then body schema.
func VerifyLineBytes(lines [][]byte) Verdict {
	if len(lines) == 0 {
		return Verdict{Class: ClassEmpty, Seq: -1, Detail: "no events"}
	}
	var prevCanon []byte
	var last *Event
	for i, line := range lines {
		ev, err := DecodeEnvelope(line)
		if err != nil {
			if _, ok := err.(*parseError); ok {
				return Verdict{Class: classifyParse(err), Seq: int64(i), Detail: err.Error()}
			}
			return Verdict{Class: ClassEnvelope, Seq: int64(i), Detail: err.Error()}
		}
		// B3-11: the stored bytes are the canonical bytes. A whitespace,
		// CRLF, key-order or number-spelling rewrite decodes to the same
		// event but is not what was stored — reject it.
		stored, err := ev.Line()
		if err != nil {
			return Verdict{Class: ClassEnvelope, Seq: int64(i), Detail: err.Error()}
		}
		if !bytes.Equal(line, stored) {
			return Verdict{Class: ClassEnvelope, Seq: int64(i),
				Detail: "stored bytes are not the canonical event bytes"}
		}
		if ev.Seq != int64(i) {
			return Verdict{Class: ClassSeq, Seq: int64(i),
				Detail: fmt.Sprintf("seq %d at position %d", ev.Seq, i)}
		}
		canon, _ := ev.CanonicalBytes()
		want := ZeroPrev
		if i > 0 {
			sum := sha256.Sum256(prevCanon)
			want = hex.EncodeToString(sum[:])
		}
		if ev.Prev != want {
			return Verdict{Class: ClassPrev, Seq: int64(i),
				Detail: fmt.Sprintf("prev %s… != %s…", ev.Prev[:8], want[:8])}
		}
		sum := sha256.Sum256(canon)
		if hex.EncodeToString(sum[:]) != ev.Hash {
			return Verdict{Class: ClassHash, Seq: int64(i), Detail: "hash mismatch"}
		}
		sch := Schemas[ev.Type]
		if sch == nil {
			return Verdict{Class: ClassSchema, Seq: int64(i),
				Detail: "no schema for " + ev.Type}
		}
		whole := eventValue(ev)
		if errs := ValidateSchema(sch, whole); len(errs) > 0 {
			return Verdict{Class: ClassSchema, Seq: int64(i), Detail: errs[0].Error()}
		}
		prevCanon = canon
		last = ev
	}
	return Verdict{Class: ClassOK, Seq: -1, Events: len(lines),
		HeadSeq: last.Seq, HeadHash: last.Hash}
}

// HeadCheck is the outcome of comparing a kept head against a verified
// ledger (CR-004): Fail is empty when the ledger holds exactly that event
// at that seq; Appended lists the events after it — reported, never a
// failure, since an append-only ledger grows past any kept head.
type HeadCheck struct {
	Fail     string
	Appended []*Event
}

// CheckHead reports whether the verified ledger contains exactly
// (seq, hash). The lines must already have passed VerifyLineBytes; a
// ledger cut back below seq, or a different event at seq, fails.
func CheckHead(lines [][]byte, seq int64, hash string) *HeadCheck {
	hc := &HeadCheck{}
	if seq < 0 || seq >= int64(len(lines)) {
		hc.Fail = fmt.Sprintf("ledger has no event at seq %d (cut back below the kept head)", seq)
		return hc
	}
	ev, err := DecodeEnvelope(lines[seq])
	if err != nil {
		hc.Fail = fmt.Sprintf("event at seq %d does not decode: %v", seq, err)
		return hc
	}
	if ev.Hash != hash {
		hc.Fail = fmt.Sprintf("event at seq %d has hash %s, not the kept %s",
			seq, ev.Hash, hash)
		return hc
	}
	for _, l := range lines[seq+1:] {
		ev, err := DecodeEnvelope(l)
		if err != nil {
			hc.Fail = fmt.Sprintf("event after the kept head does not decode: %v", err)
			return hc
		}
		hc.Appended = append(hc.Appended, ev)
	}
	return hc
}

// classifyParse tells apart structural JSON errors (class json) from
// strictness failures the envelope layer owns (class envelope): duplicate
// keys are handled by IsDup; everything else from ParseStrict is class json.
func classifyParse(err error) string { return ClassJSON }

// eventValue rebuilds the full envelope as a Value for schema validation.
func eventValue(e *Event) *Value {
	return &Value{Kind: Object, Obj: []Member{
		{Key: "seq", Val: &Value{Kind: Number, Num: fmt.Sprint(e.Seq)}},
		{Key: "prev", Val: &Value{Kind: String, Str: e.Prev}},
		{Key: "at", Val: &Value{Kind: String, Str: e.At}},
		{Key: "commission", Val: &Value{Kind: String, Str: e.Commission}},
		{Key: "actor", Val: &Value{Kind: String, Str: e.Actor}},
		{Key: "type", Val: &Value{Kind: String, Str: e.Type}},
		{Key: "body", Val: e.Body},
		{Key: "hash", Val: &Value{Kind: String, Str: e.Hash}},
	}}
}

// VerifyFile verifies one ledger file: a canonical JSONL export or a SQLite
// ledger database (detected by magic bytes).
func VerifyFile(path string) Verdict {
	lines, ver := ReadLedgerFile(path)
	if ver.Class != ClassOK {
		return ver
	}
	return VerifyLineBytes(lines)
}

var sqliteMagic = []byte("SQLite format 3\x00")

// ReadLedgerFile loads a ledger file as canonical lines. A JSONL export is
// validated for the trailing newline (truncation); a SQLite database is read
// through the ledger table and re-serialized row by row.
func ReadLedgerFile(path string) ([][]byte, Verdict) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, Verdict{Class: ClassIO, Seq: -1, Detail: err.Error()}
	}
	if bytes.HasPrefix(data, sqliteMagic) {
		return readDBLines(path)
	}
	if len(data) == 0 {
		return nil, Verdict{Class: ClassEmpty, Seq: -1, Detail: "empty file"}
	}
	if data[len(data)-1] != '\n' {
		return nil, Verdict{Class: ClassTruncated, Seq: -1,
			Detail: "file does not end with a newline"}
	}
	data = data[:len(data)-1]
	lines := bytes.Split(data, []byte{'\n'})
	for i, l := range lines {
		if len(l) == 0 {
			return nil, Verdict{Class: ClassTruncated, Seq: int64(i),
				Detail: "empty line"}
		}
	}
	return lines, Verdict{Class: ClassOK, Seq: -1}
}

// readDBLines serializes every row of the ledger table to its canonical
// line. A stored row is read, never repaired (B3-02/B3-11): a hash column
// that is blank, NULL or not sha256 hex is an envelope error — the reader
// does not recompute the hash it was not given — and a body column that
// does not hold canonical bytes is an envelope error too.
func readDBLines(path string) ([][]byte, Verdict) {
	if err := LoadSchemas(""); err != nil {
		return nil, Verdict{Class: ClassIO, Seq: -1, Detail: err.Error()}
	}
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(wal)&_pragma=busy_timeout(30000)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, Verdict{Class: ClassIO, Seq: -1, Detail: err.Error()}
	}
	defer db.Close()
	rows, err := db.Query("SELECT seq,prev,at,commission,actor,type,COALESCE(body,''),COALESCE(hash,'') FROM ledger ORDER BY seq")
	if err != nil {
		return nil, Verdict{Class: ClassIO, Seq: -1, Detail: err.Error()}
	}
	defer rows.Close()
	var lines [][]byte
	for rows.Next() {
		var (
			seq                                       int64
			prev, at, commission, actor, typ, body, h string
		)
		if err := rows.Scan(&seq, &prev, &at, &commission, &actor, &typ, &body, &h); err != nil {
			return nil, Verdict{Class: ClassIO, Seq: -1, Detail: err.Error()}
		}
		if !reHex64.MatchString(h) {
			return nil, Verdict{Class: ClassEnvelope, Seq: seq,
				Detail: "stored hash is blank or not sha256 hex"}
		}
		bv, err := ParseStrict([]byte(body))
		if err != nil {
			return nil, Verdict{Class: ClassEnvelope, Seq: seq, Detail: "stored body invalid JSON"}
		}
		bodyCanon, err := Canonical(bv)
		if err != nil {
			return nil, Verdict{Class: ClassEnvelope, Seq: seq, Detail: err.Error()}
		}
		if string(bodyCanon) != body {
			return nil, Verdict{Class: ClassEnvelope, Seq: seq,
				Detail: "stored body is not the canonical bytes"}
		}
		ev := &Event{Seq: seq, Prev: prev, At: at, Commission: commission,
			Actor: actor, Type: typ, Body: bv, Hash: h}
		line, err := ev.Line()
		if err != nil {
			return nil, Verdict{Class: ClassEnvelope, Seq: seq, Detail: err.Error()}
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		return nil, Verdict{Class: ClassIO, Seq: -1, Detail: err.Error()}
	}
	if len(lines) == 0 {
		return nil, Verdict{Class: ClassEmpty, Seq: -1, Detail: "ledger table is empty"}
	}
	return lines, Verdict{Class: ClassOK, Seq: -1}
}
