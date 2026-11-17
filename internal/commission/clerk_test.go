package commission_test

// C2.3.x — the blind clerk.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"court/internal/ledger"
)

const clerkFixture = `{"expected_decisions":[
 {"question":"which api shape?","category":"interface","paths":["src/api"],"intended_default":"rest"},
 {"question":"db schema?","category":"schema","paths":["db"],"intended_default":"sql"}]}`

const orchBackbrief = `      - backbrief:
          goal: "the commission goal"
          plan: {files: ["src/api/x.go"], lines: 30}
          expected:
            - {question: "which api shape?", category: interface, paths: ["src/api"], intended_default: "rest"}
`

func writeClerk(t *testing.T, dir, content string) string {
	t.Helper()
	p := filepath.Join(dir, "clerk.json")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestC2_3_5: the clerk's event precedes the orchestrator's first write
// tool event.
func TestC2_3_5(t *testing.T) {
	f := newFixture(t, map[string]string{
		"go.mod": "module x\n", "src/api/x.go": "package api\n",
	})
	clerk := writeClerk(t, filepath.Dir(f.data), clerkFixture)
	script := f.writeScript(orchBackbrief + `      - sleep: {ms: 400}
      - tool: {name: Edit, input: {file_path: "src/api/x.go"}}
`)
	out, err := f.commission(nil, "add an endpoint", "--data-dir", f.data,
		"--script", script, "--clerk", clerk)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	clerkEv := f.waitFor(func(e *ledger.Event) bool {
		return e.Type == "backbrief.submitted" && e.Actor == "clerk"
	}, 15*time.Second)
	if clerkEv == nil {
		t.Fatal("no clerk backbrief.submitted recorded")
	}
	// Give the script time to reach its write, then check order.
	f.waitFor(func(e *ledger.Event) bool {
		if e.Type != "tool.event" || e.Actor != "agent:orchestrator" {
			return false
		}
		return bodyStr(e, "tool") == "Edit"
	}, 15*time.Second)
	evs, err := f.events()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range evs {
		if e.Type != "tool.event" || e.Actor != "agent:orchestrator" {
			continue
		}
		tool := bodyStr(e, "tool")
		if readOnlyToolName(tool) || tool == "mcp__court__backbrief" {
			continue
		}
		if bodyStr(e, "outcome") == "denied" {
			continue
		}
		if e.Seq < clerkEv.Seq {
			t.Fatalf("first write tool %s at seq %d precedes clerk at %d",
				tool, e.Seq, clerkEv.Seq)
		}
		break
	}
}

func readOnlyToolName(tool string) bool {
	switch tool {
	case "Read", "NotebookRead", "Glob", "Grep", "LS":
		return true
	}
	return false
}

// TestC2_3_3: an invalid clerk output is recorded as invalid with its
// raw output kept, and the commission proceeds on the orchestrator's
// list with one line. The recorded event must satisfy the ledger
// schema — `court ledger verify` checks every stored body against it.
func TestC2_3_3(t *testing.T) {
	f := newFixture(t, map[string]string{
		"go.mod": "module x\n", "x.go": "package x\n",
	})
	const clerkRaw = "this is not json at all"
	clerk := writeClerk(t, filepath.Dir(f.data), clerkRaw)
	script := f.writeScript(orchBackbrief + `      - sleep: {ms: 400}
      - tool: {name: Edit, input: {file_path: "x.go"}}
`)
	out, err := f.commission(nil, "goal", "--data-dir", f.data,
		"--script", script, "--clerk", clerk)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "invalid") || !strings.Contains(out, "orchestrator") {
		t.Fatalf("missing the one-line notice: %s", out)
	}
	ev := f.waitFor(func(e *ledger.Event) bool {
		return e.Type == "backbrief.submitted" && e.Actor == "clerk"
	}, 15*time.Second)
	if ev == nil {
		t.Fatal("invalid clerk output was not recorded")
	}
	var found bool
	for _, m := range ev.Body.Obj {
		if m.Key == "invalid" && m.Val.Bool {
			found = true
		}
	}
	if !found {
		t.Fatal("clerk event is not marked invalid")
	}
	if got := bodyStr(ev, "output"); got != clerkRaw {
		t.Fatalf("recorded output %q, want the raw clerk output %q", got, clerkRaw)
	}
	// The stored event must verify against the schema: an undeclared
	// body field fails `court ledger verify` (additionalProperties).
	db := filepath.Join(f.data, "ledger.db")
	vout, verr := exec.Command(f.bin, "ledger", "verify", db).CombinedOutput()
	if verr != nil || !strings.Contains(string(vout), " ok") {
		t.Fatalf("ledger verify failed on the invalid clerk event: %v\n%s",
			verr, vout)
	}
}

// TestC2_3_4: no clerk configured — the C2.3.3 path without error.
func TestC2_3_4(t *testing.T) {
	f := newFixture(t, map[string]string{"go.mod": "module x\n"})
	script := f.writeScript(orchBackbrief)
	out, err := f.commission(nil, "goal", "--data-dir", f.data,
		"--script", script)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "no clerk model configured") {
		t.Fatalf("missing the no-model line: %s", out)
	}
}

// TestC2_3_1: the clerk's request holds the commission and nothing of
// the orchestrator's backbrief or the repository's files.
func TestC2_3_1(t *testing.T) {
	f := newFixture(t, map[string]string{
		"go.mod": "module x\n", "secret.go": "package x // SECRETTOKEN\n",
	})
	sink := filepath.Join(filepath.Dir(f.data), "clerk-stdin.json")
	canned := writeClerk(t, filepath.Dir(f.data), clerkFixture)
	script := f.writeScript(orchBackbrief)
	cmd := "cat > " + sink + "; cat " + canned
	out, err := f.commission(nil, "my commission goal", "--data-dir", f.data,
		"--script", script, "--clerk-cmd", cmd)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	raw, err := os.ReadFile(sink)
	if err != nil {
		t.Fatalf("clerk request not on stdin: %v", err)
	}
	var req map[string]any
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatalf("request is not JSON: %v", err)
	}
	s := string(raw)
	if !strings.Contains(s, "my commission goal") {
		t.Fatal("request lacks the commission goal")
	}
	for _, banned := range []string{"SECRETTOKEN", "src/api/x.go", "which api shape", "rest"} {
		if strings.Contains(s, banned) {
			t.Fatalf("request leaks %q into the blind clerk", banned)
		}
	}
	for _, k := range []string{"commission", "goal", "schema"} {
		if _, ok := req[k]; !ok {
			t.Fatalf("request lacks key %q", k)
		}
	}
}
