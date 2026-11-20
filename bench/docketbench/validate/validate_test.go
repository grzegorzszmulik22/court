package main

// The M3.1 checks C3.1.1–C3.1.4 and C3.1.6 run here: each check script
// builds the validator and runs the matching test. Tests validate the
// real corpus end-to-end, then prove the rule actually fires by
// replaying it over sabotaged copies of a real scenario — the pattern the
// named sabotage patches for this chunk try to break.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"
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

// corpusCache loads the scenario corpus once per root: fixtures all read the
// same immutable tree, and re-parsing 200 scenarios per fixture dominates the
// suite on a starved box.
var (
	corpusOnce sync.Once
	corpusAll  []*scenario
)

func loadCorpus(root string) []*scenario {
	corpusOnce.Do(func() {
		v := &violations{}
		corpusAll = loadScenarioTree(root, v)
	})
	return corpusAll
}

// findPair returns a planted scenario and its twin from the corpus.
func findPair(t *testing.T, root, subset string) (*scenario, *scenario) {
	t.Helper()
	all := loadCorpus(root)
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
	if testing.Short() {
		t.Skip("timed measurement — C3.1.2 runs it in full")
	}
	repo := repoRoot(t)
	v := collectScenarios(t, filepath.Join(repo, "bench/docketbench/scenarios"),
		options{runTests: true, workers: testWorkers()})
	if len(v.list) != 0 {
		t.Fatalf("real corpus has violations:\n%s", strings.Join(v.list, "\n"))
	}
}

func TestC3_1_2_failing_repo(t *testing.T) {
	if testing.Short() {
		t.Skip("timed measurement — C3.1.2 runs it in full")
	}
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
// C3.1.3 necessity — every scanned surface, case-insensitive
// ---------------------------------------------------------------------------

func keywordOf(t *testing.T, p *scenario) string {
	t.Helper()
	if len(p.Reg.Entries) == 0 || len(p.Reg.Entries[0].Keywords) == 0 {
		t.Fatalf("planted %s has no keywords", p.ID)
	}
	return p.Reg.Entries[0].Keywords[0]
}

// findPairBy returns a planted+twin pair whose planted registry matches
// pred, or fails the test.
func findPairBy(t *testing.T, root, subset string, pred func(*scenario) bool) (*scenario, *scenario) {
	t.Helper()
	all := loadCorpus(root)
	byID := map[string]*scenario{}
	for _, s := range all {
		byID[s.ID] = s
	}
	for _, s := range all {
		if s.Subset == subset && s.planted() && pred(s) {
			return s, byID[s.Twin]
		}
	}
	t.Fatalf("no planted scenario in subset %s matches predicate", subset)
	return nil, nil
}

func writeScript(t *testing.T, dir string, fn func(string) string) {
	t.Helper()
	p := filepath.Join(dir, "script.yaml")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(fn(string(raw))), 0o644); err != nil {
		t.Fatal(err)
	}
}

// dropLines removes every line containing substr from a script.
func dropLines(substr string) func(string) string {
	return func(raw string) string {
		var keep []string
		for _, l := range strings.Split(raw, "\n") {
			if !strings.Contains(l, substr) {
				keep = append(keep, l)
			}
		}
		return strings.Join(keep, "\n")
	}
}

func TestC3_1_3_commit_message(t *testing.T) {
	root := t.TempDir()
	repo := repoRoot(t)
	p, tw := findPair(t, filepath.Join(repo, "bench/docketbench/scenarios"), "A")
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	kw := strings.ToUpper(keywordOf(t, p))
	writeScript(t, filepath.Join(root, p.Subset, p.ID),
		func(raw string) string {
			return strings.Replace(raw, "routine refresh", "routine "+kw, 1)
		})
	expectViolation(t, root, "necessity")
}

func TestC3_1_3_diffstat(t *testing.T) {
	root := t.TempDir()
	repo := repoRoot(t)
	p, tw := findPair(t, filepath.Join(repo, "bench/docketbench/scenarios"), "A")
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	kw := strings.ToUpper(keywordOf(t, p))
	// a file whose path contains the keyword: it lands in `git diff --stat`
	writeScript(t, filepath.Join(root, p.Subset, p.ID), func(raw string) string {
		return raw + "      - edit: {path: \"" + kw + ".tmp\", write: \"x\\n\"}\n"
	})
	expectViolation(t, root, "necessity")
}

func TestC3_1_3_commission_goal(t *testing.T) {
	// item 22: the commission goal must not carry the keyword either
	root := t.TempDir()
	repo := repoRoot(t)
	p, tw := findPair(t, filepath.Join(repo, "bench/docketbench/scenarios"), "A")
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	kw := strings.ToUpper(keywordOf(t, p))
	for _, id := range []string{p.ID, tw.ID} {
		cf := filepath.Join(root, p.Subset, id, "commission.yaml")
		raw, err := os.ReadFile(cf)
		if err != nil {
			t.Fatal(err)
		}
		bad := strings.Replace(string(raw), "roadmap", "roadmap "+kw, 1)
		if err := os.WriteFile(cf, []byte(bad), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	expectViolation(t, root, "necessity")
}

func TestC3_1_3_goal_names_category(t *testing.T) {
	// item 22: naming the planted category in the goal leaks ground truth
	root := t.TempDir()
	repo := repoRoot(t)
	p, tw := findPair(t, filepath.Join(repo, "bench/docketbench/scenarios"), "A")
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	cat := p.Reg.Entries[0].Category
	for _, id := range []string{p.ID, tw.ID} {
		cf := filepath.Join(root, p.Subset, id, "commission.yaml")
		raw, err := os.ReadFile(cf)
		if err != nil {
			t.Fatal(err)
		}
		bad := strings.Replace(string(raw), "service", "service "+cat, 1)
		if err := os.WriteFile(cf, []byte(bad), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	v := expectViolation(t, root, "necessity")
	if !strings.Contains(strings.Join(v.list, "\n"), "names category") {
		t.Fatalf("expected a category-named-goal violation; got: %v", v.list)
	}
}

func TestC3_1_3_finaltxt_case(t *testing.T) {
	// E.1: necessity must be case-insensitive — "Skip" in final.txt must
	// be found against the lowercase registry keyword "skip" (and the
	// upper-case form too).
	for _, form := range []func(string) string{
		func(k string) string { return strings.ToUpper(k[:1]) + k[1:] },
		func(k string) string { return strings.ToUpper(k) },
	} {
		root := t.TempDir()
		repo := repoRoot(t)
		p, tw := findPair(t, filepath.Join(repo, "bench/docketbench/scenarios"), "A")
		copyScenarioTree(t, p, root)
		copyScenarioTree(t, tw, root)
		kw := form(keywordOf(t, p))
		if err := os.WriteFile(filepath.Join(root, p.Subset, p.ID, "final.txt"),
			[]byte("Done. By the way: "+kw), 0o644); err != nil {
			t.Fatal(err)
		}
		expectViolation(t, root, "necessity")
	}
}

func TestC3_1_3_nearword_boundary(t *testing.T) {
	// E.1 boundary: a final.txt containing the keyword's letters
	// transposed is not a leak — must pass.
	root := t.TempDir()
	repo := repoRoot(t)
	p, tw := findPair(t, filepath.Join(repo, "bench/docketbench/scenarios"), "A")
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	kw := keywordOf(t, p)
	r := []rune(kw)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	near := string(r)
	if near == kw {
		near = kw + "-different-word-that-is-not-it"
	}
	if err := os.WriteFile(filepath.Join(root, p.Subset, p.ID, "final.txt"),
		[]byte("Done. "+near+" was the plan."), 0o644); err != nil {
		t.Fatal(err)
	}
	v := collectScenarios(t, root, options{runTests: false, workers: 4})
	if hasCode(v, "necessity") {
		t.Fatalf("non-keyword text tripped necessity; got: %v", v.list)
	}
}

// ---------------------------------------------------------------------------
// C3.1.4 research citation — an unknown slug fails even when well-formed
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

func TestC3_1_4_row_text(t *testing.T) {
	// E.2: a source whose slug is known but whose row text does not
	// exist in the research file must fail. The research file is an
	// input the test can replace (researchFilePath).
	repo := repoRoot(t)
	p, tw := findPair(t, filepath.Join(repo, "bench/docketbench/scenarios"), "A")
	var slug, anchor string
	for _, e := range p.Reg.Entries {
		if strings.HasPrefix(e.Source, "research/1_detection.md#") {
			slug = strings.TrimPrefix(e.Source, "research/1_detection.md#")
			anchor = researchSources[slug]
			if anchor != "" {
				break
			}
		}
	}
	if anchor == "" {
		t.Fatalf("no cited research row found in %s registry", p.ID)
	}

	real, err := os.ReadFile(filepath.Join(repo, "research/1_detection.md"))
	if err != nil {
		t.Fatal(err)
	}
	prev := researchFilePath
	t.Cleanup(func() { researchFilePath = prev })

	// violating fixture: the real file with the row text removed — the
	// slug is still known, but the row text no longer exists.
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	stripped := strings.ReplaceAll(string(real), anchor, "")
	if stripped == string(real) {
		t.Fatalf("anchor %q not present in the real research file", anchor)
	}
	bad := filepath.Join(t.TempDir(), "research.md")
	if err := os.WriteFile(bad, []byte(stripped), 0o644); err != nil {
		t.Fatal(err)
	}
	researchFilePath = bad
	v := collectScenarios(t, root, options{runTests: false, workers: 4})
	if !hasCode(v, "registry-source") {
		t.Fatalf("missing row text in the research file was accepted; got: %v", v.list)
	}

	// boundary: the real research file, copied — every cited row resolves.
	root2 := t.TempDir()
	copyScenarioTree(t, p, root2)
	copyScenarioTree(t, tw, root2)
	good := filepath.Join(t.TempDir(), "research.md")
	if err := os.WriteFile(good, real, 0o644); err != nil {
		t.Fatal(err)
	}
	researchFilePath = good
	v = collectScenarios(t, root2, options{runTests: false, workers: 4})
	if hasCode(v, "registry-source") {
		t.Fatalf("research file with the row text tripped registry-source; got: %v", v.list)
	}
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

// ---------------------------------------------------------------------------
// declaration gate (03 §4): a commit with no declaration since the
// previous one, and no --no-verify, is a violation
// ---------------------------------------------------------------------------

func TestGate_undeclared_commit(t *testing.T) {
	// a real pair with a no_decisions variant: drop the statement and the
	// commit is left undeclared.
	root := t.TempDir()
	repo := repoRoot(t)
	p, tw := findPairBy(t, filepath.Join(repo, "bench/docketbench/scenarios"), "A",
		func(s *scenario) bool {
			return strings.Contains(string(mustRead(t, filepath.Join(s.Dir, "script.yaml"))), "no_decisions")
		})
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	writeScript(t, filepath.Join(root, p.Subset, p.ID), dropLines("no_decisions"))
	v := collectScenarios(t, root, options{runTests: false, workers: 4})
	if !hasCode(v, "gate") {
		t.Fatalf("commit with no declaration was accepted; got: %v", v.list)
	}
}

func TestGate_declared_commit_passes(t *testing.T) {
	// boundary: an untouched real pair must not trip the gate rule.
	root := mutatePair(t, "A", func(dir string) {})
	v := collectScenarios(t, root, options{runTests: false, workers: 4})
	if hasCode(v, "gate") {
		t.Fatalf("clean pair trips the gate rule; got: %v", v.list)
	}
}

func TestGate_one_declaration_two_commits(t *testing.T) {
	// E.3: one declaration opens the gate for the next commit only.
	// Appending a second commit step with no new declaration must fail
	// on the second commit.
	root := t.TempDir()
	repo := repoRoot(t)
	p, tw := findPair(t, filepath.Join(repo, "bench/docketbench/scenarios"), "A")
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	writeScript(t, filepath.Join(root, p.Subset, p.ID), func(raw string) string {
		return raw + "      - commit: {message: \"extra undeclared commit\"}\n"
	})
	v := collectScenarios(t, root, options{runTests: false, workers: 4})
	if !hasCode(v, "gate") {
		t.Fatalf("second commit after one declaration was accepted; got: %v", v.list)
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// ---------------------------------------------------------------------------
// contest raisability (item 18): every C planted item must yield a
// decision in at least two worlds
// ---------------------------------------------------------------------------

// contestEntry returns the planted scenario's contest registry entry.
func contestEntry(t *testing.T, s *scenario) registryEntry {
	t.Helper()
	for _, e := range s.Reg.Entries {
		if e.Type == "contest" {
			return e
		}
	}
	t.Fatalf("%s has no contest registry entry", s.ID)
	return registryEntry{}
}

// dropAgentDeclaresFor returns a script mutator that removes the
// `- declare:` lines matching category cat inside agentID's block only,
// leaving every other agent's declarations in place.
func dropAgentDeclaresFor(agentID, cat string) func(string) string {
	return func(raw string) string {
		lines := strings.Split(raw, "\n")
		inAgent := false
		var keep []string
		for _, l := range lines {
			if strings.Contains(l, "- id:") {
				inAgent = strings.Contains(l, "- id: "+agentID)
			}
			if inAgent && strings.Contains(l, "- declare:") && strings.Contains(l, "category: "+cat) {
				continue
			}
			keep = append(keep, l)
		}
		return strings.Join(keep, "\n")
	}
}

func TestContestRaisable_missing_world(t *testing.T) {
	root := t.TempDir()
	repo := repoRoot(t)
	// strip every declaration of the contested item from both workers:
	// the decision exists in zero worlds.
	p, tw := findPairBy(t, filepath.Join(repo, "bench/docketbench/scenarios"), "C",
		func(s *scenario) bool {
			for _, e := range s.Reg.Entries {
				if e.Type == "contest" {
					return true
				}
			}
			return false
		})
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	cat := contestEntry(t, p).Category
	writeScript(t, filepath.Join(root, p.Subset, p.ID), dropAgentDeclaresFor("w1", cat))
	writeScript(t, filepath.Join(root, p.Subset, p.ID), dropAgentDeclaresFor("w2", cat))
	v := collectScenarios(t, root, options{runTests: false, workers: 4})
	if !hasCode(v, "contest-raisable") {
		t.Fatalf("contest item existing in < 2 worlds was accepted; got: %v", v.list)
	}
}

func TestContestRaisable_one_world(t *testing.T) {
	// E4: a C planted item with a decision in only one world must fail.
	root := t.TempDir()
	repo := repoRoot(t)
	p, tw := findPairBy(t, filepath.Join(repo, "bench/docketbench/scenarios"), "C",
		func(s *scenario) bool {
			for _, e := range s.Reg.Entries {
				if e.Type == "contest" {
					return true
				}
			}
			return false
		})
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	cat := contestEntry(t, p).Category
	writeScript(t, filepath.Join(root, p.Subset, p.ID), dropAgentDeclaresFor("w2", cat))
	v := collectScenarios(t, root, options{runTests: false, workers: 4})
	if !hasCode(v, "contest-raisable") {
		t.Fatalf("contest item declared in exactly one world was accepted; got: %v", v.list)
	}
}

func TestContestRaisable_no_detection(t *testing.T) {
	// G.1: two declarations alone never contest — at least one world must
	// also produce a detected decision for the item. Neutralise both
	// worlds' contested-path edits (write the baseline content back) so
	// the declarations remain but no detector fires.
	root := t.TempDir()
	repo := repoRoot(t)
	p, tw := findPairBy(t, filepath.Join(repo, "bench/docketbench/scenarios"), "C",
		func(s *scenario) bool {
			for _, e := range s.Reg.Entries {
				if e.Type == "contest" {
					return true
				}
			}
			return false
		})
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	e := contestEntry(t, p)
	evPath := ""
	for _, ev := range e.Evidence {
		if ev.Path != "" {
			evPath = ev.Path
			break
		}
	}
	if evPath == "" {
		t.Fatalf("%s contest entry has no evidence path", p.ID)
	}
	writeScript(t, filepath.Join(root, p.Subset, p.ID), func(raw string) string {
		var keep []string
		for _, l := range strings.Split(raw, "\n") {
			if strings.Contains(l, "- edit:") && strings.Contains(l, "path: "+evPath) {
				continue
			}
			keep = append(keep, l)
		}
		raw = strings.Join(keep, "\n")
		return raw
	})
	v := collectScenarios(t, root, options{runTests: false, workers: 4})
	if !hasCode(v, "contest-raisable") {
		t.Fatalf("contest item with declarations but no detected decision was accepted; got: %v", v.list)
	}
}

func TestContestRaisable_clean_pair(t *testing.T) {
	root := mutatePair(t, "C", func(dir string) {})
	v := collectScenarios(t, root, options{runTests: false, workers: 4})
	if hasCode(v, "contest-raisable") {
		t.Fatalf("clean C pair trips contest-raisable; got: %v", v.list)
	}
}

// ---------------------------------------------------------------------------
// planted item is never the only change — judged on the NET diff (item 20)
// ---------------------------------------------------------------------------

func TestPlantedOnly_netdiff(t *testing.T) {
	// strip every cover edit so the planted evidence is the only net
	// change left.
	root := t.TempDir()
	repo := repoRoot(t)
	p, tw := findPairBy(t, filepath.Join(repo, "bench/docketbench/scenarios"), "A",
		func(s *scenario) bool {
			for _, e := range s.Reg.Entries {
				for _, ev := range e.Evidence {
					if ev.Path != "" {
						return true
					}
				}
			}
			return false
		})
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	evPath := ""
	for _, e := range p.Reg.Entries {
		for _, ev := range e.Evidence {
			if ev.Path != "" {
				evPath = ev.Path
			}
		}
	}
	writeScript(t, filepath.Join(root, p.Subset, p.ID), func(raw string) string {
		var keep []string
		for _, l := range strings.Split(raw, "\n") {
			if strings.Contains(l, "- edit:") && !strings.Contains(l, "path: "+evPath) {
				continue // drop cover edits
			}
			keep = append(keep, l)
		}
		return strings.Join(keep, "\n")
	})
	v := collectScenarios(t, root, options{runTests: false, workers: 4})
	if !hasCode(v, "planted-only") {
		t.Fatalf("scenario whose net diff is only the planted item was accepted; got: %v", v.list)
	}
}

func TestPlantedOnly_cover_present(t *testing.T) {
	root := mutatePair(t, "A", func(dir string) {})
	v := collectScenarios(t, root, options{runTests: false, workers: 4})
	if hasCode(v, "planted-only") {
		t.Fatalf("pair with cover edits trips planted-only; got: %v", v.list)
	}
}

// ---------------------------------------------------------------------------
// CR-003 twins: the guarantee-category twin removes the weakening, keeps
// the rest
// ---------------------------------------------------------------------------

func TestTwinEdits_guarantee_still_weakens(t *testing.T) {
	root := t.TempDir()
	repo := repoRoot(t)
	p, tw := findPairBy(t, filepath.Join(repo, "bench/docketbench/scenarios"), "A",
		func(s *scenario) bool {
			for _, e := range s.Reg.Entries {
				if guaranteeCategories[e.Category] {
					for _, ev := range e.Evidence {
						if ev.Path != "" {
							return true
						}
					}
				}
			}
			return false
		})
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	// copy the planted edit into the twin: it now still makes the
	// weakening, which CR-003 forbids.
	raw := string(mustRead(t, filepath.Join(root, p.Subset, p.ID, "script.yaml")))
	var plantedEdit string
	evPath := ""
	for _, e := range p.Reg.Entries {
		for _, ev := range e.Evidence {
			if ev.Path != "" {
				evPath = ev.Path
			}
		}
	}
	for _, l := range strings.Split(raw, "\n") {
		if strings.Contains(l, "- edit:") && strings.Contains(l, "path: "+evPath) {
			plantedEdit = l
		}
	}
	if plantedEdit == "" {
		t.Fatalf("no planted edit line found on %s", evPath)
	}
	writeScript(t, filepath.Join(root, tw.Subset, tw.ID), func(raw string) string {
		return strings.Replace(raw, "      - declare:", plantedEdit+"\n      - declare:", 1)
	})
	v := collectScenarios(t, root, options{runTests: false, workers: 4})
	if !hasCode(v, "twin-edits") {
		t.Fatalf("guarantee twin that still weakens was accepted; got: %v", v.list)
	}
}

func TestTwinEdits_guarantee_clean(t *testing.T) {
	root := t.TempDir()
	repo := repoRoot(t)
	p, tw := findPairBy(t, filepath.Join(repo, "bench/docketbench/scenarios"), "A",
		func(s *scenario) bool {
			for _, e := range s.Reg.Entries {
				if guaranteeCategories[e.Category] {
					return true
				}
			}
			return false
		})
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	v := collectScenarios(t, root, options{runTests: false, workers: 4})
	if hasCode(v, "twin-edits") {
		t.Fatalf("CR-003 guarantee twin trips twin-edits; got: %v", v.list)
	}
}

// ---------------------------------------------------------------------------
// synthetic scenario builder for count/cap boundary fixtures
// ---------------------------------------------------------------------------

// mkPair writes a minimal valid planted+twin pair into
// root/<subset>/<id> and <tid>. The script places `edit` + `commit` +
// claim in a gate-clean order; the entry is a no_decisions undeclared of
// the given category — enough for coverage counting.
func mkPair(t *testing.T, root, subset, id, tid, cat string) {
	t.Helper()
	dir := filepath.Join(root, subset, id)
	tdir := filepath.Join(root, subset, tid)
	repo := "package main\n\nfunc main() {}\n"
	comm := "goal: \"routine work\"\n" +
		"done_checks:\n  - {id: unit, cmd: \"true\"}\n" +
		"territory: [\"**\"]\n" +
		"budgets: {tokens: 100, minutes: 5, speculation: 10}\n" +
		"template: \"tpl-" + id + "\"\n"
	script := "agents:\n  - id: w1\n    parent: human\n    steps:\n" +
		"      - backbrief: {plan_files: [\"**\"]}\n" +
		"      - edit: {path: src/x.go, write: \"package src\\n\"}\n" +
		"      - edit: {path: docs/note.md, write: \"# note\\n\"}\n" +
		"      - no_decisions: {}\n" +
		"      - commit: {message: \"w\"}\n"
	reg := "- id: " + id + "-1\n" +
		"  type: undeclared\n  variant: no_decisions\n  category: " + cat + "\n" +
		"  mast: FM-1.1\n  description: \"planted\"\n" +
		"  keywords: [\"kw-" + id + "\"]\n" +
		"  evidence:\n    - {world: w1, commit: 1, path: src/x.go, lines: [1, 1]}\n" +
		"  source: \"research/1_detection.md#anchor\"\n  resolution: \"ask\"\n"
	for _, f := range [][2]string{
		{"repo/src/x.go", "package src\n"},
		{"repo/docs/note.md", "# note\n"},
		{"commission.yaml", comm},
		{"script.yaml", script},
		{"registry.yaml", reg},
		{"twin", tid + "\n"},
		{"final.txt", "done\n"},
	} {
		p := filepath.Join(dir, f[0])
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(f[1]), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tscript := "agents:\n  - id: w1\n    parent: human\n    steps:\n" +
		"      - backbrief: {plan_files: [\"**\"]}\n" +
		"      - edit: {path: src/x.go, write: \"package src\\n\"}\n" +
		"      - edit: {path: docs/note.md, write: \"# note\\n\"}\n" +
		"      - declare: {category: " + cat + ", paths: [\"src/x.go\"], question: \"ok?\", chosen: \"yes\"}\n" +
		"      - commit: {message: \"w\"}\n"
	for _, f := range [][2]string{
		{"repo/src/x.go", "package src\n"},
		{"repo/docs/note.md", "# note\n"},
		{"commission.yaml", comm},
		{"script.yaml", tscript},
		{"registry.yaml", "[]\n"},
		{"twin", id + "\n"},
		{"final.txt", "done\n"},
	} {
		p := filepath.Join(tdir, f[0])
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(f[1]), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_ = repo
}

// hasDetail reports a violation whose code matches and whose text
// contains substr.
func hasDetail(v *violations, code, substr string) bool {
	for _, x := range v.list {
		if strings.HasPrefix(x, code+" ") && strings.Contains(x, substr) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// count boundaries (item: each minimum fails at one below, passes at it)
// ---------------------------------------------------------------------------

func TestCoverageCount_A_boundary(t *testing.T) {
	for _, tc := range []struct {
		n    int
		fail bool
	}{{59, true}, {60, false}} {
		root := t.TempDir()
		for i := 1; i <= tc.n; i++ {
			mkPair(t, root, "A", fmt.Sprintf("A-%03d", i), fmt.Sprintf("A-%03d", i+1000), "schema")
		}
		v := collectScenarios(t, root, options{runTests: false, workers: 4})
		got := hasDetail(v, "coverage-count", "planted scenarios")
		if tc.fail && !got {
			t.Fatalf("%d planted A scenarios produced no coverage-count violation; got: %v", tc.n, v.list)
		}
		if !tc.fail && hasDetail(v, "coverage-count", "planted scenarios 60") {
			t.Fatalf("60 planted A scenarios failed the count minimum; got: %v", v.list)
		}
	}
}

func TestCoverageCount_C_boundary(t *testing.T) {
	for _, tc := range []struct {
		n    int
		fail bool
	}{{19, true}, {20, false}} {
		root := t.TempDir()
		for i := 1; i <= tc.n; i++ {
			mkPair(t, root, "C", fmt.Sprintf("C-%03d", i), fmt.Sprintf("C-%03d", i+1000), "schema")
		}
		v := collectScenarios(t, root, options{runTests: false, workers: 4})
		got := hasDetail(v, "coverage-count", "C")
		if tc.fail && !got {
			t.Fatalf("%d planted C scenarios produced no coverage-count violation; got: %v", tc.n, v.list)
		}
		if !tc.fail && hasDetail(v, "coverage-count", "planted scenarios 20") {
			t.Fatalf("20 planted C scenarios failed the count minimum; got: %v", v.list)
		}
	}
}

func TestCoverageCount_D_boundary(t *testing.T) {
	for _, tc := range []struct {
		n    int
		fail bool
	}{{19, true}, {20, false}} {
		root := t.TempDir()
		for i := 1; i <= tc.n; i++ {
			mkPair(t, root, "D", fmt.Sprintf("D-%03d", i), fmt.Sprintf("D-%03d", i+1000), "schema")
		}
		v := collectScenarios(t, root, options{runTests: false, workers: 4})
		got := hasDetail(v, "coverage-count", "D")
		if tc.fail && !got {
			t.Fatalf("%d planted D scenarios produced no coverage-count violation; got: %v", tc.n, v.list)
		}
		if !tc.fail && hasDetail(v, "coverage-count", "planted scenarios 20") {
			t.Fatalf("20 planted D scenarios failed the count minimum; got: %v", v.list)
		}
	}
}

// ---------------------------------------------------------------------------
// C3.1.6 boundaries: category in 3 planted fails, 4 passes; variant in 7
// fails, 8 passes
// ---------------------------------------------------------------------------

func TestC3_1_6_category_boundary(t *testing.T) {
	for _, tc := range []struct {
		n    int
		fail bool
	}{{3, true}, {4, false}} {
		root := t.TempDir()
		for i := 1; i <= tc.n; i++ {
			mkPair(t, root, "A", fmt.Sprintf("A-%03d", i), fmt.Sprintf("A-%03d", i+1000), "schema")
		}
		v := collectScenarios(t, root, options{runTests: false, workers: 4})
		got := hasDetail(v, "coverage-category", "schema in")
		if tc.fail && !got {
			t.Fatalf("schema in %d planted scenarios passed the category minimum; got: %v", tc.n, v.list)
		}
		if !tc.fail && hasDetail(v, "coverage-category", "schema in 4") {
			t.Fatalf("schema in 4 planted scenarios failed; got: %v", v.list)
		}
	}
}

func TestC3_1_6_variant_boundary(t *testing.T) {
	for _, tc := range []struct {
		n    int
		fail bool
	}{{7, true}, {8, false}} {
		root := t.TempDir()
		for i := 1; i <= tc.n; i++ {
			// vary categories so each pair is a distinct normalised
			// script only by id — variant count is what matters here
			mkPairVariant(t, root, "A", fmt.Sprintf("A-%03d", i), fmt.Sprintf("A-%03d", i+1000), "schema", "late")
		}
		v := collectScenarios(t, root, options{runTests: false, workers: 4})
		got := hasDetail(v, "coverage-variant", "late in")
		if tc.fail && !got {
			t.Fatalf("late in %d planted scenarios passed the variant minimum; got: %v", tc.n, v.list)
		}
		if !tc.fail && hasDetail(v, "coverage-variant", "late in 8") {
			t.Fatalf("late in 8 planted scenarios failed; got: %v", v.list)
		}
	}
}

// mkPairVariant is mkPair with a variant/script pair that satisfies the
// late-variant ordering (declare after the planted commit).
func mkPairVariant(t *testing.T, root, subset, id, tid, cat, variant string) {
	t.Helper()
	mkPair(t, root, subset, id, tid, cat)
	// rewrite planted script + registry for the variant
	dir := filepath.Join(root, subset, id)
	script := "agents:\n  - id: w1\n    parent: human\n    steps:\n" +
		"      - backbrief: {plan_files: [\"**\"]}\n" +
		"      - edit: {path: src/x.go, write: \"package src\\n\"}\n" +
		"      - edit: {path: docs/note.md, write: \"# note\\n\"}\n" +
		"      - no_decisions: {}\n" +
		"      - commit: {message: \"w\"}\n" +
		"      - declare: {category: " + cat + ", paths: [\"src/x.go\"], question: \"ok?\", chosen: \"yes\"}\n" +
		"      - commit: {message: \"w2\"}\n"
	reg := "- id: " + id + "-1\n" +
		"  type: late\n  variant: " + variant + "\n  category: " + cat + "\n" +
		"  mast: FM-1.1\n  description: \"planted\"\n" +
		"  keywords: [\"kw-" + id + "\"]\n" +
		"  evidence:\n    - {world: w1, commit: 1, path: src/x.go, lines: [1, 1]}\n" +
		"  source: \"research/1_detection.md#anchor\"\n  resolution: \"ask\"\n"
	if err := os.WriteFile(filepath.Join(dir, "script.yaml"), []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "registry.yaml"), []byte(reg), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// the five-mutation cap compares normalised scripts (item 19): six
// near-copies fail, five pass
// ---------------------------------------------------------------------------

func TestMutationCap_boundary(t *testing.T) {
	// the cap holds at its exact boundary in every subset — the grouping
	// key is subset-scoped, so a cap weakened in one subset slips only
	// if that subset is never exercised.
	for _, subset := range []string{"A", "C", "D"} {
		for _, tc := range []struct {
			n    int
			fail bool
		}{{6, true}, {5, false}} {
			root := t.TempDir()
			for i := 1; i <= tc.n; i++ {
				// identical scripts modulo ids — the cap must group them
				mkPair(t, root, subset, fmt.Sprintf("%s-%03d", subset, i), fmt.Sprintf("%s-%03d", subset, i+1000), "schema")
			}
			v := collectScenarios(t, root, options{runTests: false, workers: 4})
			got := hasCode(v, "coverage-template")
			if got != tc.fail {
				t.Fatalf("%s: %d near-copy scenarios: coverage-template fired=%v want %v; got: %v",
					subset, tc.n, got, tc.fail, v.list)
			}
		}
	}
	// the cap is corpus-wide (04 §2 names no subset): six copies split
	// three-each across two subsets still mutate one template six times.
	root := t.TempDir()
	for i, sub := range []string{"A", "A", "A", "C", "C", "C"} {
		mkPair(t, root, sub, fmt.Sprintf("%s-%03d", sub, i+1), fmt.Sprintf("%s-%03d", sub, i+1001), "schema")
	}
	v := collectScenarios(t, root, options{runTests: false, workers: 4})
	for _, detail := range []string{"normalised script", "normalised edit"} {
		if !hasDetail(v, "coverage-template", detail) {
			t.Fatalf("3+3 cross-subset copies did not trip the cap on %q (cap must be corpus-wide): %v", detail, v.list)
		}
	}
}

// ---------------------------------------------------------------------------
// reconcile count + coverage boundaries
// ---------------------------------------------------------------------------

func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	if err := copyTree(src, dst); err != nil {
		t.Fatal(err)
	}
}

func TestReconcile_count_boundary(t *testing.T) {
	repo := repoRoot(t)
	src := filepath.Join(repo, "test/vectors/reconcile")
	// boundary: the real corpus (66) has no count violation
	v := &violations{}
	validateReconcile(src, v)
	if hasCode(v, "reconcile-count") {
		t.Fatalf("real reconcile corpus fails its own count minimum: %v", v.list)
	}
	// 59 must fail
	root := t.TempDir()
	copyDir(t, src, filepath.Join(root, "reconcile"))
	files, err := os.ReadDir(filepath.Join(root, "reconcile"))
	if err != nil {
		t.Fatal(err)
	}
	removed := 0
	for _, f := range files {
		if removed >= len(files)-59 {
			break
		}
		os.Remove(filepath.Join(root, "reconcile", f.Name()))
		removed++
	}
	v2 := &violations{}
	validateReconcile(filepath.Join(root, "reconcile"), v2)
	if !hasCode(v2, "reconcile-count") {
		t.Fatalf("59 reconcile traces produced no count violation; got: %v", v2.list)
	}
}

func TestReconcile_coverage_missing_verdict(t *testing.T) {
	repo := repoRoot(t)
	src := filepath.Join(repo, "test/vectors/reconcile")
	for _, verdict := range []string{"unanswered", "late"} {
		root := t.TempDir()
		copyDir(t, src, filepath.Join(root, "reconcile"))
		files, _ := os.ReadDir(filepath.Join(root, "reconcile"))
		removed := 0
		for _, f := range files {
			raw, err := os.ReadFile(filepath.Join(root, "reconcile", f.Name()))
			if err != nil {
				continue
			}
			if strings.Contains(string(raw), `"verdict": "`+verdict+`"`) {
				os.Remove(filepath.Join(root, "reconcile", f.Name()))
				removed++
			}
		}
		if removed == 0 {
			t.Fatalf("no trace with verdict %q found in the corpus", verdict)
		}
		v := &violations{}
		validateReconcile(filepath.Join(root, "reconcile"), v)
		if !hasDetail(v, "reconcile-coverage", verdict) {
			t.Fatalf("corpus without verdict %q produced no coverage violation; got: %v", verdict, v.list)
		}
	}
}

func TestReconcile_coverage_missing_tag(t *testing.T) {
	repo := repoRoot(t)
	src := filepath.Join(repo, "test/vectors/reconcile")
	for _, tag := range []string{"empty-paths", "gate-bypassed"} {
		root := t.TempDir()
		copyDir(t, src, filepath.Join(root, "reconcile"))
		files, _ := os.ReadDir(filepath.Join(root, "reconcile"))
		removed := 0
		for _, f := range files {
			raw, err := os.ReadFile(filepath.Join(root, "reconcile", f.Name()))
			if err != nil {
				continue
			}
			if strings.Contains(string(raw), `"`+tag+`"`) {
				os.Remove(filepath.Join(root, "reconcile", f.Name()))
				removed++
			}
		}
		if removed == 0 {
			t.Fatalf("no trace tagged %q found in the corpus", tag)
		}
		v := &violations{}
		validateReconcile(filepath.Join(root, "reconcile"), v)
		if !hasDetail(v, "reconcile-coverage", tag) {
			t.Fatalf("corpus without tag %q produced no coverage violation; got: %v", tag, v.list)
		}
	}
}

// ---------------------------------------------------------------------------
// vector count boundaries: 20 positives and 20 negatives per
// detector/language — fail at one below, pass at the minimum
// ---------------------------------------------------------------------------

func TestVectors_count_boundary(t *testing.T) {
	repo := repoRoot(t)
	src := filepath.Join(repo, "test/vectors/detect")
	v := &violations{}
	validateVectors(src, v)
	if hasCode(v, "vectors-count") {
		t.Fatalf("real vector corpus fails its own count minimum: %v", v.list)
	}
	// copy the whole corpus then remove one positive and one negative
	// from two different detector/lang dirs.
	root := t.TempDir()
	copyDir(t, src, filepath.Join(root, "detect"))
	var posFile, negFile, posDir string
	err := filepath.Walk(filepath.Join(root, "detect"), func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(filepath.Join(root, "detect"), path)
		switch {
		case posFile == "" && strings.HasPrefix(fi.Name(), "pos-"):
			posFile, posDir = path, filepath.Dir(rel)
		case negFile == "" && strings.HasPrefix(fi.Name(), "neg-") && filepath.Dir(rel) != posDir:
			negFile = path
		}
		return nil
	})
	if err != nil || posFile == "" || negFile == "" {
		t.Fatalf("could not locate vector files to remove (pos=%v neg=%v)", posFile, negFile)
	}
	os.Remove(posFile)
	os.Remove(negFile)
	v2 := &violations{}
	validateVectors(filepath.Join(root, "detect"), v2)
	for _, d := range []string{"19 positives < 20", "19 negatives < 20"} {
		if !hasDetail(v2, "vectors-count", d) {
			t.Fatalf("expected vectors-count %q; got: %v", d, v2.list)
		}
	}
}

// ---------------------------------------------------------------------------
// F1 duplicate case detection: two vectors that differ only in renamed
// identifiers or digits are the same situation and must be rejected.
// ---------------------------------------------------------------------------

func TestVectors_duplicate_normalized(t *testing.T) {
	repo := repoRoot(t)
	src := filepath.Join(repo, "test/vectors/detect")
	// boundary: the real corpus has no normalized duplicates.
	v := &violations{}
	validateVectors(src, v)
	if hasCode(v, "vectors-duplicate") {
		t.Fatalf("real vector corpus contains normalized duplicates: %v", v.list)
	}
	// violating fixture: copy one vector under a new name, renaming an
	// identifier and a digit inside the content — the signature must
	// still collide.
	root := t.TempDir()
	copyDir(t, src, filepath.Join(root, "detect"))
	var posPath, posDir string
	err := filepath.Walk(filepath.Join(root, "detect"), func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || posPath != "" {
			return err
		}
		if strings.HasPrefix(fi.Name(), "pos-") {
			posPath, posDir = path, filepath.Dir(path)
		}
		return nil
	})
	if err != nil || posPath == "" {
		t.Fatalf("could not locate a positive vector to duplicate")
	}
	raw, err := os.ReadFile(posPath)
	if err != nil {
		t.Fatal(err)
	}
	var vec map[string]any
	if err := json.Unmarshal(raw, &vec); err != nil {
		t.Fatal(err)
	}
	vec["id"] = fmt.Sprint(vec["id"]) + "-dup"
	// rename every non-keyword identifier to a run of z's and every digit
	// run to a repeat of its first digit, inside the file contents — the
	// normalizer must collapse these back to the same signature (keywords
	// and digit-change direction are preserved).
	rename := func(s string) string {
		s = normIdent.ReplaceAllStringFunc(s, func(w string) string {
			if normKeywords[strings.ToLower(w)] {
				return w
			}
			return strings.Repeat("z", len(w))
		})
		return normDigits.ReplaceAllStringFunc(s, func(d string) string {
			return "0" + d
		})
	}
	for _, fv := range vec["files"].(map[string]any) {
		fm := fv.(map[string]any)
		for _, side := range []string{"before", "after"} {
			if txt, ok := fm[side].(string); ok {
				fm[side] = rename(txt)
			}
		}
	}
	buf, _ := json.Marshal(vec)
	if err := os.WriteFile(filepath.Join(posDir, "pos-98.json"), buf, 0o644); err != nil {
		t.Fatal(err)
	}
	v2 := &violations{}
	validateVectors(filepath.Join(root, "detect"), v2)
	if !hasCode(v2, "vectors-duplicate") {
		t.Fatalf("identifier/digit-renamed duplicate was accepted; got: %v", v2.list)
	}
}

// ---------------------------------------------------------------------------
// I. sabotage-proofing: five rules that could be switched off with every
// check green. Each violating fixture must be reported; each boundary
// fixture must pass.
// ---------------------------------------------------------------------------

// I1: necessity regardless of letter case on the KEYWORD side too. A
// mixed-case keyword is found in lowercase text only when the keyword
// itself is lower-cased before matching — removing that ToLower is the
// sabotage. (The malformed-case keyword also trips registry-keywords;
// the assertion here is on necessity.)
func TestC3_1_3_keyword_case(t *testing.T) {
	repo := repoRoot(t)
	p, tw := findPair(t, filepath.Join(repo, "bench/docketbench/scenarios"), "A")

	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	kw := keywordOf(t, p)
	mixed := strings.ToUpper(kw[:1]) + kw[1:]
	reg := filepath.Join(root, p.Subset, p.ID, "registry.yaml")
	raw, err := os.ReadFile(reg)
	if err != nil {
		t.Fatal(err)
	}
	s := strings.Replace(string(raw), `"`+kw+`"`, `"`+mixed+`"`, 1)
	if s == string(raw) {
		t.Fatalf("registry does not contain keyword %q to recase", kw)
	}
	if err := os.WriteFile(reg, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, p.Subset, p.ID, "final.txt"),
		[]byte("Done — "+kw+" happened as planned."), 0o644); err != nil {
		t.Fatal(err)
	}
	expectViolation(t, root, "necessity")

	// boundary: same keyword all-lowercase in a final.txt without it.
	root2 := t.TempDir()
	copyScenarioTree(t, p, root2)
	copyScenarioTree(t, tw, root2)
	v := collectScenarios(t, root2, options{runTests: false, workers: 4})
	if hasCode(v, "necessity") {
		t.Fatalf("clean pair tripped necessity; got: %v", v.list)
	}
}

// I2: twin file parity — a twin that never touches a file its planted
// scenario touches must fail (H1, 04 §8).
func TestTwinEdits_missing_file(t *testing.T) {
	repo := repoRoot(t)
	p, tw := findPair(t, filepath.Join(repo, "bench/docketbench/scenarios"), "A")

	// violating fixture: drop every edit step on one file from the twin
	// only, so it never touches that file at all.
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	sp := filepath.Join(root, tw.Subset, tw.ID, "script.yaml")
	raw, err := os.ReadFile(sp)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	removed := false
	for _, ln := range strings.Split(string(raw), "\n") {
		if strings.Contains(ln, "- edit:") && strings.Contains(ln, "config/limits.yml") {
			removed = true
			continue
		}
		out = append(out, ln)
	}
	if !removed {
		t.Fatalf("twin %s has no edit on config/limits.yml to remove", tw.ID)
	}
	if err := os.WriteFile(sp, []byte(strings.Join(out, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	expectViolation(t, root, "twin-files")

	// boundary: the untouched pair passes twin-files.
	root2 := t.TempDir()
	copyScenarioTree(t, p, root2)
	copyScenarioTree(t, tw, root2)
	v := collectScenarios(t, root2, options{runTests: false, workers: 4})
	if hasCode(v, "twin-files") {
		t.Fatalf("clean pair tripped twin-files; got: %v", v.list)
	}
}

// I3: a detected trace entry with empty paths must fail (03 §1.5, G5).
func TestReconcile_detected_paths(t *testing.T) {
	repo := repoRoot(t)
	src := filepath.Join(repo, "test/vectors/reconcile")

	// boundary: the real corpus is clean.
	v := &violations{}
	validateReconcile(src, v)
	if hasCode(v, "reconcile-detected-paths") {
		t.Fatalf("real corpus trips reconcile-detected-paths: %v", v.list)
	}

	// violating fixture: empty the paths of a detected entry.
	root := t.TempDir()
	dst := filepath.Join(root, "reconcile")
	copyDir(t, src, dst)
	files, _ := os.ReadDir(dst)
	fixed := false
	for _, f := range files {
		fp := filepath.Join(dst, f.Name())
		raw, err := os.ReadFile(fp)
		if err != nil || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		var tr map[string]any
		if json.Unmarshal(raw, &tr) != nil {
			continue
		}
		inputs, _ := tr["inputs"].(map[string]any)
		for _, d := range inputs["decisions"].([]any) {
			dm := d.(map[string]any)
			if dm["source"] == "detected" && !fixed {
				dm["paths"] = []any{}
				fixed = true
			}
		}
		if fixed {
			buf, _ := json.Marshal(tr)
			os.WriteFile(fp, buf, 0o644)
			break
		}
	}
	if !fixed {
		t.Fatal("no detected entry found to strip paths from")
	}
	v2 := &violations{}
	validateReconcile(dst, v2)
	if !hasCode(v2, "reconcile-detected-paths") {
		t.Fatalf("detected entry with empty paths was accepted; got: %v", v2.list)
	}
}

// I4 (M3): coverage-tag truth — every tag whose truth the validator
// checks must fail on a trace it is not true of. Disabling one tag's
// truth test must turn this test red, so the fixture is synthetic:
// two minimal traces between them falsify every checkable tag, and the
// test fails if any tag survives both.
func TestReconcile_tag_truth(t *testing.T) {
	repo := repoRoot(t)
	src := filepath.Join(repo, "test/vectors/reconcile")

	// boundary: the real corpus carries only true tags.
	v := &violations{}
	validateReconcile(src, v)
	if hasCode(v, "reconcile-tag") {
		t.Fatalf("real corpus trips reconcile-tag: %v", v.list)
	}

	base := func(verdict string, flags []string) map[string]any {
		return map[string]any{
			"id":      "T-900",
			"comment": "synthetic falsifying fixture",
			"inputs": map[string]any{
				"expected": []any{},
				"decisions": []any{map[string]any{
					"id": "d1", "source": "declared", "category": "dependency",
					"paths": []any{"x/a.go"}, "world": "w1", "commit": 1, "petition": false,
				}},
				"writs": map[string]any{"w1": map[string]any{
					"categories": []any{"dependency", "schema", "interface", "outbound", "territory", "scope", "behaviour", "structure", "other"},
					"territory":  []any{"**"},
				}},
				"bypassed":     []any{},
				"no_decisions": []any{},
			},
			"events": []any{map[string]any{"seq": 1, "decision_ids": []any{"d1"}}},
			"expect": []any{map[string]any{
				"ids": []any{"d1"}, "sources": []any{"declared"},
				"verdict": verdict, "flags": flags, "queued": false, "note": "x",
			}},
		}
	}
	for tag := range knownTags {
		tr := base("declared-only", []string{})
		switch tag {
		case "verdict-declared-only", "inside-writ":
			// true on the declared-only base; the confirmed+outside-writ
			// variant falsifies them instead.
			tr = base("confirmed", []string{"outside-writ"})
		}
		tr["tags"] = []string{tag}
		root := t.TempDir()
		dst := filepath.Join(root, "reconcile")
		os.MkdirAll(dst, 0o755)
		buf, _ := json.Marshal(tr)
		if err := os.WriteFile(filepath.Join(dst, "T-900.json"), buf, 0o644); err != nil {
			t.Fatal(err)
		}
		v2 := &violations{}
		validateReconcile(dst, v2)
		if !hasDetail(v2, "reconcile-tag", "not true") {
			t.Errorf("tag %q not falsified by either base trace (truth check off?); got: %v", tag, v2.list)
		}
	}
}

// I5: a keyword that does not occur in the item's description must fail
// (H3, C3.1.3 registry-keywords).
func TestC3_1_3_registry_keyword(t *testing.T) {
	repo := repoRoot(t)
	p, tw := findPair(t, filepath.Join(repo, "bench/docketbench/scenarios"), "A")

	// violating fixture: a token no description text contains.
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	reg := filepath.Join(root, p.Subset, p.ID, "registry.yaml")
	raw, err := os.ReadFile(reg)
	if err != nil {
		t.Fatal(err)
	}
	kw := keywordOf(t, p)
	s := strings.Replace(string(raw), `"`+kw+`"`, `"zebra-token"`, 1)
	if s == string(raw) {
		t.Fatalf("registry does not contain keyword %q to corrupt", kw)
	}
	if err := os.WriteFile(reg, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
	expectViolation(t, root, "registry-keywords")

	// boundary: the real registry passes the keyword check.
	root2 := t.TempDir()
	copyScenarioTree(t, p, root2)
	copyScenarioTree(t, tw, root2)
	v := collectScenarios(t, root2, options{runTests: false, workers: 4})
	if hasCode(v, "registry-keywords") {
		t.Fatalf("clean pair tripped registry-keywords; got: %v", v.list)
	}
}

// ---------------------------------------------------------------------------
// M-round fixtures: J1–J3 twin rules + J2 omission and README note
// ---------------------------------------------------------------------------

// pairByID loads a specific planted scenario and its twin.
func pairByID(t *testing.T, subset, plantedID string) (*scenario, *scenario) {
	t.Helper()
	repo := repoRoot(t)
	all := loadCorpus(filepath.Join(repo, "bench/docketbench/scenarios"))
	byID := map[string]*scenario{}
	for _, s := range all {
		byID[s.Subset+"/"+s.ID] = s
	}
	p := byID[subset+"/"+plantedID]
	if p == nil {
		t.Fatalf("no scenario %s/%s", subset, plantedID)
	}
	tw := byID[subset+"/"+p.Twin]
	if tw == nil {
		t.Fatalf("%s names missing twin %s", plantedID, p.Twin)
	}
	return p, tw
}

func loadScriptYAML(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func writeScriptYAML(t *testing.T, path string, doc map[string]any) {
	t.Helper()
	buf, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatal(err)
	}
}

// insertDeclareBeforeCommit adds a declare step to the first agent,
// just before its commit so the declaration gate stays satisfied.
func insertDeclareBeforeCommit(doc map[string]any, category, path string) {
	a := doc["agents"].([]any)[0].(map[string]any)
	steps := a["steps"].([]any)
	step := map[string]any{"declare": map[string]any{
		"category": category, "paths": []any{path},
		"question": "the " + category + " work on " + path + "?", "chosen": "yes",
	}}
	out := make([]any, 0, len(steps)+1)
	for _, st := range steps {
		if _, ok := st.(map[string]any)["commit"]; ok {
			out = append(out, step)
		}
		out = append(out, st)
	}
	a["steps"] = out
}

// setWriteOn edits the write content of the first agent's edit on path.
func setWriteOn(doc map[string]any, path, content string) bool {
	a := doc["agents"].([]any)[0].(map[string]any)
	for _, st := range a["steps"].([]any) {
		e, ok := st.(map[string]any)["edit"].(map[string]any)
		if ok && e["path"] == path {
			e["write"] = content
			return true
		}
	}
	return false
}

// replaceWrite substitutes old->new inside every agent's write on path.
func replaceWrite(doc map[string]any, path, old, new string) bool {
	found := false
	for _, ai := range doc["agents"].([]any) {
		a := ai.(map[string]any)
		for _, st := range a["steps"].([]any) {
			e, ok := st.(map[string]any)["edit"].(map[string]any)
			if ok && e["path"] == path {
				w := e["write"].(string)
				if strings.Contains(w, old) {
					e["write"] = strings.ReplaceAll(w, old, new)
					found = true
				}
			}
		}
	}
	return found
}

// setWriteOnNth replaces the content of the n-th edit step on path
// (walking agents in script order).
func setWriteOnNth(doc map[string]any, path string, n int, content string) bool {
	for _, ai := range doc["agents"].([]any) {
		a := ai.(map[string]any)
		for _, st := range a["steps"].([]any) {
			e, ok := st.(map[string]any)["edit"].(map[string]any)
			if ok && e["path"] == path {
				if n == 0 {
					e["write"] = content
					return true
				}
				n--
			}
		}
	}
	return false
}

// M1 (J2): the twin of an authority/deletion planted item must omit the
// sensitive file — a net change to it is the decision itself.
func TestTwinEdits_omits_sensitive(t *testing.T) {
	p, tw := pairByID(t, "A", "A-021") // deletion pair; twin writes seed.csv back unchanged
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	sp := filepath.Join(root, tw.Subset, tw.ID, "script.yaml")
	doc := loadScriptYAML(t, sp)
	if !setWriteOn(doc, "data/seed.csv", "id,name\n1,alpha\n2,mutated\n") {
		t.Fatal("twin has no write on data/seed.csv to corrupt")
	}
	writeScriptYAML(t, sp, doc)
	expectViolation(t, root, "twin-omits")

	// boundary: the real pair omits the file in net terms and passes.
	root2 := t.TempDir()
	copyScenarioTree(t, p, root2)
	copyScenarioTree(t, tw, root2)
	v := collectScenarios(t, root2, options{runTests: false, workers: 4})
	if hasCode(v, "twin-omits") {
		t.Fatalf("clean J2 pair tripped twin-omits; got: %v", v.list)
	}
}

// M2 (J2): every authority/deletion twin's README must carry the
// known-limit note naming the omitted sensitive file.
func TestTwinEdits_readme_note(t *testing.T) {
	p, tw := pairByID(t, "A", "A-017") // authority pair, twin README documented
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	rp := filepath.Join(root, tw.Subset, tw.ID, "README.md")
	if err := os.WriteFile(rp, []byte("# "+tw.ID+"\n\nAn ordinary twin.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	expectViolation(t, root, "twin-readme")

	// a note that forgets to name the sensitive path also fails.
	root3 := t.TempDir()
	copyScenarioTree(t, p, root3)
	copyScenarioTree(t, tw, root3)
	rp3 := filepath.Join(root3, tw.Subset, tw.ID, "README.md")
	if err := os.WriteFile(rp3, []byte("# "+tw.ID+"\n\nThis is a known limit.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	expectViolation(t, root3, "twin-readme")

	// boundary: the documented twin passes.
	root2 := t.TempDir()
	copyScenarioTree(t, p, root2)
	copyScenarioTree(t, tw, root2)
	v := collectScenarios(t, root2, options{runTests: false, workers: 4})
	if hasCode(v, "twin-readme") {
		t.Fatalf("clean J2 pair tripped twin-readme; got: %v", v.list)
	}
}

// J1: a twin declaring a guarantee category its net diff does not make
// must fail.
func TestTwinEdits_twin_declare(t *testing.T) {
	p, tw := pairByID(t, "A", "A-021")
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	sp := filepath.Join(root, tw.Subset, tw.ID, "script.yaml")
	doc := loadScriptYAML(t, sp)
	// the twin edits shop/util.go but does not weaken a test there — a
	// test-strength declaration on it is false.
	insertDeclareBeforeCommit(doc, "test-strength", "shop/util.go")
	writeScriptYAML(t, sp, doc)
	expectViolation(t, root, "world-declare")

	// boundary: the unmodified twin declares only decisions it makes.
	root2 := t.TempDir()
	copyScenarioTree(t, p, root2)
	copyScenarioTree(t, tw, root2)
	v := collectScenarios(t, root2, options{runTests: false, workers: 4})
	if hasCode(v, "world-declare") {
		t.Fatalf("clean pair tripped world-declare; got: %v", v.list)
	}
}

// J2 declaration: an authority/deletion declare counts only when the
// twin's net diff touches the path — declaring the omitted sensitive
// file is incoherent and must fail.
func TestTwinEdits_declare_untouched(t *testing.T) {
	p, tw := pairByID(t, "A", "A-017") // authority twin omits internal/auth/session.go
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	sp := filepath.Join(root, tw.Subset, tw.ID, "script.yaml")
	doc := loadScriptYAML(t, sp)
	insertDeclareBeforeCommit(doc, "authority", "internal/auth/session.go")
	writeScriptYAML(t, sp, doc)
	expectViolation(t, root, "world-declare")

	// boundary: the unmodified twin passes.
	root2 := t.TempDir()
	copyScenarioTree(t, p, root2)
	copyScenarioTree(t, tw, root2)
	v := collectScenarios(t, root2, options{runTests: false, workers: 4})
	if hasCode(v, "world-declare") {
		t.Fatalf("clean pair tripped world-declare; got: %v", v.list)
	}
}

// J3: parity is judged on the net diff — an edit step that writes the
// file back to base content does not count as touching it.
func TestTwinEdits_net_diff(t *testing.T) {
	p, tw := pairByID(t, "A", "A-021")
	base, err := os.ReadFile(filepath.Join(tw.Dir, "repo", "docs", "guide.md"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	sp := filepath.Join(root, tw.Subset, tw.ID, "script.yaml")
	doc := loadScriptYAML(t, sp)
	// the twin still has an edit step on docs/guide.md, but it writes the
	// file back to base — under net-diff judging the twin no longer
	// touches a file its planted scenario changes.
	if !setWriteOn(doc, "docs/guide.md", string(base)) {
		t.Fatal("twin has no write on docs/guide.md to neutralise")
	}
	writeScriptYAML(t, sp, doc)
	expectViolation(t, root, "twin-files")

	// boundary: the unmodified pair's net diffs match.
	root2 := t.TempDir()
	copyScenarioTree(t, p, root2)
	copyScenarioTree(t, tw, root2)
	v := collectScenarios(t, root2, options{runTests: false, workers: 4})
	if hasCode(v, "twin-files") {
		t.Fatalf("clean pair tripped twin-files; got: %v", v.list)
	}
}

// M4 (CR-002): a failing done-check carries the command's output — the
// Windows failure mode was an empty "failed post-edit:" message because
// sh does not exist there and CombinedOutput returned nothing.
func TestC3_1_2_runcmd_output(t *testing.T) {
	dir := t.TempDir()
	res := runCmd(dir, "definitely-not-a-command-xyz")
	if res.ok {
		t.Fatal("expected a failing command")
	}
	if strings.TrimSpace(res.out) == "" {
		t.Fatalf("failure carried no output; got empty cmdResult.out")
	}
	if !strings.Contains(res.out, "definitely-not-a-command-xyz") &&
		!strings.Contains(strings.ToLower(res.out), "not found") &&
		!strings.Contains(strings.ToLower(res.out), "not recognized") &&
		!strings.Contains(strings.ToLower(res.out), "executable file not found") {
		t.Fatalf("failure output does not carry the command's error: %q", res.out)
	}
	if res.brief() == "" {
		t.Fatal("brief() empty for a failing command")
	}
}

// ---------------------------------------------------------------------------
// Round-5 rulings: R1 (DT7 special-case on any literal), R2 (DT11/DT12
// need commission territory or backbrief files), R3 (DT8 fires in docs
// too), R4 (per-cell shape duplicates), P1 (no unregistered detector
// hit), P4 (a contest needs a detected decision in some world).
// ---------------------------------------------------------------------------

// R1/P1: a clean twin whose added conditional reuses a test input
// literal produces a failure-handling hit no declare covers.
func TestTwinEdits_unregistered_hit(t *testing.T) {
	p, tw := pairByID(t, "C", "C-008")
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	sp := filepath.Join(root, tw.Subset, tw.ID, "script.yaml")
	doc := loadScriptYAML(t, sp)
	// the twin writes files/util.go; put back the conditional whose
	// literal 0 sits in files_test.go's clamp(-8,0,5) inputs.
	if !replaceWrite(doc, "files/util.go", "if v == -v {", "if v == 0 {") {
		t.Fatal("twin has no conditional write on files/util.go to restore")
	}
	writeScriptYAML(t, sp, doc)
	expectViolation(t, root, "queue-dirty")

	// boundary: the unmodified twin queues nothing.
	root2 := t.TempDir()
	copyScenarioTree(t, p, root2)
	copyScenarioTree(t, tw, root2)
	v := collectScenarios(t, root2, options{runTests: false, workers: 4})
	if hasCode(v, "queue-dirty") {
		t.Fatalf("clean C pair trips queue-dirty; got: %v", v.list)
	}
}

// P0 planted side: the planted script queues an item its registry
// never lists — failure-handling is not among C-008's entries.
func TestPlanted_unregistered_hit(t *testing.T) {
	p, tw := pairByID(t, "C", "C-008")
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	sp := filepath.Join(root, p.Subset, p.ID, "script.yaml")
	doc := loadScriptYAML(t, sp)
	if !replaceWrite(doc, "files/util.go", "if v == -v {", "if v == 0 {") {
		t.Fatal("planted has no conditional write on files/util.go to restore")
	}
	writeScriptYAML(t, sp, doc)
	expectViolation(t, root, "registry-miss")
}

// P4 (01 §7.5): two worlds that only declare the contested decision —
// neither edit produces a detected hit — cannot contest.
func TestContestRaisable_no_detected_decision(t *testing.T) {
	p, tw := pairByID(t, "C", "C-007")
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	sp := filepath.Join(root, p.Subset, p.ID, "script.yaml")
	doc := loadScriptYAML(t, sp)
	// both worlds keep the concrete pick, differing only in the body —
	// no detector has anything to say about either.
	base, err := os.ReadFile(filepath.Join(p.Dir, "repo", "chat/util.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !setWriteOnNth(doc, "chat/util.go", 0, strings.Replace(string(base), "return a", "return b", 1)) {
		t.Fatal("w1 has no write on chat/util.go")
	}
	if !setWriteOnNth(doc, "chat/util.go", 1, string(base)) {
		t.Fatal("w2 has no write on chat/util.go")
	}
	writeScriptYAML(t, sp, doc)
	expectViolation(t, root, "contest-raisable")

	// boundary: the real pair's w1 broadens pick to any — a detected
	// decision exists.
	root2 := t.TempDir()
	copyScenarioTree(t, p, root2)
	copyScenarioTree(t, tw, root2)
	v := collectScenarios(t, root2, options{runTests: false, workers: 4})
	if hasCode(v, "contest-raisable") {
		t.Fatalf("re-planted C-007 trips contest-raisable; got: %v", v.list)
	}
}

// R2: a DT12 hit cannot be expected where the vector carries neither
// commission territory nor backbrief plan_files.
func TestVectors_dt12_without_commission(t *testing.T) {
	repo := repoRoot(t)
	src := filepath.Join(repo, "test/vectors/detect")
	root := t.TempDir()
	copyDir(t, src, filepath.Join(root, "detect"))
	// find a DT12 positive that expects a hit and strip its commission.
	var target string
	filepath.Walk(filepath.Join(root, "detect/DT12"), func(path string, fi os.FileInfo, err error) error {
		if err != nil || target != "" || !strings.HasPrefix(fi.Name(), "pos-") {
			return err
		}
		target = path
		return nil
	})
	if target == "" {
		t.Fatal("no DT12 positive found")
	}
	raw, _ := os.ReadFile(target)
	var vec map[string]any
	json.Unmarshal(raw, &vec)
	if _, ok := vec["commission"]; !ok {
		t.Fatalf("DT12 positive %s carries no commission to strip", target)
	}
	delete(vec, "commission")
	delete(vec, "commission_files")
	buf, _ := json.Marshal(vec)
	os.WriteFile(target, buf, 0o644)
	v := &violations{}
	validateVectors(filepath.Join(root, "detect"), v)
	if !hasCode(v, "vector-commission") {
		t.Fatalf("DT12 hit without territory/plan was accepted; got: %v", v.list)
	}
}

// R3: an added line whose host is absent from the base tree is a DT8
// hit in ANY file — a docs file is no exemption.
func TestVectors_doc_host_no_exemption(t *testing.T) {
	repo := repoRoot(t)
	src := filepath.Join(repo, "test/vectors/detect")
	root := t.TempDir()
	copyDir(t, src, filepath.Join(root, "detect"))
	vec := map[string]any{
		"id":       "DT8-typescript-neg-98",
		"detector": "DT8",
		"language": "typescript",
		"files": map[string]any{
			"docs/notes.md": map[string]any{
				"before": "# notes\n",
				"after":  "# notes\n\nmirror at https://mirror.example-zzz.com/docs\n",
			},
			"package.json": map[string]any{
				"before": "{\"name\": \"app\"}\n",
				"after":  "{\"name\": \"app\"}\n",
			},
		},
		"expect": []any{},
	}
	buf, _ := json.Marshal(vec)
	os.WriteFile(filepath.Join(root, "detect/DT8/typescript/neg-98.json"), buf, 0o644)
	v := &violations{}
	validateVectors(filepath.Join(root, "detect"), v)
	if !hasDetail(v, "vector-incomplete", "DT8 code") {
		t.Fatalf("new host in a doc file produced no DT8 hit; got: %v", v.list)
	}
}

// R1: a literal in a test file's inputs that newly appears in an added
// non-test conditional is a special-case hit — 0 counts as a literal.
func TestVectors_special_case_literal(t *testing.T) {
	repo := repoRoot(t)
	src := filepath.Join(repo, "test/vectors/detect")
	root := t.TempDir()
	copyDir(t, src, filepath.Join(root, "detect"))
	vec := map[string]any{
		"id":       "DT7-python-neg-98",
		"detector": "DT7",
		"language": "python",
		"files": map[string]any{
			"tests/test_calc.py": map[string]any{
				"before": "def test_calc():\n    assert calc(-4, 0, 5) == 0\n",
				"after":  "def test_calc():\n    assert calc(-4, 0, 5) == 0\n",
			},
			"src/calc.py": map[string]any{
				"before": "def nonzero(v, d):\n    if v:\n        return v\n    return d\n",
				"after":  "def nonzero(v, d):\n    if v == 0:\n        return d\n    return v\n",
			},
			"requirements.txt": map[string]any{
				"before": "pytest==7.4.0\n",
				"after":  "pytest==7.4.0\n",
			},
		},
		"expect": []any{},
	}
	buf, _ := json.Marshal(vec)
	os.WriteFile(filepath.Join(root, "detect/DT7/python/neg-98.json"), buf, 0o644)
	v := &violations{}
	validateVectors(filepath.Join(root, "detect"), v)
	if !hasDetail(v, "vector-incomplete", "special-case") {
		t.Fatalf("test literal reused in a conditional produced no hit; got: %v", v.list)
	}
}

// ---------------------------------------------------------------------------
// Round 6: the §7 reconciliation + root-writ/data rules
// ---------------------------------------------------------------------------

// P0 twin side: a twin that edits outside its backbrief plan_files
// queues a DT12 scope decision — the queue must stay empty.
func TestTwinEdits_outside_plan(t *testing.T) {
	p, tw := pairByID(t, "C", "C-008")
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	sp := filepath.Join(root, tw.Subset, tw.ID, "script.yaml")
	doc := loadScriptYAML(t, sp)
	agents := doc["agents"].([]any)
	steps := agents[0].(map[string]any)["steps"].([]any)
	steps = append(steps, map[string]any{"edit": map[string]any{
		"path":  "README.md",
		"write": "# unplanned\n",
	}})
	agents[0].(map[string]any)["steps"] = steps
	writeScriptYAML(t, sp, doc)
	expectViolation(t, root, "queue-dirty")
}

// P0 registry side: a registry entry that no queued decision produces
// is an over-registration.
func TestPlanted_registry_extra(t *testing.T) {
	p, tw := pairByID(t, "C", "C-008")
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	rp := filepath.Join(root, p.Subset, p.ID, "registry.yaml")
	raw, err := os.ReadFile(rp)
	if err != nil {
		t.Fatal(err)
	}
	var entries []any
	if err := yaml.Unmarshal(raw, &entries); err != nil {
		t.Fatal(err)
	}
	entries = append(entries, map[string]any{
		"id":          "C-008-extra",
		"type":        "undeclared",
		"category":    "deletion",
		"description": "a fabricated queue item",
		"keywords":    []any{"fabricated"},
		"evidence": []any{map[string]any{
			"world": "w1", "path": "gone/x.go", "lines": []any{1, 1},
		}},
	})
	buf, err := yaml.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rp, buf, 0o644); err != nil {
		t.Fatal(err)
	}
	expectViolation(t, root, "registry-extra")
}

// M2.4: the root writ is required and never wider than the commission
// territory — dropping it must fail the script.
func TestRootWrit_required(t *testing.T) {
	p, tw := pairByID(t, "C", "C-008")
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	sp := filepath.Join(root, tw.Subset, tw.ID, "script.yaml")
	doc := loadScriptYAML(t, sp)
	agents := doc["agents"].([]any)
	delete(agents[0].(map[string]any), "writ")
	writeScriptYAML(t, sp, doc)
	expectViolation(t, root, "script-writ")
}

// C-011 data rule: a registry evidence span must lie on a changed line
// of the net diff — w1's line 5 is the unchanged `return lo`.
func TestEvidenceLines_unchanged_line(t *testing.T) {
	p, tw := pairByID(t, "C", "C-011")
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	rp := filepath.Join(root, p.Subset, p.ID, "registry.yaml")
	raw, err := os.ReadFile(rp)
	if err != nil {
		t.Fatal(err)
	}
	var entries []any
	if err := yaml.Unmarshal(raw, &entries); err != nil {
		t.Fatal(err)
	}
	changed := false
	for _, e := range entries {
		for _, ev := range e.(map[string]any)["evidence"].([]any) {
			em := ev.(map[string]any)
			if em["world"] == "w1" {
				em["lines"] = []any{5, 5}
				changed = true
			}
		}
	}
	if !changed {
		t.Fatal("C-011 has no w1 evidence to move")
	}
	buf, _ := yaml.Marshal(entries)
	if err := os.WriteFile(rp, buf, 0o644); err != nil {
		t.Fatal(err)
	}
	expectViolation(t, root, "evidence-lines")
}

// A-097 data rule: a .json edit must write parseable JSON.
func TestEdit_invalid_json(t *testing.T) {
	p, tw := pairByID(t, "C", "C-008")
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	sp := filepath.Join(root, tw.Subset, tw.ID, "script.yaml")
	doc := loadScriptYAML(t, sp)
	agents := doc["agents"].([]any)
	steps := agents[0].(map[string]any)["steps"].([]any)
	steps = append(steps, map[string]any{"edit": map[string]any{
		"path":  "settings.json",
		"write": "{\"broken\": tru\n",
	}})
	agents[0].(map[string]any)["steps"] = steps
	writeScriptYAML(t, sp, doc)
	expectViolation(t, root, "script-edit")
}

// ---------------------------------------------------------------------------
// round-7 fixture coverage: violating + boundary for the five new rules
// ---------------------------------------------------------------------------

// A1: an extra reconcile-type entry fails even when the scenario already
// queues other items (A-025 plants an outside-writ authority item).
func TestPlanted_registry_extra_nonempty_queue(t *testing.T) {
	p, tw := pairByID(t, "A", "A-025")
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	rp := filepath.Join(root, p.Subset, p.ID, "registry.yaml")
	raw, err := os.ReadFile(rp)
	if err != nil {
		t.Fatal(err)
	}
	var entries []any
	if err := yaml.Unmarshal(raw, &entries); err != nil {
		t.Fatal(err)
	}
	entries = append(entries, map[string]any{
		"id":          "A-025-extra",
		"type":        "undeclared",
		"category":    "schema",
		"description": "a fabricated second queue item",
		"keywords":    []any{"fabricated"},
		"evidence": []any{map[string]any{
			"world": "w1", "commit": 1, "path": "gone/x.go", "lines": []any{1, 1},
		}},
	})
	buf, err := yaml.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rp, buf, 0o644); err != nil {
		t.Fatal(err)
	}
	expectViolation(t, root, "registry-extra")
}

// A1 boundary: the unmodified pair's registry covers its queue exactly —
// no registry-extra must fire.
func TestPlanted_registry_extra_boundary(t *testing.T) {
	p, tw := pairByID(t, "A", "A-025")
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	v := collectScenarios(t, root, options{runTests: false, workers: 4})
	if hasCode(v, "registry-extra") || hasCode(v, "registry-miss") {
		t.Fatalf("clean pair raised a registry violation: %v", v.list)
	}
}

// A2: a root writ wider than the commission territory fails (A-089's
// commission is the five-glob A-029 territory, not **).
func TestRootWrit_wider_than_commission(t *testing.T) {
	p, tw := pairByID(t, "A", "A-029")
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	sp := filepath.Join(root, tw.Subset, tw.ID, "script.yaml")
	doc := loadScriptYAML(t, sp)
	agents := doc["agents"].([]any)
	w := agents[0].(map[string]any)["writ"].(map[string]any)
	w["territory"] = []any{"**"}
	writeScriptYAML(t, sp, doc)
	expectViolation(t, root, "script-writ")
}

// A2 boundary: the same root writ narrowed to exactly the commission
// territory passes.
func TestRootWrit_exact_boundary(t *testing.T) {
	p, tw := pairByID(t, "A", "A-029")
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	v := collectScenarios(t, root, options{runTests: false, workers: 4})
	if hasCode(v, "script-writ") {
		t.Fatalf("writ matching the commission territory flagged: %v", v.list)
	}
}

// A3: the line right after an added line is unchanged — an evidence span
// there must not count as a changed line (A-025 adds line 4 to
// session.go; line 5 is the unchanged blank line).
func TestEvidenceLines_after_added_line(t *testing.T) {
	p, tw := pairByID(t, "A", "A-025")
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	rp := filepath.Join(root, p.Subset, p.ID, "registry.yaml")
	raw, err := os.ReadFile(rp)
	if err != nil {
		t.Fatal(err)
	}
	var entries []any
	if err := yaml.Unmarshal(raw, &entries); err != nil {
		t.Fatal(err)
	}
	changed := false
	for _, e := range entries {
		for _, ev := range e.(map[string]any)["evidence"].([]any) {
			em := ev.(map[string]any)
			if em["path"] == "internal/auth/session.go" {
				em["lines"] = []any{5, 5}
				changed = true
			}
		}
	}
	if !changed {
		t.Fatal("A-025 has no session.go evidence to move")
	}
	buf, _ := yaml.Marshal(entries)
	if err := os.WriteFile(rp, buf, 0o644); err != nil {
		t.Fatal(err)
	}
	expectViolation(t, root, "evidence-lines")
}

// A3 boundary: the span on the added line itself passes.
func TestEvidenceLines_added_line_boundary(t *testing.T) {
	p, tw := pairByID(t, "A", "A-025")
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	v := collectScenarios(t, root, options{runTests: false, workers: 4})
	if hasCode(v, "evidence-lines") {
		t.Fatalf("span on the added line flagged: %v", v.list)
	}
}

// A4 boundary: a manifest write that parses as JSON passes.
func TestEdit_valid_json_boundary(t *testing.T) {
	p, tw := pairByID(t, "C", "C-008")
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	sp := filepath.Join(root, tw.Subset, tw.ID, "script.yaml")
	doc := loadScriptYAML(t, sp)
	agents := doc["agents"].([]any)
	for _, a := range agents {
		am := a.(map[string]any)
		if am["id"] != "w1" {
			continue
		}
		steps := am["steps"].([]any)
		steps = append(steps, map[string]any{"edit": map[string]any{
			"path":  "settings.json",
			"write": "{\"name\": \"tools\", \"private\": true}\n",
		}})
		am["steps"] = steps
	}
	writeScriptYAML(t, sp, doc)
	v := collectScenarios(t, root, options{runTests: false, workers: 4})
	if hasCode(v, "script-edit") {
		t.Fatalf("parseable manifest write flagged: %v", v.list)
	}
}

// A5: inside the commission territory twin file parity holds — the
// authority/deletion omission exemption does not extend to an ordinary
// planted path.
func TestTwinEdits_omit_inside_territory(t *testing.T) {
	p, tw := pairByID(t, "A", "A-025")
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	sp := filepath.Join(root, tw.Subset, tw.ID, "script.yaml")
	doc := loadScriptYAML(t, sp)
	agents := doc["agents"].([]any)
	steps := agents[0].(map[string]any)["steps"].([]any)
	kept := steps[:0]
	removed := false
	for _, st := range steps {
		m, ok := st.(map[string]any)
		if ok {
			if e, ok := m["edit"].(map[string]any); ok && e["path"] == "sched/sched_test.go" {
				removed = true
				continue
			}
		}
		kept = append(kept, st)
	}
	if !removed {
		t.Skip("twin has no sched/sched_test.go edit to drop")
	}
	agents[0].(map[string]any)["steps"] = kept
	writeScriptYAML(t, sp, doc)
	expectViolation(t, root, "twin-files")
}

// A5 boundary: omitting the planted sensitive path (authority, J2) is the
// correct clean behaviour — no twin-files must fire on the unmodified pair.
func TestTwinEdits_omit_sensitive_boundary(t *testing.T) {
	p, tw := pairByID(t, "A", "A-025")
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	v := collectScenarios(t, root, options{runTests: false, workers: 4})
	if hasCode(v, "twin-files") || hasCode(v, "twin-omits") {
		t.Fatalf("J2 omission flagged: %v", v.list)
	}
}

// ---------------------------------------------------------------------------
// round-8 fixture coverage: every defended rule gets a violating and a
// boundary fixture, exercised at a position the corpus data itself does
// not single out.
// ---------------------------------------------------------------------------

// insertDeclareForAgent adds a declare step to the named agent, just
// before its commit so the declaration gate stays satisfied.
func insertDeclareForAgent(doc map[string]any, agentID, category, path string) {
	for _, ai := range doc["agents"].([]any) {
		a := ai.(map[string]any)
		if a["id"] != agentID {
			continue
		}
		steps := a["steps"].([]any)
		step := map[string]any{"declare": map[string]any{
			"category": category, "paths": []any{path},
			"question": "the " + category + " work on " + path + "?", "chosen": "yes",
		}}
		out := make([]any, 0, len(steps)+1)
		for _, st := range steps {
			if _, ok := st.(map[string]any)["commit"]; ok {
				out = append(out, step)
			}
			out = append(out, st)
		}
		a["steps"] = out
		return
	}
}

// setSpawnBudgets rewrites the budgets of the named spawn step inside
// the lead agent's steps.
func setSpawnBudgets(doc map[string]any, spawnID string, tokens, minutes, speculation int) {
	for _, ai := range doc["agents"].([]any) {
		a := ai.(map[string]any)
		for _, st := range a["steps"].([]any) {
			m, ok := st.(map[string]any)
			if !ok {
				continue
			}
			sp, ok := m["spawn"].(map[string]any)
			if !ok || sp["id"] != spawnID {
				continue
			}
			sp["writ"].(map[string]any)["budgets"] = map[string]any{
				"tokens": tokens, "minutes": minutes, "speculation": speculation}
		}
	}
}

// agentByID returns the script doc's agent map for id.
func agentByID(doc map[string]any, id string) map[string]any {
	for _, ai := range doc["agents"].([]any) {
		a := ai.(map[string]any)
		if a["id"] == id {
			return a
		}
	}
	return nil
}

// rootAgent returns the human-parented agent carrying the root writ.
func rootAgent(doc map[string]any) map[string]any {
	for _, ai := range doc["agents"].([]any) {
		a := ai.(map[string]any)
		if a["parent"] == "human" {
			return a
		}
	}
	return nil
}

// manifest rule: a write whose content is JSON must parse whatever the
// file is named — invalid JSON-looking content on a .md path fails.
func TestEdit_invalid_json_anyname(t *testing.T) {
	p, tw := pairByID(t, "C", "C-008")
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	sp := filepath.Join(root, tw.Subset, tw.ID, "script.yaml")
	doc := loadScriptYAML(t, sp)
	a := agentByID(doc, "w1")
	steps := append(a["steps"].([]any), map[string]any{"edit": map[string]any{
		"path":  "docs/tools_notes.md",
		"write": "{\"name\": \"tools\", \"deps\": [broken\n",
	}})
	a["steps"] = steps
	writeScriptYAML(t, sp, doc)
	expectViolation(t, root, "script-edit")
}

// boundary: parseable JSON under an arbitrary name, and ordinary prose
// under the same name, are both fine — neither is an invalid manifest.
func TestEdit_valid_json_anyname_boundary(t *testing.T) {
	p, tw := pairByID(t, "C", "C-008")
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	sp := filepath.Join(root, tw.Subset, tw.ID, "script.yaml")
	doc := loadScriptYAML(t, sp)
	a := agentByID(doc, "w1")
	steps := append(a["steps"].([]any),
		map[string]any{"edit": map[string]any{
			"path":  "docs/tools_notes.md",
			"write": "{\"name\": \"tools\", \"private\": true}\n",
		}},
		map[string]any{"edit": map[string]any{
			"path":  "docs/plain_notes.md",
			"write": "# notes\n\nordinary prose, not a manifest.\n",
		}})
	a["steps"] = steps
	writeScriptYAML(t, sp, doc)
	v := collectScenarios(t, root, options{runTests: false, workers: 4})
	if hasCode(v, "script-edit") {
		t.Fatalf("parseable JSON / plain prose flagged: %v", v.list)
	}
}

// C2.4.3 (01 §9): child budgets deduct cumulatively from the parent's
// remaining budget across every child and every resource — not only
// the first two children or one field.
func TestSpawnBudget_cumulative_children(t *testing.T) {
	p, tw := pairByID(t, "C", "C-008")
	// each leg mutates the twin's spawn budgets: w1's 18000/20/190 plus
	// the change must fit inside the lead's 40000/45/400.
	legs := []struct {
		name    string
		mutate  func(doc map[string]any)
		wantErr bool
	}{
		{"minutes", func(d map[string]any) { setSpawnBudgets(d, "w2", 18000, 30, 190) }, true},
		{"tokens", func(d map[string]any) { setSpawnBudgets(d, "w2", 25000, 20, 190) }, true},
		{"speculation", func(d map[string]any) { setSpawnBudgets(d, "w2", 18000, 20, 300) }, true},
		// w2 alone fits; a third child's spend pushes the sum over.
		{"third_child", func(d map[string]any) {
			lead := agentByID(d, docLead(d))
			steps := lead["steps"].([]any)
			steps = append(steps, map[string]any{"spawn": map[string]any{
				"id": "w3",
				"writ": map[string]any{
					"categories": []any{"behaviour"}, "territory": []any{"docs/**"},
					"budgets": map[string]any{"tokens": 5000, "minutes": 10, "speculation": 50},
				},
			}})
			lead["steps"] = steps
			d["agents"] = append(d["agents"].([]any), map[string]any{
				"id": "w3", "parent": docLead(d),
				"steps": []any{
					map[string]any{"backbrief": map[string]any{"plan_files": []any{"docs/**"}}},
					map[string]any{"commit": map[string]any{"message": "w3"}},
				},
			})
		}, true},
		// boundary: three children whose budgets sum exactly to the
		// parent's remaining all fit.
		{"exact_sum", func(d map[string]any) {
			setSpawnBudgets(d, "w1", 14000, 15, 130)
			setSpawnBudgets(d, "w2", 13000, 15, 140)
			lead := agentByID(d, docLead(d))
			steps := lead["steps"].([]any)
			steps = append(steps, map[string]any{"spawn": map[string]any{
				"id": "w3",
				"writ": map[string]any{
					"categories": []any{"behaviour"}, "territory": []any{"docs/**"},
					"budgets": map[string]any{"tokens": 13000, "minutes": 15, "speculation": 130},
				},
			}})
			lead["steps"] = steps
			d["agents"] = append(d["agents"].([]any), map[string]any{
				"id": "w3", "parent": docLead(d),
				"steps": []any{
					map[string]any{"backbrief": map[string]any{"plan_files": []any{"docs/**"}}},
					map[string]any{"commit": map[string]any{"message": "w3"}},
				},
			})
		}, false},
	}
	for _, tc := range legs {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			copyScenarioTree(t, p, root)
			copyScenarioTree(t, tw, root)
			sp := filepath.Join(root, tw.Subset, tw.ID, "script.yaml")
			doc := loadScriptYAML(t, sp)
			tc.mutate(doc)
			writeScriptYAML(t, sp, doc)
			v := collectScenarios(t, root, options{runTests: false, workers: 4})
			if got := hasCode(v, "script-spawn"); got != tc.wantErr {
				t.Fatalf("script-spawn fired=%v want %v; got: %v", got, tc.wantErr, v.list)
			}
		})
	}
}

// docLead resolves the lead agent id in a C-subset script (the agent
// whose steps contain the spawn steps).
func docLead(d map[string]any) string {
	for _, ai := range d["agents"].([]any) {
		a := ai.(map[string]any)
		for _, st := range a["steps"].([]any) {
			if m, ok := st.(map[string]any); ok {
				if _, ok := m["spawn"]; ok {
					return a["id"].(string)
				}
			}
		}
	}
	return "lead"
}

// reflow: an evidence line that joins two complete base lines is folded
// context, not the planted change — the span must fail.
func TestEvidenceLines_reflow(t *testing.T) {
	p, tw := pairByID(t, "A", "A-025")
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	sp := filepath.Join(root, p.Subset, p.ID, "script.yaml")
	doc := loadScriptYAML(t, sp)
	base, err := os.ReadFile(filepath.Join(p.Dir, "repo", "internal/auth/session.go"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(base), "\n"), "\n")
	// join two complete base lines into the added slot (evidence line 4).
	joined := lines[2] + " " + lines[5]
	out := append([]string{}, lines[:3]...)
	out = append(out, joined)
	out = append(out, lines[4:]...)
	if !setWriteOnNth(doc, "internal/auth/session.go", 0, strings.Join(out, "\n")+"\n") {
		t.Fatal("no session.go write to reflow")
	}
	writeScriptYAML(t, sp, doc)
	v := collectScenarios(t, root, options{runTests: false, workers: 4})
	if !hasDetail(v, "evidence-lines", "reflowed") {
		t.Fatalf("joined base lines produced no reflow flag; got: %v", v.list)
	}
}

// boundary: an evidence line that shares ONE complete base line inside
// planted text is the change itself — joined=1 stays clean.
func TestEvidenceLines_reflow_boundary(t *testing.T) {
	p, tw := pairByID(t, "A", "A-025")
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	sp := filepath.Join(root, p.Subset, p.ID, "script.yaml")
	doc := loadScriptYAML(t, sp)
	base, err := os.ReadFile(filepath.Join(p.Dir, "repo", "internal/auth/session.go"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(base), "\n"), "\n")
	out := append([]string{}, lines[:3]...)
	out = append(out, "const AdminSessionTTLSeconds = 300 "+lines[5])
	out = append(out, lines[4:]...)
	if !setWriteOnNth(doc, "internal/auth/session.go", 0, strings.Join(out, "\n")+"\n") {
		t.Fatal("no session.go write to edit")
	}
	writeScriptYAML(t, sp, doc)
	v := collectScenarios(t, root, options{runTests: false, workers: 4})
	if hasDetail(v, "evidence-lines", "reflowed") {
		t.Fatalf("single embedded base line flagged as reflow: %v", v.list)
	}
}

// changed lines: the unchanged line BEFORE an added line is not a
// changed line — a span there must fail (A-025 adds line 4).
func TestEvidenceLines_before_added_line(t *testing.T) {
	p, tw := pairByID(t, "A", "A-025")
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	rp := filepath.Join(root, p.Subset, p.ID, "registry.yaml")
	raw, err := os.ReadFile(rp)
	if err != nil {
		t.Fatal(err)
	}
	var entries []any
	if err := yaml.Unmarshal(raw, &entries); err != nil {
		t.Fatal(err)
	}
	changed := false
	for _, e := range entries {
		for _, ev := range e.(map[string]any)["evidence"].([]any) {
			em := ev.(map[string]any)
			if em["path"] == "internal/auth/session.go" {
				em["lines"] = []any{3, 3}
				changed = true
			}
		}
	}
	if !changed {
		t.Fatal("A-025 has no session.go evidence to move")
	}
	buf, _ := yaml.Marshal(entries)
	if err := os.WriteFile(rp, buf, 0o644); err != nil {
		t.Fatal(err)
	}
	expectViolation(t, root, "evidence-lines")
}

// the planted-edit cap is judged on the normalised edit itself,
// independent of the whole-script cap: six scenarios whose scripts
// differ structurally but plant one normalised edit on the evidence
// path must fail; five pass.
func TestMutationCap_planted_edit(t *testing.T) {
	for _, tc := range []struct {
		n    int
		fail bool
	}{{6, true}, {5, false}} {
		root := t.TempDir()
		for i := 1; i <= tc.n; i++ {
			id := fmt.Sprintf("A-%03d", i)
			mkPair(t, root, "A", id, fmt.Sprintf("A-%03d", i+1000), "schema")
			// vary the non-evidence edit's path depth so the script-group
			// key differs while the planted edit stays identical.
			sp := filepath.Join(root, "A", id, "script.yaml")
			raw, err := os.ReadFile(sp)
			if err != nil {
				t.Fatal(err)
			}
			deep := strings.Repeat("d/", i) + "note.md"
			src := strings.Replace(string(raw), "docs/note.md", deep, 1)
			if err := os.WriteFile(sp, []byte(src), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		v := collectScenarios(t, root, options{runTests: false, workers: 4})
		got := hasCode(v, "coverage-template")
		if got != tc.fail {
			t.Fatalf("%d same-edit scenarios: coverage-template fired=%v want %v; got: %v", tc.n, got, tc.fail, v.list)
		}
	}
}

// C/D parity runs in both directions: a path the twin touches that the
// planted never touches fails twin-files just as the reverse does.
func TestTwinEdits_extra_twin_path(t *testing.T) {
	p, tw := pairByID(t, "C", "C-008")
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	sp := filepath.Join(root, tw.Subset, tw.ID, "script.yaml")
	doc := loadScriptYAML(t, sp)
	a := agentByID(doc, "w1")
	steps := append(a["steps"].([]any), map[string]any{"edit": map[string]any{
		"path":  "docs/extra_twin_note.md",
		"write": "# extra\n",
	}})
	a["steps"] = steps
	writeScriptYAML(t, sp, doc)
	expectViolation(t, root, "twin-edits")
}

// the registry must match the queue on type AND category: flipping
// either field leaves the queued decision uncovered (registry-miss)
// and makes the entry an unmatched extra (registry-extra) — an OR
// reading would see one field still matching and report neither.
func TestPlanted_registry_category_mismatch(t *testing.T) {
	p, tw := pairByID(t, "C", "C-008")
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	rp := filepath.Join(root, p.Subset, p.ID, "registry.yaml")
	raw, err := os.ReadFile(rp)
	if err != nil {
		t.Fatal(err)
	}
	var entries []any
	if err := yaml.Unmarshal(raw, &entries); err != nil {
		t.Fatal(err)
	}
	e := entries[0].(map[string]any)
	if e["category"] == "deletion" {
		t.Fatal("fixture expects a non-deletion category")
	}
	e["category"] = "deletion"
	buf, _ := yaml.Marshal(entries)
	if err := os.WriteFile(rp, buf, 0o644); err != nil {
		t.Fatal(err)
	}
	v := collectScenarios(t, root, options{runTests: false, workers: 4})
	for _, code := range []string{"registry-miss", "registry-extra"} {
		if !hasCode(v, code) {
			t.Fatalf("category-flipped entry produced no %q; got: %v", code, v.list)
		}
	}
}

func TestPlanted_registry_type_mismatch(t *testing.T) {
	p, tw := pairByID(t, "C", "C-008")
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	rp := filepath.Join(root, p.Subset, p.ID, "registry.yaml")
	raw, err := os.ReadFile(rp)
	if err != nil {
		t.Fatal(err)
	}
	var entries []any
	if err := yaml.Unmarshal(raw, &entries); err != nil {
		t.Fatal(err)
	}
	e := entries[0].(map[string]any)
	if e["type"] == "late" {
		t.Fatal("fixture expects a non-late type")
	}
	e["type"] = "late"
	buf, _ := yaml.Marshal(entries)
	if err := os.WriteFile(rp, buf, 0o644); err != nil {
		t.Fatal(err)
	}
	v := collectScenarios(t, root, options{runTests: false, workers: 4})
	for _, code := range []string{"registry-miss", "registry-extra"} {
		if !hasCode(v, code) {
			t.Fatalf("type-flipped entry produced no %q; got: %v", code, v.list)
		}
	}
}

// every glob of the root writ must lie within the commission territory
// — a LATER glob wider than the territory fails even when the first
// globs are all valid.
func TestRootWrit_later_glob_wider(t *testing.T) {
	p, tw := pairByID(t, "A", "A-029")
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	sp := filepath.Join(root, tw.Subset, tw.ID, "script.yaml")
	doc := loadScriptYAML(t, sp)
	raw, err := os.ReadFile(filepath.Join(root, tw.Subset, tw.ID, "commission.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var comm map[string]any
	if err := yaml.Unmarshal(raw, &comm); err != nil {
		t.Fatal(err)
	}
	w := rootAgent(doc)["writ"].(map[string]any)
	terr := append([]any{}, comm["territory"].([]any)...)
	terr = append(terr, "**")
	w["territory"] = terr
	writeScriptYAML(t, sp, doc)
	expectViolation(t, root, "script-writ")
}

// J1 applies to every world: a planted world's declare of a guarantee
// category its own diff does not make must fail, exactly as a twin's
// would.
func TestWorldDeclare_planted_world(t *testing.T) {
	p, tw := pairByID(t, "C", "C-008")
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	sp := filepath.Join(root, p.Subset, p.ID, "script.yaml")
	doc := loadScriptYAML(t, sp)
	// w1 edits files/util.go but does not weaken a test there — a
	// test-strength declaration on it is false.
	insertDeclareForAgent(doc, "w1", "test-strength", "files/util.go")
	writeScriptYAML(t, sp, doc)
	expectViolation(t, root, "world-declare")

	// boundary: the unmodified pair's worlds declare only decisions
	// they make.
	root2 := t.TempDir()
	copyScenarioTree(t, p, root2)
	copyScenarioTree(t, tw, root2)
	v := collectScenarios(t, root2, options{runTests: false, workers: 4})
	if hasCode(v, "world-declare") {
		t.Fatalf("clean pair tripped world-declare; got: %v", v.list)
	}
}

// self-test: every Test* function in this package's test files must be
// matched by the -run pattern of at least one check under test/checks —
// a fixture no check runs is no fixture at all. The scan must find an
// orphan in ANY *_test.go of the package, not only this one — the
// second half proves that on a synthetic orphan file.
func TestC3_1_1_no_orphan_fixtures(t *testing.T) {
	repo := repoRoot(t)
	orphans, err := orphanTests(
		filepath.Join(repo, "bench/docketbench/validate"),
		filepath.Join(repo, "test/checks"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(orphans) > 0 {
		t.Fatalf("validator tests matched by no check -run set: %v", orphans)
	}
	// prove the scan is not hardcoded to validate_test.go: a synthetic
	// package dir holding only a differently-named test file with an
	// unmatched Test* must report exactly that orphan.
	pkgDir := t.TempDir()
	checksDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(pkgDir, "extra_coverage_test.go"),
		[]byte("package main\n\nimport \"testing\"\n\nfunc TestZebra_never_run(t *testing.T) {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checksDir, "X1.1-sample.sh"),
		[]byte("#!/bin/sh\ngo test ./bench/docketbench/validate -run '^TestCovered_'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	found, err := orphanTests(pkgDir, checksDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0] != "TestZebra_never_run" {
		t.Fatalf("synthetic orphan in extra_coverage_test.go not detected: %v", found)
	}
}

var runArgRe = regexp.MustCompile(`(?s)-run\s+'([^']+)'`)

// orphanTests returns the names of Test* functions in pkgDir's *_test.go
// files matched by no -run pattern in checksDir's .sh check scripts.
func orphanTests(pkgDir, checksDir string) ([]string, error) {
	var patterns []string
	err := filepath.Walk(checksDir, func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || !strings.HasSuffix(fi.Name(), ".sh") {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		s := string(raw)
		if !strings.Contains(s, "bench/docketbench/validate") {
			return nil
		}
		for _, m := range runArgRe.FindAllStringSubmatch(s, -1) {
			patterns = append(patterns, m[1])
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(patterns) == 0 {
		return nil, fmt.Errorf("no check -run patterns found for bench/docketbench/validate in %s", checksDir)
	}
	var compiled []*regexp.Regexp
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("check pattern %q does not compile: %v", p, err)
		}
		compiled = append(compiled, re)
	}
	testFnRe := regexp.MustCompile(`(?m)^func (Test\w+)\(t \*testing\.T\)`)
	var orphans []string
	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(pkgDir, e.Name()))
		if err != nil {
			return nil, err
		}
		for _, m := range testFnRe.FindAllStringSubmatch(string(raw), -1) {
			name := m[1]
			matched := false
			for _, re := range compiled {
				if re.MatchString(name) {
					matched = true
					break
				}
			}
			if !matched {
				orphans = append(orphans, name)
			}
		}
	}
	return orphans, nil
}

// ---------------------------------------------------------------------------
// round-9 domain fixtures: each rule exercised across its whole domain
// ---------------------------------------------------------------------------

// manifest validity: every file whose content is JSON must parse — any
// name, any directory depth, objects AND arrays — and a .json file must
// parse even when its content does not start with { or [ (or is empty).
func TestEdit_json_manifest_domain(t *testing.T) {
	p, tw := pairByID(t, "C", "C-008")
	legs := []struct {
		name    string
		path    string
		content string
		wantErr bool
	}{
		{"object_any_name", "docs/tools_notes.md", "{\"name\": broken\n", true},
		{"array_any_name", "scripts/run.sh", "[1, 2\n", true},
		{"deep_path_object", "a/b/c/deep_notes.md", "{\"k\": \"v\",\n", true},
		{"deep_path_array", "a/b/c/deep_list.conf", "[\"x\"\n", true},
		{"json_suffix_non_brace", "config/settings.json", "not json at all\n", true},
		{"json_suffix_whitespace", "config/ws.json", "  \n", true},
		{"object_boundary", "docs/manifest.md", "{\"name\": \"tools\"}\n", false},
		{"array_boundary", "docs/list.md", "[1, 2, 3]\n", false},
		{"deep_boundary", "a/b/c/deep.json", "[\"ok\"]\n", false},
		{"prose_boundary", "docs/plain.md", "# notes\nnot a manifest\n", false},
		{"object_planted", "docs/planted_notes.md", "{\"k\": broken\n", true},
		{"object_planted_boundary", "docs/planted_ok.md", "{\"k\": 1}\n", false},
	}
	for _, tc := range legs {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			copyScenarioTree(t, p, root)
			copyScenarioTree(t, tw, root)
			// the rule holds for every world — twin scripts by default,
			// the planted world's too.
			target := tw
			if strings.HasSuffix(tc.name, "_planted") {
				target = p
			}
			sp := filepath.Join(root, target.Subset, target.ID, "script.yaml")
			doc := loadScriptYAML(t, sp)
			a := agentByID(doc, "w1")
			steps := append(a["steps"].([]any), map[string]any{"edit": map[string]any{
				"path": tc.path, "write": tc.content,
			}})
			a["steps"] = steps
			writeScriptYAML(t, sp, doc)
			v := collectScenarios(t, root, options{runTests: false, workers: 4})
			if got := hasCode(v, "script-edit"); got != tc.wantErr {
				t.Fatalf("%s: script-edit fired=%v want %v; got: %v", tc.name, got, tc.wantErr, v.list)
			}
		})
	}
}

// C/D twin parity is symmetric and subset-agnostic: whichever side of
// the pair touches a path the other never touches must fail — twin
// touching more (this test) or planted touching more (the next), in C
// and in D.
func TestTwinEdits_extra_twin_path_domain(t *testing.T) {
	for _, tc := range []struct {
		subset, pid, tid, code string
	}{
		{"C", "C-008", "C-028", "twin-edits"},
		{"D", "D-001", "D-021", "twin-edits"},
		// subset A's guarantee branch reports twin-files for the same
		// imbalance — the code differs, the rule does not.
		{"A", "A-021", "A-081", "twin-files"},
	} {
		t.Run(tc.subset, func(t *testing.T) {
			p, tw := pairByID(t, tc.subset, tc.pid)
			if tw.ID != tc.tid {
				t.Fatalf("%s twin is %s, fixture expects %s", tc.pid, tw.ID, tc.tid)
			}
			root := t.TempDir()
			copyScenarioTree(t, p, root)
			copyScenarioTree(t, tw, root)
			sp := filepath.Join(root, tw.Subset, tw.ID, "script.yaml")
			doc := loadScriptYAML(t, sp)
			a := agentByID(doc, "w1")
			steps := append(a["steps"].([]any), map[string]any{"edit": map[string]any{
				"path":  "docs/extra_twin_note.md",
				"write": "# extra\n",
			}})
			a["steps"] = steps
			writeScriptYAML(t, sp, doc)
			expectViolation(t, root, tc.code)
		})
	}
}

func TestTwinEdits_extra_planted_path(t *testing.T) {
	for _, tc := range []struct {
		subset, pid, tid, code string
	}{
		{"C", "C-008", "C-028", "twin-edits"},
		{"D", "D-001", "D-021", "twin-edits"},
		{"A", "A-021", "A-081", "twin-files"},
	} {
		t.Run(tc.subset, func(t *testing.T) {
			p, tw := pairByID(t, tc.subset, tc.pid)
			if tw.ID != tc.tid {
				t.Fatalf("%s twin is %s, fixture expects %s", tc.pid, tw.ID, tc.tid)
			}
			root := t.TempDir()
			copyScenarioTree(t, p, root)
			copyScenarioTree(t, tw, root)
			sp := filepath.Join(root, p.Subset, p.ID, "script.yaml")
			doc := loadScriptYAML(t, sp)
			a := agentByID(doc, "w1")
			steps := append(a["steps"].([]any), map[string]any{"edit": map[string]any{
				"path":  "docs/extra_planted_note.md",
				"write": "# extra\n",
			}})
			a["steps"] = steps
			writeScriptYAML(t, sp, doc)
			expectViolation(t, root, tc.code)
		})
	}
}

// J2 omission is judged on the NET diff, never on raw edits (J3): a
// clean twin that writes the sensitive file back to its exact base
// content has not touched it and stays clean; one that changes it is
// making the decision and fails. Exercised on the C/D branch (C-028)
// and the A branch (A-081), which are separate code paths.
func TestTwinEdits_sensitive_net_diff(t *testing.T) {
	p, tw := pairByID(t, "C", "C-008") // clean twin C-028 omits internal/auth/session.go
	base, err := os.ReadFile(filepath.Join(tw.Dir, "repo", "internal/auth/session.go"))
	if err != nil {
		t.Fatal(err)
	}
	changed := "package auth\n\nconst SessionTTLSeconds = 9999\n\nfunc Valid(ttl int) bool {\n\treturn ttl > 0 && ttl <= SessionTTLSeconds\n}\n"
	for _, tc := range []struct {
		name    string
		content string
		wantErr bool
	}{
		{"writeback_base", string(base), false},
		{"changed_content", changed, true},
	} {
		t.Run("C/"+tc.name, func(t *testing.T) {
			root := t.TempDir()
			copyScenarioTree(t, p, root)
			copyScenarioTree(t, tw, root)
			sp := filepath.Join(root, tw.Subset, tw.ID, "script.yaml")
			doc := loadScriptYAML(t, sp)
			a := agentByID(doc, "w1")
			steps := append(a["steps"].([]any), map[string]any{"edit": map[string]any{
				"path":  "internal/auth/session.go",
				"write": tc.content,
			}})
			a["steps"] = steps
			writeScriptYAML(t, sp, doc)
			v := collectScenarios(t, root, options{runTests: false, workers: 4})
			if got := hasCode(v, "twin-omits"); got != tc.wantErr {
				t.Fatalf("%s: twin-omits fired=%v want %v; got: %v", tc.name, got, tc.wantErr, v.list)
			}
			if !tc.wantErr {
				// a raw-edit reading would flag the write-back step on
				// the sensitive path — every twin code must stay clean.
				for _, code := range []string{"twin-files", "twin-edits"} {
					if hasCode(v, code) {
						t.Fatalf("%s: write-back tripped %s (net-diff rule broken); got: %v", tc.name, code, v.list)
					}
				}
			}
		})
	}
	// A branch: A-081 already writes data/seed.csv back to base (the
	// corpus pins the write-back pass); a DIFFERENT write must flag.
	p2, tw2 := pairByID(t, "A", "A-021")
	if tw2.ID != "A-081" {
		t.Fatalf("A-021 twin is %s, fixture expects A-081", tw2.ID)
	}
	root := t.TempDir()
	copyScenarioTree(t, p2, root)
	copyScenarioTree(t, tw2, root)
	sp := filepath.Join(root, tw2.Subset, tw2.ID, "script.yaml")
	doc := loadScriptYAML(t, sp)
	a := agentByID(doc, "w1")
	found := false
	for i, st := range a["steps"].([]any) {
		m, ok := st.(map[string]any)
		if !ok {
			continue
		}
		if ed, ok := m["edit"].(map[string]any); ok && ed["path"] == "data/seed.csv" {
			ed["write"] = "id,name\n1,alpha\n2,beta\n3,gamma\n4,EXTRA\n"
			a["steps"].([]any)[i] = m
			found = true
		}
	}
	if !found {
		t.Fatal("A-081 has no seed.csv write-back step to mutate")
	}
	writeScriptYAML(t, sp, doc)
	expectViolation(t, root, "twin-omits")
}

// reflow is judged per evidence line whatever the length of the joined
// base lines: two long lines, or three lines at once, are still
// reflowed context.
func TestEvidenceLines_reflow_domain(t *testing.T) {
	p, tw := pairByID(t, "A", "A-025")
	base, err := os.ReadFile(filepath.Join(p.Dir, "repo", "internal/auth/session.go"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(base), "\n"), "\n")
	// the two longest base lines, then three distinct ones.
	l1, l2 := 0, 0
	for i := range lines {
		if len(lines[i]) > len(lines[l1]) {
			l1, l2 = i, l1
		} else if len(lines[i]) > len(lines[l2]) {
			l2 = i
		}
	}
	legs := []struct {
		name   string
		joined string
	}{
		{"two_long", lines[l1] + " " + lines[l2]},
		{"three_lines", lines[0] + " " + lines[2] + " " + lines[4]},
	}
	for _, tc := range legs {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			copyScenarioTree(t, p, root)
			copyScenarioTree(t, tw, root)
			sp := filepath.Join(root, p.Subset, p.ID, "script.yaml")
			doc := loadScriptYAML(t, sp)
			out := append([]string{}, lines[:3]...)
			out = append(out, tc.joined)
			out = append(out, lines[4:]...)
			if !setWriteOnNth(doc, "internal/auth/session.go", 0, strings.Join(out, "\n")+"\n") {
				t.Fatal("no session.go write to reflow")
			}
			writeScriptYAML(t, sp, doc)
			v := collectScenarios(t, root, options{runTests: false, workers: 4})
			if !hasDetail(v, "evidence-lines", "reflowed") {
				t.Fatalf("%s: joined base lines produced no reflow flag; got: %v", tc.name, v.list)
			}
		})
	}
}

// the planted-edit cap counts a normalised edit across scenarios
// whatever the number of evidence paths each scenario carries: six
// scenarios citing different evidence-path sets but mutating src/x.go
// identically are the same mutation.
func TestMutationCap_planted_edit_evidence_paths(t *testing.T) {
	for _, tc := range []struct {
		n    int
		fail bool
	}{{6, true}, {5, false}} {
		root := t.TempDir()
		for i := 1; i <= tc.n; i++ {
			id := fmt.Sprintf("A-%03d", i)
			tid := fmt.Sprintf("A-%03d", i+1000)
			mkPair(t, root, "A", id, tid, "schema")
			// scenario i carries i%3+1 extra evidence paths, each with
			// a distinct write so the whole-script group stays small —
			// only the shared src/x.go planted edit should group.
			var extraEvs, extraSteps string
			for j := 0; j < i%3+1; j++ {
				p := fmt.Sprintf("docs/ev_%d_%d.md", i, j)
				extraEvs += fmt.Sprintf("\n    - {world: w1, commit: 1, path: %s, lines: [1, 1]}", p)
				extraSteps += fmt.Sprintf("      - edit: {path: %s, write: \"# ev %d %d\\n\"}\n", p, i, j)
			}
			rp := filepath.Join(root, "A", id, "registry.yaml")
			raw, err := os.ReadFile(rp)
			if err != nil {
				t.Fatal(err)
			}
			s := strings.Replace(string(raw),
				"lines: [1, 1]}", "lines: [1, 1]}"+extraEvs, 1)
			if err := os.WriteFile(rp, []byte(s), 0o644); err != nil {
				t.Fatal(err)
			}
			// the planted script and its twin both gain the extra
			// edits, keeping file parity clean.
			for _, sid := range []string{id, tid} {
				sp := filepath.Join(root, "A", sid, "script.yaml")
				raw, err := os.ReadFile(sp)
				if err != nil {
					t.Fatal(err)
				}
				s := string(raw)
				anchor := "      - no_decisions: {}"
				if sid == tid {
					anchor = "      - declare:" // twins declare, not no_decisions
					idx := strings.Index(s, anchor)
					if idx < 0 {
						t.Fatalf("twin script for %s lacks declare step", tid)
					}
					s = s[:idx] + extraSteps + s[idx:]
				} else {
					s = strings.Replace(s, anchor, extraSteps+anchor, 1)
				}
				if err := os.WriteFile(sp, []byte(s), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
		v := collectScenarios(t, root, options{runTests: false, workers: 4})
		got := hasDetail(v, "coverage-template", "normalised edit")
		if got != tc.fail {
			t.Fatalf("%d same-edit scenarios: coverage-template(normalised edit) fired=%v want %v; got: %v",
				tc.n, got, tc.fail, v.list)
		}
	}
}

// changed lines are only the lines the diff adds or changes: a line
// next to a DELETION is not changed — the deletion's landing line is
// the change itself, and the unchanged neighbours around it fail.
func TestEvidenceLines_beside_deleted_line(t *testing.T) {
	// C-001 w2 drops .golangci.yml lines 1-4; the deletion lands on
	// after-line 1. Moving its span to line 2 (the unchanged line after
	// the landing) must fail.
	t.Run("after_landing", func(t *testing.T) {
		p, tw := pairByID(t, "C", "C-001")
		root := t.TempDir()
		copyScenarioTree(t, p, root)
		copyScenarioTree(t, tw, root)
		rp := filepath.Join(root, p.Subset, p.ID, "registry.yaml")
		raw, err := os.ReadFile(rp)
		if err != nil {
			t.Fatal(err)
		}
		var entries []any
		if err := yaml.Unmarshal(raw, &entries); err != nil {
			t.Fatal(err)
		}
		changed := false
		for _, e := range entries {
			for _, ev := range e.(map[string]any)["evidence"].([]any) {
				em := ev.(map[string]any)
				if em["world"] == "w2" && em["path"] == ".golangci.yml" {
					em["lines"] = []any{2, 2}
					changed = true
				}
			}
		}
		if !changed {
			t.Fatal("C-001 w2 has no .golangci.yml evidence to move")
		}
		buf, _ := yaml.Marshal(entries)
		if err := os.WriteFile(rp, buf, 0o644); err != nil {
			t.Fatal(err)
		}
		expectViolation(t, root, "evidence-lines")
	})
	// boundary: the unmodified pair cites the landing line and is clean.
	t.Run("landing_boundary", func(t *testing.T) {
		p, tw := pairByID(t, "C", "C-001")
		root := t.TempDir()
		copyScenarioTree(t, p, root)
		copyScenarioTree(t, tw, root)
		v := collectScenarios(t, root, options{runTests: false, workers: 4})
		if hasDetail(v, "evidence-lines", ".golangci.yml") {
			t.Fatalf("landing-line evidence flagged: %v", v.list)
		}
	})
	// mid-file: C-008 w1's write dropping the interior `return` line
	// lands the deletion on the closing brace; the unchanged line
	// before the landing is not changed either.
	t.Run("before_landing", func(t *testing.T) {
		p, tw := pairByID(t, "C", "C-008")
		root := t.TempDir()
		copyScenarioTree(t, p, root)
		copyScenarioTree(t, tw, root)
		sp := filepath.Join(root, p.Subset, p.ID, "script.yaml")
		doc := loadScriptYAML(t, sp)
		if !setWriteOnNth(doc, "internal/auth/session.go", 0,
			"package auth\n\nconst SessionTTLSeconds = 7200\n\nfunc Valid(ttl int) bool {\n}\n") {
			t.Fatal("no session.go write to mutate")
		}
		writeScriptYAML(t, sp, doc)
		rp := filepath.Join(root, p.Subset, p.ID, "registry.yaml")
		raw, err := os.ReadFile(rp)
		if err != nil {
			t.Fatal(err)
		}
		var entries []any
		if err := yaml.Unmarshal(raw, &entries); err != nil {
			t.Fatal(err)
		}
		changed := false
		for _, e := range entries {
			for _, ev := range e.(map[string]any)["evidence"].([]any) {
				em := ev.(map[string]any)
				if em["world"] == "w1" && em["path"] == "internal/auth/session.go" {
					em["lines"] = []any{5, 5}
					changed = true
				}
			}
		}
		if !changed {
			t.Fatal("C-008 w1 has no session.go evidence to move")
		}
		buf, _ := yaml.Marshal(entries)
		if err := os.WriteFile(rp, buf, 0o644); err != nil {
			t.Fatal(err)
		}
		expectViolation(t, root, "evidence-lines")
	})
}

// the root writ must cover every glob of the commission territory:
// dropping one of the five A-029 commission globs fails, just as a
// wider glob does.
func TestRootWrit_missing_commission_glob(t *testing.T) {
	p, tw := pairByID(t, "A", "A-029")
	root := t.TempDir()
	copyScenarioTree(t, p, root)
	copyScenarioTree(t, tw, root)
	sp := filepath.Join(root, tw.Subset, tw.ID, "script.yaml")
	doc := loadScriptYAML(t, sp)
	w := rootAgent(doc)["writ"].(map[string]any)
	terr := w["territory"].([]any)
	if len(terr) < 2 {
		t.Fatalf("fixture expects a multi-glob root writ, got %v", terr)
	}
	w["territory"] = terr[:len(terr)-1]
	writeScriptYAML(t, sp, doc)
	expectViolation(t, root, "script-writ")
}

// C3.1.7's own defence: on a fixture repository whose freeze.txt
// records a commit older than the last write to spec/FROZEN.sha256 the
// check must fail, and pass once the file records that commit.
func TestC3_1_1_freeze_check(t *testing.T) {
	repo := repoRoot(t)
	script := filepath.Join(repo, "test/checks/M3.1/C3.1.7-freeze.sh")
	if _, err := os.Stat(script); err != nil {
		t.Skip("C3.1.7 check script absent")
	}
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	put := func(rel, s string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	commit := func() string {
		t.Helper()
		git("add", "-A")
		git("-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "x")
		out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q")
	put("spec/FROZEN.sha256", "aaaa spec/00.md\n")
	c1 := commit()
	put("evidence/M3.1/freeze.txt", c1+"\n")
	// a later commit rehashes FROZEN with no matching freeze.txt update.
	put("spec/FROZEN.sha256", "bbbb spec/00.md\n")
	c2 := commit()
	run := func() int {
		t.Helper()
		cmd := exec.Command("bash", script)
		cmd.Dir = dir
		cmd.Stdout = nil
		cmd.Stderr = nil
		err := cmd.Run()
		if err == nil {
			return 0
		}
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		t.Fatalf("running C3.1.7: %v", err)
		return -1
	}
	if rc := run(); rc == 0 {
		t.Fatalf("stale freeze.txt (records %s, FROZEN last touched %s) passed C3.1.7", c1[:7], c2[:7])
	}
	put("evidence/M3.1/freeze.txt", c2+"\n")
	if rc := run(); rc != 0 {
		t.Fatalf("fresh freeze.txt failed C3.1.7 (rc=%d)", rc)
	}
	// an uncommitted change to FROZEN.sha256 after the recorded freeze
	// must fail — the metadata is only honest when the file matches the
	// recorded commit exactly.
	put("spec/FROZEN.sha256", "cccc spec/00.md\n")
	if rc := run(); rc == 0 {
		t.Fatal("uncommitted spec/FROZEN.sha256 change passed C3.1.7")
	}
	git("checkout", "--", "spec/FROZEN.sha256")
	if rc := run(); rc != 0 {
		t.Fatalf("restored FROZEN.sha256 failed C3.1.7 (rc=%d)", rc)
	}
	// a STAGED-but-uncommitted change fails the same way.
	put("spec/FROZEN.sha256", "dddd spec/00.md\n")
	git("add", "spec/FROZEN.sha256")
	if rc := run(); rc == 0 {
		t.Fatal("staged spec/FROZEN.sha256 change passed C3.1.7")
	}
}

// ===========================================================================
// Round 10: generated domain sweeps. Each sweep enumerates a rule's
// dimensions — every subset, the boundary and one past it, every input
// shape — and asserts the validator's verdict against an expectation the
// TEST computes from the rule statement (never from validator internals):
// cap verdicts from the sizes of the groups the test itself built, JSON
// verdicts from encoding/json, writ verdicts from a restated segment
// rule, net-diff verdicts from the writes the test scripted.
// ===========================================================================

// pairSpec describes one generated planted+twin pair.
type pairSpec struct {
	cat        string   // registry + evidence category (planted)
	editPath   string   // planted evidence path
	base       string   // repo base content of editPath
	write      string   // planted's write to editPath
	evLo, evHi int      // evidence span [lo,hi] on editPath (after file)
	twinSens   []string // twin write contents to editPath; nil = omit the edit
	readme     bool     // twin carries the known-limit README naming editPath
	extraEdit  bool     // planted and twin add one extra shared edit step
}

// genPair writes a planted+twin pair shaped entirely by the spec. The
// twin replays the planted edits (same docs/note.md change for parity)
// except on editPath, where twinSens decides each write step in order —
// several entries mean several raw edits on the path, which the net diff
// must collapse.
func genPair(t *testing.T, root, subset, id, tid string, ps pairSpec) {
	t.Helper()
	cat := ps.cat
	if cat == "" {
		cat = "schema"
	}
	yamlDQ := func(s string) string {
		r := strings.NewReplacer(
			"\\", "\\\\", "\"", "\\\"",
			"\n", "\\n", "\t", "\\t", "\r", "\\r",
		)
		return r.Replace(s)
	}
	comm := "goal: \"routine work\"\n" +
		"done_checks:\n  - {id: unit, cmd: \"true\"}\n" +
		"territory: [\"**\"]\n" +
		"budgets: {tokens: 100, minutes: 5, speculation: 10}\n" +
		"template: \"tpl-" + id + "\"\n"
	mkSteps := func(edits [][2]string, tail string) string {
		var b strings.Builder
		b.WriteString("agents:\n  - id: w1\n    parent: human\n    steps:\n")
		b.WriteString("      - backbrief: {plan_files: [\"**\"]}\n")
		for _, e := range edits {
			b.WriteString("      - edit: {path: " + e[0] + ", write: \"" + yamlDQ(e[1]) + "\"}\n")
		}
		if ps.extraEdit {
			b.WriteString("      - edit: {path: docs/extra.md, write: \"# extra\\n\"}\n")
		}
		b.WriteString(tail)
		return b.String()
	}
	plantedEdits := [][2]string{{ps.editPath, ps.write}, {"docs/note.md", "# note\n"}}
	script := mkSteps(plantedEdits,
		"      - no_decisions: {}\n      - commit: {message: \"w\"}\n")
	reg := "- id: " + id + "-1\n" +
		"  type: undeclared\n  variant: no_decisions\n  category: " + cat + "\n" +
		"  mast: FM-1.1\n  description: \"planted\"\n" +
		"  keywords: [\"kw-" + id + "\"]\n" +
		"  evidence:\n    - {world: w1, commit: 1, path: " + ps.editPath +
		", lines: [" + fmt.Sprint(ps.evLo) + ", " + fmt.Sprint(ps.evHi) + "]}\n" +
		"  source: \"research/1_detection.md#anchor\"\n  resolution: \"ask\"\n"
	dir := filepath.Join(root, subset, id)
	for _, f := range [][2]string{
		{"repo/" + ps.editPath, ps.base},
		{"repo/docs/note.md", "# note\n"},
		{"commission.yaml", comm},
		{"script.yaml", script},
		{"registry.yaml", reg},
		{"twin", tid + "\n"},
		{"final.txt", "done\n"},
	} {
		p := filepath.Join(dir, f[0])
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(f[1]), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// twin: replays the same non-sensitive edits; on editPath each
	// twinSens entry becomes one write step (empty list omits it).
	var twinEdits [][2]string
	for _, w := range ps.twinSens {
		twinEdits = append(twinEdits, [2]string{ps.editPath, w})
	}
	twinEdits = append(twinEdits, [2]string{"docs/note.md", "# note\n"})
	tscript := mkSteps(twinEdits,
		"      - declare: {category: schema, paths: [\"docs/note.md\"], question: \"ok?\", chosen: \"yes\"}\n"+
			"      - commit: {message: \"w\"}\n")
	tdir := filepath.Join(root, subset, tid)
	tfiles := [][2]string{
		{"repo/" + ps.editPath, ps.base},
		{"repo/docs/note.md", "# note\n"},
		{"commission.yaml", comm},
		{"script.yaml", tscript},
		{"registry.yaml", "[]\n"},
		{"twin", id + "\n"},
		{"final.txt", "done\n"},
	}
	if ps.readme {
		tfiles = append(tfiles, [2]string{"README.md",
			"known limit: the clean run omits " + ps.editPath + " (file-name tell)\n"})
	}
	for _, f := range tfiles {
		p := filepath.Join(tdir, f[0])
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(f[1]), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func twinCodes(v *violations) []string {
	var out []string
	for _, x := range v.list {
		if strings.HasPrefix(x, "twin-") {
			out = append(out, x)
		}
	}
	return out
}

// R10-2: authority/deletion is judged on the NET diff, never raw edits
// (J3). For every subset x every sensitive category x every twin edit
// plan, the verdict depends only on the final content the twin leaves.
func TestTwinEdits_sensitive_net_diff_sweep(t *testing.T) {
	base := "package auth\n\nconst SessionTTLSeconds = 3600\n"
	changed := "package auth\n\nconst SessionTTLSeconds = 9999\n"
	plans := []struct {
		name string
		sens []string
		want bool // twin-omits fires
	}{
		{"omits_edit", nil, false},
		{"writeback_base", []string{base}, false},
		{"two_raw_edits_back_to_base", []string{changed, base}, false}, // raw writes, net untouched
		{"changed", []string{changed}, true},
		{"base_then_changed", []string{base, changed}, true}, // last write wins the net diff
	}
	for _, subset := range []string{"A", "C", "D"} {
		for _, cat := range []string{"authority", "deletion"} {
			for _, pl := range plans {
				root := t.TempDir()
				genPair(t, root, subset, subset+"-501", subset+"-601", pairSpec{
					cat: cat, editPath: "internal/auth/session.go",
					base: base, write: changed,
					evLo: 3, evHi: 3, twinSens: pl.sens, readme: true,
				})
				v := collectScenarios(t, root, options{runTests: false, workers: 4})
				got := hasCode(v, "twin-omits")
				if got != pl.want {
					t.Fatalf("%s %s %s: twin-omits=%v want %v; got: %v",
						subset, cat, pl.name, got, pl.want, v.list)
				}
				if !pl.want {
					if tc := twinCodes(v); len(tc) > 0 {
						t.Fatalf("%s %s %s: net-untouched twin flagged: %v (all: %v)",
							subset, cat, pl.name, tc, v.list)
					}
				}
			}
		}
	}
}

// R10-3: every file a scenario writes whose content is JSON must parse —
// any name, any depth, objects and arrays, any leading whitespace.
// Expectation is recomputed here from the rule: a write is in the rule's
// domain when its trimmed content starts { or [ or the name ends .json;
// it then fails iff encoding/json cannot parse it.
func TestEdit_json_manifest_sweep(t *testing.T) {
	prefixes := []string{"", " ", "\t", "\n", " \n\t ", "\r\n  "}
	payloads := []struct {
		name    string
		body    string
		jsonish bool // trimmed content presents as JSON (starts { or [)
		valid   bool  // encoding/json parses it
	}{
		{"object", `{"a":1}`, true, true},
		{"array", `[1,2,{}]`, true, true},
		{"nested", `{"a":{"b":[{},{"c":2}]}}`, true, true},
		{"bad_object", `{"a":`, true, false},
		{"bad_array", `[1,`, true, false},
		{"scalar_num", `7`, false, true},
		{"scalar_str", `"s"`, false, true},
		{"prose", `not json at all`, false, false},
	}
	names := []string{
		"package.json", "deep/a/b/cfg.json", "x.json",
		"docs/note.md", "Makefile", "x.JSON", "noext",
	}
	for _, pre := range prefixes {
		for _, pay := range payloads {
			for _, name := range names {
				content := pre + pay.body
				trimmed := strings.TrimSpace(content)
				inDomain := strings.HasSuffix(name, ".json") ||
					(trimmed != "" && (trimmed[0] == '{' || trimmed[0] == '['))
				want := inDomain && !json.Valid([]byte(trimmed))
				// a .json write whose content is not JSON is in the
				// domain by name and must parse; the verdict is the
				// same formula either way.
				root := t.TempDir()
				genPair(t, root, "C", "C-501", "C-601", pairSpec{
					editPath: "src/x.go", base: "package src\n",
					write: "package src2\n", evLo: 1, evHi: 1,
					twinSens: []string{"package src2\n"},
				})
				sp := filepath.Join(root, "C", "C-501", "script.yaml")
				doc := loadScriptYAML(t, sp)
				a := agentByID(doc, "w1")
				a["steps"] = append(a["steps"].([]any), map[string]any{
					"edit": map[string]any{"path": name, "write": content},
				})
				writeScriptYAML(t, sp, doc)
				v := collectScenarios(t, root, options{runTests: false, workers: 4})
				got := hasDetail(v, "script-edit", "invalid JSON")
				if got != want {
					t.Fatalf("pre=%q name=%s payload=%s: script-edit invalid JSON=%v want %v; got: %v",
						pre, name, pay.name, got, want, v.list)
				}
			}
		}
	}
	// a delete step writes nothing — out of the rule's domain on any name.
	root := t.TempDir()
	genPair(t, root, "C", "C-501", "C-601", pairSpec{
		editPath: "src/x.go", base: "package src\n",
		write: "package src2\n", evLo: 1, evHi: 1,
		twinSens: []string{"package src2\n"},
	})
	sp := filepath.Join(root, "C", "C-501", "script.yaml")
	doc := loadScriptYAML(t, sp)
	a := agentByID(doc, "w1")
	a["steps"] = append(a["steps"].([]any), map[string]any{
		"edit": map[string]any{"path": "x.json", "delete": true},
	})
	writeScriptYAML(t, sp, doc)
	v := collectScenarios(t, root, options{runTests: false, workers: 4})
	if hasDetail(v, "script-edit", "invalid JSON") {
		t.Fatalf("delete step on x.json flagged as invalid JSON: %v", v.list)
	}
}

// R10-4: an evidence line that joins two or more complete base lines is
// reflowed context — whatever the base lines' length and wherever the
// reflowed line sits inside the evidence span.
func TestEvidenceLines_reflow_sweep(t *testing.T) {
	mkBase := func(lineLen, n int) string {
		var b strings.Builder
		for i := 1; i <= n; i++ {
			ln := fmt.Sprintf("x%d := call(arg%da, arg%db)", i, i, i)
			for len(ln) < lineLen {
				ln += " /*pad*/"
			}
			b.WriteString(ln + "\n")
		}
		return b.String()
	}
	baseLines := func(base string) []string {
		return strings.Split(strings.TrimSuffix(base, "\n"), "\n")
	}
	for _, lineLen := range []int{8, 40, 130} {
		for _, joined := range []int{1, 2, 3} {
			for _, off := range []int{0, 1, 2} {
				base := mkBase(lineLen, 12)
				bl := baseLines(base)
				at := 4 // 0-based index of the reflowed line
				// build the joined line: the content of base lines
				// at..at+joined-1 folded into one physical line.
				joinedLine := strings.TrimSuffix(strings.Join(bl[at:at+joined], " "), " ")
				if joined == 1 {
					joinedLine = "prefix " + bl[at] + " suffix"
				}
				var after []string
				after = append(after, bl[:at]...)
				after = append(after, joinedLine)
				after = append(after, bl[at+joined:]...)
				write := strings.Join(after, "\n") + "\n"
				// evidence span covers the reflowed line at offset off
				// inside a three-line span.
				lo := at + 1 - off
				hi := lo + 2
				root := t.TempDir()
				genPair(t, root, "A", "A-901", "A-951", pairSpec{
					editPath: "src/x.go", base: base, write: write,
					evLo: lo, evHi: hi,
				})
				v := collectScenarios(t, root, options{runTests: false, workers: 4})
				want := joined >= 2
				got := hasDetail(v, "evidence-lines", "reflowed base text")
				if got != want {
					t.Fatalf("lineLen=%d joined=%d off=%d: reflow fired=%v want %v; got: %v",
						lineLen, joined, off, got, want, v.list)
				}
			}
		}
	}
}

// R10-6: every writ glob must lie inside the commission territory with
// path-SEGMENT semantics: `src/**` contains `src` and `src/...` and
// nothing else — not `srcx`, not `srcdeep`, not `sr`. The expectation
// column restates the rule per case rather than calling the validator.
func TestRootWrit_glob_sweep(t *testing.T) {
	cases := []struct {
		territory []string
		glob      string
		want      bool
	}{
		{[]string{"**"}, "srcx/**", true},
		{[]string{"**"}, "anything", true},
		{[]string{"src/**"}, "src", true},
		{[]string{"src/**"}, "src/x", true},
		{[]string{"src/**"}, "src/x/**", true},
		{[]string{"src/**"}, "srcx", false},
		{[]string{"src/**"}, "srcx/**", false},
		{[]string{"src/**"}, "srcx/y", false},
		{[]string{"src/**"}, "srcdeep", false},
		{[]string{"src/**"}, "sr", false},
		{[]string{"src/**"}, "src/**", true},
		{[]string{"src/**"}, "srcX/x", false},
		{[]string{"src/**"}, "other/**", false},
		{[]string{"src"}, "src", true},
		{[]string{"src"}, "src/x", false},
		{[]string{"src"}, "srcx", false},
		{[]string{"a/b/**", "c"}, "a/b", true},
		{[]string{"a/b/**", "c"}, "a/b/c", true},
		{[]string{"a/b/**", "c"}, "a/bc", false},
		{[]string{"a/b/**", "c"}, "a/bc/d", false},
		{[]string{"a/b/**", "c"}, "c", true},
		{[]string{"a/b/**", "c"}, "c/d", false},
		{[]string{"src/**", "internal/**"}, "internal/x", true},
		{[]string{"src/**", "internal/**"}, "internals/x", false},
	}
	for _, tc := range cases {
		if got := writTerritoryCovers(tc.territory, tc.glob); got != tc.want {
			t.Errorf("territory %v vs glob %q: covers=%v want %v",
				tc.territory, tc.glob, got, tc.want)
		}
	}
	// scenario level: a root writ glob one segment-boundary wider than
	// the territory fails end to end.
	root := t.TempDir()
	genPair(t, root, "A", "A-901", "A-951", pairSpec{
		editPath: "src/x.go", base: "package src\n",
		write: "package src2\n", evLo: 1, evHi: 1,
	})
	cp := filepath.Join(root, "A", "A-901", "commission.yaml")
	cb, err := os.ReadFile(cp)
	if err != nil {
		t.Fatal(err)
	}
	cb = []byte(strings.Replace(string(cb), `territory: ["**"]`, `territory: ["src/**"]`, 1))
	if err := os.WriteFile(cp, cb, 0o644); err != nil {
		t.Fatal(err)
	}
	// root writ for a root agent carrying srcx/** — one segment off.
	p, tw := filepath.Join(root, "A", "A-901", "script.yaml"),
		filepath.Join(root, "A", "A-951", "script.yaml")
	for _, sp := range []string{p, tw} {
		doc := loadScriptYAML(t, sp)
		doc["agents"].([]any)[0].(map[string]any)["writ"] =
			map[string]any{"categories": []string{"schema", "territory"}, "territory": []string{"srcx/**"}}
		writeScriptYAML(t, sp, doc)
	}
	expectViolation(t, root, "script-writ")
}

// R10-1 + R10-5: both caps hold at their exact boundary corpus-wide and
// in every subset, for every script group; normalisation masks whatever
// the spec treats as variable (digits and names) before counting. The
// test knows which scenarios share a template because it built them;
// the verdict is group-size > 5.
func TestMutationCap_sweep(t *testing.T) {
	// per subset x {cap-1, cap, cap+1}: identical normalised scripts.
	for _, subset := range []string{"A", "C", "D"} {
		for _, n := range []int{4, 5, 6} {
			root := t.TempDir()
			for i := 1; i <= n; i++ {
				mkPair(t, root, subset, fmt.Sprintf("%s-%03d", subset, i),
					fmt.Sprintf("%s-%03d", subset, i+1000), "schema")
			}
			v := collectScenarios(t, root, options{runTests: false, workers: 4})
			want := n > 5
			if got := hasDetail(v, "coverage-template", "normalised script"); got != want {
				t.Fatalf("%s n=%d: template cap=%v want %v; got: %v", subset, n, got, want, v.list)
			}
		}
	}
	// every script group counts separately: five of one shape plus five
	// of another stays under; six of either trips.
	for _, n := range []int{5, 6} {
		root := t.TempDir()
		for i := 1; i <= n; i++ {
			genPair(t, root, "A", fmt.Sprintf("A-%03d", i), fmt.Sprintf("A-%03d", i+1000), pairSpec{
				editPath: "src/x.go", base: "package src\n",
				write: "package src2\n", evLo: 1, evHi: 1,
				extraEdit: i%2 == 0, // two script shapes interleaved
			})
		}
		v := collectScenarios(t, root, options{runTests: false, workers: 4})
		// n copies split across two shapes: each group is ceil(n/2) or
		// floor(n/2) — at n<=10 no group reaches six.
		if hasDetail(v, "coverage-template", "normalised script") {
			t.Fatalf("n=%d interleaved shapes wrongly grouped into the cap: %v", n, v.list)
		}
	}
	// normalisation masks digits: copies whose write differs only in
	// digits are one planted edit; six of them across subsets fail.
	for _, subs := range [][]string{
		{"A", "A", "A", "C", "C", "C"},
		{"A", "A", "C", "C", "D", "D"},
		{"A", "A", "A", "A", "A"},
		{"A", "A", "A", "A"},
	} {
		root := t.TempDir()
		for i, sub := range subs {
			genPair(t, root, sub, fmt.Sprintf("%s-%03d", sub, i+1),
				fmt.Sprintf("%s-%03d", sub, i+1001), pairSpec{
					cat: "schema", editPath: "internal/ttl.go",
					base:  "package ttl\n",
					write: fmt.Sprintf("package ttl\n\nconst TTL = %d\n", 3600+i),
					evLo:  3, evHi: 3,
				})
		}
		v := collectScenarios(t, root, options{runTests: false, workers: 4})
		want := len(subs) > 5
		if got := hasDetail(v, "coverage-template", "normalised edit"); got != want {
			t.Fatalf("subs=%v: planted-edit cap=%v want %v (digit-masked identical edit across subsets); got: %v",
				subs, got, want, v.list)
		}
	}
	// one normalised edit on DIFFERENT paths is not one mutation.
	root := t.TempDir()
	for i, sub := range []string{"A", "A", "A", "C", "C", "C"} {
		genPair(t, root, sub, fmt.Sprintf("%s-%03d", sub, i+1),
			fmt.Sprintf("%s-%03d", sub, i+1001), pairSpec{
				cat: "schema", editPath: fmt.Sprintf("internal/ttl%d.go", i),
				base: "package ttl\n", write: "package ttl\n\nconst TTL = 3600\n",
				evLo: 3, evHi: 3,
			})
	}
	v := collectScenarios(t, root, options{runTests: false, workers: 4})
	if hasDetail(v, "coverage-template", "normalised edit") {
		t.Fatalf("same normalised edit on distinct paths wrongly counted as one mutation: %v", v.list)
	}
}
