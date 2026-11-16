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
	"time"

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

// TestC2_5_2: more than seven candidates shows seven and reports the
// rest open, in the guarantee / single-list / both ordering.
func TestC2_5_2(t *testing.T) {
	f := newFixture(t, map[string]string{
		"go.mod": "module x\n", "x.go": "package x\n",
	})
	var b strings.Builder
	b.WriteString(`      - backbrief:
          goal: "restated"
          plan: {files: ["x.go"], lines: 10}
          expected:
`)
	// 10 candidates: one guarantee, five singles (four orchestrator-only
	// plus the clerk's own) and four foreseen by both lists.
	b.WriteString(`            - {question: "g-guard?", category: guardrail, paths: ["x.go"], intended_default: "y"}
            - {question: "o-only-1?", category: schema, paths: ["a"], intended_default: "y"}
            - {question: "o-only-2?", category: interface, paths: ["b"], intended_default: "y"}
            - {question: "o-only-3?", category: dependency, paths: ["c"], intended_default: "y"}
            - {question: "o-only-4?", category: outbound, paths: ["d"], intended_default: "y"}
            - {question: "shared-1?", category: behaviour, paths: ["s"], intended_default: "y"}
            - {question: "shared-2?", category: structure, paths: ["t"], intended_default: "y"}
            - {question: "shared-3?", category: scope, paths: ["u"], intended_default: "y"}
            - {question: "shared-4?", category: dependency, paths: ["v"], intended_default: "y"}
`)
	b.WriteString("      - sleep: {ms: 1500}\n")
	script := f.writeScript(b.String())
	// The clerk foresees all four "shared" ones plus one of its own.
	clerk := writeClerk(t, filepath.Dir(f.data), `{"expected_decisions":[
 {"question":"shared-1?","category":"behaviour","paths":["s"],"intended_default":"y"},
 {"question":"shared-2?","category":"structure","paths":["t"],"intended_default":"y"},
 {"question":"shared-3?","category":"scope","paths":["u"],"intended_default":"y"},
 {"question":"shared-4?","category":"dependency","paths":["v"],"intended_default":"y"},
 {"question":"c-only?","category":"territory","paths":["q"],"intended_default":"y"}]}`)
	// Answer every shown question yes.
	out, err := f.commissionPTY(strings.Repeat("y\n\n", 7), "g",
		"--data-dir", f.data, "--script", script, "--clerk", clerk)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	// Question prompts and the change prompt both end in "[y/n]" — the
	// shown count is questions asked, not prompts printed.
	asked := strings.Count(out, "[y/n]") - strings.Count(out, "change? [y/n]")
	if asked != 7 {
		t.Fatalf("want 7 questions shown, saw %d:\n%s", asked, out)
	}
	if !strings.Contains(out, "3 more stay open") {
		t.Fatalf("want the open count printed:\n%s", out)
	}
	// Ordering: the guarantee question is first, single-list entries
	// precede the both-lists ones.
	gi := strings.Index(out, "g-guard?")
	ci := strings.Index(out, "c-only?")
	si := strings.Index(out, "shared-1?")
	if !(gi >= 0 && gi < ci && ci < si) {
		t.Fatalf("ordering guarantee<single<both broken:\n%s", out)
	}
	if n := len(ruledEvents(t, f)); n != 7 {
		t.Fatalf("want 7 rulings, got %d", n)
	}
	// Only the seven shown were ruled — the orchestrator's D1..D6 (the
	// guarantee + four singles + shared-1) and the clerk-only D14.
	for _, e := range ruledEvents(t, f) {
		switch id := bodyStr(e, "decision"); id {
		case "D1", "D2", "D3", "D4", "D5", "D6", "D14":
		default:
			t.Fatalf("unshown candidate %s was ruled", id)
		}
	}
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

// waitFor polls the ledger for a matching event (helper shared with
// the M2.3 tests).
var _ = time.Second
