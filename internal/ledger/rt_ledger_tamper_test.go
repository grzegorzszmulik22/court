package ledger

// Red-team B3 properties for the M1 ledger (findings 01, 02, 03, 11 and
// CR-004). Every test states the property the spec claims and fails when
// the code silently re-accepts a tampered or hostile ledger.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
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

// rtHead returns the (seq, hash) of the newest event of a verified ledger.
func rtHead(t *testing.T, path string) (int64, string) {
	t.Helper()
	v := VerifyFile(path)
	if v.Class != ClassOK {
		t.Fatalf("verify: %s %s", v.Class, v.Detail)
	}
	return v.HeadSeq, v.HeadHash
}

// B3-01 / CR-004: a bare chain protects only events that have a successor,
// so an edited-and-rehashed tail still verifies — and the kept head is what
// catches it. Asserts both halves: the documented limit and the tool that
// closes it.
func TestHashCoversEveryFieldRT_TailEdit(t *testing.T) {
	path := rtDB(t)
	hseq, hhash := rtHead(t, path)
	rtRewriteTailEvent(t, path, 2, func(b *Value) {
		b.Obj[3].Val.Str = "go test ./..."
		b.Obj[4].Val.Arr = nil
	}, false)

	v := VerifyFile(path)
	if v.Class != ClassOK {
		t.Fatalf("tail edit+rehash should still verify (the CR-004 limit): %s %s",
			v.Class, v.Detail)
	}
	lines, ver := ReadLedgerFile(path)
	if ver.Class != ClassOK {
		t.Fatalf("read: %s", ver.Detail)
	}
	hc := CheckHead(lines, hseq, hhash)
	if hc.Fail == "" {
		t.Fatalf("kept head %d:%s not checked: edited tail accepted", hseq, hhash)
	}
}

// B3-01 / CR-004: deleting the newest event leaves a shorter chain that
// verifies; the kept head reports the ledger was cut back.
func TestHashCoversEveryFieldRT_TailDeletion(t *testing.T) {
	path := rtDB(t)
	hseq, hhash := rtHead(t, path)
	rtSQL(t, path, "DELETE FROM ledger WHERE seq=2")

	v := VerifyFile(path)
	if v.Class != ClassOK {
		t.Fatalf("tail deletion still verifies (the CR-004 limit): %s %s",
			v.Class, v.Detail)
	}
	lines, _ := ReadLedgerFile(path)
	hc := CheckHead(lines, hseq, hhash)
	if hc.Fail == "" {
		t.Fatalf("cut-back ledger accepted against kept head %d:%s", hseq, hhash)
	}
}

// B3-02: a stored blank hash is an error in every reader; no reader
// recomputes a hash it was not given.
func TestHashCoversEveryFieldRT_BlankHash(t *testing.T) {
	path := rtDB(t)
	rtRewriteTailEvent(t, path, 2, func(b *Value) {
		b.Obj[3].Val.Str = "echo CANARY-POST-EDIT"
	}, true)

	if v := VerifyFile(path); v.Class != ClassEnvelope {
		t.Fatalf("blank stored hash: class %q, want envelope", v.Class)
	}
	// The same row read back must not be laundered: s.Events + Line never
	// invents the missing hash, so an export attempt stops on the row.
	s, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	evs, err := s.Events()
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if _, err := evs[len(evs)-1].Line(); err == nil {
		t.Fatal("Line recomputed a hash it was not given")
	}
}

// B-JSONL: a JSONL line whose hash is missing or blank is an envelope
// error too (the reader never invents one).
func TestHashCoversEveryFieldRT_MissingHashJSONL(t *testing.T) {
	lines, err := Generate(5, 20)
	if err != nil {
		t.Fatal(err)
	}
	last, err := DecodeEnvelope(lines[len(lines)-1])
	if err != nil {
		t.Fatal(err)
	}
	mut := append([][]byte{}, lines[:len(lines)-1]...)
	mut = append(mut, editField(lines[len(lines)-1], "hash", strVal("")))
	if v := VerifyLineBytes(mut); v.Class != ClassEnvelope {
		t.Fatalf("blank hash in line: class %q, want envelope", v.Class)
	}
	// Missing the hash member entirely.
	raw := `{"seq":` + fmt.Sprint(last.Seq) + `,"prev":"` + last.Prev +
		`","at":"` + last.At + `","commission":"` + last.Commission +
		`","actor":"` + last.Actor + `","type":"` + last.Type +
		`","body":` + string(MustCanonical(last.Body)) + `}`
	mut[len(mut)-1] = []byte(raw)
	if v := VerifyLineBytes(mut); v.Class != ClassEnvelope {
		t.Fatalf("missing hash field: class %q, want envelope", v.Class)
	}
}

// B3-11: the stored bytes of an event are the canonical bytes — whitespace,
// CRLF, member-order and number-spelling rewrites are all envelope errors.
func TestJCSRT_NonCanonicalStored(t *testing.T) {
	path := rtDB(t)
	lines, ver := ReadLedgerFile(path)
	if ver.Class != ClassOK {
		t.Fatalf("read: %s", ver.Detail)
	}
	cases := map[string]func([]byte) []byte{
		"space after {": func(l []byte) []byte {
			return append(append([]byte{'{'}, ' '), l[1:]...)
		},
		"CRLF": func(l []byte) []byte {
			return append(append([]byte{}, l...), '\r')
		},
		"tab after {": func(l []byte) []byte {
			return append(append([]byte{'{'}, '\t'), l[1:]...)
		},
		"seq spelled 0.0": func(l []byte) []byte {
			return []byte(strings.Replace(string(l), `"seq":0`, `"seq":0.0`, 1))
		},
		"escape-spelled value": func(l []byte) []byte {
			// "human" spelled with a \u escape decodes the same but is not
			// the stored (minimal-escape) form.
			return []byte(strings.Replace(string(l), `"actor":"human"`,
				`"actor":"\u0068uman"`, 1))
		},
		"key order": func(l []byte) []byte {
			ev, err := DecodeEnvelope(l)
			if err != nil {
				t.Fatal(err)
			}
			// "hash" first, everything else canonical: same event, wrong bytes.
			return []byte(`{"hash":"` + ev.Hash + `","seq":` +
				fmt.Sprint(ev.Seq) + `,"prev":"` + ev.Prev + `","at":"` + ev.At +
				`","commission":"` + ev.Commission + `","actor":"` + ev.Actor +
				`","type":"` + ev.Type + `","body":` +
				string(MustCanonical(ev.Body)) + `}`)
		},
	}
	for name, f := range cases {
		mut := dupAll(lines)
		mut[0] = f(mut[0])
		if v := VerifyLineBytes(mut); v.Class != ClassEnvelope {
			t.Fatalf("%s: class %q seq %d, want envelope", name, v.Class, v.Seq)
		}
	}
	// The whole file with CRLF endings: every stored line differs.
	mut := dupAll(lines)
	for i := range mut {
		mut[i] = append(append([]byte{}, mut[i]...), '\r')
	}
	if v := VerifyLineBytes(mut); v.Class != ClassEnvelope {
		t.Fatalf("CRLF file: class %q, want envelope", v.Class)
	}
	// The SQLite path: a stored body column that is valid JSON but not
	// canonical bytes is an envelope error, never silently re-serialized.
	rtSQL(t, path, "UPDATE ledger SET body=' ' || body WHERE seq=1")
	if v := VerifyFile(path); v.Class != ClassEnvelope {
		t.Fatalf("non-canonical stored body: class %q, want envelope", v.Class)
	}
}

// B3-02: export never launders a tamper — a database whose stored rows do
// not verify cannot be turned into a clean JSONL file.
func TestHashCoversEveryFieldRT_ExportLaunder(t *testing.T) {
	path := rtDB(t)
	rtRewriteTailEvent(t, path, 2, func(b *Value) {
		b.Obj[3].Val.Str = "echo CANARY-POST-EDIT"
	}, true)
	// The export path must not produce a verifying file from a tampered
	// database: VerifyFile reports the row, and the line writer refuses a
	// hash it was not given.
	if v := VerifyFile(path); v.Class == ClassOK {
		t.Fatal("tampered database verifies")
	}
	s, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	evs, err := s.Events()
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	var exported [][]byte
	fail := false
	for _, ev := range evs {
		l, err := ev.Line()
		if err != nil {
			fail = true
			break
		}
		exported = append(exported, l)
	}
	if !fail {
		if v := VerifyLineBytes(exported); v.Class == ClassOK {
			t.Fatal("export laundered a tampered row into a verifying line")
		}
	}
}

// B3-01 / CR-004: CheckHead compares a kept head against a verified chain —
// match, appended events reported, cut-back and wrong-hash failures.
func TestHashCoversEveryFieldRT_HeadCheck(t *testing.T) {
	path := rtDB(t)
	lines, ver := ReadLedgerFile(path)
	if ver.Class != ClassOK {
		t.Fatalf("read: %s", ver.Detail)
	}
	last, err := DecodeEnvelope(lines[len(lines)-1])
	if err != nil {
		t.Fatal(err)
	}
	// Matching head: no failure, nothing appended.
	if hc := CheckHead(lines, last.Seq, last.Hash); hc.Fail != "" || len(hc.Appended) != 0 {
		t.Fatalf("matching head: %+v", hc)
	}
	// An earlier head: reported appended events, not a failure.
	hc := CheckHead(lines, 1, "unused")
	if hc.Fail == "" {
		t.Fatalf("earlier seq with a wrong hash must fail, got %+v", hc)
	}
	ev1, _ := DecodeEnvelope(lines[1])
	hc = CheckHead(lines, 1, ev1.Hash)
	if hc.Fail != "" || len(hc.Appended) != 1 || hc.Appended[0].Seq != 2 {
		t.Fatalf("appended reporting: %+v", hc)
	}
	// A forged tail event appended after the kept head is reported, and a
	// ledger cut back below the kept seq fails.
	forged := append(dupAll(lines), forgeNext(t, lines))
	if hc := CheckHead(forged, last.Seq, last.Hash); hc.Fail != "" || len(hc.Appended) != 1 {
		t.Fatalf("forged append: %+v", hc)
	}
	if hc := CheckHead(lines[:2], last.Seq, last.Hash); hc.Fail == "" {
		t.Fatal("cut-back ledger accepted against the kept head")
	}
	// A kept hash that is not the event's fails.
	bad := strings.Repeat("b", 64)
	if bad == last.Hash {
		bad = strings.Repeat("c", 64)
	}
	if hc := CheckHead(lines, last.Seq, bad); hc.Fail == "" {
		t.Fatal("wrong kept hash accepted")
	}
}

// forgeNext appends one valid event chained off the last line (a forged
// human ruling — undetectable until rulings are signed).
func forgeNext(t *testing.T, lines [][]byte) []byte {
	t.Helper()
	last, err := DecodeEnvelope(lines[len(lines)-1])
	if err != nil {
		t.Fatal(err)
	}
	bv, err := BodyFrom(&DecisionRuled{Decision: "D1", Option: "o1"})
	if err != nil {
		t.Fatal(err)
	}
	ev := &Event{Seq: last.Seq + 1, Prev: last.Hash, At: Now(),
		Commission: last.Commission, Actor: "human",
		Type: "decision.ruled", Body: bv}
	if _, err := ev.ComputeHash(); err != nil {
		t.Fatal(err)
	}
	l, err := ev.Line()
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// B3-03: every event written through the front door is validated against
// the same schema the verifier uses. An invalid body is an error and
// writes nothing — no row, no derived state — and the ledger still verifies.
func TestEnvelopeStrictRT_AppendRejectsInvalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "poison.sqlite")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	open := &CommissionOpened{
		Goal: "red team", DoneChecks: []string{"true"}, Territory: []string{"**"},
		Budgets:   Budgets{Tokens: 1, Minutes: 1, Speculation: 1},
		Attention: 1, Defaults: []string{}, Exemptions: []string{},
	}
	if _, err := s.Append("C1", "human", "commission.opened", open); err != nil {
		t.Fatalf("append open: %v", err)
	}
	// The named red-team cases: plan.lines below the schema minimum, an
	// empty contested world, an oversized clerk output.
	bads := []struct {
		name       string
		actor, typ string
		body       any
	}{
		{"plan.lines=-1", "agent:a1", "backbrief.submitted",
			&BackbriefSubmitted{Goal: "g", Plan: Plan{Files: []string{}, Lines: -1},
				ExpectedDecisions: []Expected{}, WillNotTouch: []string{}}},
		{"empty contested world", "court", "decision.contested",
			&DecisionContested{Decisions: []string{"D1", "D2"},
				Worlds: []string{"", ""}, Evidence: []string{rtHex64, rtHex64}}},
		{"clerk output over 2000 chars", "clerk", "backbrief.submitted",
			&BackbriefSubmitted{Goal: "g", Plan: Plan{Files: []string{}, Lines: 0},
				ExpectedDecisions: []Expected{}, WillNotTouch: []string{},
				Invalid: true, Output: strings.Repeat("x", 2001)}},
		{"empty commission id", "", "alarm.raised",
			&AlarmRaised{Text: "x", Paths: []string{}}},
		{"unknown type", "agent:a1", "agent.elevated",
			&AlarmRaised{Text: "x", Paths: []string{}}},
		{"bad actor shape", "agent:", "alarm.raised",
			&AlarmRaised{Text: "x", Paths: []string{}}},
	}
	for _, bc := range bads {
		if _, err := s.Append("C1", bc.actor, bc.typ, bc.body); err == nil {
			t.Fatalf("%s: append accepted a schema-invalid event", bc.name)
		}
	}
	if seq, _, err := s.Head(); err != nil || seq != 0 {
		t.Fatalf("invalid appends wrote: head=%d err=%v", seq, err)
	}
	// A randomized sequence of hostile bodies across every event type:
	// whatever the append accepts must be schema-valid, and the ledger
	// verifies afterwards.
	r := rand.New(rand.NewSource(3))
	for i := 0; i < 300; i++ {
		typ := TypeNames()[r.Intn(len(TypeNames()))]
		b := hostileBody(typ, r)
		ev, err := s.Append("C1", "agent:a1", typ, b)
		if err != nil {
			continue // rejected: nothing written
		}
		sch := Schemas[ev.Type]
		if errs := ValidateSchema(sch, eventValue(ev)); len(errs) > 0 {
			t.Fatalf("append accepted a schema-invalid %s: %v", typ, errs[0])
		}
	}
	s.Close()
	if v := VerifyFile(path); v.Class != ClassOK {
		t.Fatalf("ledger after hostile appends: %s seq=%d %s", v.Class, v.Seq, v.Detail)
	}
}

// hostileBody corrupts a valid body with a random schema violation.
func hostileBody(typ string, r *rand.Rand) any {
	mut := func(b *Value, keys ...string) *Value {
		for _, k := range keys {
			if v, ok := objGet(b, k); ok {
				switch r.Intn(5) {
				case 0:
					v.Kind, v.Str = String, ""
				case 1:
					v.Kind, v.Num = Number, "-1"
				case 2:
					v.Kind, v.Str = String, strings.Repeat("x", 5000)
				case 3:
					v.Kind, v.Arr = Array, nil
				default:
					v.Kind = Bool
				}
			}
		}
		return b
	}
	b, err := BodyFrom(validBody(typ))
	if err != nil {
		panic(err)
	}
	mut(b, "goal", "text", "agent", "world", "tool", "question",
		"decision", "task", "detector", "outcome", "id")
	if r.Intn(2) == 0 {
		mut(b, "lines", "seq")
		if p, ok := objGet(b, "plan"); ok {
			mut(p, "lines")
		}
	}
	var out any
	if err := unmarshalValue(b, &out); err != nil {
		panic(err)
	}
	return out
}

// unmarshalValue turns a Value tree back into a generic body so the append
// path sees plain maps/slices (what a decoded JSON body looks like).
func unmarshalValue(v *Value, out any) error {
	return json.Unmarshal(MustCanonical(v), out)
}

// validBody returns a schema-valid body for each event type.
func validBody(typ string) any {
	switch typ {
	case "commission.opened":
		return &CommissionOpened{Goal: "g", DoneChecks: []string{"x"},
			Territory: []string{"**"}, Attention: 1,
			Budgets:  Budgets{Tokens: 1, Minutes: 1, Speculation: 1},
			Defaults: []string{}, Exemptions: []string{}}
	case "commission.closed":
		return &CommissionClosed{Outcome: "merged", Counts: ReconCounts{}}
	case "agent.started":
		return &AgentStarted{Agent: "a1", Parent: "human", Tool: "scripted",
			Version: "1", World: "w1", Writ: "W1"}
	case "agent.stopped":
		return &AgentStopped{Agent: "a1", Reason: "done"}
	case "tool.event":
		return &ToolEvent{Tool: "Bash", ToolUseID: "tu", InputDigest: rtHex64,
			Fields:  ToolFields{Paths: []string{}, Command: "x", Hosts: []string{}},
			Outcome: "ok"}
	case "world.created":
		return &WorldCreated{World: "w1", Base: strings.Repeat("a", 40),
			Branch: "b"}
	case "world.discarded":
		return &WorldDiscarded{World: "w1", Base: strings.Repeat("a", 40),
			Branch: "b"}
	case "world.commit":
		return &WorldCommit{World: "w1", Commit: strings.Repeat("a", 40),
			Parents: []string{strings.Repeat("a", 40)},
			Changes: []PathChange{{Path: "f", Added: 1, Deleted: 1}},
			Gated:   true, Assumes: []string{}}
	case "backbrief.submitted":
		return &BackbriefSubmitted{Goal: "g",
			Plan:              Plan{Files: []string{"f"}, Lines: 1},
			ExpectedDecisions: []Expected{}, WillNotTouch: []string{}}
	case "writ.issued":
		return &WritIssued{ID: "W1", Agent: "a1", Parent: "human",
			Categories: []string{"other"}, Class: "reversible",
			Territory: []string{"**"},
			Budgets:   Budgets{Tokens: 1, Minutes: 1, Speculation: 1}}
	case "writ.refused":
		return &WritRefused{Agent: "a1", Parent: "human",
			Categories: []string{"authority"}, Class: "reversible",
			Territory: []string{"**"}, Rule: "guarantee", Detail: "d",
			Budgets: Budgets{Tokens: 1, Minutes: 1, Speculation: 1}}
	case "decision.declared":
		return &DecisionDeclared{Question: "q?", Category: "other",
			Paths:   []string{"f"},
			Options: []Option{{ID: "o1", Summary: "s"}}, Chosen: "o1"}
	case "decision.none":
		return &DecisionNone{}
	case "decision.detected":
		return &DecisionDetected{Detector: "DT1", Category: "other",
			Paths: []string{"f"}, Commit: strings.Repeat("a", 40),
			Evidence: []Evidence{{File: "f", StartLine: 1, EndLine: 2,
				Digest: rtHex64}}}
	case "decision.phrased":
		return &DecisionPhrased{Decision: "D1", Question: "q?",
			Alternative: "a"}
	case "decision.reconciled":
		return &DecisionReconciled{Decision: "D1",
			Sources: []string{"declared"}, Recon: "declared-only", Flags: []string{}}
	case "decision.contested":
		return &DecisionContested{Decisions: []string{"D1", "D2"},
			Worlds: []string{"w1", "w2"}, Evidence: []string{rtHex64, rtHex64}}
	case "decision.ruled":
		return &DecisionRuled{Decision: "D1", Option: "o1"}
	case "claim.done":
		return &ClaimDone{Task: "t", Checks: []string{"go test"}}
	case "claim.checked":
		return &ClaimChecked{Task: "t",
			Results:           []CheckResult{{Check: "go test", Result: "pass"}},
			UnverifiedByAgent: false}
	case "alarm.raised":
		return &AlarmRaised{Text: "x", Paths: []string{"f"}}
	}
	panic("no valid body for " + typ)
}
