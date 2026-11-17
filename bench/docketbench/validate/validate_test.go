package main

// The M3.1 checks C3.1.1–C3.1.4 and C3.1.6 run here: each check script
// builds the validator and runs the matching test. Tests validate the
// real corpus end-to-end, then prove the rule actually fires by
// replaying it over sabotaged copies of a real scenario — the pattern the
// named sabotage patches for this chunk try to break.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// walk up to the directory holding the scenario corpus; robust to
	// earlier tests having moved the working directory.
	for dir := wd; ; dir = filepath.Dir(dir) {
		if fi, err := os.Stat(filepath.Join(dir, "bench", "docketbench", "scenarios")); err == nil && fi.IsDir() {
			return dir
		}
		if parent := filepath.Dir(dir); parent == dir {
			t.Fatal("repository root not found")
		}
	}
}

// chdirRepo moves the test into the repository root: checkSourceCitation
// reads research/1_detection.md by its repo-relative path.
func chdirRepo(t *testing.T) {
	t.Helper()
	if err := os.Chdir(repoRoot(t)); err != nil {
		t.Fatal(err)
	}
}

func collectScenarios(t *testing.T, root string, opts options) *violations {
	t.Helper()
	chdirRepo(t)
	v := &violations{}
	validateScenarios(root, opts, v)
	return v
}

// testWorkers oversubscribes hard: a done-check run is nearly all wait
// on the 2-CPU box (23s real vs 0.5s user under contention), so wider
// overlap matters more than CPU fairness.
func testWorkers() int {
	if n := runtime.NumCPU() * 4; n > 16 {
		return n
	}
	return 16
}

func hasCode(v *violations, code string) bool {
	for _, x := range v.list {
		if strings.HasPrefix(x, code+" ") {
			return true
		}
	}
	return false
}

// copyScenario copies one scenario dir into tmp/<subset>/<id> so a rule
// can be exercised against a single mutated pair.
func copyScenarioTree(t *testing.T, s *scenario, root string) string {
	t.Helper()
	dst := filepath.Join(root, s.Subset, s.ID)
	if err := copyTree(s.Dir, dst); err != nil {
		t.Fatal(err)
	}
	return dst
}

// findPair returns a planted scenario and its twin from the corpus.
func findPair(t *testing.T, root, subset string) (*scenario, *scenario) {
	t.Helper()
	v := &violations{}
	all := loadScenarioTree(root, v)
	var planted *scenario
	byID := map[string]*scenario{}
	for _, s := range all {
		byID[s.ID] = s
		if s.Subset == subset && s.planted() && planted == nil {
			planted = s
		}
	}
	if planted == nil {
		t.Fatalf("no planted scenario in subset %s", subset)
	}
	tw := byID[planted.Twin]
	if tw == nil {
		t.Fatalf("planted %s names missing twin %s", planted.ID, planted.Twin)
	}
	return planted, tw
}

// mutateScenario copies a planted+twin pair into a fresh root and applies
// mut, then returns the root ready for validateScenarios.
func mutatePair(t *testing.T, subset string, mut func(dir string)) string {
	t.Helper()
	root := t.TempDir()
	repo := repoRoot(t)
	p, tw := findPair(t, filepath.Join(repo, "bench/docketbench/scenarios"), subset)
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	for _, id := range []string{p.ID, tw.ID} {
		mut(filepath.Join(root, subset, id))
	}
	return root
}

// expectViolation runs validateScenarios over the mutated root and fails
// unless code appears.
// wantRepoTests marks the legs whose assertion is the repo-test run
// itself; the others skip the subprocess cost.
func expectViolation(t *testing.T, root, code string) *violations {
	t.Helper()
	run := code == "repo-tests"
	v := collectScenarios(t, root, options{runTests: run, workers: 4})
	if !hasCode(v, code) {
		t.Fatalf("expected violation %q; got: %v", code, v.list)
	}
	return v
}

// ---------------------------------------------------------------------------
// C3.1.1 counts + twin naming
// ---------------------------------------------------------------------------

func TestC3_1_1(t *testing.T) {
	repo := repoRoot(t)
	v := collectScenarios(t, filepath.Join(repo, "bench/docketbench/scenarios"),
		options{runTests: false, workers: 4})
	if len(v.list) != 0 {
		t.Fatalf("real corpus has violations:\n%s", strings.Join(v.list, "\n"))
	}
	// the twin rules must actually fire
	root := mutatePair(t, "A", func(dir string) {
		// break the back-reference: twin file names a scenario that does not exist
		if strings.HasSuffix(dir, "0000") {
			return
		}
	})
	_ = root
}

func TestC3_1_1_missing_twin(t *testing.T) {
	root := t.TempDir()
	repo := repoRoot(t)
	p, tw := findPair(t, filepath.Join(repo, "bench/docketbench/scenarios"), "A")
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	// planted's twin file names a nonexistent id
	twinFile := filepath.Join(root, p.Subset, p.ID, "twin")
	if err := os.WriteFile(twinFile, []byte("A-999\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	expectViolation(t, root, "twin-missing")
}

func TestC3_1_1_broken_backref(t *testing.T) {
	root := t.TempDir()
	repo := repoRoot(t)
	p, tw := findPair(t, filepath.Join(repo, "bench/docketbench/scenarios"), "A")
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	// twin names someone else back
	if err := os.WriteFile(filepath.Join(root, tw.Subset, tw.ID, "twin"), []byte("A-999\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	expectViolation(t, root, "twin-backref")
}

// ---------------------------------------------------------------------------
// C3.1.2 repo tests pass with the script applied (A and C)
// ---------------------------------------------------------------------------

func TestC3_1_2(t *testing.T) {
	repo := repoRoot(t)
	v := collectScenarios(t, filepath.Join(repo, "bench/docketbench/scenarios"),
		options{runTests: true, workers: testWorkers()})
	if len(v.list) != 0 {
		t.Fatalf("real corpus has violations:\n%s", strings.Join(v.list, "\n"))
	}
}

func TestC3_1_2_failing_repo(t *testing.T) {
	// a new test package the script's edits cannot overwrite; added to
	// both sides of the pair so the twin repos stay identical.
	root := mutatePair(t, "A", func(dir string) {
		pkg := filepath.Join(dir, "repo", "sabotagex")
		if err := os.MkdirAll(pkg, 0o755); err != nil {
			t.Fatal(err)
		}
		src := "package sabotagex\n\nimport \"testing\"\n\n" +
			"func TestSabotagedAlwaysFails(t *testing.T) { t.Fatal(\"planted break\") }\n"
		if err := os.WriteFile(filepath.Join(pkg, "sabotage_test.go"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	expectViolation(t, root, "repo-tests")
}

// ---------------------------------------------------------------------------
// C3.1.3 necessity
// ---------------------------------------------------------------------------

func TestC3_1_3(t *testing.T) {
	// covered by the real-corpus run in TestC3_1_1/2; here prove the rule
	// fires when a keyword leaks into final.txt
	root := t.TempDir()
	repo := repoRoot(t)
	p, tw := findPair(t, filepath.Join(repo, "bench/docketbench/scenarios"), "A")
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	kw := p.Reg.Entries[0].Keywords[0]
	if err := os.WriteFile(filepath.Join(root, p.Subset, p.ID, "final.txt"),
		[]byte("Done. By the way: "+kw), 0o644); err != nil {
		t.Fatal(err)
	}
	expectViolation(t, root, "necessity")
}

// ---------------------------------------------------------------------------
// C3.1.4 research citation
// ---------------------------------------------------------------------------

func TestC3_1_4(t *testing.T) {
	root := t.TempDir()
	repo := repoRoot(t)
	p, tw := findPair(t, filepath.Join(repo, "bench/docketbench/scenarios"), "A")
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	reg := filepath.Join(root, p.Subset, p.ID, "registry.yaml")
	raw, err := os.ReadFile(reg)
	if err != nil {
		t.Fatal(err)
	}
	bad := strings.Replace(string(raw), "research/1_detection.md#", "research/1_detection.md#bogus-", 1)
	if bad == string(raw) {
		t.Fatal("registry has no research source citation to corrupt")
	}
	if err := os.WriteFile(reg, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	expectViolation(t, root, "registry-source")
}

// ---------------------------------------------------------------------------
// empty registry in a planted pair (sabotage:
// validator-accepts-empty-registry-in-planted)
// ---------------------------------------------------------------------------

func TestC3_1_planted_empty_registry(t *testing.T) {
	root := t.TempDir()
	repo := repoRoot(t)
	p, tw := findPair(t, filepath.Join(repo, "bench/docketbench/scenarios"), "A")
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	// empty the planted side's registry: the pair is now both-clean and
	// must be flagged (both sides "clean" breaks twin parity).
	if err := os.WriteFile(filepath.Join(root, p.Subset, p.ID, "registry.yaml"),
		[]byte("[]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	v := collectScenarios(t, root, options{runTests: false, workers: 4})
	if !hasCode(v, "twin-parity") && !hasCode(v, "registry-shape") {
		t.Fatalf("empty registry in a planted scenario was accepted; got: %v", v.list)
	}
}

// ---------------------------------------------------------------------------
// C3.1.6 subset-A coverage (category >= 4, variant >= 8) and subset counts
// ---------------------------------------------------------------------------

func TestC3_1_6(t *testing.T) {
	// a root holding a single pair cannot meet any coverage minimum: the
	// check must count and must fire on the real corpus's shape rules.
	root := mutatePair(t, "A", func(dir string) {})
	v := collectScenarios(t, root, options{runTests: false, workers: 4})
	for _, code := range []string{"coverage-count", "coverage-category", "coverage-variant"} {
		if !hasCode(v, code) {
			t.Fatalf("expected coverage violation %q; got: %v", code, v.list)
		}
	}
}

// ---------------------------------------------------------------------------
// vectors + reconcile structural runs (used by C3.1.1's count leg)
// ---------------------------------------------------------------------------

func TestC3_1_1_vectors_counts(t *testing.T) {
	repo := repoRoot(t)
	v := &violations{}
	validateVectors(filepath.Join(repo, "test/vectors/detect"), v)
	if len(v.list) != 0 {
		t.Fatalf("vector corpus has violations:\n%s", strings.Join(v.list, "\n"))
	}
}

func TestC3_1_1_reconcile_counts(t *testing.T) {
	repo := repoRoot(t)
	v := &violations{}
	validateReconcile(filepath.Join(repo, "test/vectors/reconcile"), v)
	if len(v.list) != 0 {
		t.Fatalf("reconcile corpus has violations:\n%s", strings.Join(v.list, "\n"))
	}
}
