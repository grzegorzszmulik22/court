package scripted_test

// B3-09 permanent tests: a scripted-agent write path stays inside the
// world. `..`, an absolute path, and a symlink that leaves the world are
// denied — by the adapter's own check (even when the endpoint abstains)
// and by the harness policy, which records a denied tool.event with the
// reason. Named TestC1_5_7* so check C1.5.7 runs them.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"court/internal/adapter/scripted"
)

// TestC1_5_7_EditEscapesWorld: the red-team case. With an abstaining
// stand-in endpoint the adapter still refuses the write: the file is
// never created, the step errors naming the escape, and the attempted
// call is still posted (recorded) rather than silently dropped.
func TestC1_5_7_EditEscapesWorld(t *testing.T) {
	base := t.TempDir()
	worktree := filepath.Join(base, "data", "worlds", "w1")
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	posts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`)) // abstain, like the daemon's default policy
	}))
	defer srv.Close()
	c := &scripted.Client{
		HookURL: srv.URL, MCPURL: srv.URL, Worktree: worktree,
		SessionID: "s1", Agent: "a1",
	}
	err := c.RunStep(&scripted.Step{Edit: &scripted.EditStep{
		Path:  filepath.Join("..", "..", "..", "outside", "escape.txt"),
		Write: "CANARY-ESCAPED\n",
	}})
	if err == nil {
		t.Fatal("escape step ran to completion")
	}
	if _, err := os.Stat(filepath.Join(outside, "escape.txt")); err == nil {
		t.Fatal("scripted Edit wrote outside the world")
	}
	if posts < 2 {
		t.Fatalf("attempt not recorded through the hook flow (posts=%d)", posts)
	}
}

// TestC1_5_7_EditDenials covers every escape shape: an absolute path, a
// `..` that climbs out, and symlinks inside the world pointing outside.
// Each leaves no file outside the world; benign relative paths still
// write.
func TestC1_5_7_EditDenials(t *testing.T) {
	base := t.TempDir()
	worktree := filepath.Join(base, "data", "worlds", "w1")
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c := &scripted.Client{
		HookURL: srv.URL, MCPURL: srv.URL, Worktree: worktree,
		SessionID: "s1", Agent: "a1",
	}
	// A symlink inside the world pointing outside.
	if err := os.Symlink(outside, filepath.Join(worktree, "linkout")); err != nil {
		t.Fatal(err)
	}
	denials := []struct {
		name   string
		path   string
		victim string // must never exist afterwards
	}{
		{"dotdot", filepath.Join("..", "..", "..", "outside", "a.txt"),
			filepath.Join(outside, "a.txt")},
		{"dotdot-mid", filepath.Join("sub", "..", "..", "..", "..", "outside", "b.txt"),
			filepath.Join(outside, "b.txt")},
		{"absolute", filepath.Join(outside, "c.txt"),
			filepath.Join(outside, "c.txt")},
		{"symlink-dir", filepath.Join("linkout", "d.txt"),
			filepath.Join(outside, "d.txt")},
		// A sibling directory sharing the world's string prefix: only a
		// whole-component check keeps this outside.
		{"sibling-dash", filepath.Join("..", "w1-x", "s.txt"),
			filepath.Join(worktree, "..", "w1-x", "s.txt")},
		{"sibling-digit", filepath.Join("..", "w12", "s2.txt"),
			filepath.Join(worktree, "..", "w12", "s2.txt")},
	}
	for _, tc := range denials {
		err := c.RunStep(&scripted.Step{Edit: &scripted.EditStep{
			Path: tc.path, Write: "CANARY\n"}})
		if err == nil {
			t.Fatalf("%s: escape step ran to completion", tc.name)
		}
		if _, err := os.Stat(tc.victim); err == nil {
			t.Fatalf("%s: wrote outside the world at %s", tc.name, tc.victim)
		}
	}
	// A symlinked file (not dir) inside the world pointing outside.
	victim := filepath.Join(outside, "real.txt")
	if err := os.WriteFile(victim, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(worktree, "filelink")); err != nil {
		t.Fatal(err)
	}
	err := c.RunStep(&scripted.Step{Edit: &scripted.EditStep{
		Path: "filelink", Write: "CANARY\n"}})
	if err == nil {
		t.Fatal("edit through a file symlink ran to completion")
	}
	if data, _ := os.ReadFile(victim); string(data) != "old" {
		t.Fatal("edit through a file symlink rewrote a file outside the world")
	}
	// Benign paths still write: a plain file and a `..` that stays inside.
	for _, p := range []string{"ok.txt", filepath.Join("sub", "..", "ok2.txt")} {
		if err := c.RunStep(&scripted.Step{Edit: &scripted.EditStep{
			Path: p, Write: "fine\n"}}); err != nil {
			t.Fatalf("benign edit %q: %v", p, err)
		}
	}
	for _, p := range []string{"ok.txt", "ok2.txt"} {
		if _, err := os.Stat(filepath.Join(worktree, p)); err != nil {
			t.Fatalf("benign edit %q did not write: %v", p, err)
		}
	}
}

// TestC1_5_7_EveryWriteToolConfined: every tool that writes a file path
// is confined to the world — Write, Edit, MultiEdit, NotebookEdit (the
// whole set, and the only path-writing tools in the recorded hook
// fixtures: Read and Agent carry no write path and Bash writes through a
// command, which the outward-action policy owns). Each tool is exercised
// by name: an out-of-world target is denied and recorded, an in-world
// target is not denied. Removing any one tool from the confined set
// fails this test.
func TestC1_5_7_EveryWriteToolConfined(t *testing.T) {
	h := newHarness(t)
	c, err := h.AddAgent(&scripted.AgentSpec{ID: "w1", Parent: "human"})
	if err != nil {
		t.Fatal(err)
	}
	// A sibling directory that shares the world's string prefix — the
	// case a raw HasPrefix check would wrongly allow. worldDir's parent
	// holds <world>-x alongside <world>.
	worldDir := c.Worktree
	sibling := filepath.Join(filepath.Dir(worldDir),
		filepath.Base(worldDir)+"-x", "f.txt")
	inside := filepath.Join(worldDir, "f.txt")
	// Far outside: a path under neither the world nor the data dir, so no
	// inner policy (fileTools protects the court's own data dir) can
	// produce the denial — confinement alone does.
	dataDir := filepath.Dir(filepath.Dir(worldDir))
	outside := filepath.Join(filepath.Dir(dataDir), "w1-x", "f.txt")
	writeTools := []struct {
		name string
		key  string // canonical path member for this tool
	}{
		{"Write", "file_path"},
		{"Edit", "file_path"},
		{"MultiEdit", "file_path"},
		{"NotebookEdit", "notebook_path"},
	}
	pathKeys := []string{"file_path", "path", "notebook_path"}
	// The decision must come from WorldPolicy itself: no inner policy.
	wp := scripted.WorldPolicy{}
	for _, tool := range writeTools {
		// Every path-holding member of the input is confined.
		for _, key := range pathKeys {
			if d, _ := wp.DecideHook(tool.name,
				map[string]any{key: sibling}, worldDir); d != "deny" {
				t.Fatalf("%s %s=%q (%s-x sibling): decision %q, want deny",
					tool.name, key, sibling, filepath.Base(worldDir), d)
			}
			if d, _ := wp.DecideHook(tool.name,
				map[string]any{key: outside}, worldDir); d != "deny" {
				t.Fatalf("%s %s=%q: decision %q, want deny",
					tool.name, key, outside, d)
			}
		}
		// Its canonical path member inside the world is not denied.
		if d, _ := wp.DecideHook(tool.name,
			map[string]any{tool.key: inside}, worldDir); d == "deny" {
			t.Fatalf("%s in-world %s=%q was denied", tool.name, tool.key, inside)
		}
	}
	// Through the real daemon each tool's outside-path call is denied and
	// recorded as a denied tool.event carrying the tool's name and a
	// reason.
	for _, tool := range writeTools {
		_, denied, err := c.PreToolUse(tool.name,
			map[string]any{tool.key: outside}, "", "")
		if err != nil {
			t.Fatalf("%s %s: %v", tool.name, tool.key, err)
		}
		if !denied {
			t.Fatalf("%s %s=%q was not denied through the daemon",
				tool.name, tool.key, outside)
		}
	}
	denied := map[string]int{}
	for _, e := range toolEvents(t, h) {
		if strOf(bodyField(t, e, "outcome")) != "denied" {
			continue
		}
		if strOf(bodyField(t, e, "reason")) == "" {
			t.Fatalf("denied tool.event without reason: %v", e.Body)
		}
		denied[strOf(bodyField(t, e, "tool"))]++
	}
	for _, tool := range writeTools {
		if denied[tool.name] < 1 {
			t.Fatalf("%s: no denied tool.event recorded", tool.name)
		}
	}
}

// TestC1_5_7_DeniedRecorded: through the real harness the escape is
// denied before the write, and the ledger records one denied tool.event
// carrying the reason.
func TestC1_5_7_DeniedRecorded(t *testing.T) {
	h := newHarness(t)
	c, err := h.AddAgent(&scripted.AgentSpec{ID: "w1", Parent: "human"})
	if err != nil {
		t.Fatal(err)
	}
	rel := filepath.Join("..", "..", "..", "..", "..", "..", "escape-target", "x.txt")
	victim := filepath.Join(c.Worktree, rel)
	err = c.RunStep(&scripted.Step{Edit: &scripted.EditStep{
		Path: rel, Write: "CANARY\n"}})
	if err != nil {
		t.Logf("step ended: %v", err)
	}
	if _, err := os.Stat(victim); err == nil {
		t.Fatal("escape wrote outside the world")
	}
	evs, err := h.Events()
	if err != nil {
		t.Fatal(err)
	}
	denied := false
	for _, e := range evs {
		if e.Type != "tool.event" {
			continue
		}
		if strOf(bodyField(t, e, "outcome")) != "denied" {
			continue
		}
		if r := strOf(bodyField(t, e, "reason")); r == "" {
			t.Fatal("denied tool.event has no reason")
		}
		denied = true
	}
	if !denied {
		t.Fatal("no denied tool.event recorded for the escape")
	}
}
