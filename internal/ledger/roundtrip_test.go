package ledger

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
	"testing"

	apischema "court/api/schema"
)

// C1.2.5: every event type round-trips body -> Value -> canonical -> typed;
// every Go body type has a schema and every schema a Go type; strict
// decoding rejects unknown fields and duplicate keys.

func sampleBody(typ string) (any, error) {
	switch typ {
	case "commission.opened":
		return &CommissionOpened{Goal: "g", DoneChecks: []string{"c"},
			Territory: []string{"**"}, Budgets: Budgets{1, 2, 3},
			Attention: 10, Defaults: []string{"territory"}, Exemptions: []string{}}, nil
	case "commission.closed":
		return &CommissionClosed{Outcome: "merged",
			Counts: ReconCounts{Confirmed: 1, Undeclared: 2}}, nil
	case "agent.started":
		return &AgentStarted{Agent: "a1", Parent: "human", Tool: "t",
			Version: "1", World: "w1", Writ: "W1"}, nil
	case "agent.stopped":
		return &AgentStopped{Agent: "a1", Reason: "done"}, nil
	case "tool.event":
		return &ToolEvent{Tool: "Read", ToolUseID: "tu_1",
			InputDigest: strings.Repeat("a", 64),
			Fields: ToolFields{Paths: []string{"f.go"}, Command: "cat f.go",
				Hosts: []string{}},
			Outcome: "ok"}, nil
	case "world.created":
		return &WorldCreated{World: "w1", Base: "abc", Branch: "b"}, nil
	case "world.discarded":
		return &WorldDiscarded{World: "w1", Base: "abc", Branch: "b"}, nil
	case "world.commit":
		return &WorldCommit{World: "w1", Commit: "c1",
			Parents: []string{"p"}, Changes: []PathChange{{"f.go", 3, 1}},
			Gated: true, Assumes: []string{"D1"}}, nil
	case "backbrief.submitted":
		return &BackbriefSubmitted{Goal: "g",
			Plan: Plan{Files: []string{"f"}, Lines: 1},
			ExpectedDecisions: []Expected{{
				Question: "Q?", Category: "scope", Paths: []string{"f"},
				IntendedDefault: "o"}},
			WillNotTouch: []string{}}, nil
	case "writ.issued":
		return &WritIssued{ID: "W1", Agent: "a1", Parent: "human",
			Categories: []string{"scope"}, Class: "reversible",
			Territory: []string{"**"}, Budgets: Budgets{1, 1, 1}}, nil
	case "writ.refused":
		return &WritRefused{Agent: "a1", Parent: "a2",
			Categories: []string{"authority"}, Class: "reversible",
			Territory: []string{"**"}, Budgets: Budgets{1, 1, 1},
			Rule: "guarantee-category", Detail: "d"}, nil
	case "decision.declared":
		return &DecisionDeclared{Question: "Q?", Category: "scope",
			Paths: []string{"f"}, Options: []Option{{ID: "o1", Summary: "s"}},
			Chosen: "o1", Petition: true}, nil
	case "decision.none":
		return &DecisionNone{}, nil
	case "decision.detected":
		return &DecisionDetected{Detector: "DT1", Category: "scope",
			Paths: []string{"f"}, Commit: "c",
			Evidence: []Evidence{{"f", 1, 2, strings.Repeat("0", 64)}}}, nil
	case "decision.phrased":
		return &DecisionPhrased{Decision: "D1", Question: "Q?",
			Alternative: "alt"}, nil
	case "decision.reconciled":
		return &DecisionReconciled{Decision: "D1", Sources: []string{"declared"},
			Recon: "declared-only", Flags: []string{"outside-writ"}}, nil
	case "decision.contested":
		return &DecisionContested{Decisions: []string{"D1", "D2"},
			Worlds: []string{"w1", "w2"}, Evidence: []string{"e1", "e2"}}, nil
	case "decision.ruled":
		return &DecisionRuled{Decision: "D1", Option: "o1"}, nil
	case "claim.done":
		return &ClaimDone{Task: "t", Checks: []string{"go test"}}, nil
	case "claim.checked":
		return &ClaimChecked{Task: "t",
			Results:           []CheckResult{{Check: "go test", Result: "pass"}},
			UnverifiedByAgent: false}, nil
	case "alarm.raised":
		return &AlarmRaised{Text: "t", Paths: []string{"f"}}, nil
	}
	return nil, fmt.Errorf("no sample for %q", typ)
}

// sampleActor returns an actor string that satisfies the type's schema.
func sampleActor(typ string) string {
	sch := Schemas[typ]
	if sch == nil {
		return "court"
	}
	props, _ := objGet(sch, "properties")
	actor, _ := objGet(props, "actor")
	if c, ok := objGet(actor, "const"); ok && c != nil {
		return c.Str
	}
	if a, ok := objGet(actor, "anyOf"); ok && a != nil {
		if c, ok := objGet(a.Arr[0], "const"); ok && c != nil {
			return c.Str
		}
	}
	return "agent:a1"
}

func TestBodyRoundTrip(t *testing.T) {
	if err := LoadSchemas(""); err != nil {
		t.Fatal(err)
	}
	for _, typ := range TypeNames() {
		body, err := sampleBody(typ)
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateBody(typ, body); err != nil {
			t.Fatalf("%s: sample invalid: %v", typ, err)
		}
		bv, err := BodyFrom(body)
		if err != nil {
			t.Fatalf("%s: %v", typ, err)
		}
		// Body must validate against the whole-envelope schema.
		ev := &Event{Seq: 0, Prev: ZeroPrev, At: "2026-11-15T00:00:00.000Z",
			Commission: "C1", Actor: sampleActor(typ), Type: typ, Body: bv}
		if _, err := ev.ComputeHash(); err != nil {
			t.Fatal(err)
		}
		if errs := ValidateSchema(Schemas[typ], eventValue(ev)); len(errs) > 0 {
			t.Fatalf("%s: schema rejected valid body: %v", typ, errs[0])
		}
		// Round-trip the canonical bytes back through the strict decoder.
		line, err := ev.Line()
		if err != nil {
			t.Fatal(err)
		}
		ev2, err := DecodeEnvelope(line)
		if err != nil {
			t.Fatalf("%s: decode own line: %v", typ, err)
		}
		raw, _ := marshalBody(body)
		got := MustCanonical(bv)
		orig, err := ParseStrict(raw)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(MustCanonical(orig)) {
			t.Fatalf("%s: canonical body changed on round-trip", typ)
		}
		if string(MustCanonical(ev2.Body)) != string(got) {
			t.Fatalf("%s: body differs after decode", typ)
		}
	}
}

func TestSchemaBijection(t *testing.T) {
	if err := LoadSchemas(""); err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	err := fs.WalkDir(apischema.FS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(p, ".schema.json") {
			names[strings.TrimSuffix(p, ".schema.json")] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, typ := range TypeNames() {
		if _, ok := BodyType(typ); !ok {
			t.Fatalf("no Go body type for %s", typ)
		}
		if !names[typ] {
			t.Fatalf("no schema file for %s", typ)
		}
		if Schemas[typ] == nil {
			t.Fatalf("schema did not load for %s", typ)
		}
	}
	for n := range names {
		if _, ok := BodyType(n); !ok {
			t.Fatalf("schema %s has no Go body type", n)
		}
	}
}

// TestStrictDecode feeds malformed envelopes to DecodeEnvelope.
func TestStrictDecode(t *testing.T) {
	base := func(over func(v *Value)) []byte {
		bv, _ := BodyFrom(toolBody(0))
		v := &Value{Kind: Object, Obj: []Member{
			{Key: "seq", Val: &Value{Kind: Number, Num: "0"}},
			{Key: "prev", Val: &Value{Kind: String, Str: ZeroPrev}},
			{Key: "at", Val: &Value{Kind: String, Str: "2026-11-15T00:00:00.000Z"}},
			{Key: "commission", Val: &Value{Kind: String, Str: "C1"}},
			{Key: "actor", Val: &Value{Kind: String, Str: "agent:a1"}},
			{Key: "type", Val: &Value{Kind: String, Str: "tool.event"}},
			{Key: "body", Val: bv},
			{Key: "hash", Val: &Value{Kind: String, Str: strings.Repeat("0", 64)}},
		}}
		over(v)
		return MustCanonical(v)
	}
	cases := map[string][]byte{
		"unknown-field": base(func(v *Value) {
			v.Obj = append(v.Obj, Member{Key: "zap", Val: &Value{Kind: Null}})
		}),
		"missing-hash": base(func(v *Value) {
			v.Obj = v.Obj[:len(v.Obj)-1]
		}),
		"seq-string": base(func(v *Value) {
			v.Obj[0].Val = &Value{Kind: String, Str: "0"}
		}),
		"prev-not-hex": base(func(v *Value) {
			v.Obj[1].Val = &Value{Kind: String, Str: "xyz"}
		}),
		"at-bad": base(func(v *Value) {
			v.Obj[2].Val = &Value{Kind: String, Str: "now"}
		}),
		"actor-bad": base(func(v *Value) {
			v.Obj[4].Val = &Value{Kind: String, Str: "mallory"}
		}),
		"type-unknown": base(func(v *Value) {
			v.Obj[5].Val = &Value{Kind: String, Str: "bogus.type"}
		}),
		"body-not-object": base(func(v *Value) {
			v.Obj[6].Val = &Value{Kind: Number, Num: "5"}
		}),
		"not-object":       []byte(`[1,2]`),
		"trailing-garbage": append(base(func(v *Value) {}), ' ', 'x'),
		"dup-key": []byte(
			`{"seq":0,"seq":1,"prev":"` + strings.Repeat("0", 64) +
				`","at":"2026-11-15T00:00:00.000Z","commission":"C1",` +
				`"actor":"agent:a1","type":"tool.event","body":{},` +
				`"hash":"` + strings.Repeat("0", 64) + `"}`),
		"lone-surrogate": []byte(
			`{"seq":0,"prev":"` + strings.Repeat("0", 64) +
				`","at":"2026-11-15T00:00:00.000Z","commission":"C1",` +
				`"actor":"agent:a1","type":"tool.event","body":{},` +
				`"hash":"` + strings.Repeat("0", 64) + `","x":"\ud800"}`),
	}
	for name, line := range cases {
		if _, err := DecodeEnvelope(line); err == nil {
			t.Fatalf("%s: accepted", name)
		}
	}
}

// TestEnvelopeStrict covers seq edge cases and field type strictness.
func TestEnvelopeStrict(t *testing.T) {
	mk := func(seqLit string) []byte {
		bv, _ := BodyFrom(toolBody(0))
		v := &Value{Kind: Object, Obj: []Member{
			{Key: "seq", Val: &Value{Kind: Number, Num: seqLit}},
			{Key: "prev", Val: &Value{Kind: String, Str: ZeroPrev}},
			{Key: "at", Val: &Value{Kind: String, Str: "2026-11-15T00:00:00.000Z"}},
			{Key: "commission", Val: &Value{Kind: String, Str: "C1"}},
			{Key: "actor", Val: &Value{Kind: String, Str: "agent:a1"}},
			{Key: "type", Val: &Value{Kind: String, Str: "tool.event"}},
			{Key: "body", Val: bv},
			{Key: "hash", Val: &Value{Kind: String, Str: strings.Repeat("0", 64)}},
		}}
		return MustCanonical(v)
	}
	for _, lit := range []string{"1.5", "-1", "1e20", "9007199254740993"} {
		if _, err := DecodeEnvelope(mk(lit)); err == nil {
			t.Fatalf("seq %s accepted", lit)
		}
	}
	for _, lit := range []string{"0", "1", "9007199254740991", "1e3"} {
		ev, err := DecodeEnvelope(mk(lit))
		if err != nil {
			t.Fatalf("seq %s rejected: %v", lit, err)
		}
		want, _ := json.Number(lit).Float64()
		if ev.Seq != int64(want) {
			t.Fatalf("seq %s decoded as %d", lit, ev.Seq)
		}
	}
}

// TestBodyFieldShapes asserts the body-type ↔ schema field-name bijection:
// the JSON names on each Go body struct are exactly the schema's properties.
func TestBodyFieldShapes(t *testing.T) {
	if err := LoadSchemas(""); err != nil {
		t.Fatal(err)
	}
	for _, typ := range TypeNames() {
		rt, _ := BodyType(typ)
		sch := Schemas[typ]
		props, _ := objGet(sch, "properties")
		bodySch, _ := objGet(props, "body")
		sp, _ := objGet(bodySch, "properties")
		want := map[string]bool{}
		for _, m := range sp.Obj {
			want[m.Key] = true
		}
		got := map[string]bool{}
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			j := strings.Split(f.Tag.Get("json"), ",")[0]
			if j == "" || j == "-" {
				continue
			}
			got[j] = true
			if !want[j] {
				t.Fatalf("%s: Go field %q not in schema", typ, j)
			}
		}
		for w := range want {
			if !got[w] {
				t.Fatalf("%s: schema field %q has no Go field", typ, w)
			}
		}
	}
}
