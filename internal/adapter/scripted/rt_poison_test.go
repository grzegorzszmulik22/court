package scripted_test

// Red-team reproduction: an agent call that the append path accepts can
// leave an event the ledger's own verifier rejects. `backbrief` passes
// plan.lines through without a bounds check (internal/tools/tools.go), and
// ledger.ValidateBody does not check it either, while
// api/schema/backbrief.submitted.schema.json requires plan.lines >= 0
// (minimum 0). The result is a ledger that `court ledger verify` and
// `court replay` reject for the whole commission.

import (
	"path/filepath"
	"testing"

	"court/internal/adapter/scripted"
	"court/internal/ledger"
)

func TestRT_BackbriefPoisonsLedger(t *testing.T) {
	h := newHarness(t)
	sc := scriptWith("w1", &scripted.Step{Backbrief: &scripted.BackbriefStep{
		Goal:     "restate the goal",
		Plan:     map[string]any{"files": []string{}, "lines": -1},
		Expected: []map[string]any{},
	}})
	if err := h.RunScript(sc); err != nil {
		t.Fatalf("script: %v", err)
	}
	evs, err := h.Events()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range evs {
		if e.Type == "backbrief.submitted" {
			found = true
		}
	}
	if !found {
		t.Fatalf("backbrief.submitted not in the ledger")
	}
	v := ledger.VerifyFile(filepath.Join(h.DataDir, "ledger.db"))
	if v.Class != ledger.ClassOK {
		t.Fatalf("ledger written through the front door does not verify: class=%s seq=%d %s",
			v.Class, v.Seq, v.Detail)
	}
}
