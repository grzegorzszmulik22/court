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
	d.mu.Unlock()
	if !ok {
		writeJSON(w, 200, map[string]any{"gate": "open"})
		return
	}
	// A gate pass belongs to the world it names only when it describes
	// that worktree's current HEAD and index tree (M3.2 attribution rule).
	if err := verifyGatePre(sess, gp.Head, gp.Tree); err != nil {
		d.refuse(w, r, "gate", err.Error(), http.StatusForbidden)
		return
	}
	key := gate.WorldKey(sess.Commission, sess.World)
	d.mu.Lock()
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
	writeJSON(w, 200, map[string]any{"gate": "open"})
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
	// Attribution rule (M3.2, red-team B3-10): the posted commit must be
	// on this world's own branch, in this worktree's first-parent history
	// after the fork point. A dangling object created in a sibling world,
	// or a commit merged in from another world's branch, is not this
	// world's commit and is refused rather than recorded under its path.
	if err := verifyWorldCommit(sess, gp.Commit); err != nil {
		wl.Unlock()
		d.refuse(w, r, "gate", err.Error(), http.StatusForbidden)
		return
	}
	err := d.recordCommits(sess, gp.Commit, clientKey(r, "gate"))
	wl.Unlock()
	if err != nil {
		d.refuse(w, r, "gate", err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, 200, map[string]any{})
}

// verifyWorldCommit checks that commit belongs to the world's own history:
// it is the world's tip or on its first-parent chain after the fork point.
// First-parent excludes dangling objects (the B3-10 attack) and commits a
// merge brought in from another world's branch; those were made elsewhere
// and must be recorded under the world that made them.
func verifyWorldCommit(sess *Session, commit string) error {
	if commit == "" {
		return fmt.Errorf("gate: empty commit")
	}
	if sess.Base == "" {
		return fmt.Errorf("gate: world %s has no base commit", sess.World)
	}
	headOut, err := gitOut(sess.Worktree, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return fmt.Errorf("gate: world %s has no HEAD: %v", sess.World, err)
	}
	head := strings.TrimSpace(headOut)
	out, err := gitOut(sess.Worktree, "rev-list", "--first-parent", "--reverse",
		sess.Base+".."+head)
	if err != nil {
		return fmt.Errorf("gate: world %s history: %v", sess.World, err)
	}
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if l == commit {
			return nil
		}
	}
	return fmt.Errorf("gate: commit %s is not on world %s's branch", commit, sess.World)
}

// verifyGatePre checks that a pre-commit post describes this worktree's
// current state: its HEAD must be the worktree's HEAD and its tree the
// index tree the commit would record. A pass for another world's state
// cannot be created under this world's path.
func verifyGatePre(sess *Session, head, tree string) error {
	cur := ""
	if out, err := gitOut(sess.Worktree, "rev-parse", "--verify", "HEAD"); err == nil {
		cur = strings.TrimSpace(out)
	}
	if head != cur {
		return fmt.Errorf("gate: pre-commit head %q is not world %s's HEAD %q",
			head, sess.World, cur)
	}
	idxOut, err := gitOut(sess.Worktree, "write-tree")
	if err != nil {
		return fmt.Errorf("gate: world %s index: %v", sess.World, err)
	}
	idx := strings.TrimSpace(idxOut)
	if tree == "" || tree != idx {
		return fmt.Errorf("gate: pre-commit tree %q is not world %s's index tree %q",
			tree, sess.World, idx)
	}
	return nil
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
	// First-parent only: a merge can bring another world's commits into
	// this branch's object graph, but they were made in that other world
	// and must not be recorded here (M3.2 attribution rule, B3-10).
	out, err := gitOut(sess.Worktree, "rev-list", "--first-parent", "--reverse", revRange)
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
			if _, err := fmt.Sscanf(f[0], "%d", &pc.Added); err != nil {
				return fmt.Errorf("gate: bad numstat count %q: %w", f[0], err)
			}
			if _, err := fmt.Sscanf(f[1], "%d", &pc.Deleted); err != nil {
				return fmt.Errorf("gate: bad numstat count %q: %w", f[1], err)
			}
		}
		changes = append(changes, pc)
	}
	gated, err := d.takeGatePass(sess, parent, commit)
	if err != nil {
		return err
	}
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

// takeGatePass reports whether the pre-commit hook authorized this exact
// commit (same parent and same index tree) and consumes the pass. One pass
// authorizes one commit: any observed commit in the world consumes it, so a
// further commit on the same parent after an amend, reset or --no-verify
// attempt is ungated. A pass recorded without a tree authorizes nothing.
func (d *Daemon) takeGatePass(sess *Session, parent, commit string) (bool, error) {
	key := gate.WorldKey(sess.Commission, sess.World)
	d.mu.Lock()
	p, ok := d.gatePasses[key]
	if ok {
		delete(d.gatePasses, key)
	}
	var saveErr error
	if ok {
		saveErr = d.saveGatePassesLocked()
	}
	d.mu.Unlock()
	if saveErr != nil {
		return false, fmt.Errorf("gate: consume pass: %w", saveErr)
	}
	if !ok || p.Parent != parent || p.Tree == "" {
		return false, nil
	}
	tree, err := gitOut(sess.Worktree, "rev-parse", commit+"^{tree}")
	if err != nil {
		return false, fmt.Errorf("gate: tree of %s: %w", commit, err)
	}
	return strings.TrimSpace(tree) == p.Tree, nil
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
		if err := d.syncWorlds(); err != nil {
			d.logf("gate sync: %v", err)
		}
	}
}

// syncWorlds compares every registered world's HEAD with the last commit in
// the ledger and records the difference. It is the fallback path for a
// commit that reached a world with no hook at all. Every failure is
// returned (the scanner logs it) rather than discarded.
func (d *Daemon) syncWorlds() error {
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
	var first error
	note := func(err error) {
		if first == nil {
			first = err
		}
	}
	for _, sess := range sessions {
		if _, err := os.Stat(sess.Worktree); err != nil {
			if !os.IsNotExist(err) {
				note(fmt.Errorf("world %s: %w", sess.World, err))
			}
			continue
		}
		out, err := gitOut(sess.Worktree, "rev-parse", "--verify", "HEAD")
		if err != nil {
			note(fmt.Errorf("world %s: %w", sess.World, err))
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
		err = d.recordCommits(sess, head, "")
		wl.Unlock()
		if err != nil {
			note(fmt.Errorf("world %s: %w", sess.World, err))
		}
	}
	return first
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
