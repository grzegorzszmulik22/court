package scripted_test

// Red-team reproduction: POST /gate accepts any commit object in the shared
// store and records it under the world named in the request, without
// checking that the commit belongs to that world. The commit here is made
// with `git commit-tree` in a sibling world, so no hook exists to record it
// legitimately; it is then posted under the first agent's world path.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"
	"testing"

	"court/internal/adapter/scripted"
	"court/internal/ledger"
)

func rtGit(t *testing.T, dir string, stdin string, args ...string) string {
	t.Helper()
	// Own identity for every call: commit-tree must work on a host with no
	// global git identity (oci-free-a1).
	base := []string{"-C", dir, "-c", "user.name=redteam",
		"-c", "user.email=redteam@court.local"}
	cmd := exec.Command("git", append(base, args...)...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestRT_GateAcceptsForeignCommit(t *testing.T) {
	h := newHarness(t)
	w1, err := h.AddAgent(&scripted.AgentSpec{ID: "w1", Parent: "human"})
	if err != nil {
		t.Fatal(err)
	}
	w2, err := h.AddAgent(&scripted.AgentSpec{ID: "w2", Parent: "human"})
	if err != nil {
		t.Fatal(err)
	}
	base := rtGit(t, w2.Worktree, "", "rev-parse", "HEAD")
	tree := rtGit(t, w2.Worktree, "", "rev-parse", "HEAD^{tree}")
	// A commit object in the shared store with no ref, no branch and no
	// post-commit hook: w2 never "made" it as far as the ledger knows.
	foreign := rtGit(t, w2.Worktree, "fabricated work\n", "commit-tree", tree, "-p", base)

	raw, _ := json.Marshal(map[string]any{
		"world_path": w1.Worktree, "commit": foreign, "parents": []string{base},
	})
	resp, err := http.Post(h.Daemon.URL()+"/gate", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("gate post: %v", err)
	}
	resp.Body.Close()

	evs, err := h.Events()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range evs {
		if e.Type != "world.commit" {
			continue
		}
		if got := rtBody(e.Body, "commit"); got == foreign {
			t.Fatalf("gate recorded %s as world.commit of world %q (seq %d); that world never had it",
				foreign, rtBody(e.Body, "world"), e.Seq)
		}
	}
}

func rtBody(v *ledger.Value, key string) string {
	for _, m := range v.Obj {
		if m.Key == key && m.Val.Kind == ledger.String {
			return m.Val.Str
		}
	}
	return ""
}
