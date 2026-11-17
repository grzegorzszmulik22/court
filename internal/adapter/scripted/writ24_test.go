package scripted_test

// C2.4.x — writs and spawn.

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"court/internal/adapter/scripted"
	"court/internal/ledger"
	cwrit "court/internal/writ"
)

func writHarness(t *testing.T) (*scripted.Harness, string) {
	t.Helper()
	h := newHarnessC(t, "cw")
	if err := h.OpenCommissionID("cw", "goal", nil); err != nil {
		t.Fatal(err)
	}
	root := cwrit.Root("orch", []string{"src/**"},
		ledger.Budgets{Tokens: 100, Minutes: 50, Speculation: 10})
	wid, err := h.IssueWrit("cw", root)
	if err != nil {
		t.Fatal(err)
	}
	return h, wid
}

func spawnCall(c *scripted.Client, args map[string]any) (map[string]any, error) {
	return c.CallTool("spawn", args)
}

func evs(t *testing.T, h *scripted.Harness) []*ledger.Event {
	t.Helper()
	e, err := h.Events()
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func ofType(evs []*ledger.Event, typ string) []*ledger.Event {
	var out []*ledger.Event
	for _, e := range evs {
		if e.Type == typ {
			out = append(out, e)
		}
	}
	return out
}

func field(e *ledger.Event, k string) string {
	for _, m := range e.Body.Obj {
		if m.Key == k {
			return m.Val.Str
		}
	}
	return ""
}

// TestC2_4_2: a guarantee category in a requested writ is refused.
func TestC2_4_2(t *testing.T) {
	h, wid := writHarness(t)
	c, err := h.AddSession("cw", &scripted.AgentSpec{ID: "orch", Writ: wid})
	if err != nil {
		t.Fatal(err)
	}
	_, err = spawnCall(c, map[string]any{
		"agent": "k1", "task": "t",
		"categories": []string{"guardrail"},
		"territory":  []string{"src/**"},
		"budgets":    map[string]any{"tokens": 10, "minutes": 5, "speculation": 1},
	})
	if err == nil {
		t.Fatal("guarantee-category writ was issued")
	}
	ref := ofType(evs(t, h), "writ.refused")
	if len(ref) != 1 || !strings.Contains(field(ref[0], "rule"), "categor") {
		t.Fatalf("want one writ.refused on categories, got %v", ref)
	}
}

// TestC2_4_3: every budget narrows (spec/01 §9). For each of tokens,
// minutes and speculation the check covers a request above the parent's
// total, a request inside the total but above the parent's remaining
// after earlier children, two spawns whose sum exceeds it (the second
// refused) and the exact boundary — equal to the remaining issues, one
// more refuses. The refusal must name the budget that bound, so a writ
// path that never checks one budget fails here.
func TestC2_4_3(t *testing.T) {
	// The harness root writ is {tokens:100, minutes:50, speculation:10}.
	type budget struct {
		name  string
		key   string
		total int64
	}
	budgets := []budget{
		{"tokens", "tokens", 100},
		{"minutes", "minutes", 50},
		{"speculation", "speculation", 10},
	}
	// mk builds a spawn call asking v of budget b (and 1 of each other,
	// safely inside every limit).
	mk := func(b budget, agent string, v int64) map[string]any {
		bs := map[string]any{"tokens": int64(1), "minutes": int64(1),
			"speculation": int64(1)}
		bs[b.key] = v
		return map[string]any{
			"agent": agent, "task": "t", "categories": []string{"interface"},
			"territory": []string{"src/**"}, "budgets": bs,
		}
	}
	orch := func(t *testing.T, h *scripted.Harness, wid string) *scripted.Client {
		c, err := h.AddSession("cw", &scripted.AgentSpec{ID: "orch", Writ: wid})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	// refuse asserts one refused spawn: the tool call fails, a
	// writ.refused on rule "budgets" names the field, and no writ.issued
	// names the child.
	refuse := func(t *testing.T, h *scripted.Harness, c *scripted.Client,
		b budget, args map[string]any) {
		t.Helper()
		if _, err := spawnCall(c, args); err == nil {
			t.Fatalf("%s: spawn that breaks the %s budget was issued", b.name, b.name)
		}
		issuedFor := func(agent string) bool {
			for _, e := range ofType(evs(t, h), "writ.issued") {
				if field(e, "agent") == agent {
					return true
				}
			}
			return false
		}
		ref := ofType(evs(t, h), "writ.refused")
		var last *ledger.Event
		if len(ref) > 0 {
			last = ref[len(ref)-1]
		}
		if last == nil || field(last, "rule") != "budgets" ||
			!strings.Contains(field(last, "detail"), b.name) {
			t.Fatalf("%s: want writ.refused on budgets naming %q, got %v",
				b.name, b.name, ref)
		}
		if issuedFor(args["agent"].(string)) {
			t.Fatalf("%s: refused spawn %s has a writ.issued", b.name, args["agent"])
		}
	}
	for _, b := range budgets {
		t.Run(b.name+"/above_total", func(t *testing.T) {
			h, wid := writHarness(t)
			c := orch(t, h, wid)
			refuse(t, h, c, b, mk(b, "k1", b.total+1))
		})
		t.Run(b.name+"/above_remaining", func(t *testing.T) {
			h, wid := writHarness(t)
			c := orch(t, h, wid)
			// Two earlier children spend 60% of the total between them;
			// the next request is inside the total but above the 40% left.
			if _, err := spawnCall(c, mk(b, "k1", b.total*3/10)); err != nil {
				t.Fatalf("first spawn refused: %v", err)
			}
			if _, err := spawnCall(c, mk(b, "k2", b.total*3/10)); err != nil {
				t.Fatalf("second spawn refused: %v", err)
			}
			// remaining = total - 0.6*total = 0.4*total; total/2 exceeds it
			// while staying under the total itself.
			refuse(t, h, c, b, mk(b, "k3", b.total/2))
		})
		t.Run(b.name+"/together_exceed", func(t *testing.T) {
			h, wid := writHarness(t)
			c := orch(t, h, wid)
			// Each request fits alone; their sum is over the total.
			if _, err := spawnCall(c, mk(b, "k1", b.total*6/10)); err != nil {
				t.Fatalf("first spawn refused: %v", err)
			}
			refuse(t, h, c, b, mk(b, "k2", b.total*5/10))
		})
		t.Run(b.name+"/boundary", func(t *testing.T) {
			h, wid := writHarness(t)
			c := orch(t, h, wid)
			if _, err := spawnCall(c, mk(b, "k1", b.total*4/10)); err != nil {
				t.Fatalf("first spawn refused: %v", err)
			}
			// Exactly the remaining budget issues.
			if _, err := spawnCall(c, mk(b, "k2", b.total*6/10)); err != nil {
				t.Fatalf("spawn at the exact remaining budget refused: %v", err)
			}
			// One more unit is over.
			refuse(t, h, c, b, mk(b, "k3", 1))
			// And only the two issued children have writs.
			if n := len(ofType(evs(t, h), "writ.issued")); n != 3 { // root+k1+k2
				t.Fatalf("want 3 writ.issued (root+k1+k2), got %d", n)
			}
		})
	}
}

// TestC2_4_4: a refused spawn creates no world and no process — no
// world.created or agent.started for the child.
func TestC2_4_4(t *testing.T) {
	h, wid := writHarness(t)
	c, err := h.AddSession("cw", &scripted.AgentSpec{ID: "orch", Writ: wid})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := spawnCall(c, map[string]any{
		"agent": "k1", "task": "t", "categories": []string{"interface"},
		"territory": []string{"etc/**"}, // outside parent's src/**
		"budgets":   map[string]any{"tokens": 10, "minutes": 5, "speculation": 1},
	}); err == nil {
		t.Fatal("out-of-territory spawn was issued")
	}
	for _, e := range evs(t, h) {
		if e.Type == "world.created" && field(e, "world") == "cw-orch-world-k1" {
			t.Fatal("refused spawn created a world")
		}
		if e.Type == "agent.started" && field(e, "agent") == "k1" {
			t.Fatal("refused spawn started an agent")
		}
		if e.Type == "writ.issued" && field(e, "agent") == "k1" {
			t.Fatal("refused spawn recorded writ.issued")
		}
	}
	// No worktree directory for the refused world either.
	glob := filepath.Join(h.DataDir, "worlds", "*k1*")
	if m, _ := filepath.Glob(glob); len(m) > 0 {
		t.Fatalf("refused spawn left a worktree: %v", m)
	}
}

// TestC2_4_5: a declared decision outside the agent's writ is marked
// outside-writ.
func TestC2_4_5(t *testing.T) {
	h, wid := writHarness(t)
	c, err := h.AddSession("cw", &scripted.AgentSpec{ID: "orch", Writ: wid})
	if err != nil {
		t.Fatal(err)
	}
	declare := func(cat string, paths []string) {
		t.Helper()
		if _, err := c.CallTool("record_decision", map[string]any{
			"question": "q?", "category": cat, "paths": paths,
			"options": []map[string]any{{"id": "a", "summary": "a"}},
			"chosen":  "a",
		}); err != nil {
			t.Fatal(err)
		}
	}
	declare("authority", []string{"src/x.go"}) // category outside the writ
	declare("interface", []string{"etc/y"})    // path outside territory
	declare("interface", []string{"src/z.go"}) // inside
	var marks []bool
	for _, e := range ofType(evs(t, h), "decision.declared") {
		m := false
		for _, mb := range e.Body.Obj {
			if mb.Key == "outside_writ" && mb.Val.Bool {
				m = true
			}
		}
		marks = append(marks, m)
	}
	if len(marks) != 3 || !marks[0] || !marks[1] || marks[2] {
		t.Fatalf("outside-writ marks %v, want [true true false]", marks)
	}
}

// TestC2_4_6: a chain of five nested spawns narrows correctly at every
// level.
func TestC2_4_6(t *testing.T) {
	h, wid := writHarness(t)
	c, err := h.AddSession("cw", &scripted.AgentSpec{ID: "orch", Writ: wid})
	if err != nil {
		t.Fatal(err)
	}
	type level struct {
		cats []string
		terr string
		tok  int64
	}
	levels := []level{
		{[]string{"interface", "schema"}, "src/**", 80},
		{[]string{"interface"}, "src/a/**", 60},
		{[]string{"interface"}, "src/a/b/**", 40},
		{[]string{"interface"}, "src/a/b/c/**", 20},
		{[]string{"interface"}, "src/a/b/c/d/**", 10},
	}
	var mcpURL string
	for i, lv := range levels {
		args := map[string]any{
			"agent": fmt.Sprintf("k%d", i+1), "task": "t",
			"categories": lv.cats, "territory": []string{lv.terr},
			"budgets": map[string]any{
				"tokens": lv.tok, "minutes": 5, "speculation": 1},
		}
		var res map[string]any
		if i == 0 {
			res, err = c.CallTool("spawn", args)
		} else {
			cc := &scripted.Client{MCPURL: mcpURL}
			res, err = cc.CallTool("spawn", args)
		}
		if err != nil {
			t.Fatalf("level %d spawn refused: %v", i+1, err)
		}
		// The spawn result is the tool's text content.
		mcpURL = findMCPURL(res)
		if mcpURL == "" {
			t.Fatalf("level %d spawn returned no mcp_url", i+1)
		}
	}
	iss := ofType(evs(t, h), "writ.issued")
	if len(iss) != 6 { // root + 5 children
		t.Fatalf("want 6 writ.issued, got %d", len(iss))
	}
	// Each child's writ narrows its parent: check the deepest.
	last := iss[len(iss)-1]
	if field(last, "agent") != "k5" {
		t.Fatalf("deepest writ names %q, want k5", field(last, "agent"))
	}
	for _, m := range last.Body.Obj {
		if m.Key == "territory" && (len(m.Val.Arr) != 1 || m.Val.Arr[0].Str != "src/a/b/c/d/**") {
			t.Fatalf("deepest territory wrong: %v", m.Val)
		}
	}
}

func findMCPURL(res map[string]any) string {
	for _, c := range res["content"].([]any) {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if txt, ok := m["text"].(string); ok {
			for _, part := range strings.Split(txt, `"`) {
				if strings.Contains(part, "/mcp/") {
					return part
				}
			}
		}
	}
	return ""
}
