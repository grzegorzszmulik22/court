package scripted_test

// B3-03 permanent tests: an agent tool call that would write an event the
// verifier rejects fails instead, and writes nothing. The ledger stays
// verifiable after any sequence of hostile tool inputs.
// Named TestC1_5_7* so check C1.5.7 runs them.

import (
	"math/rand"
	"path/filepath"
	"testing"

	"court/internal/adapter/scripted"
	"court/internal/ledger"
)

// TestC1_5_7_BackbriefPoisonsLedger: the red-team case. plan.lines = -1
// must fail the call and leave no event; the ledger still verifies.
func TestC1_5_7_BackbriefPoisonsLedger(t *testing.T) {
	h := newHarness(t)
	c, err := h.AddAgent(&scripted.AgentSpec{ID: "w1", Parent: "human"})
	if err != nil {
		t.Fatal(err)
	}
	args := map[string]any{
		"goal":               "restate the goal",
		"plan":               map[string]any{"files": []string{}, "lines": -1},
		"expected_decisions": []map[string]any{},
		"will_not_touch":     []string{},
	}
	tuid, denied, err := c.PreToolUse("mcp__court__backbrief", args, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if denied {
		t.Fatal("backbrief denied")
	}
	_, callErr := c.CallTool("backbrief", args)
	if callErr == nil {
		t.Fatal("backbrief with plan.lines = -1 was accepted")
	}
	if err := c.PostToolUse(tuid, "mcp__court__backbrief", args, true, "", ""); err != nil {
		t.Fatal(err)
	}
	evs, err := h.Events()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range evs {
		if e.Type == "backbrief.submitted" {
			t.Fatal("the invalid backbrief is in the ledger")
		}
	}
	if v := ledger.VerifyFile(filepath.Join(h.DataDir, "ledger.db")); v.Class != ledger.ClassOK {
		t.Fatalf("ledger does not verify after the rejected call: %s seq=%d %s",
			v.Class, v.Seq, v.Detail)
	}
}

// TestC1_5_7_HostileToolInputs: a seeded random sequence of hostile and
// benign tool inputs. Every hostile call errors and writes nothing (one
// denied/error tool.event per attempt at most); the ledger verifies at
// the end.
func TestC1_5_7_HostileToolInputs(t *testing.T) {
	h := newHarness(t)
	c, err := h.AddAgent(&scripted.AgentSpec{ID: "w1", Parent: "human"})
	if err != nil {
		t.Fatal(err)
	}
	rnd := rand.New(rand.NewSource(20261117))
	hostile := []map[string]any{
		{"name": "backbrief", "args": map[string]any{
			"goal":               "g",
			"plan":               map[string]any{"files": []string{}, "lines": -1},
			"expected_decisions": []map[string]any{}, "will_not_touch": []string{}}},
		{"name": "backbrief", "args": map[string]any{
			"goal":               "g",
			"plan":               map[string]any{"files": []string{"a"}, "lines": -9223372036854775808},
			"expected_decisions": []map[string]any{}, "will_not_touch": []string{}}},
		{"name": "record_decision", "args": map[string]any{
			"question": "not a question", "category": "interface",
			"paths": []string{}, "options": []map[string]any{}, "chosen": "", "petition": false}},
		{"name": "record_decision", "args": map[string]any{
			"question": "q?", "category": "bogus-category",
			"paths": []string{}, "options": []map[string]any{}, "chosen": "", "petition": false}},
		{"name": "record_decision", "args": map[string]any{
			"question": "q?", "category": "interface", "paths": []string{"x"},
			"options": []map[string]any{{"id": "a", "summary": "s"}},
			"chosen":  "not-an-option", "petition": false}},
		{"name": "claim_done", "args": map[string]any{
			"task": "", "checks": []string{}}},
		{"name": "alarm", "args": map[string]any{
			"text": "", "paths": []string{}}},
	}
	rejected, accepted := 0, 0
	for i := 0; i < 40; i++ {
		h := hostile[rnd.Intn(len(hostile))]
		if _, err := c.CallTool(h["name"].(string), h["args"].(map[string]any)); err != nil {
			rejected++
		} else {
			accepted++
		}
	}
	if rejected == 0 {
		t.Fatal("no hostile input was rejected — the sequence was useless")
	}
	evs, err := h.Events()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range evs {
		if e.Type == "backbrief.submitted" || e.Type == "decision.declared" ||
			e.Type == "claim.done" || e.Type == "alarm.raised" {
			t.Fatalf("a hostile call left a %s event", e.Type)
		}
	}
	if v := ledger.VerifyFile(filepath.Join(h.DataDir, "ledger.db")); v.Class != ledger.ClassOK {
		t.Fatalf("ledger does not verify after hostile inputs: %s seq=%d %s",
			v.Class, v.Seq, v.Detail)
	}
	t.Logf("hostile calls rejected=%d accepted=%d", rejected, accepted)
}
