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

	"court/internal/gate"
	"court/internal/ledger"
)

// This file implements /gate and /gate/pre: the internal endpoints the
// generated git hooks call from inside a world (spec/02 §2). The hooks are
// written by internal/gate into <DataDir>/hooks, the directory the world's
// per-world git config (core.hooksPath via GIT_CONFIG_*) points at.

// gatePost is the body of a post-commit hook call.
type gatePost struct {
	WorldPath string   `json:"world_path"`
	Commit    string   `json:"commit"`
	Parents   []string `json:"parents"`
}

// gatePre is the body of a pre-commit hook call: the world, the current
// HEAD (empty on an unborn branch) and the index tree the commit would
// record.
type gatePre struct {
	WorldPath string `json:"world_path"`
	Head      string `json:"head"`
	Tree      string `json:"tree"`
}

// handleGatePre serves POST /gate/pre: the declaration query of spec/03
// §4.2. A world whose gate is closed refuses with the exact one-line
// message; an open gate records the pass so the post-commit observation can
// mark the commit gated. A path that is not a registered Court world is not
// gated (there is no agent to declare).
func (d *Daemon) handleGatePre(w http.ResponseWriter, r *http.Request) {
	var gp gatePre
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&gp); err != nil {
		d.refuse(w, r, "gate", "bad gate pre body", http.StatusBadRequest)
		return
	}
	if gp.WorldPath == "" {
		d.refuse(w, r, "gate", "gate pre post lacks world_path", http.StatusBadRequest)
		return
	}
	d.mu.Lock()
	sess, ok := d.byTree[normPath(gp.WorldPath)]
	if ok {
		key := gate.WorldKey(sess.Commission, sess.World)
		if !d.gateState.Open(key) {
			d.mu.Unlock()
			writeJSON(w, http.StatusForbidden, map[string]any{"error": gate.Message})
			return
		}
		d.gatePasses[key] = gate.Pass{Parent: gp.Head, Tree: gp.Tree}
		err := d.saveGatePassesLocked()
		d.mu.Unlock()
		if err != nil {
			d.refuse(w, r, "gate", err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, 200, map[string]any{})
		return
	}
	d.mu.Unlock()
	writeJSON(w, 200, map[string]any{})
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
	err := d.recordCommits(sess, gp.Commit, clientKey(r, "gate"))
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
// commit), then appends world.commit oldest-first. Every event carries the
// commit id as its request identity plus any client-supplied key, so a
// reposted gate call can never double-record.
func (d *Daemon) recordCommits(sess *Session, head, key string) error {
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
		if err := d.recordOne(sess, id, key); err != nil {
			return err
		}
	}
	return nil
}

// recordOne emits one world.commit with parents and the numstat change
// list, resolved inside the world's own worktree. Gated is true only when
// the pre-commit hook passed for this commit's parent and tree (spec/03
// §4.5): a commit that bypassed the hook is recorded gated:false, and every
// detector hit in it is then undeclared.
func (d *Daemon) recordOne(sess *Session, commit, key string) error {
	pout, err := gitOut(sess.Worktree, "rev-list", "--parents", "-n", "1", commit)
	if err != nil {
		return fmt.Errorf("gate: parents of %s: %w", commit[:12], err)
	}
	parts := strings.Fields(pout)
	parents := append([]string{}, parts[1:]...)
	parent := ""
	if len(parents) > 0 {
		parent = parents[0]
	}
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
	gated := d.takeGatePass(sess, parent, commit)
	ev, _, err := d.emit(sess.Commission, "agent:"+sess.Agent, "world.commit",
		&ledger.WorldCommit{
			World: sess.World, Commit: commit, Parents: parents,
			Changes: changes, Gated: gated, Assumes: []string{},
		}, "world.commit|"+commit, key)
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

// takeGatePass reports whether the pre-commit hook authorized this commit
// and consumes the pass. A pass is bound to the pending commit's parent and
// index tree, so a commit made with --no-verify after the gate passed for a
// different tree is still ungated.
func (d *Daemon) takeGatePass(sess *Session, parent, commit string) bool {
	key := gate.WorldKey(sess.Commission, sess.World)
	d.mu.Lock()
	p, ok := d.gatePasses[key]
	d.mu.Unlock()
	if !ok || p.Parent != parent {
		return false
	}
	if p.Tree != "" {
		tree, err := gitOut(sess.Worktree, "rev-parse", commit+"^{tree}")
		if err != nil || strings.TrimSpace(tree) != p.Tree {
			return false
		}
	}
	d.mu.Lock()
	delete(d.gatePasses, key)
	d.saveGatePassesLocked()
	d.mu.Unlock()
	return true
}

// --------------------------------------------------------------------------
// Gate-pass persistence: a daemon restart between the pre-commit gate and
// the commit must not turn an authorized commit ungated.
// --------------------------------------------------------------------------

func (d *Daemon) gatePassPath() string {
	return filepath.Join(d.opts.DataDir, "gate-passes.json")
}

func (d *Daemon) loadGatePasses() error {
	raw, err := os.ReadFile(d.gatePassPath())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var m map[string]gate.Pass
	if err := json.Unmarshal(raw, &m); err != nil {
		return fmt.Errorf("daemon: gate-passes.json: %w", err)
	}
	if m != nil {
		d.gatePasses = m
	}
	return nil
}

func (d *Daemon) saveGatePassesLocked() error {
	raw, err := json.Marshal(d.gatePasses)
	if err != nil {
		return err
	}
	path := d.gatePassPath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// --------------------------------------------------------------------------
// Head scanner: commits that bypassed the post-commit hook (hooks disabled)
// are recorded gated:false at the next ledger write (spec/03 §4.5).
// --------------------------------------------------------------------------

// requestSyncLocked wakes the scanner; callers hold d.mu. Bursts coalesce.
func (d *Daemon) requestSyncLocked() {
	select {
	case d.syncCh <- struct{}{}:
	default:
	}
}

func (d *Daemon) syncLoop() {
	var last time.Time
	for {
		select {
		case <-d.closed:
			return
		case <-d.syncCh:
		}
		if wait := d.syncMin - time.Since(last); wait > 0 {
			t := time.NewTimer(wait)
			select {
			case <-d.closed:
				t.Stop()
				return
			case <-t.C:
			}
		}
		for drained := false; !drained; {
			select {
			case <-d.syncCh:
			default:
				drained = true
			}
		}
		last = time.Now()
		d.syncWorlds()
	}
}

// syncWorlds compares every registered world's HEAD with the last commit in
// the ledger and records the difference. It is the fallback path for a
// commit that reached a world with no hook at all.
func (d *Daemon) syncWorlds() {
	d.mu.Lock()
	seen := map[string]bool{}
	var sessions []*Session
	for _, sess := range d.sessions {
		if sess.World == "" || sess.Worktree == "" {
			continue
		}
		key := sess.Commission + "/" + sess.World
		if seen[key] {
			continue
		}
		seen[key] = true
		sessions = append(sessions, sess)
	}
	d.mu.Unlock()
	for _, sess := range sessions {
		if _, err := os.Stat(sess.Worktree); err != nil {
			continue
		}
		out, err := gitOut(sess.Worktree, "rev-parse", "--verify", "HEAD")
		if err != nil {
			continue
		}
		head := strings.TrimSpace(out)
		d.mu.Lock()
		last := d.lastCommit[sess.World]
		d.mu.Unlock()
		if head == "" || head == last {
			continue
		}
		wl := d.worldMuFor(sess.World)
		wl.Lock()
		_ = d.recordCommits(sess, head, "")
		wl.Unlock()
	}
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
