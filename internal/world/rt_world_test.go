package world_test

// Red-team reproductions for M1.4 worlds (brief B3, aims 2 and 4). They
// assert the properties spec/02 §3 and H3 state and fail when Discard or Run
// does not keep them.

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"court/internal/world"
)

// rtScratch returns a directory under this clone (brief rule 1: no writes
// outside the folder), freshly emptied.
func rtScratch(t *testing.T, name string) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(file))) // internal/world -> repo
	dir := filepath.Join(repoRoot, ".rt", "scratch", "world", name)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func rtConfig(t *testing.T, repo string) string {
	t.Helper()
	out, err := gitE(repo, "config", "--local", "--list")
	if err != nil {
		t.Fatalf("git config: %v\n%s", err, out)
	}
	return out
}

func rtRefs(t *testing.T, repo string) string {
	t.Helper()
	out, err := gitE(repo, "for-each-ref", "--format=%(refname) %(objectname)")
	if err != nil {
		t.Fatalf("for-each-ref: %v\n%s", err, out)
	}
	return out
}

// TestRT_DiscardLeavesConfigAndHooks: a world may run git through World.Run.
// `git config --local` writes the repository's shared configuration
// (spec/02 §3 says refs and configuration are shared), and a hook file under
// the common .git/hooks survives Discard. H3's Digest hashes refs, reachable
// objects and the worktree list only, so it reports the repository restored
// while `git config --list` (C1.4.4) and the hook tree differ.
func TestRT_DiscardLeavesConfigAndHooks(t *testing.T) {
	repo := newRepo(t, 1)
	root := open(t, repo)
	base := head(t, repo)
	before := rtConfig(t, repo)
	d0, err := root.Digest()
	if err != nil {
		t.Fatal(err)
	}

	w, err := root.Fork(base, "rtcfg")
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	gw := w.(*world.GitWorld)

	// Two ordinary-looking agent commands, both allowed by the adapter's
	// policy (no outward program, no data-directory string).
	if r := mustRun(gw, "git", "config", "--local", "court.canary", "CANARY-CONFIG"); r.ExitCode != 0 {
		t.Fatalf("git config: %s", r.Stderr)
	}
	r := mustRun(gw, "git", "rev-parse", "--git-common-dir")
	if r.ExitCode != 0 {
		t.Fatalf("rev-parse: %s", r.Stderr)
	}
	common := strings.TrimSpace(r.Stdout)
	if !filepath.IsAbs(common) {
		common = filepath.Join(gw.Path(), common)
	}
	hook := filepath.Join(common, "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho CANARY-HOOK\n"), 0o755); err != nil {
		t.Fatalf("write hook: %v", err)
	}

	if err := gw.Discard(); err != nil {
		t.Fatalf("discard: %v", err)
	}
	if _, err := os.Stat(hook); err == nil {
		t.Errorf("hook installed from the world survives Discard: %s", hook)
	}
	if after := rtConfig(t, repo); after != before {
		t.Errorf("repository configuration not restored by Discard:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	d1, err := root.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if d1 != d0 {
		t.Errorf("digest changed: %s -> %s", d0[:12], d1[:12])
	}
}

// TestRT_DiscardLeavesExtraRef: a ref created from inside a world is shared
// state; `git worktree remove` plus `git branch -D` removes only the world's
// own branch. Discard returns nil while the repository differs, so H3 ("a
// world is treated as reversible only because M1.3 proves, on every check
// run, that discarding a world restores the repository bit for bit") is
// false for a world that ran this.
func TestRT_DiscardLeavesExtraRef(t *testing.T) {
	repo := newRepo(t, 1)
	root := open(t, repo)
	base := head(t, repo)
	before := rtRefs(t, repo)
	d0, err := root.Digest()
	if err != nil {
		t.Fatal(err)
	}

	w, err := root.Fork(base, "rtref")
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	gw := w.(*world.GitWorld)
	if r := mustRun(gw, "git", "tag", "canary-ref", string(base)); r.ExitCode != 0 {
		t.Fatalf("tag: %s", r.Stderr)
	}
	if err := gw.Discard(); err != nil {
		t.Fatalf("discard: %v", err)
	}
	after := rtRefs(t, repo)
	d1, err := root.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("refs not restored by Discard:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if d1 != d0 {
		t.Fatalf("H3 digest not restored: %s -> %s", d0[:12], d1[:12])
	}
}

// TestRT_DiscardFollowsJunction asks whether worktree removal deletes
// through a Windows directory junction created inside the world. If it does,
// discarding one world destroys files outside the repository.
func TestRT_DiscardFollowsJunction(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("junction")
	}
	repo := newRepo(t, 1)
	root := open(t, repo)
	base := head(t, repo)
	outside := rtScratch(t, "junction-target")
	canary := filepath.Join(outside, "canary.txt")
	if err := os.WriteFile(canary, []byte("CANARY-JUNCTION\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	w, err := root.Fork(base, "rtjunc")
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	gw := w.(*world.GitWorld)
	link := filepath.Join(gw.Path(), "junc")
	cmd := exec.Command("cmd", "/c", "mklink", "/J", link, outside)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot create junction: %v: %s", err, out)
	}
	if err := gw.Discard(); err != nil {
		t.Logf("discard reported: %v", err)
	}
	if _, err := os.Stat(canary); err != nil {
		t.Fatalf("Discard deleted a file outside the world through a junction: %v", err)
	}
}

// TestRT_RunDetachedChildHangs: Run's timeout must end the call (C1.4.6,
// "the time limit kills descendants"). A command that launches a detached
// descendant inheriting stdout (cmd /c start /b) makes Wait block on the
// output pipe after the direct child exits; taskkill /T run against the
// already-exited parent cannot see the descendant, so Run returns when the
// descendant exits, not at the time limit.
func TestRT_RunDetachedChildHangs(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("cmd start")
	}
	repo := newRepo(t, 1)
	root := open(t, repo)
	w, err := root.Fork(head(t, repo), "rthang")
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	gw := w.(*world.GitWorld)
	defer gw.Discard()

	start := time.Now()
	res, err := gw.Run([]string{"cmd", "/c", "start /b ping -n 6 127.0.0.1"},
		world.Limits{Timeout: 500 * time.Millisecond})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	t.Logf("Run returned after %v (timedOut=%v exit=%d)", elapsed, res.TimedOut, res.ExitCode)
	if elapsed > 3*time.Second {
		t.Fatalf("Run ignored its 500ms limit for a detached descendant: returned after %v", elapsed)
	}
}

// TestRT_DiscardBenignResidue checks the ordinary "used" content the brief
// lists: untracked and ignored files, a nested repository, a read-only file
// and a path longer than 260 characters. These are expected to be removed.
func TestRT_DiscardBenignResidue(t *testing.T) {
	repo := newRepo(t, 1)
	root := open(t, repo)
	base := head(t, repo)
	d0, err := root.Digest()
	if err != nil {
		t.Fatal(err)
	}
	fs0 := rtConfig(t, repo)

	w, err := root.Fork(base, "rtjunk")
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	gw := w.(*world.GitWorld)
	p := gw.Path()
	writeFile(t, p, ".gitignore", "ignored.txt\n")
	writeFile(t, p, "tracked.txt", "x\n")
	writeFile(t, p, "untracked.txt", "untracked\n")
	writeFile(t, p, "ignored.txt", "ignored\n")
	writeFile(t, p, "nested/.keep", "nested\n")
	if out, err := gitE(filepath.Join(p, "nested"), "init", "-q"); err != nil {
		t.Fatalf("nested init: %v: %s", err, out)
	}
	ro := filepath.Join(p, "readonly.txt")
	if err := os.WriteFile(ro, []byte("ro\n"), 0o444); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(p, strings.Repeat("d", 40), strings.Repeat("e", 40), strings.Repeat("f", 40), strings.Repeat("g", 40), strings.Repeat("h", 40), strings.Repeat("i", 40), "long.txt")
	if err := os.MkdirAll(filepath.Dir(deep), 0o755); err != nil {
		t.Logf("long dir: %v", err)
	} else if err := os.WriteFile(deep, []byte("long\n"), 0o644); err != nil {
		t.Logf("long file: %v", err)
	}
	if err := gw.Discard(); err != nil {
		t.Fatalf("discard: %v", err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("world dir still present after Discard: %v", err)
	}
	d1, err := root.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if d1 != d0 {
		t.Fatalf("digest changed: %s -> %s", d0[:12], d1[:12])
	}
	if c := rtConfig(t, repo); c != fs0 {
		t.Fatalf("config changed")
	}
}

// TestRT_DiscardLeavesMainWorktreeWrite: a shell command run inside a world
// can write into the repository's main worktree with an absolute path. No
// adapter policy exists at the world layer, World.Run runs it, and Discard
// leaves the file. Digest still reports the repository restored because it
// hashes refs, reachable objects and the worktree list, not worktree
// contents.
func TestRT_DiscardLeavesMainWorktreeWrite(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("cmd shell")
	}
	repo := newRepo(t, 1)
	root := open(t, repo)
	base := head(t, repo)
	d0, err := root.Digest()
	if err != nil {
		t.Fatal(err)
	}
	w, err := root.Fork(base, "rtwrite")
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	gw := w.(*world.GitWorld)
	victim := filepath.Join(repo, "canary-main.txt")
	line := `echo CANARY-MAIN > ` + victim
	if r := mustRun(gw, "cmd", "/c", line); r.ExitCode != 0 {
		t.Fatalf("outside write: %s", r.Stderr)
	}
	if err := gw.Discard(); err != nil {
		t.Fatalf("discard: %v", err)
	}
	if _, err := os.Stat(victim); err == nil {
		t.Fatalf("write into the main worktree survives Discard: %s", victim)
	}
	d1, err := root.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if d1 != d0 {
		t.Fatalf("digest changed: %s -> %s", d0[:12], d1[:12])
	}
}
