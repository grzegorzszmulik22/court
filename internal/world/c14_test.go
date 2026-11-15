package world_test

// Check tests for chunk M1.4 (worlds). Each TestC1_4_* backs one check file
// under test/checks/M1.4/. Test repositories live in t.TempDir; nothing here
// forks or configures a repository it did not create.

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
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
// sequences of fork, edit, commit, run and discard across up to 8 worlds,
// >=200 sessions; after discarding every world the repository digest equals
// the digest before any world was created. Per discard it also asserts the
// world's branch, worktree, admin files and objects are gone while sibling
// worlds are untouched.
func TestC1_4_1_MM13(t *testing.T) {
	const sessions = 200
	samples := make([]mm13Sample, sessions)
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	for s := int64(0); s < sessions; s++ {
		wg.Add(1)
		go func(seed int64) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			samples[seed] = mm13Session(t, seed)
		}(s)
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

// mm13Session runs one seeded session. It returns a sample instead of
// failing so the caller sees every failing seed.
func mm13Session(t *testing.T, seed int64) (s mm13Sample) {
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
	repo := newRepo(t, 1+rng.Intn(3))
	root, err := world.Open(repo, filepath.Join(t.TempDir(), "data"), "Ctest")
	if err != nil {
		fail("open: %v", err)
	}
	d0, err := root.Digest()
	if err != nil {
		fail("digest: %v", err)
	}

	type live struct {
		w       *world.GitWorld
		commits []string // shas this world produced (object-level check at discard)
	}
	var worlds []*live
	forks := 0

	first := world.Commit(strings.TrimSpace(git(t, repo, "rev-list", "--max-parents=0", "HEAD")))
	fork := func() {
		base := head(t, repo)
		if rng.Intn(3) == 0 {
			base = first
		}
		w, err := root.Fork(base, "w"+strconv.Itoa(forks))
		if err != nil {
			fail("fork %d: %v", forks, err)
		}
		worlds = append(worlds, &live{w: w.(*world.GitWorld)})
		forks++
		s.Ops++
	}
	commit := func(l *live) {
		if r := mustRun(l.w, "git", "add", "-A", "."); r.ExitCode != 0 {
			fail("git add in %s: %s", l.w.ID(), r.Stderr)
		}
		r := mustRun(l.w, "git", "commit", "-qm", fmt.Sprintf("s%d %s", seed, l.w.ID()))
		s.Ops++
		if r.ExitCode != 0 {
			return // nothing staged; not a failure of the world backend
		}
		sha := strings.TrimSpace(mustRun(l.w, "git", "rev-parse", "HEAD").Stdout)
		l.commits = append(l.commits, sha)
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
		refs := git(t, repo, "for-each-ref", "--format=%(refname)", "refs/heads/court/")
		if strings.Contains(refs, "refs/heads/court/Ctest/"+l.w.ID()) {
			fail("discard %s: branch still resolves", l.w.ID())
		}
		for _, o := range worlds {
			if !strings.Contains(refs, "refs/heads/court/Ctest/"+o.w.ID()) {
				fail("discard %s: sibling branch lost", o.w.ID())
			}
		}
		// Object-level check: every commit the world produced must be out of
		// the object store, in one batched query.
		if len(l.commits) > 0 {
			var b strings.Builder
			for _, sha := range l.commits {
				b.WriteString(sha + "\n")
			}
			cmd := exec.Command("git", "cat-file", "--batch-check")
			cmd.Dir = repo
			cmd.Env = cleanGitEnv()
			cmd.Stdin = strings.NewReader(b.String())
			out, err := cmd.Output()
			if err != nil {
				fail("discard %s: cat-file batch: %v", l.w.ID(), err)
			}
			for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				if !strings.HasSuffix(line, "missing") {
					fail("discard %s: object still in the object store: %s", l.w.ID(), line)
				}
			}
		}
	}

	steps := 8 + rng.Intn(9) // 8..16 ops
	for i := 0; i < steps; i++ {
		var pick string
		switch {
		case len(worlds) == 0:
			pick = "fork"
		default:
			pool := []string{"edit", "commit", "run", "discard", "discard"}
			if len(worlds) < 8 {
				pool = append(pool, "fork", "fork")
			}
			pick = pool[rng.Intn(len(pool))]
		}
		switch pick {
		case "fork":
			fork()
		case "edit":
			l := worlds[rng.Intn(len(worlds))]
			writeFile(t, l.w.Path(), filepath.Join("sub", fmt.Sprintf("f%d.txt", rng.Intn(6))),
				fmt.Sprintf("seed %d step %d\n", seed, i))
			s.Ops++
		case "commit":
			commit(worlds[rng.Intn(len(worlds))])
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

	d1, err := root.Digest()
	if err != nil {
		fail("digest after discards: %v", err)
	}
	if d1 != d0 {
		fail("digest not restored: before %s after %s", d0[:12], d1[:12])
	}
	if wt := strings.TrimSpace(git(t, repo, "worktree", "list", "--porcelain")); strings.Count(wt, "worktree ") != 1 {
		fail("worktree list not restored:\n%s", wt)
	}
	if ents, _ := os.ReadDir(filepath.Join(repo, ".git", "worktrees")); len(ents) > 0 {
		fail("stale worktree admin files: %v", ents)
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
// and reports a timeout distinctly from a non-zero exit.
func TestC1_4_6_RunTimeout(t *testing.T) {
	repo := newRepo(t, 1)
	root := open(t, repo)
	w, err := root.Fork(head(t, repo), "rt")
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	gw := w.(*world.GitWorld)

	start := time.Now()
	r, err := gw.Run([]string{"sleep", "30"}, world.Limits{Timeout: 300 * time.Millisecond})
	if err != nil {
		t.Fatalf("run sleep: %v", err)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("timed-out run took %s; the limit is not enforced", d)
	}
	if !r.TimedOut || r.ExitCode != -1 {
		t.Fatalf("timeout reported as exit=%d timedOut=%v; want timedOut=true exit=-1", r.ExitCode, r.TimedOut)
	}

	// A child that keeps running must not hang the call (process group).
	start = time.Now()
	r, err = gw.Run([]string{"sh", "-c", "sleep 30 & wait"}, world.Limits{Timeout: 300 * time.Millisecond})
	if err != nil {
		t.Fatalf("run group: %v", err)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("timed-out process group took %s", d)
	}
	if !r.TimedOut {
		t.Fatalf("process-group timeout reported as exit=%d timedOut=%v", r.ExitCode, r.TimedOut)
	}

	// Non-zero exits keep their code and are not timeouts.
	for _, code := range []int{1, 3, 42} {
		r, err = gw.Run([]string{"sh", "-c", "exit " + strconv.Itoa(code)}, world.Limits{Timeout: 10 * time.Second})
		if err != nil {
			t.Fatalf("run exit %d: %v", code, err)
		}
		if r.ExitCode != code || r.TimedOut {
			t.Fatalf("exit %d reported as exit=%d timedOut=%v", code, r.ExitCode, r.TimedOut)
		}
	}

	r, err = gw.Run([]string{"sh", "-c", "echo out; echo err >&2"}, world.Limits{Timeout: 10 * time.Second})
	if err != nil || r.ExitCode != 0 {
		t.Fatalf("run echo: err=%v exit=%d", err, r.ExitCode)
	}
	if r.Stdout != "out\n" || r.Stderr != "err\n" {
		t.Fatalf("streams not captured: stdout=%q stderr=%q", r.Stdout, r.Stderr)
	}
}
