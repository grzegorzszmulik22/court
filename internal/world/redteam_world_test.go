package world_test

// Permanent red-team tests for B3-04, B3-05, B3-06 and B3-07 (findings
// 04-07). They run under the C1.4.1 and C1.4.6 checks. Everything here
// uses only the public World API plus the repository's own files.

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"court/internal/world"
)

// rtWorldWrite and rtWorldDelete run a process in the world that writes or
// deletes an absolute path: the shape of a shell command that reaches the
// shared repository or worktree.
func rtWorldWrite(t *testing.T, gw *world.GitWorld, path, content string) {
	t.Helper()
	argv := append(helperArgv("write"), path, content)
	r := mustRun(gw, argv...)
	if r.ExitCode != 0 {
		t.Fatalf("world write %s: exit %d: %s", path, r.ExitCode, r.Stderr)
	}
}

func rtWorldDelete(t *testing.T, gw *world.GitWorld, path string) {
	t.Helper()
	argv := append(helperArgv("delete"), path)
	r := mustRun(gw, argv...)
	if r.ExitCode != 0 {
		t.Fatalf("world delete %s: exit %d: %s", path, r.ExitCode, r.Stderr)
	}
}

func rtConfig(t *testing.T, repo string) string {
	t.Helper()
	return git(t, repo, "config", "--local", "--list")
}

func rtRefs(t *testing.T, repo string) string {
	t.Helper()
	return git(t, repo, "for-each-ref", "--format=%(refname) %(objectname)")
}

// TestC1_4_1_DiscardRestoresSharedState: config, hooks, an extra ref, the
// main worktree's untracked/tracked files and the digest. A world changes
// each; Digest must see each change; Discard must put each back and never
// report success before it has.
func TestC1_4_1_DiscardRestoresSharedState(t *testing.T) {
	t.Setenv("COURT_TEST_HELPER", "1")
	repo := newRepo(t, 2)
	untracked := filepath.Join(repo, "untracked.txt")
	if err := os.WriteFile(untracked, []byte("baseline\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := open(t, repo)
	base := head(t, repo)
	cfg0 := rtConfig(t, repo)
	refs0 := rtRefs(t, repo)
	d0, err := root.Digest()
	if err != nil {
		t.Fatal(err)
	}

	w, err := root.Fork(base, "rtshared")
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	gw := w.(*world.GitWorld)
	hook := filepath.Join(repo, ".git", "hooks", "pre-commit")
	canary := filepath.Join(repo, "canary-main.txt")

	last := d0
	changed := func(what string) {
		t.Helper()
		d, err := root.Digest()
		if err != nil {
			t.Fatal(err)
		}
		if d == last {
			t.Fatalf("Digest does not see a change to %s", what)
		}
		last = d
	}

	if r := mustRun(gw, "git", "config", "--local", "court.canary", "CANARY-CONFIG"); r.ExitCode != 0 {
		t.Fatalf("git config in world: %s", r.Stderr)
	}
	changed("git config --local")

	rtWorldWrite(t, gw, hook, "hook")
	changed("a file under .git/hooks")

	rtWorldWrite(t, gw, canary, "CANARY-MAIN")
	changed("a file added to the main worktree")

	if r := mustRun(gw, "git", "tag", "canary-ref", string(base)); r.ExitCode != 0 {
		t.Fatalf("git tag in world: %s", r.Stderr)
	}
	changed("a ref")

	rtWorldWrite(t, gw, filepath.Join(repo, "file0.txt"), "MAIN-MODIFIED")
	rtWorldDelete(t, gw, filepath.Join(repo, "file1.txt"))
	rtWorldWrite(t, gw, untracked, "CHANGED-UNTRACKED")

	if err := gw.Discard(); err != nil {
		t.Fatalf("discard: %v", err)
	}

	if got := rtConfig(t, repo); got != cfg0 {
		t.Fatalf("config not restored:\nbefore:\n%s\nafter:\n%s", cfg0, got)
	}
	if _, err := os.Stat(hook); err == nil {
		t.Fatalf("hook installed from the world survives Discard: %s", hook)
	}
	if got := rtRefs(t, repo); got != refs0 {
		t.Fatalf("refs not restored:\nbefore:\n%s\nafter:\n%s", refs0, got)
	}
	if _, err := os.Stat(canary); err == nil {
		t.Fatalf("write into the main worktree survives Discard: %s", canary)
	}
	if raw, err := os.ReadFile(filepath.Join(repo, "file0.txt")); err != nil || string(raw) != "commit 0\n" {
		t.Fatalf("modified tracked file not restored: %q %v", raw, err)
	}
	if raw, err := os.ReadFile(filepath.Join(repo, "file1.txt")); err != nil || string(raw) != "commit 1\n" {
		t.Fatalf("deleted tracked file not restored: %q %v", raw, err)
	}
	if raw, err := os.ReadFile(untracked); err != nil || string(raw) != "baseline\n" {
		t.Fatalf("modified untracked file not restored: %q %v", raw, err)
	}
	d1, err := root.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if d1 != d0 {
		t.Fatalf("H3 digest not restored: %s -> %s", d0[:12], d1[:12])
	}
}

// TestC1_4_6_RunKillsDetachedDescendant: a descendant that detaches with
// setsid (Unix) or a new process group without a console (Windows) and
// keeps writing must not extend Run, and must be dead when Run returns.
func TestC1_4_6_RunKillsDetachedDescendant(t *testing.T) {
	t.Setenv("COURT_TEST_HELPER", "1")
	repo := newRepo(t, 1)
	root := open(t, repo)
	w, err := root.Fork(head(t, repo), "rtdetach")
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	gw := w.(*world.GitWorld)
	defer gw.Discard()

	beat := filepath.Join(t.TempDir(), "beat.txt")
	argv := append(helperArgv("detach"), beat)
	start := time.Now()
	r := mustRun(gw, argv...)
	elapsed := time.Since(start)
	if r.ExitCode != 0 {
		t.Fatalf("launching the detached descendant failed: %s", r.Stderr)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("Run waited %v for a detached descendant", elapsed)
	}

	// Give the descendant a moment to be killed, then assert its
	// heartbeat has stopped.
	time.Sleep(300 * time.Millisecond)
	s1 := rtFileSize(t, beat)
	time.Sleep(600 * time.Millisecond)
	s2 := rtFileSize(t, beat)
	if s2 > s1 {
		t.Fatalf("detached descendant survived Run (heartbeat grew %d -> %d bytes)", s1, s2)
	}
}

func rtFileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatal(err)
	}
	return info.Size()
}
