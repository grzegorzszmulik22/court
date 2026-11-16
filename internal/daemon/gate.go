package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"court/internal/ledger"
)

// This file implements /gate: the internal endpoint the generated
// post-commit git hook calls from inside a world (spec/02 §2). The hook is
// written by the daemon into <DataDir>/hooks/, the directory the world's
// per-world git config (core.hooksPath via GIT_CONFIG_*) points at.

// gatePost is the body of a post-commit hook call.
type gatePost struct {
	WorldPath string   `json:"world_path"`
	Commit    string   `json:"commit"`
	Parents   []string `json:"parents"`
}

// writeGateHook writes <DataDir>/hooks/post-commit: a /bin/sh script that
// posts the new commit to the daemon. git for Windows runs hooks through
// its bundled sh, so one script covers both platforms.
func (d *Daemon) writeGateHook() error {
	dir := filepath.Join(d.opts.DataDir, "hooks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	url := d.URL() + "/gate"
	script := "#!/bin/sh\n" +
		"# Written by the Court daemon; do not edit.\n" +
		"# Reports the new HEAD commit to the gate endpoint, then exits 0 so a\n" +
		"# commit never fails on a ledger problem: a post that cannot be\n" +
		"# delivered is picked up by the next post's parent scan (spec/02 §2).\n" +
		"top=$(git rev-parse --show-toplevel 2>/dev/null) || exit 0\n" +
		"head=$(git rev-parse HEAD 2>/dev/null) || exit 0\n" +
		"parents=$(git rev-list --parents -n 1 HEAD 2>/dev/null | cut -d' ' -f2-)\n"
	if d.opts.CourtBin != "" {
		// The preferred path needs no curl or wget: `court gate` posts
		// the commit itself.
		script += "'" + d.opts.CourtBin + "' gate --url '" + url +
			"' --world \"$top\" --commit \"$head\" --parents \"$parents\" || true\n"
	} else {
		script += "pj='['; first=1\n" +
			"for p in $parents; do\n" +
			"  if [ $first -eq 1 ]; then first=0; else pj=\"$pj,\"; fi\n" +
			"  pj=\"$pj\\\"$p\\\"\"\n" +
			"done\n" +
			"pj=\"$pj]\"\n" +
			"tw=$(printf '%s' \"$top\" | sed 's/\\\\/\\\\\\\\/g; s/\"/\\\\\"/g')\n" +
			"body=\"{\\\"world_path\\\":\\\"$tw\\\",\\\"commit\\\":\\\"$head\\\",\\\"parents\\\":$pj}\"\n" +
			"if command -v curl >/dev/null 2>&1; then\n" +
			"  curl -s -o /dev/null -m 10 -X POST -H 'Content-Type: application/json' \\\n" +
			"    --data-binary \"$body\" '" + url + "' || true\n" +
			"elif command -v wget >/dev/null 2>&1; then\n" +
			"  wget -q -O /dev/null -T 10 --post-data \"$body\" \\\n" +
			"    --header='Content-Type: application/json' '" + url + "' || true\n" +
			"fi\n"
	}
	script += "exit 0\n"
	path := filepath.Join(dir, "post-commit")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		return err
	}
	return os.Chmod(path, 0o755)
}

// handleGate serves POST /gate. The commit is attributed to the session
// whose world it happened in (worlds never move, so the registered path is
// authoritative), and the ledger is caught up with every ancestor the hook
// missed (failed posts, commits made while the daemon was down).
func (d *Daemon) handleGate(w http.ResponseWriter, r *http.Request) {
	var gp gatePost
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&gp); err != nil {
		d.refuse(w, r, "gate", "bad gate body", http.StatusBadRequest)
		return
	}
	if gp.WorldPath == "" || gp.Commit == "" {
		d.refuse(w, r, "gate", "gate post lacks world_path or commit", http.StatusBadRequest)
		return
	}
	d.mu.Lock()
	sess, ok := d.byTree[normPath(gp.WorldPath)]
	var wl *sync.Mutex
	if ok {
		wl = d.worldMuFor(sess.World)
	}
	d.mu.Unlock()
	if !ok {
		d.refuse(w, r, "gate", "unknown world path", http.StatusNotFound)
		return
	}
	// Serialize per world so two near-simultaneous posts record in order.
	wl.Lock()
	err := d.recordCommits(sess, gp.Commit)
	wl.Unlock()
	if err != nil {
		d.refuse(w, r, "gate", err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, 200, map[string]any{})
}

func (d *Daemon) worldMuFor(world string) *sync.Mutex {
	m, ok := d.worldMu[world]
	if !ok {
		m = &sync.Mutex{}
		d.worldMu[world] = m
	}
	return m
}

// recordCommits walks back from `head` until it reaches commits already in
// the ledger (or the world's base — the fork point is not the world's
// commit), then appends world.commit oldest-first.
func (d *Daemon) recordCommits(sess *Session, head string) error {
	// List commits from head back to the oldest already-recorded commit of
	// this world; without one, the world's base bounds the walk.
	d.mu.Lock()
	last := d.lastCommit[sess.World]
	d.mu.Unlock()
	bound := last
	if bound == "" {
		bound = sess.Base
	}
	revRange := head
	if bound != "" {
		revRange = bound + ".." + head
	}
	out, err := gitOut(sess.Worktree, "rev-list", "--reverse", revRange)
	if err != nil {
		return fmt.Errorf("gate: rev-list: %w", err)
	}
	var ids []string
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if l != "" {
			ids = append(ids, l)
		}
	}
	for _, id := range ids {
		d.mu.Lock()
		seen := d.recorded[id]
		d.mu.Unlock()
		if seen {
			continue
		}
		if err := d.recordOne(sess, id); err != nil {
			return err
		}
	}
	return nil
}

// recordOne emits one world.commit with parents and the numstat change
// list, resolved inside the world's own worktree.
func (d *Daemon) recordOne(sess *Session, commit string) error {
	pout, err := gitOut(sess.Worktree, "rev-list", "--parents", "-n", "1", commit)
	if err != nil {
		return fmt.Errorf("gate: parents of %s: %w", commit[:12], err)
	}
	parts := strings.Fields(pout)
	parents := append([]string{}, parts[1:]...)
	var changes []ledger.PathChange
	diffArgs := []string{"diff", "--numstat", "-z", "--no-renames"}
	if len(parents) == 0 {
		// Root commit: diff against the empty tree.
		diffArgs = append(diffArgs, "4b825dc642cb6eb9a060e54bf8d69288fbee4904", commit)
	} else {
		diffArgs = append(diffArgs, parents[0], commit)
	}
	dout, err := gitOut(sess.Worktree, diffArgs...)
	if err != nil {
		return fmt.Errorf("gate: numstat of %s: %w", commit[:12], err)
	}
	changes = []ledger.PathChange{}
	for _, rec := range strings.Split(strings.TrimRight(dout, "\x00"), "\x00") {
		if rec == "" {
			continue
		}
		f := strings.SplitN(rec, "\t", 3)
		if len(f) != 3 {
			return fmt.Errorf("gate: bad numstat record %q", rec)
		}
		pc := ledger.PathChange{Path: f[2], Added: -1, Deleted: -1}
		if f[0] != "-" {
			fmt.Sscanf(f[0], "%d", &pc.Added)
			fmt.Sscanf(f[1], "%d", &pc.Deleted)
		}
		changes = append(changes, pc)
	}
	ev, _, err := d.emit(sess.Commission, "agent:"+sess.Agent, "world.commit",
		&ledger.WorldCommit{
			World: sess.World, Commit: commit, Parents: parents,
			Changes: changes, Gated: false, Assumes: []string{},
		})
	if err != nil {
		return err
	}
	if ev != nil {
		d.mu.Lock()
		d.recorded[commit] = true
		d.lastCommit[sess.World] = commit
		d.mu.Unlock()
	}
	return nil
}

// gitOut runs a read-only git command inside a worktree.
func gitOut(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return string(out), nil
}
