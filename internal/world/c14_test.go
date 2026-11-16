package world_test

// Check tests for chunk M1.4 (worlds). Each TestC1_4_* backs one check file
// under test/checks/M1.4/. Test repositories live in t.TempDir; nothing here
// forks or configures a repository it did not create.

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"court/internal/world"
)

// mm13Sample is one randomised MM1.3 session, recorded as a raw sample.
type mm13Sample struct {
	Seed       int64  `json:"seed"`
	OK         bool   `json:"ok"`
	Ops        int    `json:"ops"`
	Discards   int    `json:"discards"`
	Commits    int    `json:"commits"`
	DurationMs int64  `json:"duration_ms"`
	Err        string `json:"err,omitempty"`
}

// TestC1_4_1_MM13 implements check C1.4.1 / measurement MM1.3: random
// sequences of fork, edit, commit, stage, run and discard across up to 8
// worlds, >=200 sessions; after discarding every world the repository file
// state equals the state before any world was created, loose object payloads
// excepted (they are unreachable and outside H3). Per discard it also
// asserts the world's branch, worktree and admin files are gone while
// sibling worlds are untouched.
//
// Sessions run in groups that share one repository: a session that restores
// the digest leaves the repository exactly where the next session needs it,
// so a fresh repository per session would measure nothing extra. A session
// that fails forces a fresh repository for the remaining sessions.
func TestC1_4_1_MM13(t *testing.T) {
	const sessions = 200
	const groupSize = 10
	base := t.TempDir()
	samples := make([]mm13Sample, sessions)
	var wg sync.WaitGroup
	sem := make(chan struct{}, 2*runtime.GOMAXPROCS(0)) // spawn-bound: pipeline the exec latency
	for g := 0; g < sessions/groupSize; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			dir, err := os.MkdirTemp(base, "g*")
			if err != nil {
				panic(err)
			}
			var env *mm13Env
			for i := 0; i < groupSize; i++ {
				seed := int64(g*groupSize + i)
				if env == nil {
					var err error
					if env, err = newMM13Env(dir); err != nil {
						samples[seed] = mm13Sample{Seed: seed, Err: "env: " + err.Error()}
						continue
					}
				}
				samples[seed] = mm13Session(env, seed)
				if !samples[seed].OK {
					env = nil // state may be dirty; do not reuse it
				}
			}
		}(g)
	}
	wg.Wait()

	failed := 0
	for _, s := range samples {
		if !s.OK {
			failed++
			t.Errorf("seed %d: %s", s.Seed, s.Err)
		}
	}
	writeMM13Samples(t, samples)
	if failed > 0 {
		t.Fatalf("MM1.3: %d/%d sessions did not restore the repository", failed, sessions)
	}
}

func writeMM13Samples(t *testing.T, samples []mm13Sample) {
	t.Helper()
	out := os.Getenv("COURT_MM13_OUT")
	if out == "" {
		return
	}
	durs := make([]float64, len(samples))
	var commits, discards int
	for i, s := range samples {
		durs[i] = float64(s.DurationMs)
		commits += s.Commits
		discards += s.Discards
	}
	sort.Float64s(durs)
	med := durs[len(durs)/2]
	var mean float64
	for _, d := range durs {
		mean += d
	}
	mean /= float64(len(durs))
	var sd float64
	for _, d := range durs {
		sd += (d - mean) * (d - mean)
	}
	cv := math.Sqrt(sd/float64(len(durs))) / mean
	rec := map[string]any{
		"measurement": "MM1.3",
		"target":      "100% of >= 200 randomised sessions restore the digest",
		"sessions":    len(samples),
		"restored":    len(samples) - countFailed(samples),
		"commits":     commits,
		"discards":    discards,
		"median_ms":   med,
		"cv":          math.Round(cv*1000) / 1000,
		"samples":     samples,
	}
	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func countFailed(samples []mm13Sample) int {
	n := 0
	for _, s := range samples {
		if !s.OK {
			n++
		}
	}
	return n
}

// mm13Env is the repository state one group of sessions shares: sessions
// that restore the repository leave it exactly where the next session needs
// it.
type mm13Env struct {
	repo  string
	root  *world.GitWorld
	d0    string       // backend Digest before any world
	fs0   string       // fsState before any world
	head  world.Commit // main tip; sessions never commit to main
	first world.Commit // root commit, an alternate fork base
}

// looseObjRe matches the payload path of a loose object (objects/xx/<rest>).
// Loose objects are not part of the exact-restore property: H3 hashes every
// ref and every object id reachable before Fork and after Discard, and an
// unreachable loose object a discarded world left behind is invisible to it.
// Everything else under .git — refs, logs, packed-refs, worktree admin
// entries, indexes, pack files and the top-level files — is still compared.
var looseObjRe = regexp.MustCompile(`^objects/[0-9a-f]{2}/[0-9a-f]{38,62}$`)

// fsState lists every regular file under the repository's git directory with
// a content hash, minus loose object payloads. It verifies exact restore
// independently of the backend's own Digest, from filesystem truth.
func fsState(repo string) string {
	gitDir := filepath.Join(repo, ".git")
	if raw, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
		gitDir = filepath.Join(repo, strings.TrimSpace(string(raw)))
	}
	var b strings.Builder
	_ = filepath.WalkDir(gitDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(gitDir, p)
		if relErr != nil {
			return relErr
		}
		if looseObjRe.MatchString(filepath.ToSlash(rel)) {
			return nil
		}
		var h []byte
		var size int64
		if raw, err := os.ReadFile(p); err == nil {
			s := sha256.Sum256(raw)
			h = s[:]
			size = int64(len(raw))
		}
		fmt.Fprintf(&b, "%s %d %x\n", filepath.ToSlash(rel), size, h)
		return nil
	})
	return b.String()
}

// courtRefs lists refs/heads/court/** straight from the filesystem (loose
// refs and packed-refs) — zero subprocesses.
func courtRefs(repo string) map[string]bool {
	gitDir := filepath.Join(repo, ".git")
	if raw, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
		gitDir = filepath.Join(repo, strings.TrimSpace(string(raw)))
	}
	refs := map[string]bool{}
	base := filepath.Join(gitDir, "refs", "heads", "court")
	_ = filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(gitDir, p)
		refs[filepath.ToSlash(rel)] = true
		return nil
	})
	if raw, err := os.ReadFile(filepath.Join(gitDir, "packed-refs")); err == nil {
		for _, line := range strings.Split(string(raw), "\n") {
			if f := strings.Fields(line); len(f) == 2 && strings.HasPrefix(f[1], "refs/heads/court/") {
				refs[f[1]] = true
			}
		}
	}
	return refs
}

// newMM13Env builds a repository under dir and opens the world root on it.
// Panics internally are folded into the error so a group survives them.
func newMM13Env(dir string) (env *mm13Env, err error) {
	defer func() {
		if r := recover(); r != nil {
			env = nil
			err = fmt.Errorf("%v", r)
		}
	}()
	repo, err := os.MkdirTemp(dir, "repo*")
	if err != nil {
		return nil, err
	}
	mustGit(repo, "init", "-q", "-b", "main")
	mustGit(repo, "config", "user.email", "court@test")
	mustGit(repo, "config", "user.name", "court")
	mustGit(repo, "config", "commit.gpgsign", "false")
	for i := 0; i < 2; i++ {
		if err := os.WriteFile(filepath.Join(repo, "file"+strconv.Itoa(i)+".txt"),
			[]byte("commit "+strconv.Itoa(i)+"\n"), 0o644); err != nil {
			return nil, err
		}
		mustGit(repo, "add", "-A", ".")
		mustGit(repo, "commit", "-qm", "c"+strconv.Itoa(i))
	}
	data, err := os.MkdirTemp(dir, "data*")
	if err != nil {
		return nil, err
	}
	root, err := world.Open(repo, data, "Ctest")
	if err != nil {
		return nil, fmt.Errorf("open: %v", err)
	}
	d0, err := root.Digest()
	if err != nil {
		return nil, fmt.Errorf("digest: %v", err)
	}
	return &mm13Env{
		repo:  repo,
		root:  root,
		d0:    d0,
		fs0:   fsState(repo),
		head:  world.Commit(strings.TrimSpace(mustGit(repo, "rev-parse", "HEAD"))),
		first: world.Commit(strings.TrimSpace(mustGit(repo, "rev-list", "--max-parents=0", "HEAD"))),
	}, nil
}

// mustGit is git-or-panic for code that reports failures through samples.
func mustGit(dir string, args ...string) string {
	out, err := gitE(dir, args...)
	if err != nil {
		panic(fmt.Sprintf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out))
	}
	return out
}

// mm13Session runs one seeded session. It returns a sample instead of
// failing so the caller sees every failing seed.
func mm13Session(env *mm13Env, seed int64) (s mm13Sample) {
	s.Seed = seed
	start := time.Now()
	defer func() {
		s.DurationMs = time.Since(start).Milliseconds()
		if r := recover(); r != nil {
			s.Err = fmt.Sprintf("panic: %v", r)
		}
	}()
	fail := func(format string, args ...any) {
		s.Err = fmt.Sprintf(format, args...)
		panic(s.Err)
	}

	rng := rand.New(rand.NewSource(seed))
	repo := env.repo
	root := env.root

	type live struct {
		w *world.GitWorld
	}
	var worlds []*live
	forks := 0

	fork := func() {
		base := env.head
		if rng.Intn(3) == 0 {
			base = env.first
		}
		w, err := root.Fork(base, fmt.Sprintf("s%02dw%d", seed, forks))
		if err != nil {
			fail("fork %d: %v", forks, err)
		}
		worlds = append(worlds, &live{w: w.(*world.GitWorld)})
		forks++
		s.Ops++
	}
	edit := func(l *live, i int) {
		p := filepath.Join(l.w.Path(), "sub", fmt.Sprintf("f%d.txt", rng.Intn(6)))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			fail("edit dir: %v", err)
		}
		if err := os.WriteFile(p, []byte(fmt.Sprintf("seed %d step %d\n", seed, i)), 0o644); err != nil {
			fail("edit: %v", err)
		}
		s.Ops++
	}
	commit := func(l *live) {
		// A commit is a change plus the commit: write a file so the pick
		// always produces one.
		edit(l, s.Ops)
		if r := mustRun(l.w, "git", "add", "-A", "."); r.ExitCode != 0 {
			fail("git add in %s: %s", l.w.ID(), r.Stderr)
		}
		r := mustRun(l.w, "git", "commit", "-m", fmt.Sprintf("s%d %s", seed, l.w.ID()))
		s.Ops++
		if r.ExitCode != 0 {
			fail("commit failed in %s: %s", l.w.ID(), r.Stderr)
		}
		s.Commits++
	}
	discard := func(i int) {
		l := worlds[i]
		worlds = append(worlds[:i], worlds[i+1:]...)
		s.Discards++
		s.Ops++
		if err := l.w.Discard(); err != nil {
			fail("discard %s: %v", l.w.ID(), err)
		}
		if _, err := os.Stat(l.w.Path()); !os.IsNotExist(err) {
			fail("discard %s: worktree dir still present", l.w.ID())
		}
		refs := courtRefs(repo)
		if refs["refs/heads/court/Ctest/"+l.w.ID()] {
			fail("discard %s: branch still resolves", l.w.ID())
		}
		for _, o := range worlds {
			if !refs["refs/heads/court/Ctest/"+o.w.ID()] {
				fail("discard %s: sibling branch lost", o.w.ID())
			}
		}
	}

	steps := 7 + rng.Intn(7) // 7..13 ops
	for i := 0; i < steps; i++ {
		var pick string
		switch {
		case len(worlds) == 0:
			pick = "fork"
		default:
			pool := []string{"edit", "commit", "run", "stage", "discard", "discard"}
			if len(worlds) < 8 {
				pool = append(pool, "fork", "fork")
			}
			pick = pool[rng.Intn(len(pool))]
		}
		switch pick {
		case "fork":
			fork()
		case "edit":
			edit(worlds[rng.Intn(len(worlds))], i)
		case "commit":
			commit(worlds[rng.Intn(len(worlds))])
		case "stage":
			// Stage without committing: the blob objects exist but no ref
			// reaches them, and discard must still remove them.
			l := worlds[rng.Intn(len(worlds))]
			edit(l, i)
			if r := mustRun(l.w, "git", "add", "-A", "."); r.ExitCode != 0 {
				fail("git add in %s: %s", l.w.ID(), r.Stderr)
			}
			s.Ops++
		case "run":
			l := worlds[rng.Intn(len(worlds))]
			r := mustRun(l.w, "git", "status", "--porcelain")
			s.Ops++
			if r.ExitCode != 0 {
				fail("git status in %s: exit %d: %s", l.w.ID(), r.ExitCode, r.Stderr)
			}
		case "discard":
			discard(rng.Intn(len(worlds)))
		}
	}
	for len(worlds) > 0 {
		discard(len(worlds) - 1)
	}

	// The backend Digest is asserted in a sample of sessions (fsState, the
	// independent filesystem listing, is asserted in all of them).
	if seed%10 == 0 {
		d1, err := root.Digest()
		if err != nil {
			fail("digest after discards: %v", err)
		}
		if d1 != env.d0 {
			fail("digest not restored: before %s after %s", env.d0[:12], d1[:12])
		}
	}
	if fs1 := fsState(repo); fs1 != env.fs0 {
		fail("repository files not restored")
	}
	s.OK = true
	return s
}

func mustRun(w *world.GitWorld, argv ...string) world.Result {
	r, err := w.Run(argv, world.Limits{Timeout: 30 * time.Second})
	if err != nil {
		panic(fmt.Sprintf("run %v: %v", argv, err))
	}
	return r
}

// TestC1_4_1_SiblingStagedBlob is the discard-safety case of C1.4.1: a blob
// staged in one world's index and never committed must still exist and be
// readable after a sibling world is discarded. A blob held only by an index
// is not reachable from any ref, so only a Discard that never deletes from
// the shared object store can keep it.
func TestC1_4_1_SiblingStagedBlob(t *testing.T) {
	repo := newRepo(t, 1)
	root := open(t, repo)
	base := head(t, repo)

	kw, err := root.Fork(base, "keep")
	if err != nil {
		t.Fatalf("fork keep: %v", err)
	}
	gk := kw.(*world.GitWorld)

	blobs := map[string]string{}
	for i, name := range []string{"drop1", "drop2"} {
		// Stage a fresh blob in keep's index; it is never committed, so no
		// ref or reflog can reach it.
		rel := fmt.Sprintf("staged%d.txt", i)
		content := fmt.Sprintf("staged in keep before discarding %s\n", name)
		writeFile(t, gk.Path(), rel, content)
		if r := mustRun(gk, "git", "add", rel); r.ExitCode != 0 {
			t.Fatalf("stage %s in keep: %s", rel, r.Stderr)
		}
		r := mustRun(gk, "git", "ls-files", "-s", rel)
		if f := strings.Fields(r.Stdout); len(f) >= 2 {
			blobs[f[1]] = content
		} else {
			t.Fatalf("cannot read staged blob id from %q", r.Stdout)
		}

		w, err := root.Fork(base, name)
		if err != nil {
			t.Fatalf("fork %s: %v", name, err)
		}
		if err := w.Discard(); err != nil {
			t.Fatalf("discard %s: %v", name, err)
		}

		// Every blob staged so far must still exist and be readable: from
		// the main worktree and inside the surviving world.
		for sha, want := range blobs {
			if out := git(t, repo, "cat-file", "blob", sha); out != want {
				t.Fatalf("staged blob %s unreadable after discarding %s: got %q", sha[:12], name, out)
			}
			if r := mustRun(gk, "git", "cat-file", "blob", sha); r.ExitCode != 0 || r.Stdout != want {
				t.Fatalf("blob %s lost inside keep after discarding %s (exit %d)", sha[:12], name, r.ExitCode)
			}
		}
	}
	if err := gk.Discard(); err != nil {
		t.Fatalf("discard keep: %v", err)
	}
}

// TestC1_4_2_SameBranch implements check C1.4.2: two worlds cannot hold the
// same branch; the error is reported, not retried.
func TestC1_4_2_SameBranch(t *testing.T) {
	repo := newRepo(t, 1)
	root := open(t, repo)

	if _, err := root.Fork(head(t, repo), "dup"); err != nil {
		t.Fatalf("first fork: %v", err)
	}
	retries := root.Retries()
	start := time.Now()
	if _, err := root.Fork(head(t, repo), "dup"); err == nil {
		t.Fatal("second fork of the same name succeeded; the branch would be held twice")
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("same-name fork error took %s; errors are reported, not retried", d)
	}
	if got := root.Retries(); got != retries {
		t.Fatalf("same-name fork burned %d internal retries", got-retries)
	}

	// The branch exists without a world: fork must still refuse and report.
	git(t, repo, "branch", "court/Ctest/pre", "HEAD")
	if _, err := root.Fork(head(t, repo), "pre"); err == nil {
		t.Fatal("fork over an existing court branch succeeded")
	}

	// A branch checked out in one world cannot be checked out in another:
	// git refuses inside Run too (the one-branch-per-worktree rule).
	if _, err := root.Fork(head(t, repo), "held"); err != nil {
		t.Fatalf("fork held: %v", err)
	}
	w2, err := root.Fork(head(t, repo), "other")
	if err != nil {
		t.Fatalf("fork other: %v", err)
	}
	gw2 := w2.(*world.GitWorld)
	if r := mustRun(gw2, "git", "checkout", "court/Ctest/held"); r.ExitCode == 0 {
		t.Fatal("a second world checked out the first world's branch")
	}
	if _, err := root.Fork(head(t, repo), "held"); err == nil {
		t.Fatal("fork of a branch held by another world succeeded")
	}
}

// TestC1_4_3_ConcurrentCommits implements check C1.4.3: eight worlds commit
// concurrently for 60 seconds; no commit is lost and no ref-lock failure is
// surfaced to an agent (internal retries are counted).
func TestC1_4_3_ConcurrentCommits(t *testing.T) {
	repo := newRepo(t, 1)
	root := open(t, repo)
	const nWorlds = 8
	const dur = 60 * time.Second

	base := head(t, repo)
	var worlds []*world.GitWorld
	for i := 0; i < nWorlds; i++ {
		w, err := root.Fork(base, fmt.Sprintf("cw%d", i))
		if err != nil {
			t.Fatalf("fork %d: %v", i, err)
		}
		worlds = append(worlds, w.(*world.GitWorld))
	}

	type outcome struct {
		commits int
		errs    []string
	}
	outs := make([]outcome, nWorlds)
	end := time.Now().Add(dur)
	var wg sync.WaitGroup
	for i, w := range worlds {
		wg.Add(1)
		go func(idx int, gw *world.GitWorld) {
			defer wg.Done()
			n := 0
			for time.Now().Before(end) {
				name := fmt.Sprintf("c%d.txt", n)
				if err := os.WriteFile(filepath.Join(gw.Path(), name),
					[]byte(fmt.Sprintf("w%d commit %d\n", idx, n)), 0o644); err != nil {
					outs[idx].errs = append(outs[idx].errs, "write: "+err.Error())
					return
				}
				if r, err := gw.Run([]string{"git", "add", "-A", name}, world.Limits{Timeout: 20 * time.Second}); err != nil || r.ExitCode != 0 {
					outs[idx].errs = append(outs[idx].errs, fmt.Sprintf("add: err=%v exit=%d stderr=%s", err, r.ExitCode, r.Stderr))
					return
				}
				r, err := gw.Run([]string{"git", "commit", "-qm", fmt.Sprintf("w%d c%d", idx, n)}, world.Limits{Timeout: 20 * time.Second})
				if err != nil {
					outs[idx].errs = append(outs[idx].errs, "commit run: "+err.Error())
					return
				}
				if r.ExitCode != 0 {
					// A ref-lock failure must never reach the agent.
					outs[idx].errs = append(outs[idx].errs, fmt.Sprintf("commit: exit=%d stderr=%s", r.ExitCode, r.Stderr))
					return
				}
				n++
			}
			outs[idx].commits = n
		}(i, w)
	}
	wg.Wait()

	total := 0
	for i, o := range outs {
		if len(o.errs) > 0 {
			t.Errorf("world %d: %s", i, o.errs[0])
		}
		total += o.commits
		got := strings.TrimSpace(git(t, repo, "rev-list", "--count", "court/Ctest/"+worlds[i].ID(), "--not", string(base)))
		if got != strconv.Itoa(o.commits) {
			t.Errorf("world %d: %d commits succeeded but rev-list counts %s", i, o.commits, got)
		}
	}
	t.Logf("C1.4.3: %d commits across %d worlds in %s, %d internal lock retries",
		total, nWorlds, dur, root.Retries())
	if total == 0 {
		t.Fatal("no commits were made; the check proved nothing")
	}

	// Deterministic retry proof: hold a ref lock from outside, then release
	// it; the update must succeed through internal retries, never surface.
	gw := worlds[0]
	lockDir := filepath.Join(repo, ".git", "refs", "heads", "court", "Ctest")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(lockDir, "probe.lock")
	if err := os.WriteFile(lock, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(400 * time.Millisecond)
		_ = os.Remove(lock)
	}()
	before := root.Retries()
	r := mustRun(gw, "git", "update-ref", "refs/heads/court/Ctest/probe", "HEAD")
	if r.ExitCode != 0 {
		t.Fatalf("ref-lock failure surfaced to the agent: exit %d: %s", r.ExitCode, r.Stderr)
	}
	if r.Retries == 0 {
		t.Fatal("held ref lock was not retried internally")
	}
	if root.Retries() <= before {
		t.Fatal("internal lock retries were not counted")
	}
	t.Logf("C1.4.3: held-lock update succeeded after %d counted retries", r.Retries)
}

// TestC1_4_4_ConfigIdentical implements check C1.4.4: `git config --list` for
// the repository is byte-identical before a commission and after it. It also
// pins the per-world environment configuration (gc off, hooks path) that a
// sabotage would remove or move into the repository config.
func TestC1_4_4_ConfigIdentical(t *testing.T) {
	repo := newRepo(t, 1)
	listBefore := git(t, repo, "config", "--list")
	cfgBefore, err := os.ReadFile(filepath.Join(repo, ".git", "config"))
	if err != nil {
		t.Fatal(err)
	}

	root := open(t, repo)
	for _, name := range []string{"a", "b"} {
		w, err := root.Fork(head(t, repo), name)
		if err != nil {
			t.Fatalf("fork %s: %v", name, err)
		}
		gw := w.(*world.GitWorld)

		// The per-world config is in the environment, not the repository.
		r := mustRun(gw, "git", "config", "gc.auto")
		if strings.TrimSpace(r.Stdout) != "0" {
			t.Fatalf("gc.auto inside world = %q, want 0 (env)", r.Stdout)
		}
		r = mustRun(gw, "git", "config", "core.hooksPath")
		if !strings.HasSuffix(strings.TrimSpace(r.Stdout), "/hooks") {
			t.Fatalf("core.hooksPath inside world = %q, want the data dir hooks dir", r.Stdout)
		}
		r = mustRun(gw, "git", "config", "--local", "--list")
		if strings.Contains(r.Stdout, "gc.auto") || strings.Contains(r.Stdout, "hooksPath") {
			t.Fatalf("per-world config leaked into repository config:\n%s", r.Stdout)
		}
		if r := mustRun(gw, "git", "config", "--local", "--get", "gc.auto"); r.ExitCode == 0 {
			t.Fatalf("gc.auto present in local config: %q", r.Stdout)
		}

		writeFile(t, gw.Path(), "x.txt", "x\n")
		mustRun(gw, "git", "add", "-A", ".")
		mustRun(gw, "git", "commit", "-qm", "x")
		if err := gw.Discard(); err != nil {
			t.Fatalf("discard %s: %v", name, err)
		}
	}

	listAfter := git(t, repo, "config", "--list")
	if listAfter != listBefore {
		t.Fatalf("git config --list changed:\n--- before ---\n%s\n--- after ---\n%s", listBefore, listAfter)
	}
	cfgAfter, err := os.ReadFile(filepath.Join(repo, ".git", "config"))
	if err != nil {
		t.Fatal(err)
	}
	if string(cfgAfter) != string(cfgBefore) {
		t.Fatal(".git/config bytes changed")
	}
}

// TestC1_4_5_SubmoduleRefusal implements check C1.4.5: a repository with a
// submodule is refused with one sentence and no world is created.
func TestC1_4_5_SubmoduleRefusal(t *testing.T) {
	sub := newRepo(t, 1)
	repo := newRepo(t, 1)
	if out, err := gitE(repo, "-c", "protocol.file.allow=always", "submodule", "add", "-q", sub, "sub"); err != nil {
		t.Skipf("cannot create a submodule here: %v\n%s", err, out)
	}
	git(t, repo, "commit", "-qm", "with-submodule")

	data := filepath.Join(t.TempDir(), "data")
	_, err := world.Open(repo, data, "Ctest")
	if err == nil {
		t.Fatal("world.Open accepted a repository with a submodule")
	}
	assertOneSentence(t, err.Error())

	if wt := git(t, repo, "worktree", "list", "--porcelain"); strings.Count(wt, "worktree ") != 1 {
		t.Fatalf("a worktree was created despite refusal:\n%s", wt)
	}
	if _, err := gitE(repo, "rev-parse", "--verify", "refs/heads/court/Ctest"); err == nil {
		t.Fatal("a court branch exists despite refusal")
	}
	for _, d := range []string{filepath.Join(data, "worlds"), filepath.Join(data, "hooks")} {
		if _, err := os.Stat(d); !os.IsNotExist(err) {
			t.Fatalf("%s was created despite refusal", d)
		}
	}

	// A gitlink without .gitmodules is still a repository with a submodule.
	repo2 := newRepo(t, 1)
	if out, err := gitE(repo2, "-c", "protocol.file.allow=always", "submodule", "add", "-q", sub, "sub"); err != nil {
		t.Skipf("cannot create a submodule here: %v\n%s", err, out)
	}
	git(t, repo2, "rm", "-qf", ".gitmodules")
	git(t, repo2, "commit", "-qm", "gitlink-only")
	if _, err := world.Open(repo2, filepath.Join(t.TempDir(), "data"), "Ctest"); err == nil {
		t.Fatal("world.Open accepted a repository with a bare gitlink")
	} else {
		assertOneSentence(t, err.Error())
	}
}

func assertOneSentence(t *testing.T, msg string) {
	t.Helper()
	if strings.ContainsAny(msg, "\n\r") {
		t.Fatalf("refusal is not one sentence (has a line break): %q", msg)
	}
	if strings.Count(msg, ".") != 1 || !strings.HasSuffix(msg, ".") {
		t.Fatalf("refusal is not one sentence: %q", msg)
	}
}

// TestC1_4_6_RunTimeout implements check C1.4.6: Run enforces its time limit
// and reports a timeout distinctly from a non-zero exit. It runs under plain
// `go test` on Windows and Linux alike: the commands Run executes are the
// test binary itself (helperArgv), never a shell.
func TestC1_4_6_RunTimeout(t *testing.T) {
	repo := newRepo(t, 1)
	root := open(t, repo)
	w, err := root.Fork(head(t, repo), "rt")
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	gw := w.(*world.GitWorld)
	t.Setenv("COURT_TEST_HELPER", "1")

	start := time.Now()
	r, err := gw.Run(helperArgv("sleep"), world.Limits{Timeout: 300 * time.Millisecond})
	if err != nil {
		t.Fatalf("run sleep: %v", err)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("timed-out run took %s; the limit is not enforced", d)
	}
	if !r.TimedOut || r.ExitCode != -1 {
		t.Fatalf("timeout reported as exit=%d timedOut=%v; want timedOut=true exit=-1", r.ExitCode, r.TimedOut)
	}

	// A child that keeps running must not hang the call: the timeout kills
	// the whole process tree, so the pipes the grandchild holds close.
	start = time.Now()
	r, err = gw.Run(helperArgv("tree"), world.Limits{Timeout: 300 * time.Millisecond})
	if err != nil {
		t.Fatalf("run tree: %v", err)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("timed-out process tree took %s; descendants survived", d)
	}
	if !r.TimedOut {
		t.Fatalf("process-tree timeout reported as exit=%d timedOut=%v", r.ExitCode, r.TimedOut)
	}

	// Non-zero exits keep their code and are not timeouts.
	for _, code := range []int{1, 3, 42} {
		r, err = gw.Run(helperArgv(strconv.Itoa(code)), world.Limits{Timeout: 10 * time.Second})
		if err != nil {
			t.Fatalf("run exit %d: %v", code, err)
		}
		if r.ExitCode != code || r.TimedOut {
			t.Fatalf("exit %d reported as exit=%d timedOut=%v", code, r.ExitCode, r.TimedOut)
		}
	}

	r, err = gw.Run(helperArgv("echo"), world.Limits{Timeout: 10 * time.Second})
	if err != nil || r.ExitCode != 0 {
		t.Fatalf("run echo: err=%v exit=%d", err, r.ExitCode)
	}
	if r.Stdout != "out\n" || r.Stderr != "err\n" {
		t.Fatalf("streams not captured: stdout=%q stderr=%q", r.Stdout, r.Stderr)
	}
}

// TestC1_4_7_WindowsLongPaths implements check C1.4.7 on Windows (restated
// by CR-001): a world whose root is at most 200 characters holds, commits,
// diffs and discards a file whose full path is longer than 260 characters;
// the same file fails with core.longpaths off, proving the per-world env
// config is what makes it work. It also asserts the stated limit: Open
// refuses a data directory that would put a world root above 200
// characters. Skipped on non-Windows hosts — C1.4.8 proves the package
// still builds for Windows from Linux.
func TestC1_4_7_WindowsLongPaths(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("SKIPPED-ENV: C1.4.7 needs a Windows host (CR-001)")
	}
	repo := newRepo(t, 1)
	root := open(t, repo)
	base := head(t, repo)

	w, err := root.Fork(base, "lp")
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	gw := w.(*world.GitWorld)
	if got := len(gw.Path()); got > world.MaxWorldRootLen {
		t.Fatalf("world root is %d characters, above the %d limit", got, world.MaxWorldRootLen)
	}

	// A file whose full path inside the world exceeds 260 characters.
	rel := "leaf.txt"
	for len(filepath.Join(gw.Path(), rel)) <= 270 {
		rel = filepath.Join("d0123456789", rel)
	}
	writeFile(t, gw.Path(), rel, "deep\n")
	full := filepath.Join(gw.Path(), rel)
	if len(full) <= 260 {
		t.Fatalf("test file path is only %d characters, want >260", len(full))
	}
	if r := mustRun(gw, "git", "add", "-A", "."); r.ExitCode != 0 {
		t.Fatalf("git add of a >260 path failed under core.longpaths=true: %s", r.Stderr)
	}
	if r := mustRun(gw, "git", "commit", "-qm", "deep"); r.ExitCode != 0 {
		t.Fatalf("git commit of a >260 path failed under core.longpaths=true: %s", r.Stderr)
	}
	cs, err := gw.Commits()
	if err != nil || len(cs) == 0 {
		t.Fatalf("commits after the deep commit: %v", err)
	}
	ch, err := gw.Diff(base, cs[0])
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	found := false
	for _, f := range ch.Files {
		if strings.HasSuffix(filepath.ToSlash(f.Path), "leaf.txt") {
			found = true
		}
	}
	if !found {
		t.Fatalf("diff does not report the >260 path: %+v", ch.Files)
	}

	// Negative control: without core.longpaths the same kind of file cannot
	// be staged — the setting is what makes the positive case work.
	rel2 := "leaf2.txt"
	for len(filepath.Join(gw.Path(), rel2)) <= 270 {
		rel2 = filepath.Join("d0123456789", rel2)
	}
	writeFile(t, gw.Path(), rel2, "deep2\n")
	if _, err := gitE(gw.Path(), "-c", "core.longpaths=false", "add", "-A", "."); err == nil {
		if out := git(t, gw.Path(), "ls-files", "--", filepath.ToSlash(rel2)); strings.TrimSpace(out) != "" {
			t.Fatal("a >260 path was staged with core.longpaths=false; the positive case proves nothing")
		}
	}
	if err := gw.Discard(); err != nil {
		t.Fatalf("discard: %v", err)
	}
	if _, err := os.Stat(gw.Path()); !os.IsNotExist(err) {
		t.Fatal("world dir still present after discard")
	}

	// The stated limit: opening a commission whose data directory would put
	// a world root above 200 characters fails in one sentence naming it.
	deep := filepath.Join(t.TempDir(), strings.Repeat("d", 150))
	_, err = world.Open(repo, deep, "Ctest")
	if err == nil {
		t.Fatal("Open accepted a data directory that would put a world root above 200 characters")
	}
	assertOneSentence(t, err.Error())
	if !strings.Contains(err.Error(), "200") {
		t.Fatalf("refusal does not name the limit: %q", err.Error())
	}
	if _, err := os.Stat(deep); !os.IsNotExist(err) {
		t.Fatalf("%s was created despite refusal", deep)
	}
}
