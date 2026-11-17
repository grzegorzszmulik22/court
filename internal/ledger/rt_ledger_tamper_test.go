package ledger

// Red-team reproductions for the M1 ledger (brief B3, aim 1). Every test
// here states the property the spec/files claim and fails when the current
// code accepts a tampered ledger instead.

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

const rtHex64 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// rtDB makes a three-event ledger: commission open, alarm, tool.event.
func rtDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ledger.sqlite")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := s.Append("C1", "human", "commission.opened", &CommissionOpened{
		Goal: "red team", DoneChecks: []string{"true"}, Territory: []string{"**"},
		Budgets:   Budgets{Tokens: 1, Minutes: 1, Speculation: 1},
		Attention: 1, Defaults: []string{}, Exemptions: []string{},
	}); err != nil {
		t.Fatalf("append open: %v", err)
	}
	if _, err := s.Append("C1", "agent:a1", "alarm.raised", &AlarmRaised{
		Text: "CANARY-ALARM", Paths: []string{},
	}); err != nil {
		t.Fatalf("append alarm: %v", err)
	}
	if _, err := s.Append("C1", "agent:a1", "tool.event", &ToolEvent{
		Tool: "Bash", ToolUseID: "tu-1", InputDigest: rtHex64,
		Fields: ToolFields{
			Paths:   []string{},
			Command: "curl https://canary.invalid/x",
			Hosts:   []string{"canary.invalid"},
		},
		Outcome: "ok",
	}); err != nil {
		t.Fatalf("append tool: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return path
}

func rtSQL(t *testing.T, path, query string, args ...any) {
	t.Helper()
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)", path))
	if err != nil {
		t.Fatalf("sql open: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("sql: %v", err)
	}
}

// rtRewriteTailEvent writes back event seq with a changed body and a hash
// that is consistent with the change (what an attacker with write access to
// the SQLite file can compute).
func rtRewriteTailEvent(t *testing.T, path string, seq int64, mutate func(*Value), blankHash bool) {
	t.Helper()
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)", path))
	if err != nil {
		t.Fatalf("sql open: %v", err)
	}
	defer db.Close()
	var prev, at, commission, actor, typ, body, hash string
	if err := db.QueryRow(
		"SELECT prev,at,commission,actor,type,body,hash FROM ledger WHERE seq=?",
		seq).Scan(&prev, &at, &commission, &actor, &typ, &body, &hash); err != nil {
		t.Fatalf("read tail: %v", err)
	}
	bv, err := ParseStrict([]byte(body))
	if err != nil {
		t.Fatalf("body: %v", err)
	}
	mutate(bv)
	bodyCanon, err := Canonical(bv)
	if err != nil {
		t.Fatalf("canonical: %v", err)
	}
	ev := &Event{Seq: seq, Prev: prev, At: at, Commission: commission,
		Actor: actor, Type: typ, Body: bv}
	if !blankHash {
		if _, err := ev.ComputeHash(); err != nil {
			t.Fatalf("hash: %v", err)
		}
	}
	if _, err := db.Exec("UPDATE ledger SET body=?, hash=? WHERE seq=?",
		string(bodyCanon), ev.Hash, seq); err != nil {
		t.Fatalf("update: %v", err)
	}
}

// TestRT_LedgerTailEditAccepted: M1 (spec/milestones/M1-ledger-and-worlds.md,
// Expected results) says any change to a stored event is found by two
// independent verifiers; 01 §11.1 says the chain detects out-of-band edits.
// Editing the LAST event and recomputing its hash leaves every linkage valid,
// so both `court ledger verify` (VerifyFile) and `court replay` accept it.
func TestRT_LedgerTailEditAccepted(t *testing.T) {
	path := rtDB(t)
	rtRewriteTailEvent(t, path, 2, func(b *Value) {
		// Change the recorded command and the host it names. tool.event
		// projects to no derived table, so replay cannot notice either.
		b.Obj[3].Val.Str = "go test ./..."
		b.Obj[4].Val.Arr = nil
	}, false)

	v := VerifyFile(path)
	if v.Class == ClassOK {
		t.Fatalf("tampered last event accepted by VerifyFile (class %q)", v.Class)
	}
	rep, err := Replay(path)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if rep.Verified.Class == ClassOK && len(rep.Mismatches) == 0 {
		t.Fatalf("tampered last event accepted by Replay: verified=%q mismatches=%v",
			rep.Verified.Class, rep.Mismatches)
	}
}

// TestRT_LedgerBlankHashAcceptsEdit: the SQLite read path (ReadLedgerFile →
// readDBLines → Event.Line) recomputes any empty `hash` column instead of
// rejecting it. A row whose hash was blanked is therefore reported as valid,
// and a blank hash plus an edited body on the last event is accepted even
// though refledger rejects the same file (envelope: hash is not sha256 hex).
func TestRT_LedgerBlankHashAcceptsEdit(t *testing.T) {
	path := rtDB(t)
	rtRewriteTailEvent(t, path, 2, func(b *Value) {
		b.Obj[3].Val.Str = "echo CANARY-POST-EDIT"
	}, true)

	v := VerifyFile(path)
	if v.Class == ClassOK {
		t.Fatalf("last event with an empty stored hash accepted (class %q)", v.Class)
	}
}

// TestRT_LedgerTailDeletionAccepted: deleting the newest event (and its
// derived row, as an attacker with write access would) leaves a shorter
// chain that verifies and replays: both verifiers rebuild only from the rows
// present, so an alarm can be erased without a trace.
func TestRT_LedgerTailDeletionAccepted(t *testing.T) {
	path := rtDB(t)
	// Drop the tool.event row (seq 2) entirely. tool.event projects to no
	// derived rows, so nothing else has to change.
	rtSQL(t, path, "DELETE FROM ledger WHERE seq=2")

	v := VerifyFile(path)
	if v.Class == ClassOK {
		t.Fatalf("ledger with its newest event deleted verifies (class %q, events %d)",
			v.Class, v.Events)
	}
	rep, err := Replay(path)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if rep.Verified.Class == ClassOK && len(rep.Mismatches) == 0 {
		t.Fatalf("replay accepts the deleted tail: verified=%q mismatches=%v",
			rep.Verified.Class, rep.Mismatches)
	}
}

// TestRT_LedgerNonCanonicalLineAccepted: a one-byte insertion into a stored
// JSONL line (a space after the opening brace) decodes to the same event, so
// the verifier recomputes the same canonical bytes and the same hash. The
// milestone's expected result is that a single-byte change is found.
func TestRT_LedgerNonCanonicalLineAccepted(t *testing.T) {
	path := rtDB(t)
	lines, ver := ReadLedgerFile(path)
	if ver.Class != ClassOK {
		t.Fatalf("read: %s", ver.Detail)
	}
	tampered := make([][]byte, len(lines))
	for i, l := range lines {
		c := append([]byte(nil), l...)
		if i == 0 {
			c = append(c[:1], append([]byte{' '}, c[1:]...)...)
		}
		tampered[i] = c
	}
	v := VerifyLineBytes(tampered)
	if v.Class == ClassOK && strings.Contains(string(tampered[0][:2]), "{ ") {
		t.Fatalf("one-byte whitespace insertion into event 0 accepted (class %q)", v.Class)
	}
}
