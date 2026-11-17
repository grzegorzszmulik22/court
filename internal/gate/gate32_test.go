package gate_test

// C3.2.1 - C3.2.6: the declaration gate of spec/milestones/M3 §M3.2.
// Each test drives a real daemon, real git worlds and the scripted agent;
// nothing is mocked and no model is called.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"court/internal/adapter/scripted"
	"court/internal/daemon"
	"court/internal/gate"
	"court/internal/ledger"
)

// --------------------------------------------------------------------------
// helpers
// --------------------------------------------------------------------------

func newGateHarness(t *testing.T) *scripted.Harness {
	t.Helper()
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := scripted.InitRepo(repo, map[string]string{
		"app.txt":   "one\ntwo\nthree\n",
		"README.md": "# fixture\n",
	}); err != nil {
		t.Fatal(err)
	}
	h, err := scripted.NewHarness(scripted.Options{
		Repo: repo, DataDir: filepath.Join(root, "data"), Commission: "c32",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.Close)
	if err := h.OpenCommission("gate fixture", nil); err != nil {
		t.Fatal(err)
	}
	return h
}

func addAgent(t *testing.T, h *scripted.Harness, id string) *scripted.Client {
	t.Helper()
	c, err := h.AddAgent(&scripted.AgentSpec{ID: id, Parent: "human"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func evs(t *testing.T, h *scripted.Harness) []*ledger.Event {
	t.Helper()
	e, err := h.Events()
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func ofType(e []*ledger.Event, typ string) []*ledger.Event {
	var out []*ledger.Event
	for _, ev := range e {
		if ev.Type == typ {
			out = append(out, ev)
		}
	}
	return out
}

func bodyStr(e *ledger.Event, key string) string {
	v, _ := ledger.ObjGetPub(e.Body, key)
	if v == nil || v.Kind != ledger.String {
		return ""
	}
	return v.Str
}

func bodyBool(e *ledger.Event, key string) bool {
	v, _ := ledger.ObjGetPub(e.Body, key)
	return v != nil && v.Kind == ledger.Bool && v.Bool
}

// commitFor returns the world.commit whose body names id.
func commitFor(e []*ledger.Event, id string) *ledger.Event {
	for _, ev := range e {
		if ev.Type == "world.commit" && bodyStr(ev, "commit") == id {
			return ev
		}
	}
	return nil
}

// waitFor polls the ledger until pred holds, failing after d.
func waitFor(t *testing.T, h *scripted.Harness, what string, d time.Duration,
	pred func([]*ledger.Event) bool) []*ledger.Event {
	t.Helper()
	deadline := time.Now().Add(d)
	for {
		e := evs(t, h)
		if pred(e) {
			return e
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %v waiting for %s", d, what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// cleanGitEnv strips inherited git overrides, as the scripted client does,
// so the world's per-world GIT_CONFIG_* entries are authoritative.
func cleanGitEnv() []string {
	var kept []string
	for _, e := range os.Environ() {
		i := strings.IndexByte(e, '=')
		if i < 0 {
			continue
		}
		k := e[:i]
		if strings.HasPrefix(k, "GIT_CONFIG_") || k == "GIT_DIR" ||
			k == "GIT_WORK_TREE" || k == "GIT_INDEX_FILE" ||
			k == "GIT_OBJECT_DIRECTORY" || k == "GIT_COMMON_DIR" ||
			k == "GIT_CONFIG_NOSYSTEM" || k == "GIT_CEILING_DIRECTORIES" {
			continue
		}
		kept = append(kept, e)
	}
	return kept
}

// gitRun runs git in the world under its per-world config and returns the
// combined output and error.
func gitRun(c *scripted.Client, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = c.Worktree
	cmd.Env = append(cleanGitEnv(), c.GitEnv...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func revParse(t *testing.T, c *scripted.Client) string {
	t.Helper()
	out, err := gitRun(c, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v: %s", err, out)
	}
	return strings.TrimSpace(out)
}

func writeFile(t *testing.T, c *scripted.Client, name, content string) {
	t.Helper()
	if err := c.RunStep(&scripted.Step{Edit: &scripted.EditStep{
		Path: name, Write: content}}); err != nil {
		t.Fatal(err)
	}
}

func stage(t *testing.T, c *scripted.Client) {
	t.Helper()
	if out, err := gitRun(c, "add", "-A"); err != nil {
		t.Fatalf("git add: %v: %s", err, out)
	}
}

// commitRaw stages everything and commits with the given extra git args,
// returning the combined output. It bypasses the scripted client so checks
// can inspect exact hook output and use --no-verify.
func commitRaw(t *testing.T, c *scripted.Client, message string, args ...string) (string, error) {
	t.Helper()
	stage(t, c)
	all := []string{"-c", "user.email=a@court.local", "-c", "user.name=" + c.Agent,
		"commit", "-m", message, "--allow-empty"}
	all = append(all, args...)
	return gitRun(c, all...)
}

// exactLine reports whether out carries the line exactly (not a substring).
func exactLine(out, want string) bool {
	for _, l := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		if l == want {
			return true
		}
	}
	return false
}

func declareStep(question, category, path string) *scripted.Step {
	return &scripted.Step{Declare: &scripted.DeclareStep{
		Question: question, Category: category, Paths: []string{path}, Chosen: "keep"}}
}

// --------------------------------------------------------------------------
// C3.2.1: a commit with no declaration is refused with the exact message
// and creates no commit.
// --------------------------------------------------------------------------

func TestC3_2_1(t *testing.T) {
	h := newGateHarness(t)
	c := addAgent(t, h, "w1")
	writeFile(t, c, "app.txt", "one\nchanged\nthree\n")

	head := revParse(t, c)
	before := len(ofType(evs(t, h), "world.commit"))
	out, err := commitRaw(t, c, "undeclared commit")
	if err == nil {
		t.Fatalf("a commit without a declaration succeeded:\n%s", out)
	}
	if !exactLine(out, gate.Message) {
		t.Fatalf("refusal does not carry exactly %q:\n%s", gate.Message, out)
	}
	if got := revParse(t, c); got != head {
		t.Fatalf("refused commit moved HEAD: %s -> %s", head, got)
	}
	if n := len(ofType(evs(t, h), "world.commit")); n != before {
		t.Fatalf("refused commit was recorded: world.commit %d -> %d", before, n)
	}

	// The declaration window closes when a commit is observed: one
	// no_decisions allows one commit, not every later commit.
	if err := c.RunStep(&scripted.Step{NoDecisions: &scripted.EmptyStep{}}); err != nil {
		t.Fatal(err)
	}
	if out, err := commitRaw(t, c, "declared commit"); err != nil {
		t.Fatalf("commit after no_decisions failed: %v\n%s", err, out)
	}
	writeFile(t, c, "app.txt", "one\nchanged again\nthree\n")
	out, err = commitRaw(t, c, "second undeclared commit")
	if err == nil {
		t.Fatalf("a second commit reused the previous declaration:\n%s", out)
	}
	if !exactLine(out, gate.Message) {
		t.Fatalf("second refusal does not carry exactly %q:\n%s", gate.Message, out)
	}
}

// --------------------------------------------------------------------------
// C3.2.2: record_decision or no_decisions opens the gate; world.commit
// follows the declaration in the ledger.
// --------------------------------------------------------------------------

func TestC3_2_2(t *testing.T) {
	t.Run("record_decision", func(t *testing.T) {
		h := newGateHarness(t)
		c := addAgent(t, h, "w1")
		writeFile(t, c, "app.txt", "one\nby record\nthree\n")
		if err := c.RunStep(declareStep(
			"Which limit should the default take?", "behaviour", "app.txt")); err != nil {
			t.Fatal(err)
		}
		if err := c.RunStep(&scripted.Step{Commit: &scripted.CommitStep{
			Message: "declared commit"}}); err != nil {
			t.Fatalf("commit after record_decision failed: %v", err)
		}
		head := revParse(t, c)
		e := waitFor(t, h, "world.commit", 5*time.Second,
			func(e []*ledger.Event) bool { return commitFor(e, head) != nil })
		dec := ofType(e, "decision.declared")
		if len(dec) != 1 {
			t.Fatalf("decision.declared events = %d, want 1", len(dec))
		}
		wc := commitFor(e, head)
		if dec[0].Seq >= wc.Seq {
			t.Fatalf("world.commit (seq %d) does not follow the declaration (seq %d)",
				wc.Seq, dec[0].Seq)
		}
		if !bodyBool(wc, "gated") {
			t.Fatalf("declared commit recorded gated:false")
		}
	})

	t.Run("survives-restart", func(t *testing.T) {
		h := newGateHarness(t)
		c := addAgent(t, h, "w1")
		writeFile(t, c, "app.txt", "one\nafter restart\nthree\n")
		if err := c.RunStep(&scripted.Step{NoDecisions: &scripted.EmptyStep{}}); err != nil {
			t.Fatal(err)
		}
		// The declaration is in the ledger; a fresh daemon must derive the
		// open window from it and install its hooks at the new URL.
		h.Daemon.Close()
		d2, err := daemon.New(h.Store, daemon.Options{
			DataDir: h.DataDir, Repo: h.Repo, CourtBin: h.CourtBin})
		if err != nil {
			t.Fatal(err)
		}
		if err := d2.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(d2.Close)
		if out, err := commitRaw(t, c, "after restart"); err != nil {
			t.Fatalf("commit after daemon restart failed: %v\n%s", err, out)
		}
		head := revParse(t, c)
		e := waitFor(t, h, "world.commit after restart", 5*time.Second,
			func(e []*ledger.Event) bool { return commitFor(e, head) != nil })
		if wc := commitFor(e, head); !bodyBool(wc, "gated") {
			t.Fatalf("declared commit after restart recorded gated:false")
		}
	})

	t.Run("no_decisions", func(t *testing.T) {
		h := newGateHarness(t)
		c := addAgent(t, h, "w1")
		writeFile(t, c, "app.txt", "one\nby none\nthree\n")
		if err := c.RunStep(&scripted.Step{NoDecisions: &scripted.EmptyStep{}}); err != nil {
			t.Fatal(err)
		}
		if err := c.RunStep(&scripted.Step{Commit: &scripted.CommitStep{
			Message: "no decisions commit"}}); err != nil {
			t.Fatalf("commit after no_decisions failed: %v", err)
		}
		head := revParse(t, c)
		e := waitFor(t, h, "world.commit", 5*time.Second,
			func(e []*ledger.Event) bool { return commitFor(e, head) != nil })
		none := ofType(e, "decision.none")
		if len(none) != 1 {
			t.Fatalf("decision.none events = %d, want 1", len(none))
		}
		wc := commitFor(e, head)
		if none[0].Seq >= wc.Seq {
			t.Fatalf("world.commit (seq %d) does not follow decision.none (seq %d)",
				wc.Seq, none[0].Seq)
		}
		if !bodyBool(wc, "gated") {
			t.Fatalf("no_decisions commit recorded gated:false")
		}
	})
}

// --------------------------------------------------------------------------
// C3.2.3: a commit that bypasses the hooks is recorded gated:false within
// one second of the next ledger write; its hits are undeclared.
// --------------------------------------------------------------------------

func TestC3_2_3(t *testing.T) {
	t.Run("no-verify", func(t *testing.T) {
		h := newGateHarness(t)
		c := addAgent(t, h, "w1")
		// A declaration sits in the window, so a gate that trusted
		// declarations alone would call the commit gated. Only the
		// pre-commit handshake may do that, and --no-verify skips it.
		if err := c.RunStep(declareStep(
			"Which default should win?", "behaviour", "app.txt")); err != nil {
			t.Fatal(err)
		}
		writeFile(t, c, "app.txt", "one\nbypassed\nthree\n")
		if out, err := commitRaw(t, c, "bypassed", "--no-verify"); err != nil {
			t.Fatalf("--no-verify commit failed: %v\n%s", err, out)
		}
		head := revParse(t, c)
		start := time.Now()
		e := waitFor(t, h, "gated:false commit (--no-verify)", time.Second,
			func(e []*ledger.Event) bool { return commitFor(e, head) != nil })
		if d := time.Since(start); d > time.Second {
			t.Fatalf("gated:false recorded after %v, want <= 1s", d)
		}
		wc := commitFor(e, head)
		if bodyBool(wc, "gated") {
			t.Fatalf("commit made with the pre-commit hook bypassed recorded gated:true")
		}
		// The pending declaration does not cover the bypassed commit:
		// it stays before the commit in the ledger, yet the commit is
		// ungated, so every hit in it is undeclared.
		dec := ofType(e, "decision.declared")
		if len(dec) != 1 || dec[0].Seq >= wc.Seq {
			t.Fatalf("fixture lost its pre-commit declaration")
		}
		// The next ledger write sees the commit already recorded and
		// ordered before it.
		if err := c.RunStep(&scripted.Step{Tool: &scripted.ToolStep{
			Name: "Read", Input: map[string]any{"file_path": "app.txt"}}}); err != nil {
			t.Fatal(err)
		}
		e = evs(t, h)
		var next *ledger.Event
		for _, ev := range e {
			if ev.Type == "tool.event" && ev.Seq > wc.Seq {
				next = ev
				break
			}
		}
		if next == nil {
			t.Fatalf("next ledger write missing after the bypassed commit")
		}
	})

	t.Run("all-hooks-bypassed", func(t *testing.T) {
		h := newGateHarness(t)
		c := addAgent(t, h, "w1")
		writeFile(t, c, "app.txt", "one\nno hooks\nthree\n")
		emptyHooks := filepath.Join(t.TempDir(), "empty-hooks")
		if err := os.MkdirAll(emptyHooks, 0o755); err != nil {
			t.Fatal(err)
		}
		stage(t, c)
		if out, err := gitRun(c, "-c", "user.email=a@court.local",
			"-c", "user.name="+c.Agent, "-c", "core.hooksPath="+emptyHooks,
			"commit", "-m", "fully bypassed", "--allow-empty", "--no-verify"); err != nil {
			t.Fatalf("fully bypassed commit failed: %v\n%s", err, out)
		}
		head := revParse(t, c)
		if commitFor(evs(t, h), head) != nil {
			t.Fatalf("fully bypassed commit was recorded before the next ledger write")
		}
		// The next ledger write triggers the head scan; the commit must be
		// recorded within one second of it.
		if err := c.RunStep(&scripted.Step{Tool: &scripted.ToolStep{
			Name: "Read", Input: map[string]any{"file_path": "app.txt"}}}); err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		e := waitFor(t, h, "gated:false commit (hooks disabled)", time.Second,
			func(e []*ledger.Event) bool { return commitFor(e, head) != nil })
		if d := time.Since(start); d > time.Second {
			t.Fatalf("gated:false recorded after %v, want <= 1s", d)
		}
		wc := commitFor(e, head)
		if bodyBool(wc, "gated") {
			t.Fatalf("commit with no hooks at all recorded gated:true")
		}
		// It arrived without a gate record; there is no declaration before
		// it either, so its hits are undeclared.
		for _, dec := range ofType(e, "decision.declared") {
			if dec.Seq < wc.Seq {
				t.Fatalf("unexpected declaration before the ungated commit")
			}
		}
	})
}

// --------------------------------------------------------------------------
// C3.2.4: a declaration made for one world does not open the gate in
// another.
// --------------------------------------------------------------------------

func TestC3_2_4(t *testing.T) {
	h := newGateHarness(t)
	a := addAgent(t, h, "a")
	b := addAgent(t, h, "b")

	// A declares but has not committed: its window is open. B never
	// declared, so B is closed.
	writeFile(t, a, "app.txt", "one\nworld a\nthree\n")
	if err := a.RunStep(declareStep(
		"Which default for world a?", "behaviour", "app.txt")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, b, "app.txt", "one\nworld b\nthree\n")
	headB := revParse(t, b)
	out, err := commitRaw(t, b, "b without a declaration")
	if err == nil {
		t.Fatalf("world b's commit was opened by world a's declaration:\n%s", out)
	}
	if !exactLine(out, gate.Message) {
		t.Fatalf("world b refusal does not carry exactly %q:\n%s", gate.Message, out)
	}
	if got := revParse(t, b); got != headB {
		t.Fatalf("refused commit in world b moved HEAD")
	}

	// A's own commit succeeds on A's declaration.
	if out, err := commitRaw(t, a, "a declared"); err != nil {
		t.Fatalf("world a's declared commit failed: %v\n%s", err, out)
	}
	headA := revParse(t, a)
	waitFor(t, h, "world.commit for a", 5*time.Second,
		func(e []*ledger.Event) bool { return commitFor(e, headA) != nil })

	// A's declaration is consumed and never opened B. B still fails.
	out, err = commitRaw(t, b, "b still undeclared")
	if err == nil {
		t.Fatalf("world b's commit succeeded with no declaration of its own:\n%s", out)
	}
	if !exactLine(out, gate.Message) {
		t.Fatalf("world b second refusal does not carry exactly %q:\n%s", gate.Message, out)
	}

	// B declares and commits on its own declaration.
	if err := b.RunStep(&scripted.Step{NoDecisions: &scripted.EmptyStep{}}); err != nil {
		t.Fatal(err)
	}
	if out, err := commitRaw(t, b, "b declared"); err != nil {
		t.Fatalf("world b's declared commit failed: %v\n%s", err, out)
	}
	headB = revParse(t, b)
	e := waitFor(t, h, "world.commit for b", 5*time.Second,
		func(e []*ledger.Event) bool { return commitFor(e, headB) != nil })
	if wc := commitFor(e, headB); !bodyBool(wc, "gated") {
		t.Fatalf("world b's declared commit recorded gated:false")
	}
}

// --------------------------------------------------------------------------
// C3.2.5: the hooks directory is outside every world and no agent tool path
// can write into it.
// --------------------------------------------------------------------------

func TestC3_2_5(t *testing.T) {
	h := newGateHarness(t)
	c := addAgent(t, h, "w1")
	hooks := gate.HooksDir(h.DataDir)
	for _, name := range []string{"pre-commit", "post-commit"} {
		if _, err := os.Stat(filepath.Join(hooks, name)); err != nil {
			t.Fatalf("hook %s is not installed: %v", name, err)
		}
	}
	// Outside every world.
	rel, err := filepath.Rel(c.Worktree, hooks)
	if err != nil {
		t.Fatal(err)
	}
	if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		t.Fatalf("hooks dir %s is inside world %s (rel %s)", hooks, c.Worktree, rel)
	}

	pre := filepath.Join(hooks, "pre-commit")
	before, err := os.ReadFile(pre)
	if err != nil {
		t.Fatal(err)
	}
	// The agent's edit path into the hooks directory: a relative escape and
	// the absolute path. Both are denied by the adapter's data-directory
	// policy and the file must not change.
	if err := c.RunStep(&scripted.Step{Edit: &scripted.EditStep{
		Path: filepath.Join("..", "hooks", "pre-commit"), Write: "tampered\n"}}); err != nil {
		t.Fatal(err)
	}
	if err := c.RunStep(&scripted.Step{Tool: &scripted.ToolStep{
		Name: "Write", Input: map[string]any{"file_path": pre, "content": "tampered\n"}}}); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(pre)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("an agent tool path rewrote %s", pre)
	}
	// The attempts are recorded as denied tool events, not silently lost.
	var denied int
	for _, ev := range ofType(evs(t, h), "tool.event") {
		if bodyStr(ev, "outcome") == "denied" &&
			(bodyStr(ev, "tool") == "Edit" || bodyStr(ev, "tool") == "Write") {
			denied++
		}
	}
	if denied < 2 {
		t.Fatalf("denied tool events for the hook edits = %d, want 2", denied)
	}

	// A shell command naming the hooks directory is denied before it runs.
	marker := filepath.Join(hooks, "pre-commit")
	if err := c.RunStep(&scripted.Step{Tool: &scripted.ToolStep{
		Name: "Bash", Input: map[string]any{
			"command": "printf tampered >> '" + marker + "'"}}}); err != nil {
		t.Fatal(err)
	}
	after, err = os.ReadFile(pre)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("a shell command naming the hooks dir rewrote %s", pre)
	}
}

// --------------------------------------------------------------------------
// C3.2.6: hooks already present in the repository still run, after Court's.
// --------------------------------------------------------------------------

func TestC3_2_6(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := scripted.InitRepo(repo, map[string]string{"app.txt": "one\n"}); err != nil {
		t.Fatal(err)
	}
	order := filepath.Join(root, "order.log")
	hooks := filepath.Join(repo, ".git", "hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	preHook := "#!/bin/sh\n" +
		"echo repo-pre >> " + shPath(order) + "\n" +
		"exit 0\n"
	postHook := "#!/bin/sh\n" +
		"echo repo-post >> " + shPath(order) + "\n" +
		"python3 - " + shPath(filepath.Join(root, "data", "ledger.db")) +
		" \"$(git rev-parse HEAD)\" <<'PYEOF' && echo post-after-record >> " +
		shPath(order) + "\n" +
		"import sqlite3, sys\n" +
		"db, commit = sys.argv[1], sys.argv[2]\n" +
		"rows = sqlite3.connect(db).execute(\n" +
		"    \"select body from ledger where type='world.commit'\").fetchall()\n" +
		"sys.exit(0 if any(commit in r[0] for r in rows) else 1)\n" +
		"PYEOF\n" +
		"exit 0\n"
	for name, body := range map[string]string{"pre-commit": preHook, "post-commit": postHook} {
		if err := os.WriteFile(filepath.Join(hooks, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	h, err := scripted.NewHarness(scripted.Options{
		Repo: repo, DataDir: filepath.Join(root, "data"), Commission: "c32r",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.Close)
	if err := h.OpenCommission("repo hooks", nil); err != nil {
		t.Fatal(err)
	}
	c := addAgent(t, h, "w1")

	readOrder := func() string {
		raw, err := os.ReadFile(order)
		if os.IsNotExist(err) {
			return ""
		}
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}

	// A refused commit must not run the repository's pre-commit hook: Court
	// runs first.
	writeFile(t, c, "app.txt", "one\nrefused\n")
	out, err := commitRaw(t, c, "refused")
	if err == nil {
		t.Fatalf("undeclared commit succeeded:\n%s", out)
	}
	if strings.Contains(readOrder(), "repo-pre") {
		t.Fatalf("repository pre-commit ran even though Court's gate refused")
	}

	// A declared commit runs both repository hooks, and the post-commit
	// hook observes its world.commit already in the ledger.
	if err := c.RunStep(&scripted.Step{NoDecisions: &scripted.EmptyStep{}}); err != nil {
		t.Fatal(err)
	}
	if out, err := commitRaw(t, c, "declared"); err != nil {
		t.Fatalf("declared commit failed: %v\n%s", err, out)
	}
	head := revParse(t, c)
	waitFor(t, h, "world.commit", 5*time.Second,
		func(e []*ledger.Event) bool { return commitFor(e, head) != nil })
	got := readOrder()
	if !strings.Contains(got, "repo-pre") || !strings.Contains(got, "repo-post") {
		t.Fatalf("repository hooks did not both run: %q", got)
	}
	if !strings.Contains(got, "post-after-record") {
		t.Fatalf("repository post-commit ran before Court recorded the commit: %q", got)
	}

	// A repository pre-commit that fails still blocks the commit, after
	// Court's gate allowed it.
	failing := "#!/bin/sh\necho repo-pre-fails >> " + shPath(order) + "\nexit 1\n"
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte(failing), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, c, "app.txt", "one\nblocked by repo\n")
	if err := c.RunStep(&scripted.Step{NoDecisions: &scripted.EmptyStep{}}); err != nil {
		t.Fatal(err)
	}
	if out, err := commitRaw(t, c, "blocked"); err == nil {
		t.Fatalf("commit succeeded although the repository pre-commit failed:\n%s", out)
	}
	if !strings.Contains(readOrder(), "repo-pre-fails") {
		t.Fatalf("repository failing pre-commit did not run after Court's gate passed")
	}
}

// A repository that configures its own core.hooksPath: Court's per-world
// GIT_CONFIG_* override must not hide it, and its hooks still run after
// Court's.
func TestC3_2_6ConfiguredHooksPath(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := scripted.InitRepo(repo, map[string]string{"app.txt": "one\n"}); err != nil {
		t.Fatal(err)
	}
	custom := filepath.Join(root, "custom-hooks")
	if err := os.MkdirAll(custom, 0o755); err != nil {
		t.Fatal(err)
	}
	order := filepath.Join(root, "order.log")
	for name, line := range map[string]string{
		"pre-commit":  "repo-pre",
		"post-commit": "repo-post",
	} {
		body := "#!/bin/sh\necho " + line + " >> " + shPath(order) + "\nexit 0\n"
		if err := os.WriteFile(filepath.Join(custom, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := exec.Command("git", "-C", repo, "config", "core.hooksPath",
		filepath.ToSlash(custom)).CombinedOutput(); err != nil {
		t.Fatalf("git config core.hooksPath: %v: %s", err, out)
	}
	h, err := scripted.NewHarness(scripted.Options{
		Repo: repo, DataDir: filepath.Join(root, "data"), Commission: "c32c",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.Close)
	if err := h.OpenCommission("configured hooks", nil); err != nil {
		t.Fatal(err)
	}
	c := addAgent(t, h, "w1")
	writeFile(t, c, "app.txt", "one\nconfigured\n")
	if err := c.RunStep(&scripted.Step{NoDecisions: &scripted.EmptyStep{}}); err != nil {
		t.Fatal(err)
	}
	if out, err := commitRaw(t, c, "configured"); err != nil {
		t.Fatalf("commit with a configured hooks path failed: %v\n%s", err, out)
	}
	head := revParse(t, c)
	waitFor(t, h, "world.commit", 5*time.Second,
		func(e []*ledger.Event) bool { return commitFor(e, head) != nil })
	raw, err := os.ReadFile(order)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	if !strings.Contains(got, "repo-pre") || !strings.Contains(got, "repo-post") {
		t.Fatalf("configured repository hooks did not both run: %q", got)
	}
}

// shPath single-quotes a path for /bin/sh.
func shPath(p string) string { return "'" + strings.ReplaceAll(p, "'", `'"'"'`) + "'" }
