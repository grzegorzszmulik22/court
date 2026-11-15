package world

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

// DefaultRunTimeout bounds Run when Limits.Timeout is zero.
const DefaultRunTimeout = 10 * time.Minute

// maxRunOutput caps captured stdout/stderr of Run.
const maxRunOutput = 1 << 20

// lockRetries bounds internal ref-lock retries per git invocation.
const lockRetries = 12

// nameRe validates world names and commission ids: one safe branch and
// directory component.
var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)

// lockRe matches git stderr on ref-lock contention, which Court retries
// internally (spec C1.4.3: never surfaced to an agent).
var lockRe = regexp.MustCompile(`(?i)cannot lock ref|failed to lock|unable to lock|cannot lock it|\.lock'?:? (File exists|Permission denied)|Unable to create '[^']*\.lock'|Another git process seems to be running`)

// store is the backend state shared by every world of one commission.
type store struct {
	repo       string // main worktree top level
	gitDir     string // repository .git directory (shared object store)
	dataDir    string // Court data directory for this repository
	commission string
	gitEnv     []string // env entries implementing the per-world git config
	retries    atomic.Int64
}

// GitWorld is the git-worktree backend for World. The root world returned by
// Open is the repository's main worktree; worlds returned by Fork are linked
// worktrees on branches court/<commission>/<name> under <dataDir>/worlds.
type GitWorld struct {
	s         *store
	id        string // "" on the root world
	dir       string // working directory git commands run in
	branch    string // "" on the root world
	adminDir  string // $GIT_DIR/worktrees/<id>, "" on the root world
	discarded atomic.Bool
}

// Open binds a repository to a Court data directory and returns its root
// world. It refuses repositories with submodules with a one-sentence error
// (spec 02 §3); no world and no data directory content is created on refusal.
func Open(repoPath, dataDir, commission string) (*GitWorld, error) {
	if !nameRe.MatchString(commission) {
		return nil, fmt.Errorf("world: bad commission id %q", commission)
	}
	repo, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, err
	}
	repo, err = filepath.EvalSymlinks(repo)
	if err != nil {
		return nil, fmt.Errorf("world: %v", err)
	}
	out, err := plainGit(repo, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("world: %s is not a git repository: %v", repoPath, err)
	}
	top, err := filepath.EvalSymlinks(strings.TrimSpace(out))
	if err != nil {
		return nil, fmt.Errorf("world: %v", err)
	}
	if top != repo {
		return nil, fmt.Errorf("world: %s is not the repository top level", repoPath)
	}
	gitDir, err := plainGit(repo, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return nil, fmt.Errorf("world: %v", err)
	}
	gitDir = strings.TrimSpace(gitDir)
	// Objects, refs and worktree admin files live in the common dir; a repo
	// opened through a linked worktree reports it differently.
	if common, err := plainGit(repo, "rev-parse", "--git-common-dir"); err == nil {
		common = strings.TrimSpace(common)
		if !filepath.IsAbs(common) {
			common = filepath.Join(repo, common)
		}
		gitDir = common
	}
	if hasSubmodules(repo) {
		return nil, errors.New("world: repositories with submodules are not supported.")
	}
	dataDir, err = filepath.Abs(dataDir)
	if err != nil {
		return nil, err
	}
	s := &store{repo: repo, gitDir: gitDir, dataDir: dataDir, commission: commission}
	if err := os.MkdirAll(s.hooksDir(), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.worldsDir(), 0o755); err != nil {
		return nil, err
	}
	s.gitEnv = s.buildGitEnv()
	return &GitWorld{s: s, dir: repo}, nil
}

func (s *store) worldsDir() string { return filepath.Join(s.dataDir, "worlds") }
func (s *store) hooksDir() string  { return filepath.Join(s.dataDir, "hooks") }

// Path is the world's working directory.
func (w *GitWorld) Path() string { return w.dir }

// Branch is the world's branch, "" on the root world.
func (w *GitWorld) Branch() string { return w.branch }

// ID is the world id used under <dataDir>/worlds, "" on the root world.
func (w *GitWorld) ID() string { return w.id }

// Retries is the number of internal ref-lock retries spent so far across all
// worlds of this repository.
func (w *GitWorld) Retries() int64 { return w.s.retries.Load() }

// buildGitEnv encodes the per-world git configuration of spec 02 §3 as
// GIT_CONFIG_* entries, so repository configuration is never touched
// (checks C1.4.4, sabotage config-written-to-repo).
func (s *store) buildGitEnv() []string {
	pairs := [][2]string{
		{"core.hooksPath", s.hooksDir()},
		{"gc.auto", "0"}, // automatic gc is off for the whole commission
	}
	if runtime.GOOS == "windows" {
		pairs = append(pairs, [2]string{"core.longpaths", "true"})
	}
	env := make([]string, 0, 2*len(pairs)+1)
	env = append(env, fmt.Sprintf("GIT_CONFIG_COUNT=%d", len(pairs)))
	for i, p := range pairs {
		env = append(env,
			fmt.Sprintf("GIT_CONFIG_KEY_%d=%s", i, p[0]),
			fmt.Sprintf("GIT_CONFIG_VALUE_%d=%s", i, p[1]))
	}
	return env
}

// GitEnv returns the environment entries that put a process inside a world
// under Court's per-world git configuration. Adapters pass these to agent
// processes (spec 02 §3).
func (w *GitWorld) GitEnv() []string {
	return append([]string(nil), w.s.gitEnv...)
}

// cleanEnv strips inherited git state variables so a world's git can only be
// configured by GitEnv.
func (w *GitWorld) cleanEnv() []string {
	drop := map[string]bool{
		"GIT_DIR": true, "GIT_WORK_TREE": true, "GIT_INDEX_FILE": true,
		"GIT_OBJECT_DIRECTORY": true, "GIT_ALTERNATE_OBJECT_DIRECTORIES": true,
		"GIT_COMMON_DIR": true, "GIT_CONFIG": true, "GIT_CONFIG_COUNT": true,
		"GIT_CONFIG_PARAMETERS": true, "GIT_CONFIG_NOSYSTEM": true,
		"GIT_CEILING_DIRECTORIES": true,
	}
	env := make([]string, 0, 64)
	for _, e := range os.Environ() {
		k := e[:strings.IndexByte(e, '=')]
		if drop[k] || strings.HasPrefix(k, "GIT_CONFIG_KEY_") || strings.HasPrefix(k, "GIT_CONFIG_VALUE_") {
			continue
		}
		env = append(env, e)
	}
	return append(env, w.s.gitEnv...)
}

// hasSubmodules reports whether the repository at dir has a .gitmodules
// file, a registered submodule or a gitlink in HEAD's tree.
func hasSubmodules(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, ".gitmodules")); err == nil {
		return true
	}
	if out, err := plainGit(dir, "submodule", "status"); err == nil && strings.TrimSpace(out) != "" {
		return true
	}
	if _, err := plainGit(dir, "rev-parse", "--verify", "HEAD"); err != nil {
		return false // unborn HEAD: nothing else to scan
	}
	out, err := plainGit(dir, "ls-tree", "-r", "HEAD")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "160000 ") {
			return true
		}
	}
	return false
}

// plainGit runs git with the ambient environment (backend bookkeeping runs
// before a store exists or outside any world).
func plainGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %v: %s", args[0], err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

// git runs one git command in dir under the per-world environment, retrying
// ref-lock failures internally. It returns stdout, stderr and the process
// error (nil on exit 0).
func (s *store) git(ctx context.Context, env []string, dir string, args ...string) (string, string, error) {
	var out, errb string
	var err error
	for attempt := 0; ; attempt++ {
		out, errb, err = s.gitOnce(ctx, env, dir, args...)
		if err == nil || ctx.Err() != nil || !lockRe.MatchString(errb) {
			return out, errb, err
		}
		if attempt >= lockRetries {
			return out, errb, fmt.Errorf("git %s: ref lock held after %d retries: %s", args[0], attempt, strings.TrimSpace(errb))
		}
		s.retries.Add(1)
		select {
		case <-ctx.Done():
			return out, errb, ctx.Err()
		case <-time.After(20*time.Millisecond + time.Duration(attempt)*15*time.Millisecond):
		}
	}
}

func (s *store) gitOnce(ctx context.Context, env []string, dir string, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = env
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	return out.String(), errb.String(), err
}

// gitEnv returns env for backend git commands (same per-world config).
func (w *GitWorld) backendEnv() []string { return w.cleanEnv() }

// Fork implements World.Fork with `git worktree add <dir> -b <branch> <base>`
// (spec 02 §3). One branch per worktree is a git rule and is relied on.
func (w *GitWorld) Fork(base Commit, name string) (World, error) {
	if w.discarded.Load() {
		return nil, fmt.Errorf("world: %s is discarded", w.id)
	}
	if !nameRe.MatchString(name) || strings.Contains(name, "..") {
		return nil, fmt.Errorf("world: bad world name %q", name)
	}
	if _, err := os.Stat(filepath.Join(w.s.repo, ".gitmodules")); err == nil {
		return nil, errors.New("world: repositories with submodules are not supported.")
	}
	branch := "court/" + w.s.commission + "/" + name
	dir := filepath.Join(w.s.worldsDir(), name)
	if _, err := os.Stat(dir); err == nil {
		return nil, fmt.Errorf("world: %s already exists", dir)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	env := w.backendEnv()
	out, _, err := w.s.git(ctx, env, w.s.repo, "rev-parse", "--verify", string(base)+"^{commit}")
	if err != nil {
		return nil, fmt.Errorf("world: base %q is not a commit", base)
	}
	base = Commit(strings.TrimSpace(out))
	if _, errb, err := w.s.git(ctx, env, w.s.repo, "worktree", "add", "-b", branch, dir, string(base)); err != nil {
		return nil, fmt.Errorf("world: fork %s: %s", name, strings.TrimSpace(errb))
	}
	admin := ""
	if b, err := os.ReadFile(filepath.Join(dir, ".git")); err == nil {
		admin = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(b)), "gitdir: "))
	}
	return &GitWorld{s: w.s, id: name, dir: dir, branch: branch, adminDir: admin}, nil
}

// Diff implements World.Diff with `git diff --numstat -z --no-renames`.
func (w *GitWorld) Diff(from, to Commit) (Changes, error) {
	if w.discarded.Load() {
		return Changes{}, fmt.Errorf("world: %s is discarded", w.id)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, errb, err := w.s.git(ctx, w.backendEnv(), w.dir, "diff", "--numstat", "-z", "--no-renames", string(from), string(to))
	if err != nil {
		return Changes{}, fmt.Errorf("world: diff: %s", strings.TrimSpace(errb))
	}
	var ch Changes
	for _, rec := range strings.Split(strings.TrimRight(out, "\x00"), "\x00") {
		if rec == "" {
			continue
		}
		f := strings.SplitN(rec, "\t", 3)
		if len(f) != 3 {
			return Changes{}, fmt.Errorf("world: bad numstat record %q", rec)
		}
		fc := FileChange{Path: f[2], Added: -1, Deleted: -1}
		if f[0] != "-" {
			fmt.Sscanf(f[0], "%d", &fc.Added)
			fmt.Sscanf(f[1], "%d", &fc.Deleted)
		}
		ch.Files = append(ch.Files, fc)
	}
	return ch, nil
}

// Commits implements World.Commits with `git rev-list HEAD`.
func (w *GitWorld) Commits() ([]Commit, error) {
	if w.discarded.Load() {
		return nil, fmt.Errorf("world: %s is discarded", w.id)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, errb, err := w.s.git(ctx, w.backendEnv(), w.dir, "rev-list", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("world: commits: %s", strings.TrimSpace(errb))
	}
	var cs []Commit
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line != "" {
			cs = append(cs, Commit(line))
		}
	}
	return cs, nil
}

// Run implements World.Run. Command outcomes — exit code, timeout — are in
// Result; error is reserved for infrastructure failure (a world that is
// gone, a command that cannot start).
func (w *GitWorld) Run(argv []string, limits Limits) (Result, error) {
	if w.discarded.Load() {
		return Result{}, fmt.Errorf("world: %s is discarded", w.id)
	}
	if len(argv) == 0 {
		return Result{}, errors.New("world: empty command")
	}
	timeout := limits.Timeout
	if timeout <= 0 {
		timeout = DefaultRunTimeout
	}
	deadline := time.Now().Add(timeout)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	var res Result
	isGit := filepath.Base(argv[0]) == "git"
	for attempt := 0; ; attempt++ {
		res = w.runOnce(ctx, argv)
		res.Retries = attempt
		if res.TimedOut || !isGit || res.ExitCode == 0 || !lockRe.MatchString(res.Stderr) {
			return res, nil
		}
		if attempt >= lockRetries || time.Now().After(deadline) {
			return res, nil
		}
		w.s.retries.Add(1)
		select {
		case <-ctx.Done():
		case <-time.After(20*time.Millisecond + time.Duration(attempt)*15*time.Millisecond):
		}
		if ctx.Err() != nil {
			res.Retries = attempt + 1
			res.TimedOut = true
			res.ExitCode = -1
			return res, nil
		}
	}
}

// runOnce executes one attempt of Run with process-group semantics: on
// timeout the whole group is killed so children holding the pipes cannot
// hang the call.
func (w *GitWorld) runOnce(ctx context.Context, argv []string) Result {
	start := time.Now()
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = w.dir
	cmd.Env = w.cleanEnv()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout := &capBuf{limit: maxRunOutput}
	stderr := &capBuf{limit: maxRunOutput}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	res := Result{ExitCode: -1}
	if err := cmd.Start(); err != nil {
		res.Stderr = err.Error()
		res.Duration = time.Since(start)
		return res
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			res.ExitCode = 0
		} else if ee, ok := err.(*exec.ExitError); ok {
			res.ExitCode = ee.ExitCode()
		}
	case <-ctx.Done():
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
		res.TimedOut = ctx.Err() == context.DeadlineExceeded
	}
	res.Stdout, res.Stderr = stdout.String(), stderr.String()
	res.Duration = time.Since(start)
	return res
}

// capBuf is a bytes.Buffer that stops growing at limit.
type capBuf struct {
	buf   bytes.Buffer
	limit int
}

func (c *capBuf) Write(p []byte) (int, error) {
	room := c.limit - c.buf.Len()
	if room > 0 {
		if len(p) > room {
			c.buf.Write(p[:room])
		} else {
			c.buf.Write(p)
		}
	}
	return len(p), nil
}
func (c *capBuf) String() string { return c.buf.String() }

// Discard implements World.Discard: remove the worktree, delete the branch
// and every object the world created, and drop the worktree's administrative
// files. The repository is restored ref for ref and object for object (H3).
func (w *GitWorld) Discard() error {
	if w.branch == "" {
		return errors.New("world: the root world cannot be discarded")
	}
	if w.discarded.Swap(true) {
		return fmt.Errorf("world: %s already discarded", w.id)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	env := w.backendEnv()

	candidates := w.worldObjects(ctx, env)

	if _, errb, err := w.s.git(ctx, env, w.s.repo, "worktree", "remove", "--force", w.dir); err != nil {
		return fmt.Errorf("world: discard %s: worktree remove: %s", w.id, strings.TrimSpace(errb))
	}
	if _, errb, err := w.s.git(ctx, env, w.s.repo, "branch", "-D", w.branch); err != nil {
		return fmt.Errorf("world: discard %s: branch delete: %s", w.id, strings.TrimSpace(errb))
	}
	if w.adminDir != "" {
		if err := os.RemoveAll(w.adminDir); err != nil {
			return fmt.Errorf("world: discard %s: admin files: %v", w.id, err)
		}
	}
	if err := w.removeObjects(ctx, env, candidates); err != nil {
		return err
	}
	return nil
}

// worldObjects returns the ids of objects only this world can have created:
// objects reachable from its branch but from no other ref, and blobs staged
// in its index.
func (w *GitWorld) worldObjects(ctx context.Context, env []string) map[string]bool {
	cand := map[string]bool{}
	out, _, err := w.s.git(ctx, env, w.s.repo, "for-each-ref", "--format=%(refname)")
	if err != nil {
		return cand
	}
	var others []string
	for _, ref := range strings.Split(strings.TrimSpace(out), "\n") {
		if ref != "" && ref != "refs/heads/"+w.branch {
			others = append(others, ref)
		}
	}
	args := []string{"rev-list", "--objects", w.branch}
	if len(others) > 0 {
		args = append(args, "--not")
		args = append(args, others...)
	}
	if out, _, err := w.s.git(ctx, env, w.s.repo, args...); err == nil {
		for _, line := range strings.Split(out, "\n") {
			if f := strings.Fields(line); len(f) > 0 {
				cand[f[0]] = true
			}
		}
	}
	// Staged blobs that were never committed.
	if out, _, err := w.s.git(ctx, env, w.dir, "ls-files", "-s"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			f := strings.Fields(line)
			if len(f) >= 2 && isHex40or64(f[1]) {
				cand[f[1]] = true
			}
		}
	}
	return cand
}

// removeObjects deletes the loose object files of candidates that are
// unreachable after the branch is gone. Objects still reachable from any
// remaining ref are kept.
func (w *GitWorld) removeObjects(ctx context.Context, env []string, cand map[string]bool) error {
	if len(cand) == 0 {
		return nil
	}
	reachable := map[string]bool{}
	out, _, err := w.s.git(ctx, env, w.s.repo, "rev-list", "--objects", "--all")
	if err != nil {
		return fmt.Errorf("world: discard %s: reachable set: %v", w.id, err)
	}
	for _, line := range strings.Split(out, "\n") {
		if f := strings.Fields(line); len(f) > 0 {
			reachable[f[0]] = true
		}
	}
	for oid := range cand {
		if reachable[oid] {
			continue
		}
		p := filepath.Join(w.s.gitDir, "objects", oid[:2], oid[2:])
		if err := os.Remove(p); err == nil || os.IsNotExist(err) {
			_ = os.Remove(filepath.Dir(p)) // drop the fan-out dir if now empty
		}
	}
	return nil
}

var hexRe = regexp.MustCompile(`^[0-9a-f]{40,64}$`)

func isHex40or64(s string) bool { return hexRe.MatchString(s) }

// Digest hashes the repository state that isolation is measured on (H3,
// spec 02 §3): every ref, every object id in the object store, and the
// worktree list. Two digests taken before Fork and after Discard are equal
// iff the repository was restored exactly.
func (w *GitWorld) Digest() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	env := w.backendEnv()
	var lines []string
	out, _, err := w.s.git(ctx, env, w.s.repo, "for-each-ref", "--format=%(refname) %(objectname)")
	if err != nil {
		return "", fmt.Errorf("world: digest: refs: %v", err)
	}
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if l != "" {
			lines = append(lines, "R "+l)
		}
	}
	out, _, err = w.s.git(ctx, env, w.s.repo, "cat-file", "--batch-all-objects", "--batch-check=%(objectname)")
	if err != nil {
		return "", fmt.Errorf("world: digest: objects: %v", err)
	}
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if l != "" {
			lines = append(lines, "O "+l)
		}
	}
	out, _, err = w.s.git(ctx, env, w.s.repo, "worktree", "list", "--porcelain")
	if err != nil {
		return "", fmt.Errorf("world: digest: worktrees: %v", err)
	}
	// Paths are session-stable (same dataDir before and after), so the
	// porcelain block can go in verbatim.
	lines = append(lines, "W "+out)
	sort.Strings(lines)
	h := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(h[:]), nil
}
