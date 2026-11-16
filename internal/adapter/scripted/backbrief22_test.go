package scripted_test

// C2.2.x — backbrief and the ordering gate.

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"court/internal/adapter/scripted"
	"court/internal/ledger"
)

func newHarnessC(t *testing.T, commission string) *scripted.Harness {
	t.Helper()
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := scripted.InitRepo(repo, map[string]string{
		"main.go": "package main\n\nfunc main() {}\n",
	}); err != nil {
		t.Fatal(err)
	}
	h, err := scripted.NewHarness(scripted.Options{
		Repo: repo, DataDir: filepath.Join(root, "data"),
		Commission: commission,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.Close)
	return h
}

// backbriefArgs builds one valid backbrief call.
func backbriefArgs(goal string, exp ...map[string]any) map[string]any {
	return map[string]any{
		"goal":               goal,
		"plan":               map[string]any{"files": []string{"a.go"}, "lines": 10},
		"expected_decisions": exp,
		"will_not_touch":     []string{},
	}
}

// submitBackbrief drives the backbrief call the way a session does:
// PreToolUse, the MCP call, PostToolUse.
func submitBackbrief(t *testing.T, c *scripted.Client, args map[string]any) {
	t.Helper()
	args2 := map[string]any{}
	for k, v := range args {
		args2[k] = v
	}
	tuid, denied, err := c.PreToolUse("mcp__court__backbrief", args2, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if denied {
		t.Fatal("backbrief itself was denied")
	}
	if _, err := c.CallTool("backbrief", args); err != nil {
		t.Fatalf("backbrief call: %v", err)
	}
	if err := c.PostToolUse(tuid, "mcp__court__backbrief", args2, false, "", ""); err != nil {
		t.Fatal(err)
	}
}

// TestC2_2_1: MM2.2 — 500 commissions, zero agent tool events ahead of
// the stored expected decisions.
func TestC2_2_1(t *testing.T) {
	h := newHarnessC(t, "c22")
	const N = 500
	for i := 0; i < N; i++ {
		comm := fmt.Sprintf("c%05d", i)
		if err := h.OpenCommissionID(comm, "goal", nil); err != nil {
			t.Fatal(err)
		}
		c, err := h.AddSession(comm, &scripted.AgentSpec{
			ID: fmt.Sprintf("orch-%d", i), RequireBackbrief: true})
		if err != nil {
			t.Fatal(err)
		}
		// Every non-read call ahead of the backbrief is denied.
		for _, tool := range []string{"Edit", "Write", "Bash"} {
			_, denied, err := c.PreToolUse(tool,
				map[string]any{"file_path": "x.go", "command": "make"}, "", "")
			if err != nil {
				t.Fatal(err)
			}
			if !denied {
				t.Fatalf("commission %s: %s before backbrief was not denied", comm, tool)
			}
		}
		submitBackbrief(t, c, backbriefArgs("the goal", map[string]any{
			"question": "q?", "category": "interface",
			"paths": []string{"x.go"}, "intended_default": "d"}))
		if _, denied, err := c.PreToolUse("Edit",
			map[string]any{"file_path": "x.go"}, "", ""); err != nil {
			t.Fatal(err)
		} else if denied {
			t.Fatalf("commission %s: Edit after backbrief denied", comm)
		}
	}
	// Ledger order: in every commission, no allowed non-read tool.event
	// precedes that commission's backbrief.submitted.
	evs, err := h.Events()
	if err != nil {
		t.Fatal(err)
	}
	briefSeq := map[string]int64{}
	violations := 0
	for _, e := range evs {
		if e.Type == "backbrief.submitted" && strings.HasPrefix(e.Actor, "agent:orch-") {
			if _, seen := briefSeq[e.Commission]; !seen {
				briefSeq[e.Commission] = e.Seq
			}
		}
		if e.Type == "tool.event" && strings.HasPrefix(e.Actor, "agent:orch-") {
			tool, outcome := "", ""
			for _, m := range e.Body.Obj {
				if m.Key == "tool" {
					tool = m.Val.Str
				}
				if m.Key == "outcome" {
					outcome = m.Val.Str
				}
			}
			if outcome == "denied" || tool == "mcp__court__backbrief" {
				continue
			}
			if seq, ok := briefSeq[e.Commission]; !ok || e.Seq < seq {
				violations++
			}
		}
	}
	if violations != 0 {
		t.Fatalf("MM2.2 violated: %d tool events precede their backbrief", violations)
	}
}

// TestC2_2_2: an expected decision without a category is rejected with
// the valid list.
func TestC2_2_2(t *testing.T) {
	h := newHarnessC(t, "c22b")
	c, err := h.AddSession("c22b", &scripted.AgentSpec{
		ID: "orch", RequireBackbrief: true})
	if err != nil {
		t.Fatal(err)
	}
	args := backbriefArgs("the goal", map[string]any{
		"question": "q?", "paths": []string{"x.go"}, "intended_default": "d"})
	if _, err := c.CallTool("backbrief", args); err == nil {
		t.Fatal("category-less expected decision was accepted")
	} else {
		for _, want := range []string{"dependency", "interface", "behaviour"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("rejection does not list valid category %q: %v", want, err)
			}
		}
	}
}

// TestC2_2_3: read-only calls before the backbrief are allowed and
// recorded.
func TestC2_2_3(t *testing.T) {
	h := newHarnessC(t, "c22c")
	c, err := h.AddSession("c22c", &scripted.AgentSpec{
		ID: "orch", RequireBackbrief: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"Read", "Grep", "Glob", "LS"} {
		tuid, denied, err := c.PreToolUse(tool,
			map[string]any{"file_path": "x.go", "path": "src", "pattern": "p"}, "", "")
		if err != nil {
			t.Fatal(err)
		}
		if denied {
			t.Fatalf("read-only %s before backbrief denied", tool)
		}
		if err := c.PostToolUse(tuid, tool,
			map[string]any{}, false, "", ""); err != nil {
			t.Fatal(err)
		}
	}
	var reads int
	evs, err := h.Events()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range evs {
		if e.Type == "tool.event" {
			for _, m := range e.Body.Obj {
				if m.Key == "outcome" && m.Val.Str == "ok" {
					reads++
				}
			}
		}
	}
	if reads != 4 {
		t.Fatalf("want 4 recorded read-only events, got %d", reads)
	}
}

// TestC2_2_4: a second backbrief is a revision — both events stand and
// the expected decisions of both are kept.
func TestC2_2_4(t *testing.T) {
	h := newHarnessC(t, "c22d")
	c, err := h.AddSession("c22d", &scripted.AgentSpec{
		ID: "orch", RequireBackbrief: true})
	if err != nil {
		t.Fatal(err)
	}
	submitBackbrief(t, c, backbriefArgs("the goal", map[string]any{
		"question": "first?", "category": "interface",
		"paths": []string{"a.go"}, "intended_default": "x"}))
	submitBackbrief(t, c, backbriefArgs("the goal, again", map[string]any{
		"question": "second?", "category": "schema",
		"paths": []string{"b.go"}, "intended_default": "y"}))
	var briefs []*ledger.Event
	evs, err := h.Events()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range evs {
		if e.Type == "backbrief.submitted" && e.Actor == "agent:orch" {
			briefs = append(briefs, e)
		}
	}
	if len(briefs) != 2 {
		t.Fatalf("want 2 backbrief events, got %d", len(briefs))
	}
	got := map[string]bool{}
	for _, e := range briefs {
		for _, m := range e.Body.Obj {
			if m.Key != "expected_decisions" {
				continue
			}
			for _, item := range m.Val.Arr {
				for _, f := range item.Obj {
					if f.Key == "question" {
						got[f.Val.Str] = true
					}
				}
			}
		}
	}
	if !got["first?"] || !got["second?"] {
		t.Fatalf("revision lost expected decisions: %v", got)
	}
}

// TestC2_2_5: the plan's file list and line estimate are stored in the
// shape DT12 reads (plan.files list, plan.lines number).
func TestC2_2_5(t *testing.T) {
	h := newHarnessC(t, "c22e")
	c, err := h.AddSession("c22e", &scripted.AgentSpec{
		ID: "orch", RequireBackbrief: true})
	if err != nil {
		t.Fatal(err)
	}
	args := backbriefArgs("the goal")
	args["plan"] = map[string]any{"files": []string{"a.go", "b.go"}, "lines": 42}
	submitBackbrief(t, c, args)
	var plan map[string]any
	for _, e := range mustEvents(t, h) {
		if e.Type != "backbrief.submitted" {
			continue
		}
		for _, m := range e.Body.Obj {
			if m.Key == "plan" {
				plan = map[string]any{}
				for _, f := range m.Val.Obj {
					switch f.Key {
					case "files":
						var fs []string
						for _, v := range f.Val.Arr {
							fs = append(fs, v.Str)
						}
						plan["files"] = fs
					case "lines":
						plan["lines"] = f.Val.Num
					}
				}
			}
		}
	}
	if plan == nil {
		t.Fatal("no backbrief plan recorded")
	}
	fs, _ := plan["files"].([]string)
	if len(fs) != 2 || fs[0] != "a.go" || fs[1] != "b.go" {
		t.Fatalf("plan.files not stored: %v", plan["files"])
	}
	if plan["lines"] != "42" {
		t.Fatalf("plan.lines not stored: %v", plan["lines"])
	}
}

func mustEvents(t *testing.T, h *scripted.Harness) []*ledger.Event {
	t.Helper()
	evs, err := h.Events()
	if err != nil {
		t.Fatal(err)
	}
	return evs
}
