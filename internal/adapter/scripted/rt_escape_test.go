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
