package commission_test

// C2.5.x — the upfront hearing. Interactive runs go through a real pty
// (script -E never) so isatty sees a terminal; piped runs exercise the
// non-terminal skip.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"court/internal/ledger"
)

// commissionPTY runs `court commission` with stdin on a pty fed the
// answer lines in answers.
func (f *fixture) commissionPTY(answers string, args ...string) (string, error) {
	f.t.Helper()
	argv := append([]string{"commission"}, args...)
	for i, a := range argv {
		argv[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	line := fmt.Sprintf("cd %s && exec %s %s", f.repo, f.bin,
		strings.Join(argv, " "))
	cmd := exec.Command("script", "-qec", line, "-E", "never", os.DevNull)
	cmd.Stdin = strings.NewReader(answers)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func ruledEvents(t *testing.T, f *fixture) []*ledger.Event {
	t.Helper()
	evs, err := f.events()
	if err != nil {
		t.Fatal(err)
	}
	var out []*ledger.Event
	for _, e := range evs {
		if e.Type == "decision.ruled" {
			out = append(out, e)
		}
	}
	return out
}

const hearingScript = `      - backbrief:
          goal: "restate-MARKER: the goal restated"
          plan: {files: ["x.go"], lines: 10}
          expected:
            - {question: "keep the flag on?", category: behaviour, paths: ["x.go"], intended_default: "yes"}
            - {question: "guard the handler?", category: guardrail, paths: ["x.go"], intended_default: "no"}
      - sleep: {ms: 1500}
`

// TestC2_5_1: one Enter starts the work; nothing is ruled and every
// expected decision stays open.
func TestC2_5_1(t *testing.T) {
	f := newFixture(t, map[string]string{
		"go.mod": "module x\n", "x.go": "package x\n",
	})
	script := f.writeScript(hearingScript)
	out, err := f.commissionPTY("\n", "hearing-goal", "--data-dir", f.data,
		"--script", script)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "skipped — stays open") {
		t.Fatalf("expected skipped questions in output:\n%s", out)
	}
	if n := len(ruledEvents(t, f)); n != 0 {
		t.Fatalf("%d decision.ruled events after a bare Enter", n)
	}
	if !strings.Contains(out, "hearing: 0 ruled, 2 open") {
		t.Fatalf("expected all open in summary:\n%s", out)
	}
}

// TestC2_5_6: the intended default never appears before its question's
// answer is read — asserted on the output stream's order.
func TestC2_5_6(t *testing.T) {
	f := newFixture(t, map[string]string{
		"go.mod": "module x\n", "x.go": "package x\n",
	})
	script := f.writeScript(`      - backbrief:
          goal: "restated"
          plan: {files: ["x.go"], lines: 10}
          expected:
            - {question: "first q?", category: behaviour, paths: ["x.go"], intended_default: "DEFAULT-AAA"}
            - {question: "second q?", category: interface, paths: ["y.go"], intended_default: "DEFAULT-BBB"}
            - {question: "third q?", category: schema, paths: ["z.go"], intended_default: "DEFAULT-CCC"}
      - sleep: {ms: 1500}
`)
	// Answer all three: y keep, n keep, y keep.
	out, err := f.commissionPTY("y\n\nn\n\ny\n\n", "g", "--data-dir", f.data,
		"--script", script)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for i, d := range []string{"DEFAULT-AAA", "DEFAULT-BBB", "DEFAULT-CCC"} {
		prompt := fmt.Sprintf("%d. ", i+1)
		pp := strings.Index(out, prompt)
		ap := strings.Index(out[pp:], "your answer:") + pp
		dp := strings.Index(out, d)
		if pp < 0 || ap < pp || dp < 0 {
			t.Fatalf("missing markers for %s:\n%s", d, out)
		}
		if dp < ap {
			t.Fatalf("default %s printed before the answer was read\n%s", d, out)
		}
	}
	// And three rulings landed.
	if n := len(ruledEvents(t, f)); n != 3 {
		t.Fatalf("want 3 decision.ruled, got %d", n)
	}
}

// TestC2_5_2: the hearing order and the seven-question cap. Guarantee
// categories come first whichever list foresaw them — orchestrator-only,
// clerk-only or both — then the decisions only one list foresaw (the
// orchestrator's, then the clerk's), then those both lists foresaw. The
// cap shows the first seven of that order, so the candidates it drops
// are always from the lowest group.
func TestC2_5_2(t *testing.T) {
	// scriptExpected renders the orchestrator's backbrief step.
	scriptExpected := func(lines ...string) string {
		return `      - backbrief:
          goal: "restated"
          plan: {files: ["x.go"], lines: 10}
          expected:
` + strings.Join(lines, "\n") + `
      - sleep: {ms: 1500}
`
	}
	// runCase runs one hearing over the pty, answering every shown
	// question yes, and returns the output.
	runCase := func(t *testing.T, f *fixture, script, clerk string) string {
		t.Helper()
		out, err := f.commissionPTY(strings.Repeat("y\n\n", 10), "g",
			"--data-dir", f.data, "--script", script, "--clerk", clerk)
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		return out
	}
	// assertOrder asserts the shown questions appear in exactly the want
	// order and none of absent appear at all.
	assertOrder := func(t *testing.T, out string, want, absent []string, open int) {
		t.Helper()
		asked := strings.Count(out, "[y/n]") - strings.Count(out, "change? [y/n]")
		if asked != 7 {
			t.Fatalf("want 7 questions shown, saw %d:\n%s", asked, out)
		}
		prev := -1
		for _, q := range want {
			i := strings.Index(out, q)
			if i < 0 {
				t.Fatalf("shown question %q missing:\n%s", q, out)
			}
			if i < prev {
				t.Fatalf("question %q out of order:\n%s", q, out)
			}
			prev = i
		}
		for _, q := range absent {
			if strings.Contains(out, q) {
				t.Fatalf("question %q should have stayed open, was shown:\n%s", q, out)
			}
		}
		if !strings.Contains(out, fmt.Sprintf("%d more stay open", open)) {
			t.Fatalf("want %q in output:\n%s",
				fmt.Sprintf("%d more stay open", open), out)
		}
	}
	// assertRuled asserts the decision ids of every ruling.
	assertRuled := func(t *testing.T, f *fixture, want map[string]bool) {
		t.Helper()
		evs := ruledEvents(t, f)
		if len(evs) != len(want) {
			t.Fatalf("want %d rulings, got %d", len(want), len(evs))
		}
		for _, e := range evs {
			if !want[bodyStr(e, "decision")] {
				t.Fatalf("unshown candidate %s was ruled", bodyStr(e, "decision"))
			}
		}
	}

	t.Run("full_order", func(t *testing.T) {
		f := newFixture(t, map[string]string{
			"go.mod": "module x\n", "x.go": "package x\n",
		})
		// Orchestrator D1..D7: orch-only guarantee, both-lists guarantee,
		// three orch-only singles, two both-lists.
		script := f.writeScript(scriptExpected(
			`            - {question: "g-orch?", category: guardrail, paths: ["g1"], intended_default: "y"}`,
			`            - {question: "shared-g?", category: test-strength, paths: ["sg"], intended_default: "y"}`,
			`            - {question: "o-only-1?", category: schema, paths: ["a"], intended_default: "y"}`,
			`            - {question: "o-only-2?", category: interface, paths: ["b"], intended_default: "y"}`,
			`            - {question: "o-only-3?", category: dependency, paths: ["c"], intended_default: "y"}`,
			`            - {question: "shared-1?", category: behaviour, paths: ["s1"], intended_default: "y"}`,
			`            - {question: "shared-2?", category: structure, paths: ["s2"], intended_default: "y"}`,
		))
		// Clerk D8..D12: clerk-only guarantee, the two shared matches,
		// clerk-only single — order: g first, then the shared entries.
		clerk := writeClerk(t, filepath.Dir(f.data), `{"expected_decisions":[
 {"question":"g-clerk?","category":"authority","paths":["ga"],"intended_default":"y"},
 {"question":"shared-g?","category":"test-strength","paths":["sg"],"intended_default":"y"},
 {"question":"shared-1?","category":"behaviour","paths":["s1"],"intended_default":"y"},
 {"question":"shared-2?","category":"structure","paths":["s2"],"intended_default":"y"},
 {"question":"c-only-1?","category":"territory","paths":["q"],"intended_default":"y"}]}`)
		out := runCase(t, f, script, clerk)
		// Order: guarantees D1(orch-only), D2(both), D8(clerk-only);
		// singles D3..D5 (orch), D12 (clerk); open: boths D6, D7.
		assertOrder(t, out,
			[]string{"g-orch?", "shared-g?", "g-clerk?", "o-only-1?",
				"o-only-2?", "o-only-3?", "c-only-1?"},
			[]string{"shared-1?", "shared-2?"}, 2)
		assertRuled(t, f, map[string]bool{
			"D1": true, "D2": true, "D3": true, "D4": true,
			"D5": true, "D8": true, "D12": true})
	})

	t.Run("cap_drops_lowest", func(t *testing.T) {
		f := newFixture(t, map[string]string{
			"go.mod": "module x\n", "x.go": "package x\n",
		})
		// Four guarantees (three orchestrator, one clerk-only) plus four
		// singles plus one both = nine candidates; the cap drops the
		// tail — one clerk single and the both-lists entry, never a
		// guarantee.
		script := f.writeScript(scriptExpected(
			`            - {question: "g-o-1?", category: guardrail, paths: ["p1"], intended_default: "y"}`,
			`            - {question: "g-o-2?", category: test-strength, paths: ["p2"], intended_default: "y"}`,
			`            - {question: "g-b-1?", category: failure-handling, paths: ["gb1"], intended_default: "y"}`,
			`            - {question: "o-s-1?", category: schema, paths: ["a"], intended_default: "y"}`,
			`            - {question: "o-s-2?", category: interface, paths: ["b"], intended_default: "y"}`,
			`            - {question: "pair-1?", category: behaviour, paths: ["bb"], intended_default: "y"}`,
		))
		clerk := writeClerk(t, filepath.Dir(f.data), `{"expected_decisions":[
 {"question":"g-c-1?","category":"authority","paths":["pc"],"intended_default":"y"},
 {"question":"g-b-1?","category":"failure-handling","paths":["gb1"],"intended_default":"y"},
 {"question":"c-s-1?","category":"territory","paths":["q1"],"intended_default":"y"},
 {"question":"c-s-2?","category":"outbound","paths":["q2"],"intended_default":"y"},
 {"question":"pair-1?","category":"behaviour","paths":["bb"],"intended_default":"y"}]}`)
		out := runCase(t, f, script, clerk)
		// Order: D1,D2,D3 (orch guarantees), D7 (clerk guarantee),
		// singles D4,D5 (orch) + D9,D10 (clerk), both D6 — shown are the
		// first seven; the last clerk single and the both stay open.
		assertOrder(t, out,
			[]string{"g-o-1?", "g-o-2?", "g-b-1?", "g-c-1?",
				"o-s-1?", "o-s-2?", "c-s-1?"},
			[]string{"c-s-2?", "pair-1?"}, 2)
		assertRuled(t, f, map[string]bool{
			"D1": true, "D2": true, "D3": true, "D4": true,
			"D5": true, "D7": true, "D9": true})
	})
}

// TestC2_5_3: standard input not a terminal skips the hearing with a
// notice and nothing is ruled.
func TestC2_5_3(t *testing.T) {
	f := newFixture(t, map[string]string{
		"go.mod": "module x\n", "x.go": "package x\n",
	})
	script := f.writeScript(hearingScript)
	out, err := f.commission(nil, "g", "--data-dir", f.data, "--script", script)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "upfront hearing skipped") {
		t.Fatalf("want the skip notice:\n%s", out)
	}
	if n := len(ruledEvents(t, f)); n != 0 {
		t.Fatalf("%d rulings under non-terminal stdin", n)
	}
	if strings.Contains(out, "[y/n]") {
		t.Fatalf("questions were asked under non-terminal stdin:\n%s", out)
	}
}

// TestC2_5_4: the restated goal shown is the orchestrator's ledger
// text — not the clerk's, not the raw commission string.
func TestC2_5_4(t *testing.T) {
	f := newFixture(t, map[string]string{
		"go.mod": "module x\n", "x.go": "package x\n",
	})
	script := f.writeScript(hearingScript)
	clerk := writeClerk(t, filepath.Dir(f.data), `{"expected_decisions":[
 {"question":"c-only?","category":"scope","paths":["q"],"intended_default":"y"}]}`)
	out, err := f.commissionPTY("\n\n\n", "RAW-GOAL-MARKER add endpoint",
		"--data-dir", f.data, "--script", script, "--clerk", clerk)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	hsec := out[strings.Index(out, "the orchestrator restates the goal:"):]
	if !strings.Contains(hsec, "restate-MARKER: the goal restated") {
		t.Fatalf("orchestrator's restated goal missing:\n%s", hsec)
	}
	if strings.Contains(hsec, "RAW-GOAL-MARKER") {
		t.Fatalf("hearing showed the raw goal instead of the restatement:\n%s", hsec)
	}
}

// TestC2_5_5: the hearing output for a fixture commission is identical
// on replay — same events, same answers, same bytes.
func TestC2_5_5(t *testing.T) {
	hearing := func(data string) string {
		f := newFixture(t, map[string]string{
			"go.mod": "module x\n", "x.go": "package x\n",
		})
		f.data = data
		script := f.writeScript(hearingScript)
		out, err := f.commissionPTY("y\n\n\n\n", "g", "--data-dir", f.data,
			"--script", script)
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		i := strings.Index(out, "the orchestrator restates the goal:")
		if i < 0 {
			t.Fatalf("no hearing section:\n%s", out)
		}
		return out[i:]
	}
	a := hearing(filepath.Join(t.TempDir(), "d1"))
	b := hearing(filepath.Join(t.TempDir(), "d2"))
	if a != b {
		t.Fatalf("hearing output differs across identical replays:\n%s\n---\n%s", a, b)
	}
}
