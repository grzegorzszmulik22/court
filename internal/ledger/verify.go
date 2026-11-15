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
}

// VerifyLineBytes verifies a ledger given as raw canonical lines.
// Order: truncation, JSON well-formedness, envelope strictness, seq
// continuity, prev linkage, hash recomputation, then body schema.
func VerifyLineBytes(lines [][]byte) Verdict {
	if len(lines) == 0 {
		return Verdict{Class: ClassEmpty, Seq: -1, Detail: "no events"}
	}
	var prevCanon []byte
	for i, line := range lines {
		ev, err := DecodeEnvelope(line)
		if err != nil {
			if IsDup(err) {
				return Verdict{Class: ClassEnvelope, Seq: int64(i), Detail: err.Error()}
			}
			if _, ok := err.(*parseError); ok {
				return Verdict{Class: classifyParse(err), Seq: int64(i), Detail: err.Error()}
			}
			return Verdict{Class: ClassEnvelope, Seq: int64(i), Detail: err.Error()}
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
	}
	return Verdict{Class: ClassOK, Seq: -1, Events: len(lines)}
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

// readDBLines serializes every row of the ledger table to its canonical line.
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
	rows, err := db.Query("SELECT seq,prev,at,commission,actor,type,body,hash FROM ledger ORDER BY seq")
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
		bv, err := ParseStrict([]byte(body))
		if err != nil {
			return nil, Verdict{Class: ClassEnvelope, Seq: seq, Detail: "stored body invalid JSON"}
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
