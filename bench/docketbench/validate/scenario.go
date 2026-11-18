package main

// Scenario validation (spec/04-benchmark.md §2–§5, M3.1 checks C3.1.1–4,6).
//
// Each rule lives in its own function and every rule runs on every
// scenario — the three sabotages for M3.1 each disable exactly one rule
// call below and must therefore be caught by the check fixtures.

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
)

var subsets = map[string]bool{"A": true, "C": true, "D": true}

var scenarioIDRe = regexp.MustCompile(`^([ACD])-(\d{3,})$`)
var mastRe = regexp.MustCompile(`^FM-\d+\.\d+$`)
var kwRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]*$`)

// subsetTypes and subsetVariants bound the registry vocabulary per subset.
var subsetTypes = map[string]map[string]bool{
	"A": {"undeclared": true, "late": true, "unanswered": true, "outside-writ": true},
	"C": {"contest": true},
	"D": {"false-done": true},
}
var aVariants = map[string]bool{
	"no_decisions": true, "gate-bypass": true, "late": true,
	"outside-writ": true, "unanswered": true,
}
var dReasons = map[string]bool{
	"check-fails": true, "never-run": true, "stale-run": true,
}

var subsetMin = map[string][2]int{"A": {60, 60}, "C": {20, 20}, "D": {20, 20}}

// required file set under a scenario directory.
var scenarioFiles = []string{"commission.yaml", "script.yaml", "registry.yaml", "twin", "final.txt"}

type scenario struct {
	Subset    string
	ID        string
	Dir       string
	Comm      *commissionFile
	Script    *scriptFile
	Reg       *registryFile
	Twin      string
	Final     string
	repoFiles map[string][]byte
	agents    map[string]*scriptAgent
}

func (s *scenario) planted() bool { return len(s.Reg.Entries) > 0 }

// validateScenarios runs every rule over the whole tree.
func validateScenarios(root string, opts options, v *violations) {
	all := loadScenarioTree(root, v)
	if all == nil {
		return
	}
	for _, s := range all {
		checkScenarioBasics(s, v)
		checkRegistry(s, v)
		checkScript(s, v)
	}
	checkTwins(all, v)
	checkPlantedNotOnlyChange(all, v)
	checkEvidence(all, v)
	checkContestRaisable(all, v)
	checkNecessity(all, v)
	checkRepoTests(all, opts, v)
	checkCoverage(all, v)
}

// ---------------------------------------------------------------------------
// loading and structural checks
// ---------------------------------------------------------------------------

func loadScenarioTree(root string, v *violations) []*scenario {
	var out []*scenario
	subs, err := os.ReadDir(root)
	if err != nil {
		v.add("scenarios-missing", root, err.Error())
		return nil
	}
	for _, sub := range subs {
		if !sub.IsDir() {
			continue
		}
		if !subsets[sub.Name()] {
			v.add("subset-unknown", sub.Name(), "subset must be A, C or D")
			continue
		}
		sdir := filepath.Join(root, sub.Name())
		dirs, err := os.ReadDir(sdir)
		if err != nil {
			v.add("scenarios-missing", sdir, err.Error())
			continue
		}
		for _, d := range dirs {
			if !d.IsDir() {
				continue
			}
			m := scenarioIDRe.FindStringSubmatch(d.Name())
			if m == nil || m[1] != sub.Name() {
				v.add("scenario-id", sub.Name()+"/"+d.Name(), "id must be <subset>-<digits>")
				continue
			}
			out = append(out, loadScenario(sub.Name(), d.Name(), filepath.Join(sdir, d.Name()), v))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Dir < out[j].Dir })
	return out
}

func loadScenario(subset, id, dir string, v *violations) *scenario {
	s := &scenario{Subset: subset, ID: id, Dir: dir, agents: map[string]*scriptAgent{}}
	where := subset + "/" + id
	// required files
	for _, f := range scenarioFiles {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			v.addf("scenario-file", where, "missing %s", f)
		}
	}
	if fi, err := os.Stat(filepath.Join(dir, "repo")); err != nil || !fi.IsDir() {
		v.add("scenario-file", where, "missing repo/ directory")
		return s
	}
	var err error
	if s.repoFiles, err = treeFiles(filepath.Join(dir, "repo")); err != nil {
		v.add("repo-read", where, err.Error())
	}
	if len(s.repoFiles) == 0 {
		v.add("repo-empty", where, "repo/ has no files")
	}
	for p := range s.repoFiles {
		if strings.HasPrefix(p, "registry") || strings.Contains(p, ".git/") {
			v.addf("registry-leak", where, "%s must not live inside repo/ (04 §2.4)", p)
		}
	}
	s.Comm = &commissionFile{}
	if err := loadYAML(filepath.Join(dir, "commission.yaml"), s.Comm); err != nil {
		v.add("commission-parse", where, err.Error())
	}
	s.Script = &scriptFile{}
	if err := loadYAML(filepath.Join(dir, "script.yaml"), s.Script); err != nil {
		v.add("script-parse", where, err.Error())
	}
	s.Reg = &registryFile{}
	if err := loadYAML(filepath.Join(dir, "registry.yaml"), s.Reg); err != nil {
		v.add("registry-parse", where, err.Error())
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "twin")); err == nil {
		s.Twin = strings.TrimSpace(string(raw))
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "final.txt")); err == nil {
		s.Final = string(raw)
	}
	for _, a := range s.Script.Agents {
		a := a
		s.agents[a.ID] = &a
	}
	return s
}

// checkScenarioBasics covers the structural invariants of one scenario:
// commission fields, final message, writ sanity.
func checkScenarioBasics(s *scenario, v *violations) {
	where := s.Subset + "/" + s.ID
	c := s.Comm
	if c == nil {
		return
	}
	if strings.TrimSpace(c.Goal) == "" {
		v.add("commission-goal", where, "goal is empty")
	}
	if len(c.DoneChecks) == 0 {
		v.add("commission-checks", where, "needs at least one done-check")
	}
	seenCheck := map[string]bool{}
	for _, dc := range c.DoneChecks {
		if dc.ID == "" || strings.TrimSpace(dc.Cmd) == "" {
			v.add("commission-checks", where, "done-check needs id and cmd")
		}
		if seenCheck[dc.ID] {
			v.addf("commission-checks", where, "duplicate done-check id %q", dc.ID)
		}
		seenCheck[dc.ID] = true
	}
	if len(c.Territory) == 0 {
		v.add("commission-territory", where, "territory is empty")
	}
	if c.Budgets.Tokens <= 0 || c.Budgets.Minutes <= 0 || c.Budgets.Speculation <= 0 {
		v.add("commission-budgets", where, "budgets must be positive")
	}
	if c.Template == "" {
		v.add("commission-template", where, "template id is required for the five-mutation cap")
	}
	if strings.TrimSpace(s.Final) == "" {
		v.add("final-missing", where, "final.txt missing or empty")
	}
	if len(s.Final) > 2000 {
		v.add("final-size", where, "final.txt over 2000 chars")
	}
}

// ---------------------------------------------------------------------------
// registry checks
// ---------------------------------------------------------------------------

func checkRegistry(s *scenario, v *violations) {
	where := s.Subset + "/" + s.ID
	if s.Reg == nil {
		return
	}
	// C3.1 sabotage target: a planted scenario with an empty registry is a
	// twin by definition; only clean twins may have no entries.
	if !s.planted() {
		checkCleanTwinShape(s, v)
		return
	}
	seen := map[string]bool{}
	for _, e := range s.Reg.Entries {
		if seen[e.ID] {
			v.addf("registry-id", where, "duplicate entry id %q", e.ID)
		}
		seen[e.ID] = true
		if !strings.HasPrefix(e.ID, s.ID+"-") {
			v.addf("registry-id", where, "entry %q must start with the scenario id", e.ID)
		}
		if !subsetTypes[s.Subset][e.Type] {
			v.addf("registry-type", where, "entry %s: type %q not allowed in subset %s", e.ID, e.Type, s.Subset)
		}
		if s.Subset == "A" && !aVariants[e.Variant] {
			v.addf("registry-variant", where, "entry %s: variant %q unknown", e.ID, e.Variant)
		}
		if !categories[e.Category] {
			v.addf("registry-category", where, "entry %s: category %q unknown", e.ID, e.Category)
		}
		if !mastRe.MatchString(e.Mast) {
			v.addf("registry-mast", where, "entry %s: mast %q is not FM-x.y", e.ID, e.Mast)
		}
		if strings.TrimSpace(e.Description) == "" {
			v.addf("registry-description", where, "entry %s: description empty", e.ID)
		}
		if len(e.Keywords) == 0 {
			v.addf("registry-keywords", where, "entry %s: fixed keyword list required (C3.1.3)", e.ID)
		}
		for _, k := range e.Keywords {
			if !kwRe.MatchString(k) {
				v.addf("registry-keywords", where, "entry %s: keyword %q malformed", e.ID, k)
			}
			// C3.1.3: keywords are the words a reader would use for the
			// item — they must literally appear in the entry's
			// description, not be invented tokens.
			if !strings.Contains(strings.ToLower(e.Description), strings.ToLower(k)) {
				v.addf("registry-keywords", where,
					"entry %s: keyword %q does not appear in the description", e.ID, k)
			}
		}
		if strings.TrimSpace(e.Resolution) == "" {
			v.addf("registry-resolution", where, "entry %s: resolution empty", e.ID)
		}
		checkSourceCitation(s, e, v)
		checkEntryEvidence(s, e, v)
	}
}

// researchFilePath is where the research citation file lives, relative to
// the repository root; tests replace it to prove the row text is searched.
var researchFilePath = "research/1_detection.md"

// checkSourceCitation enforces C3.1.4: each entry cites a row of
// research/1_detection.md, written research/1_detection.md#<slug> with the
// slug's anchor text present in the research file.
func checkSourceCitation(s *scenario, e registryEntry, v *violations) {
	where := s.Subset + "/" + s.ID
	const prefix = "research/1_detection.md#"
	if !strings.HasPrefix(e.Source, prefix) {
		v.addf("registry-source", where, "entry %s: source %q must cite %s<slug>", e.ID, e.Source, prefix)
		return
	}
	slug := strings.TrimPrefix(e.Source, prefix)
	anchor, ok := researchSources[slug]
	if !ok {
		v.addf("registry-source", where, "entry %s: unknown research row %q", e.ID, slug)
		return
	}
	raw, err := os.ReadFile(researchFilePath)
	if err != nil {
		v.add("registry-source", where, "cannot read "+researchFilePath)
		return
	}
	if !strings.Contains(string(raw), anchor) {
		v.addf("registry-source", where, "entry %s: row %q text not found in research/1_detection.md", e.ID, slug)
	}
}

// checkCleanTwinShape: a scenario with an empty registry must be a clean
// twin — no entries and a valid back-pointer, verified in checkTwins.
func checkCleanTwinShape(s *scenario, v *violations) {
	if len(s.Reg.Entries) != 0 {
		v.add("registry-shape", s.Subset+"/"+s.ID, "clean twin must have no registry entries")
	}
}

// ---------------------------------------------------------------------------
// script checks
// ---------------------------------------------------------------------------

func checkScript(s *scenario, v *violations) {
	where := s.Subset + "/" + s.ID
	sc := s.Script
	if sc == nil {
		return
	}
	if len(sc.Agents) == 0 {
		v.add("script-agents", where, "no agents")
		return
	}
	ids := map[string]bool{}
	for _, a := range sc.Agents {
		if ids[a.ID] {
			v.addf("script-agents", where, "duplicate agent id %q", a.ID)
		}
		ids[a.ID] = true
	}
	for _, a := range sc.Agents {
		awhere := fmt.Sprintf("%s agent %s", where, a.ID)
		if a.Parent != "" && a.Parent != "human" && !ids[a.Parent] {
			v.addf("script-parent", awhere, "parent %q is not an agent in this script", a.Parent)
		}
		if a.Writ != nil {
			checkWrit(a.Writ, awhere, v)
		}
		// the declaration gate (03 §4): every commit must be preceded
		// by a record_decision or no_decisions since the previous
		// commit in that world; a tool commit is exempt only when it
		// passes --no-verify.
		declared := false
		for i, st := range a.Steps {
			switch st.Kind {
			case "edit":
				checkEditStep(st.Edit, awhere, v)
			case "declare":
				checkDeclareStep(st.Declare, awhere, v)
				declared = true
			case "no_decisions":
				declared = true
			case "spawn":
				checkSpawnStep(s, &a, st.Spawn, awhere, v)
			case "claim_done":
				checkClaimStep(s, st.ClaimDone, awhere, v)
			case "backbrief":
				checkBackbriefStep(st.Backbrief, awhere, v)
			case "commit":
				if st.Commit == nil || strings.TrimSpace(st.Commit.Message) == "" {
					v.add("script-commit", awhere, "commit step needs a message")
				}
				if !declared {
					v.addf("gate", awhere,
						"commit step %d has no record_decision or no_decisions since the previous commit", i)
				}
				declared = false
			case "tool":
				if st.Tool == nil {
					v.add("script-tool", awhere, "empty tool step")
				}
				if isCommitCmd(st.Tool) {
					if !declared && !strings.Contains(st.Tool.command(), "--no-verify") {
						v.addf("gate", awhere,
							"commit step %d has no record_decision or no_decisions since the previous commit", i)
					}
					declared = false
				}
			}
		}
	}
	// every spawned id is defined as an agent with the right parent
	for _, a := range sc.Agents {
		for _, st := range a.Steps {
			if st.Kind != "spawn" || st.Spawn == nil {
				continue
			}
			ch, ok := s.agents[st.Spawn.ID]
			if !ok {
				v.addf("script-spawn", where, "agent %s spawns %q with no matching agents entry", a.ID, st.Spawn.ID)
				continue
			}
			if ch.Parent != a.ID {
				v.addf("script-spawn", where, "agent %q has parent %q, expected %q", ch.ID, ch.Parent, a.ID)
			}
		}
	}
}

func checkWrit(w *writSpec, where string, v *violations) {
	if w == nil {
		return
	}
	for _, c := range w.Categories {
		if !categories[c] {
			v.addf("writ-categories", where, "category %q unknown", c)
		}
		if guaranteeCategories[c] {
			v.addf("writ-categories", where, "guarantee category %q can be in no writ (01 §9)", c)
		}
	}
	if len(w.Territory) == 0 {
		v.add("writ-territory", where, "writ territory empty")
	}
}

func checkSpawnStep(s *scenario, a *scriptAgent, sp *spawnStep, where string, v *violations) {
	if sp == nil || sp.ID == "" {
		v.add("script-spawn", where, "spawn step needs an id")
		return
	}
	if sp.Writ == nil {
		v.addf("script-spawn", where, "spawn %s needs a writ", sp.ID)
		return
	}
	checkWrit(sp.Writ, where+" spawn "+sp.ID, v)
	pw := agentWrit(s, a)
	if pw == nil {
		v.addf("script-spawn", where, "agent %s has no writ to attenuate", a.ID)
		return
	}
	// attenuation (spec/01 §9)
	for _, c := range sp.Writ.Categories {
		if !contains(pw.Categories, c) {
			v.addf("script-spawn", where, "spawn %s: category %q outside parent writ", sp.ID, c)
		}
	}
	for _, t := range sp.Writ.Territory {
		if !writTerritoryCovers(pw.Territory, t) {
			v.addf("script-spawn", where, "spawn %s: territory %q outside parent territory", sp.ID, t)
		}
	}
	if sp.Writ.Budgets.Tokens > pw.Budgets.Tokens ||
		sp.Writ.Budgets.Minutes > pw.Budgets.Minutes ||
		sp.Writ.Budgets.Speculation > pw.Budgets.Speculation {
		v.addf("script-spawn", where, "spawn %s: budgets exceed parent's", sp.ID)
	}
}

// agentWrit resolves the writ an agent works under: its own writ field,
// else the writ issued by its parent's spawn step, else the implicit human
// root writ (every non-guarantee category, the whole tree).
func agentWrit(s *scenario, a *scriptAgent) *writSpec {
	if a.Writ != nil {
		return a.Writ
	}
	if a.Parent != "" && a.Parent != "human" {
		if p, ok := s.agents[a.Parent]; ok {
			for _, st := range p.Steps {
				if st.Kind == "spawn" && st.Spawn != nil && st.Spawn.ID == a.ID {
					return st.Spawn.Writ
				}
			}
		}
	}
	var cats []string
	for c := range categories {
		if !guaranteeCategories[c] {
			cats = append(cats, c)
		}
	}
	sort.Strings(cats)
	return &writSpec{Categories: cats, Territory: []string{"**"},
		Budgets: budgets{Tokens: 1 << 30, Minutes: 1 << 20, Speculation: 1 << 20}}
}

func checkEditStep(e *editStep, where string, v *violations) {
	if e == nil {
		return
	}
	if e.Path == "" {
		v.add("script-edit", where, "edit step needs a path")
		return
	}
	if filepath.IsAbs(filepath.FromSlash(e.Path)) || strings.Contains(e.Path, "..") ||
		strings.HasPrefix(e.Path, "/") || strings.HasPrefix(e.Path, "~") {
		v.addf("script-edit", where, "edit path %q must stay inside the repo", e.Path)
	}
	if e.Patch != "" && e.Write != "" {
		v.addf("script-edit", where, "edit on %s sets both patch and write", e.Path)
	}
}

func checkDeclareStep(d *declareStep, where string, v *violations) {
	if d == nil {
		return
	}
	if !categories[d.Category] {
		v.addf("script-declare", where, "declare category %q unknown", d.Category)
	}
	if d.Question == "" || len(d.Question) > 200 || !strings.HasSuffix(d.Question, "?") {
		v.addf("script-declare", where, "question must be ≤200 chars ending in '?'")
	}
	if d.Chosen == "" {
		v.add("script-declare", where, "chosen option required")
	}
}

func checkClaimStep(s *scenario, c *claimStep, where string, v *violations) {
	if c == nil || len(c.Checks) == 0 {
		v.add("script-claim", where, "claim_done needs check ids")
		return
	}
	ids := map[string]bool{}
	for _, dc := range s.Comm.DoneChecks {
		ids[dc.ID] = true
	}
	for _, id := range c.Checks {
		if !ids[id] {
			v.addf("script-claim", where, "claim_done names unknown done-check %q", id)
		}
	}
}

func checkBackbriefStep(b *backbriefStep, where string, v *violations) {
	if b == nil {
		return
	}
	for _, e := range b.Expected {
		if !categories[e.Category] {
			v.addf("script-backbrief", where, "expected category %q unknown", e.Category)
		}
		if e.Question == "" || !strings.HasSuffix(e.Question, "?") {
			v.add("script-backbrief", where, "expected decision needs a question ending in '?'")
		}
	}
}

// ---------------------------------------------------------------------------
// path/glob helpers (spec/01 §7.1 vocabulary)
// ---------------------------------------------------------------------------

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// pathsOverlap: any equal, directory-prefix, or glob-intersecting pair.
func pathsOverlap(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y || dirContains(x, y) || dirContains(y, x) || globsIntersect(x, y) {
				return true
			}
		}
	}
	return false
}

func dirContains(d, p string) bool {
	return strings.HasPrefix(p, strings.TrimSuffix(d, "/")+"/")
}

// globsIntersect: conservative glob intersection — true whenever the two
// patterns could name a common path. `**` intersects everything; equal
// literals intersect; a glob `x/**` intersects `x/...`; prefix-free
// literal segments must agree.
func globsIntersect(a, b string) bool {
	if a == "**" || b == "**" {
		return true
	}
	// fast paths
	if a == b {
		return true
	}
	as, bs := strings.Split(a, "/"), strings.Split(b, "/")
	for i := 0; i < len(as) && i < len(bs); i++ {
		x, y := as[i], bs[i]
		if x == "**" || y == "**" {
			return true
		}
		if x == "*" || y == "*" {
			continue
		}
		if x != y {
			return false
		}
	}
	// all shared segments agree: intersect when one side ended or continues
	return true
}

// writTerritoryCovers reports whether every glob in child is contained in
// the parent territory list.
func writTerritoryCovers(parent []string, child string) bool {
	for _, p := range parent {
		if p == "**" || p == child {
			return true
		}
		// parent dir-glob contains paths under it
		if strings.HasSuffix(p, "/**") {
			base := strings.TrimSuffix(p, "/**")
			if child == base || strings.HasPrefix(child, base+"/") {
				return true
			}
		}
	}
	return false
}

// entryMatchesDeclare: registry entry vs declare step — same category and
// overlapping paths (spec/01 §7.1).
func entryMatchesDeclare(cat string, paths []string, d *declareStep) bool {
	if d == nil || d.Category != cat {
		return false
	}
	return pathsOverlap(paths, d.Paths) || len(paths) == 0 || len(d.Paths) == 0
}

// ---------------------------------------------------------------------------
// twins (spec/04 §2 rule 1, C3.1.1)
// ---------------------------------------------------------------------------

func checkTwins(all []*scenario, v *violations) {
	byID := map[string]*scenario{}
	for _, s := range all {
		byID[s.Subset+"/"+s.ID] = s
	}
	for _, s := range all {
		where := s.Subset + "/" + s.ID
		if s.Twin == "" {
			v.add("twin-missing", where, "no twin file or empty twin id")
			continue
		}
		t, ok := byID[s.Subset+"/"+s.Twin]
		if !ok {
			v.addf("twin-missing", where, "twin %q does not exist in subset %s", s.Twin, s.Subset)
			continue
		}
		if t.Twin != s.ID {
			v.addf("twin-backref", where, "twin %q names %q back, not this scenario", t.ID, t.Twin)
		}
		// exactly one of the pair is planted
		if s.planted() == t.planted() {
			v.addf("twin-parity", where, "scenario and twin %q are both %s", t.ID,
				map[bool]string{true: "planted", false: "clean"}[s.planted()])
		}
		// same repository and same commission
		tf, err := treeFiles(filepath.Join(t.Dir, "repo"))
		if err == nil && s.repoFiles != nil && !treesEqual(s.repoFiles, tf) {
			v.addf("twin-repo", where, "repo/ differs from twin %q", t.ID)
		}
		ca, _ := os.ReadFile(filepath.Join(s.Dir, "commission.yaml"))
		cb, _ := os.ReadFile(filepath.Join(t.Dir, "commission.yaml"))
		if !bytes.Equal(ca, cb) {
			v.addf("twin-commission", where, "commission.yaml differs from twin %q", t.ID)
		}
		// same template (counts once toward the five-mutation cap)
		if s.Comm != nil && t.Comm != nil && s.Comm.Template != t.Comm.Template {
			v.addf("twin-template", where, "twin %q has a different template", t.ID)
		}
		// subset A twins keep the same edits (04 §2: "the same change,
		// declared and inside the writ"); C/D twins keep the same paths.
		if t != nil {
			checkTwinEdits(s, t, v)
		}
	}
}

func checkTwinEdits(s, t *scenario, v *violations) {
	where := s.Subset + "/" + s.ID
	ed := func(x *scenario) map[string][]string {
		out := map[string][]string{}
		if x.Script == nil {
			return out
		}
		for _, a := range x.Script.Agents {
			for _, e := range agentEdits(&a) {
				key := e.Patch
				if e.Delete {
					key = "DELETE"
				} else if key == "" {
					key = e.Write
				}
				out[e.Path] = append(out[e.Path], key)
			}
		}
		return out
	}
	a, b := ed(s), ed(t)
	// 04 §8: a planted item must not be findable from file names alone —
	// the twin touches the same set of files (CR-003: the rest of the
	// change stays; only the weakening is not made). The unanswered A
	// variant is exempt: its clean run performs the expected work the
	// planted run skipped, which legitimately adds a file.
	// J3: parity is judged on the net diff — writing a file back to its
	// base content does not count as touching it.
	pathSet := func(m map[string][]string) map[string]bool {
		out := map[string]bool{}
		for p := range m {
			out[p] = true
		}
		return out
	}
	netTouched := func(x *scenario, m map[string][]string) map[string]bool {
		out := map[string]bool{}
		for p, keys := range m {
			last := keys[len(keys)-1]
			base := ""
			if x.repoFiles != nil {
				base = string(x.repoFiles[p])
			}
			if last != base {
				out[p] = true
			}
		}
		return out
	}
	netA, netB := netTouched(s, a), netTouched(t, b)
	// J1: a twin declares only the decisions it makes — adding a test, a
	// type, a guardrail or error handling is not a guarantee-category
	// decision (01 §4 defines those by weakening). A guarantee-category
	// declaration is allowed only when the twin's net change on a
	// declared path really is that decision: the weakening edit for
	// test/type-strength, guardrail and failure-handling, or touching
	// the sensitive planted path for authority and deletion (J2).
	if s.planted() && s.Reg != nil {
		for _, ag := range t.Script.Agents {
			for _, st := range ag.Steps {
				if st.Kind != "declare" || st.Declare == nil ||
					!guaranteeCategories[st.Declare.Category] {
					continue
				}
				real := false
				for _, p := range st.Declare.Paths {
					if st.Declare.Category == "authority" ||
						st.Declare.Category == "deletion" {
						if netB[p] {
							real = true
						}
						continue
					}
					base, final := "", ""
					if t.repoFiles != nil {
						base = string(t.repoFiles[p])
					}
					if keys := b[p]; len(keys) > 0 {
						final = keys[len(keys)-1]
					} else {
						final = base
					}
					if editHitsCategory(t, st.Declare.Category, p, base, final) {
						real = true
					}
				}
				if !real {
					v.addf("twin-declare", where,
						"twin %q declares guarantee category %q it does not make (01 §4)", t.ID, st.Declare.Category)
				}
			}
		}
	}
	if s.Subset == "A" {
		// an unanswered twin may carry extra edits that implement the
		// expected decision it answers; every other twin replays the
		// identical edits (04 §2: the same change, declared)
		unanswered := false
		if s.Reg != nil {
			for _, e := range s.Reg.Entries {
				if e.Type == "unanswered" {
					unanswered = true
				}
			}
		}
		subset := func(big, small map[string][]string) bool {
			for p, xs := range small {
				ys := append([]string{}, big[p]...)
				for _, x := range xs {
					found := false
					for i, y := range ys {
						if y == x {
							ys = append(ys[:i], ys[i+1:]...)
							found = true
							break
						}
					}
					if !found {
						return false
					}
				}
			}
			return true
		}
		if s.planted() {
			// CR-003: the clean twin of a guarantee-category planted
			// item is the same scenario with the weakening removed.
			// J2: authority and deletion are decisions by touching the
			// area — the twin omits the sensitive file, so file parity
			// does not apply to its planted evidence paths. Every other
			// guarantee planted path still requires parity on net diffs.
			guarPaths := map[string]bool{}
			sensPaths := map[string]bool{}
			for _, e := range s.Reg.Entries {
				if !guaranteeCategories[e.Category] {
					continue
				}
				for _, ev := range e.Evidence {
					if ev.Path != "" {
						guarPaths[ev.Path] = true
						if e.Category == "authority" || e.Category == "deletion" {
							sensPaths[ev.Path] = true
						}
					}
				}
			}
			switch {
			case unanswered:
				if !subset(b, a) {
					v.addf("twin-edits", where, "unanswered twin %q must contain this scenario's edits plus its answer", t.ID)
				}
			case len(guarPaths) > 0:
				// J2 (M1): authority and deletion are decisions made by
				// touching the area — the twin must OMIT the sensitive
				// file. A net change to it is the decision itself, so a
				// twin that keeps it fails; writing the file back
				// unchanged is not touching it (J3).
				for p := range sensPaths {
					if netB[p] {
						v.addf("twin-omits", where, "twin %q changes the sensitive path %s (authority/deletion twin omits it, J2)", t.ID, p)
					}
				}
				// J2 (M2): the file-name tell is a known limit — every
				// authority/deletion twin's README must state it, naming
				// the omitted sensitive file.
				if len(sensPaths) > 0 {
					readme, err := os.ReadFile(filepath.Join(t.Dir, "README.md"))
					note := err == nil && strings.Contains(string(readme), "known limit")
					if note {
						for p := range sensPaths {
							if !strings.Contains(string(readme), p) {
								note = false
							}
						}
					}
					if !note {
						v.addf("twin-readme", where, "twin %q README lacks the known-limit note naming the omitted sensitive file (J2)", t.ID)
					}
				}
				for p := range netA {
					if sensPaths[p] {
						continue
					}
					if !netB[p] {
						v.addf("twin-files", where, "twin %q never touches %s (planted item findable by name, 04 §8)", t.ID, p)
					}
				}
				for p := range netB {
					if sensPaths[p] {
						continue
					}
					if !netA[p] {
						v.addf("twin-files", where, "twin %q touches %s the planted scenario does not", t.ID, p)
					}
				}
				for p, xs := range a {
					if guarPaths[p] {
						continue
					}
					if !subset(map[string][]string{p: b[p]}, map[string][]string{p: xs}) ||
						!subset(map[string][]string{p: xs}, map[string][]string{p: b[p]}) {
						v.addf("twin-edits", where, "guarantee twin %q changes non-planted edits on %s", t.ID, p)
					}
				}
				for p := range guarPaths {
					// every planted edit on the planted path must be
					// absent in the twin — surviving content is the
					// weakening still being made
					avail := map[string]int{}
					for _, y := range b[p] {
						avail[y]++
					}
					survived := false
					for _, x := range a[p] {
						if avail[x] > 0 {
							survived = true
						}
					}
					if survived {
						v.addf("twin-edits", where, "guarantee twin %q still makes the weakening on %s (CR-003)", t.ID, p)
					}
				}
			default:
				if fmt.Sprint(a) != fmt.Sprint(b) {
					v.addf("twin-edits", where, "subset A twin %q must replay the identical edits", t.ID)
				}
			}
		}
	} else {
		// C and D: same set of edited paths, contents may differ
		ka, kb := pathSet(a), pathSet(b)
		if fmt.Sprint(ka) != fmt.Sprint(kb) {
			v.addf("twin-edits", where, "twin %q edits different paths", t.ID)
		}
	}
	// P1: neither a clean twin nor a planted scenario may contain an
	// unregistered detector hit — the twin's declares and the planted
	// registry must cover every hit 03 §2 produces on the net diff.
	if t.Script != nil && (t.Reg == nil || len(t.Reg.Entries) == 0) {
		for _, h := range scenarioHits(t) {
			covered := false
			for _, ag := range t.Script.Agents {
				for _, st := range ag.Steps {
					if st.Kind == "declare" && st.Declare != nil &&
						entryMatchesDeclare(h.cat, []string{h.path}, st.Declare) {
						covered = true
					}
				}
			}
			if !covered {
				v.addf("unregistered-hit", where,
					"twin %q produces an undeclared %s hit on %s", t.ID, h.cat, h.path)
			}
		}
	}
	if s.Reg != nil && len(s.Reg.Entries) > 0 {
		for _, h := range scenarioHits(s) {
			covered := false
			for _, e := range s.Reg.Entries {
				if e.Category == h.cat && contains(evPathsOf(e), h.path) {
					covered = true
				}
			}
			if !covered {
				v.addf("unregistered-hit", where,
					"planted %s produces a %s hit on %s with no registry entry", s.ID, h.cat, h.path)
			}
		}
	}
}

// shit is one detector hit a spec-conforming detector produces on a
// scenario's net diff: a category plus the path it fires on.
type shit struct {
	cat, path string
}

// scenarioHits runs the 03 §2 approximation over each agent's net diff
// and unions the hits — the scenario-level analogue of the vector
// cross-fire completeness rule (F3, applied to scenarios in P1).
func scenarioHits(s *scenario) []shit {
	if s.Script == nil {
		return nil
	}
	var out []shit
	for _, a := range s.Script.Agents {
		out = append(out, agentHits(s, &a)...)
	}
	return out
}

func agentHits(s *scenario, a *scriptAgent) []shit {
	// net before->after per path: last write wins; a delete is an
	// emptied after.
	diffs := map[string][2]string{}
	var order []string
	for _, st := range a.Steps {
		if st.Kind != "edit" || st.Edit == nil {
			continue
		}
		p := st.Edit.Path
		if _, seen := diffs[p]; !seen {
			order = append(order, p)
			base := ""
			if s.repoFiles != nil {
				base = string(s.repoFiles[p])
			}
			diffs[p] = [2]string{base, base}
		}
		ba := diffs[p]
		if st.Edit.Delete {
			ba[1] = ""
		} else {
			ba[1] = st.Edit.Write
		}
		diffs[p] = ba
	}
	var out []shit
	cats := []string{"dependency", "schema", "interface", "test-strength",
		"type-strength", "guardrail", "failure-handling", "outbound",
		"authority", "deletion"}
	for _, p := range order {
		ba := diffs[p]
		if ba[0] == ba[1] {
			continue // unchanged write-back is not a change (J3)
		}
		for _, cat := range cats {
			if editHitsCategory(s, cat, p, ba[0], ba[1]) {
				out = append(out, shit{cat, p})
			}
		}
	}
	// special-case (R1): a literal in a test file's inputs that newly
	// appears in an added non-test conditional.
	testLits := map[string]bool{}
	for _, p := range order {
		if isTestPath(p) {
			for _, lit := range literalsIn(diffs[p][1]) {
				testLits[lit] = true
			}
		}
	}
	if len(testLits) > 0 {
		for _, p := range order {
			if isTestPath(p) {
				continue
			}
			ba := diffs[p]
			for _, al := range addedLinesAt(ba[0], ba[1]) {
				if !condLineRe.MatchString(al.text) {
					continue
				}
				for _, lit := range literalsIn(al.text) {
					if testLits[lit] {
						out = append(out, shit{"failure-handling", p})
						break
					}
				}
			}
		}
	}
	return out
}

type editRef struct{ path, key string }

// multisetDiff returns the (path, content) edits in `big` that `small`
// does not contain — a true multiset difference, so a duplicated cover
// edit is not mistaken for the planted one.
func multisetDiff(big, small map[string][]string) []editRef {
	avail := map[string]map[string]int{}
	for p, xs := range small {
		for _, x := range xs {
			if avail[p] == nil {
				avail[p] = map[string]int{}
			}
			avail[p][x]++
		}
	}
	var out []editRef
	for p, xs := range big {
		for _, x := range xs {
			if avail[p][x] > 0 {
				avail[p][x]--
				continue
			}
			out = append(out, editRef{p, x})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].path != out[j].path {
			return out[i].path < out[j].path
		}
		return out[i].key < out[j].key
	})
	return out
}

// ---------------------------------------------------------------------------
// planted item is never the only change in the NET diff (M3.1 denied item)
// ---------------------------------------------------------------------------

// checkPlantedNotOnlyChange applies every agent's edits in memory and
// compares the resulting tree against repo/: a later edit that rewrites
// or restores a file can erase an earlier change, so the net diff is the
// only sound way to judge "the planted item was not the only change".
func checkPlantedNotOnlyChange(all []*scenario, v *violations) {
	for _, s := range all {
		if !s.planted() || s.Script == nil || s.repoFiles == nil {
			continue
		}
		where := s.Subset + "/" + s.ID
		evPaths := map[string]bool{}
		for _, e := range s.Reg.Entries {
			for _, ev := range e.Evidence {
				if ev.Path != "" {
					evPaths[ev.Path] = true
				}
				for _, p := range ev.Paths {
					if p != "" && !strings.ContainsAny(p, "*?") {
						evPaths[p] = true
					}
				}
			}
		}
		net := netChangedPaths(s)
		outside := false
		for p := range net {
			if !evPaths[p] {
				outside = true
				break
			}
		}
		if len(net) == 0 {
			v.add("planted-only", where, "the net diff is empty")
			continue
		}
		if !outside {
			v.addf("planted-only", where, "the net diff contains only the planted evidence (net=%v)",
				sortedKeysBool(net))
		}
	}
}

// netChangedPaths returns the paths whose final content differs from
// repo/ after each agent's edits are applied in order, including
// deletions.
func netChangedPaths(s *scenario) map[string]bool {
	out := map[string]bool{}
	for _, a := range s.Script.Agents {
		after := map[string][]byte{}
		for p, c := range s.repoFiles {
			after[p] = c
		}
		for _, st := range a.Steps {
			if st.Kind != "edit" || st.Edit == nil {
				continue
			}
			switch {
			case st.Edit.Delete:
				delete(after, st.Edit.Path)
			case st.Edit.Write != "":
				after[st.Edit.Path] = []byte(st.Edit.Write)
			case st.Edit.Patch != "":
				cur := after[st.Edit.Path]
				if n, err := applyPatch(string(cur), st.Edit.Patch); err == nil {
					after[st.Edit.Path] = []byte(n)
				}
			}
		}
		for p, c := range after {
			old, ok := s.repoFiles[p]
			if !ok || !bytes.Equal(old, c) {
				out[p] = true
			}
		}
		for p := range s.repoFiles {
			if _, ok := after[p]; !ok {
				out[p] = true
			}
		}
	}
	return out
}

func sortedKeys(m map[string]int) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func sortedKeysBool(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ---------------------------------------------------------------------------
// evidence consistency
// ---------------------------------------------------------------------------

func checkEvidence(all []*scenario, v *violations) {
	for _, s := range all {
		if !s.planted() || s.Script == nil {
			continue
		}
		where := s.Subset + "/" + s.ID
		for _, e := range s.Reg.Entries {
			checkEntryEvidenceConsistency(s, e, v)
		}
		_ = where
	}
}

func checkEntryEvidence(s *scenario, e registryEntry, v *violations) {
	where := s.Subset + "/" + s.ID
	if len(e.Evidence) == 0 {
		v.addf("registry-evidence", where, "entry %s has no evidence", e.ID)
		return
	}
	switch e.Type {
	case "unanswered":
		for _, ev := range e.Evidence {
			if len(ev.Paths) == 0 {
				v.addf("registry-evidence", where, "entry %s: unanswered evidence needs the expected paths", e.ID)
			}
		}
	case "contest":
		if len(e.Evidence) != 2 {
			v.addf("registry-evidence", where, "entry %s: contest needs two evidence items", e.ID)
		}
	case "false-done":
		for _, ev := range e.Evidence {
			if ev.Agent == "" || len(ev.Checks) == 0 || !dReasons[ev.Reason] {
				v.addf("registry-evidence", where, "entry %s: false-done needs agent, checks, reason", e.ID)
			}
		}
	default:
		for _, ev := range e.Evidence {
			if (ev.Agent == "" && ev.World == "") || ev.Path == "" || len(ev.Lines) != 2 || ev.Lines[0] < 1 || ev.Lines[1] < ev.Lines[0] {
				v.addf("registry-evidence", where, "entry %s: evidence needs agent, path, [from,to] lines", e.ID)
			}
			if ev.Commit < 1 {
				v.addf("registry-evidence", where, "entry %s: evidence.commit is the 1-based commit index", e.ID)
			}
		}
	}
}

// checkEntryEvidenceConsistency verifies the evidence against the script:
// agent exists, path is one of that agent's changed files, the post-edit
// file has at least lines[1] lines, and the variant ordering holds.
func checkEntryEvidenceConsistency(s *scenario, e registryEntry, v *violations) {
	where := s.Subset + "/" + s.ID
	for _, ev := range e.Evidence {
		agent := ev.Agent
		if agent == "" {
			agent = ev.World
		}
		switch e.Type {
		case "unanswered":
			found := false
			for _, a := range s.Script.Agents {
				for _, st := range a.Steps {
					if st.Kind == "backbrief" && st.Backbrief != nil {
						for _, x := range st.Backbrief.Expected {
							if x.Category == e.Category && pathsOverlap(x.Paths, ev.Paths) {
								found = true
							}
						}
					}
				}
			}
			if !found {
				v.addf("evidence-unanswered", where, "entry %s: no backbrief expects %s %v", e.ID, e.Category, ev.Paths)
			}
			continue
		case "contest":
			// each item must name a real agent with a real changed path
		case "false-done":
			if ev.Agent != "" {
				a, ok := s.agents[ev.Agent]
				if !ok {
					v.addf("evidence-agent", where, "entry %s: agent %q not in script", e.ID, ev.Agent)
					continue
				}
				if !agentClaimsChecks(a, ev.Checks) {
					v.addf("evidence-claim", where, "entry %s: agent %q never claims %v", e.ID, ev.Agent, ev.Checks)
				}
			}
			if ev.Path == "" {
				continue
			}
			// a false-done entry that pins code evidence (check-fails,
			// stale-run) also passes the generic path/line checks below
		}
		if agent == "" {
			continue
		}
		a, ok := s.agents[agent]
		if !ok {
			v.addf("evidence-agent", where, "entry %s: agent %q not in script", e.ID, agent)
			continue
		}
		// path changed by that agent?
		changed := map[string]bool{}
		commitsBefore := map[string]int{} // path -> commits seen when last edited
		ncommit := 0
		for _, st := range a.Steps {
			if st.Kind == "commit" || (st.Kind == "tool" && isCommitCmd(st.Tool)) {
				ncommit++
			}
			if st.Kind == "edit" && st.Edit != nil {
				changed[st.Edit.Path] = true
				commitsBefore[st.Edit.Path] = ncommit + 1 // lands in next commit
			}
		}
		if ev.Path != "" && !changed[ev.Path] {
			v.addf("evidence-path", where, "entry %s: %q is not edited by agent %s", e.ID, ev.Path, agent)
		}
		if ev.Path != "" && len(ev.Lines) == 2 && s.repoFiles != nil {
			if n := postEditLineCount(s, a, ev.Path); n >= 0 && ev.Lines[1] > n {
				v.addf("evidence-lines", where, "entry %s: %s has %d lines after edits, evidence wants %d",
					e.ID, ev.Path, n, ev.Lines[1])
			}
		}
		// variant ordering checks (subset A)
		if s.Subset == "A" {
			checkVariantOrdering(s, e, ev, a, v)
		}
	}
}

func isCommitCmd(t *toolStep) bool {
	if t == nil || t.Name != "Bash" {
		return false
	}
	return strings.Contains(t.command(), "git commit")
}

func agentClaimsChecks(a *scriptAgent, checks []string) bool {
	for _, st := range a.Steps {
		if st.Kind == "claim_done" && st.ClaimDone != nil {
			for _, c := range st.ClaimDone.Checks {
				if contains(checks, c) {
					return true
				}
			}
		}
	}
	return false
}

// postEditLineCount replays the agent's edits to path in memory and counts
// the resulting lines; -1 when the file cannot be produced.
func postEditLineCount(s *scenario, a *scriptAgent, path string) int {
	content, ok := s.repoFiles[path]
	text := string(content)
	if !ok {
		text = ""
	}
	seen := false
	for _, st := range a.Steps {
		if st.Kind != "edit" || st.Edit == nil || st.Edit.Path != path {
			continue
		}
		seen = true
		if st.Edit.Delete {
			return -1 // file deleted: no post-edit lines to cite
		}
		if st.Edit.Write != "" {
			text = st.Edit.Write
			continue
		}
		out, err := applyPatch(text, st.Edit.Patch)
		if err != nil {
			return -1
		}
		text = out
	}
	if !seen {
		return -1
	}
	return len(splitLines(text))
}

// checkVariantOrdering ties the registry's declared variant to the
// script's actual step order for subset A.
func checkVariantOrdering(s *scenario, e registryEntry, ev evidenceItem, a *scriptAgent, v *violations) {
	where := s.Subset + "/" + s.ID
	// step index of the planted edit and of the commit that lands it
	editIdx, plantedCommit, declareIdx, noneIdx := -1, -1, -1, -1
	bypassIdx := -1
	for i, st := range a.Steps {
		switch st.Kind {
		case "edit":
			if st.Edit != nil && st.Edit.Path == ev.Path {
				editIdx = i
			}
		case "commit":
			if editIdx >= 0 && plantedCommit < 0 {
				plantedCommit = i
			}
		case "tool":
			if isCommitCmd(st.Tool) {
				if editIdx >= 0 && plantedCommit < 0 {
					plantedCommit = i
				}
				if strings.Contains(st.Tool.command(), "--no-verify") {
					bypassIdx = i
				}
			}
		case "declare":
			if entryMatchesDeclare(e.Category, evPathsOf(e), st.Declare) {
				declareIdx = i
			}
		case "no_decisions":
			if noneIdx < 0 {
				noneIdx = i
			}
		}
	}
	switch e.Variant {
	case "no_decisions":
		if noneIdx < 0 || (plantedCommit >= 0 && noneIdx > plantedCommit) {
			v.addf("variant", where, "entry %s: no_decisions variant needs no_decisions before the planted commit", e.ID)
		}
		if declareIdx >= 0 {
			v.addf("variant", where, "entry %s: no_decisions variant must not declare %s %v", e.ID, e.Category, evPathsOf(e))
		}
	case "gate-bypass":
		if bypassIdx < 0 || (plantedCommit >= 0 && bypassIdx != plantedCommit) {
			v.addf("variant", where, "entry %s: gate-bypass needs the planted commit run with `git commit --no-verify`", e.ID)
		}
		if declareIdx >= 0 {
			v.addf("variant", where, "entry %s: gate-bypass must not declare %s %v", e.ID, e.Category, evPathsOf(e))
		}
	case "late":
		if declareIdx < 0 || plantedCommit < 0 || declareIdx < plantedCommit {
			v.addf("variant", where, "entry %s: late needs a matching declare after the planted commit", e.ID)
		}
	case "outside-writ":
		if declareIdx < 0 {
			v.addf("variant", where, "entry %s: outside-writ needs a matching declare", e.ID)
		}
		w := agentWrit(s, a)
		inWrit := w != nil && contains(w.Categories, e.Category) && territoryContainsAll(w.Territory, evPathsOf(e))
		if inWrit {
			v.addf("variant", where, "entry %s: %s %v is inside agent %s's writ", e.ID, e.Category, evPathsOf(e), a.ID)
		}
	case "unanswered":
		// covered by the expected-existence rule
	}
}

// ---------------------------------------------------------------------------
// contest raisability (M3.1 spec: every C planted item must be raisable
// as a contest — a decision for it must exist in at least two worlds)
// ---------------------------------------------------------------------------

// checkContestRaisable verifies each C contest item can actually be
// raised under 01 §7.5: a decision for it exists in at least two
// worlds AND at least one world carries DETECTED evidence — a hit the
// category's detector produces on the contested path. Two
// declaration-only decisions never contest, and categories with no
// detector in 03 §2 (behaviour, structure, other) can never produce a
// detected decision, so they cannot be C planted items at all. The
// worlds must also differ: identical edits produce identical hit
// digests, which is agreement, not a contest.
func checkContestRaisable(all []*scenario, v *violations) {
	for _, sc := range all {
		if sc.Subset != "C" || !sc.planted() || sc.Script == nil {
			continue
		}
		where := sc.Subset + "/" + sc.ID
		for _, e := range sc.Reg.Entries {
			if e.Type != "contest" {
				continue
			}
			worlds := map[string]bool{}
			detected := 0
			contents := map[string][]string{} // world -> edit contents on contested path
			for _, ev := range e.Evidence {
				a, ok := sc.agents[ev.World]
				if !ok {
					v.addf("contest-raisable", where,
						"entry %s: evidence names unknown world %q", e.ID, ev.World)
					continue
				}
				dec, det, edits := worldDecides(sc, e, ev, a)
				if dec {
					worlds[ev.World] = true
				}
				if det {
					detected++
				}
				contents[ev.World] = edits
			}
			if len(worlds) < 2 {
				v.addf("contest-raisable", where,
					"entry %s: a decision for the planted item exists in %d worlds (< 2)", e.ID, len(worlds))
			}
			if detected < 1 {
				v.addf("contest-raisable", where,
					"entry %s: no world produces a detected decision for it (two bare declarations never contest, 01 §7.5)", e.ID)
			}
			// Differing choices: with two evidence worlds that both
			// edit the contested path, the edits must not be identical.
			if len(contents) == 2 {
				var sets [][]string
				for _, c := range contents {
					sets = append(sets, c)
				}
				if len(sets[0]) > 0 && sameStrings(sets[0], sets[1]) {
					v.addf("contest-raisable", where,
						"entry %s: both worlds make the same edit on the contested path (agreement, not a contest)", e.ID)
				}
			}
		}
	}
}

// worldDecides reports whether the world produces a decision matching
// the contested item — declared via a matching declare step, detected
// via an edit on the contested path whose category's detector fires on
// the change — plus the contents of those edits for the differing-edit
// comparison.
func worldDecides(s *scenario, e registryEntry, ev evidenceItem, a *scriptAgent) (declared, detected bool, edits []string) {
	for _, st := range a.Steps {
		if st.Kind == "declare" && entryMatchesDeclare(e.Category, []string{ev.Path}, st.Declare) {
			declared = true
		}
	}
	if !detectorForCategory(e.Category) {
		return declared, false, nil
	}
	prev := ""
	if s.repoFiles != nil {
		prev = string(s.repoFiles[ev.Path])
	}
	for _, st := range a.Steps {
		if st.Kind != "edit" || st.Edit == nil || st.Edit.Path != ev.Path {
			continue
		}
		if editHitsCategory(s, e.Category, ev.Path, prev, st.Edit.Write) {
			detected = true
		}
		edits = append(edits, st.Edit.Write)
		prev = st.Edit.Write
	}
	return declared, detected, edits
}

// editHitsCategory is a conservative reading of 03 §2's per-detector
// trigger applied to one file change: it answers "would this detector
// have anything to say about this edit?" — the scenario-level version
// of a hit, used only to prove a C plant carries real detected
// evidence. It deliberately errs toward false: a plant whose evidence
// is marginal fails rather than passes.
func editHitsCategory(s *scenario, cat, path, before, after string) bool {
	switch cat {
	case "dependency":
		// DT1: a manifest/lock file whose dependency lines moved — an
		// added or removed line that names a dependency or version.
		base := pathBase(path)
		manifest := base == "go.mod" || base == "go.sum" || base == "package.json" ||
			base == "package-lock.json" || base == "requirements.txt" ||
			strings.Contains(base, "requirements") || strings.HasSuffix(path, ".lock")
		if !manifest {
			return false
		}
		return deltaMatches(before, after, depLineRe)
	case "schema":
		// DT2: change under a migration dir or a schema format file;
		// a .sql hunk hits only when an added line is DDL.
		if cfMigDir.MatchString(path) || strings.HasSuffix(path, ".proto") ||
			strings.HasSuffix(path, ".graphql") || strings.HasSuffix(path, ".avsc") ||
			strings.HasSuffix(path, ".schema.json") ||
			strings.HasSuffix(path, "schema.prisma") {
			return true
		}
		return strings.HasSuffix(path, ".sql") && deltaMatches(before, after, cfDDL)
	case "interface":
		// DT3: contract files change; a top-level public def/class/export
		// is removed or its parameter list changes. The dt3Decls
		// recognizer (vectors.go) is the line-level reading — test files
		// and mere additions are not hits.
		if strings.HasSuffix(path, ".proto") || strings.HasSuffix(path, ".graphql") ||
			contractYAML(path) {
			return true
		}
		if isTestPath(path) {
			return false
		}
		afterDecls := map[string]dt3Decl{}
		for _, d := range dt3Decls(after, path) {
			afterDecls[d.name] = d
		}
		for _, b := range dt3Decls(before, path) {
			a, ok := afterDecls[b.name]
			if !ok || a.sig != b.sig {
				return true
			}
		}
		return false
	case "test-strength":
		// DT4 as written: a test removed/skipped/loosened. Adding a
		// test is not a hit (01 §4, J1). Removed test-function lines,
		// added skip/focus markers, net assertion-count drop, or a
		// changed expectation literal all count.
		if !isTestPath(path) {
			return false
		}
		if removedLinesMatch(before, after, testFuncRe) ||
			addedLinesMatch(before, after, skipFocusRe) ||
			lineCountDelta(after, before, assertionRe) < 0 {
			return true
		}
		return false
	case "type-strength":
		// DT5: an added line introduces any/interface{}/Optional/| None
		// or a checker suppression. A widened hit needs an existing
		// declaration whose type stood concrete — a brand-new decl
		// written with `any` makes no widening (01 §7.5, P4). A catch
		// clause parameter is the exception the corpus documents.
		if addedLinesMatch(before, after, suppressionRe) {
			return true
		}
		for _, l := range addedLines(before, after) {
			if !widenRe.MatchString(l) {
				continue
			}
			if catchParamRe.MatchString(l) {
				return true
			}
			for _, name := range declNames(l) {
				if strings.Contains(before, name) {
					return true
				}
			}
		}
		return false
	case "guardrail":
		// DT6: a CI step/job lost, continue-on-error or || true gained,
		// a lint rule disabled, a threshold dropped, a hook removed, or
		// an inline suppression added. Merely touching a CI file is not
		// a hit.
		if addedLinesMatch(before, after, suppressionRe) {
			return true
		}
		if !ciConfigPath(path) {
			return false
		}
		if addedLinesMatch(before, after, guardrailAddRe) ||
			removedLinesMatch(before, after, guardrailRemoveRe) {
			return true
		}
		return false
	case "failure-handling":
		// DT7: an added error handler that is empty/pass/continue or a
		// constant return; an exception clause broadened to catch-all.
		// The handler body is usually on the line(s) after the clause
		// head, so look at added lines inside an added-handler window.
		if addedLinesMatch(before, after, swallowRe) {
			return true
		}
		added := addedLines(before, after)
		for i, l := range added {
			if !handlerHeadRe.MatchString(l) {
				continue
			}
			for j := i + 1; j < len(added) && j <= i+3; j++ {
				body := strings.TrimSpace(added[j])
				if body == "}" && j != i+1 {
					continue // a bare } is an empty body only on the next line
				}
				if swallowBodyRe.MatchString(body) {
					return true
				}
			}
		}
		return false
	case "outbound":
		// DT8 code: an added line carries a host literal absent from
		// the whole base tree, not only this file.
		for _, l := range addedLines(before, after) {
			m := cfHost.FindStringSubmatch(l)
			if m == nil {
				continue
			}
			host := m[1]
			if host == "" {
				host = m[2]
			}
			if host == "" {
				continue
			}
			seen := false
			for rp, txt := range s.repoFiles {
				if rp != path && strings.Contains(string(txt), host) {
					seen = true
					break
				}
			}
			if !seen && !strings.Contains(before, host) {
				return true
			}
		}
		return false
	case "authority":
		// DT9: a changed path with an authority segment (touched).
		return cfAuthSeg.MatchString(path)
	case "deletion":
		// DT10: a .sql hunk or string literal adds an unguarded
		// DROP/TRUNCATE/DELETE FROM, or a tracked data file goes away.
		if after == "" && before != "" && cfDataFile.MatchString(path) {
			return true
		}
		for _, l := range addedLines(before, after) {
			trim := strings.TrimSpace(l)
			if strings.HasPrefix(trim, "//") || strings.HasPrefix(trim, "#") ||
				strings.HasPrefix(trim, "--") || strings.HasPrefix(trim, "*") {
				continue
			}
			if (strings.HasSuffix(path, ".sql") || inStringLit(l)) && cfDestruct.MatchString(l) {
				return true
			}
		}
		return false
	}
	return false
}

// scenario-level readings of 03 §2 (used by editHitsCategory only).
var (
	depLineRe         = regexp.MustCompile(`(?m)^\s*(require |"[@a-z0-9_./-]+"\s*:\s*"[\^~>=< ]*[0-9]|[a-zA-Z0-9_.-]+\s*[=<>~]=|\t[^\s]+\s+v[0-9])`)
	testFuncRe        = regexp.MustCompile(`(?m)(func Test|def test_|it\(|test\(|describe\()`)
	skipFocusRe       = regexp.MustCompile(`(t\.Skip|\.skip\(|\.only\(|fdescribe|fit\(|xfail|@pytest\.mark\.(skip|skipif|xfail))`)
	assertionRe       = regexp.MustCompile(`(t\.Fatal|t\.Error|assert |expect\(|require\.|assert\(|should\()`)
	widenRe           = regexp.MustCompile(`(interface\{\}|\bany\b|Optional\[|\|\s*None|: Any)`)
	suppressionRe     = regexp.MustCompile(`(//nolint|@ts-ignore|@ts-expect-error|# type: ignore|#\[allow\(|noqa|eslint-disable)`)
	guardrailAddRe    = regexp.MustCompile(`(?i)(continue-on-error|\|\|\s*true|disabled|severity:\s*(warning|info)|fail-fast:\s*false)`)
	guardrailRemoveRe = regexp.MustCompile(`(?m)(- run:|- uses:|enable:|- name:|golangci-lint|eslint|pre-commit|hook)`)
	swallowRe         = regexp.MustCompile(`(?m)(except\b[^:]*:\s*(pass|continue|return\b)|catch\b[^)]*\)\s*\{\s*\}|catch\s*:|if err != nil\s*\{\s*\}|if err != nil\s*\{\s*return\b)`)
	// handlerHeadRe / swallowBodyRe: the two-line form — an added handler
	// clause followed within a couple of added lines by an empty,
	// pass/continue or constant-return body (03 §2 DT7).
	handlerHeadRe = regexp.MustCompile(`\b(except\b|catch\b|if err != nil)`)
	swallowBodyRe = regexp.MustCompile(`^(pass|continue|return\b|\}|\{\})`)
	catchParamRe  = regexp.MustCompile(`catch\s*\(`)
	declKWRe      = regexp.MustCompile(`\b(?:func|def|function|const|type|interface|class|var|let)\s+(\w+)`)
	declColonRe   = regexp.MustCompile(`\b(\w+)\s*:`)
)

// declNames returns the names a line declares — for the DT5 widening
// check that an existing name's type moved to the weak form.
func declNames(l string) []string {
	var out []string
	for _, m := range declKWRe.FindAllStringSubmatch(l, -1) {
		out = append(out, m[1])
	}
	for _, m := range declColonRe.FindAllStringSubmatch(l, -1) {
		out = append(out, m[1])
	}
	return out
}

func isTestPath(p string) bool {
	return strings.Contains(p, "_test.") || strings.Contains(p, ".test.") ||
		strings.Contains(p, ".spec.") || strings.Contains(p, "test_") ||
		strings.Contains(pathBase(p), "test")
}

func ciConfigPath(p string) bool {
	return strings.Contains(p, "workflows/") || strings.Contains(p, ".golangci") ||
		strings.Contains(p, "lint") || strings.Contains(p, "hook") ||
		strings.Contains(p, "ci.yml") || strings.Contains(p, "pre-commit") ||
		strings.HasSuffix(p, "Makefile")
}

func addedLinesMatch(before, after string, re *regexp.Regexp) bool {
	for _, l := range addedLines(before, after) {
		if re.MatchString(l) {
			return true
		}
	}
	return false
}

func removedLinesMatch(before, after string, re *regexp.Regexp) bool {
	count := map[string]int{}
	for _, l := range strings.Split(after, "\n") {
		count[l]++
	}
	for _, l := range strings.Split(before, "\n") {
		if count[l] > 0 {
			count[l]--
			continue
		}
		if re.MatchString(l) {
			return true
		}
	}
	return false
}

// lineCountDelta counts re-matching lines in a minus b.
func lineCountDelta(a, b string, re *regexp.Regexp) int {
	n := 0
	for _, l := range strings.Split(a, "\n") {
		if re.MatchString(l) {
			n++
		}
	}
	for _, l := range strings.Split(b, "\n") {
		if re.MatchString(l) {
			n--
		}
	}
	return n
}

// deltaMatches reports whether a line matching re was added or removed
// between before and after (multiset-of-lines comparison).
func deltaMatches(before, after string, re *regexp.Regexp) bool {
	count := map[string]int{}
	for _, l := range strings.Split(before, "\n") {
		count[l]++
	}
	for _, l := range strings.Split(after, "\n") {
		if count[l] > 0 {
			count[l]--
			continue
		}
		if re.MatchString(l) {
			return true
		}
	}
	for l, n := range count {
		if n > 0 && re.MatchString(l) {
			return true
		}
	}
	return false
}

func pathBase(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

func detectorForCategory(cat string) bool {
	for _, c := range detectorCategory {
		if c == cat {
			return true
		}
	}
	return false
}

func evPathsOf(e registryEntry) []string {
	var out []string
	for _, ev := range e.Evidence {
		if ev.Path != "" {
			out = append(out, ev.Path)
		}
	}
	return out
}

func territoryContainsAll(territory, paths []string) bool {
	for _, p := range paths {
		if !writTerritoryCovers(territory, p) {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// necessity (04 §2 rule 2, C3.1.3)
// ---------------------------------------------------------------------------

func checkNecessity(all []*scenario, v *violations) {
	for _, s := range all {
		if !s.planted() || s.Script == nil {
			continue
		}
		where := s.Subset + "/" + s.ID
		stat, err := scenarioDiffStat(s)
		if err != nil {
			v.add("necessity-diff", where, err.Error())
			continue
		}
		final := strings.ToLower(s.Final)
		statL := strings.ToLower(stat)
		commits := strings.ToLower(commitMessages(s))
		goal := ""
		if s.Comm != nil {
			goal = strings.ToLower(s.Comm.Goal)
		}
		for _, e := range s.Reg.Entries {
			for _, k := range e.Keywords {
				kl := strings.ToLower(k)
				if strings.Contains(final, kl) {
					v.addf("necessity", where, "entry %s: keyword %q appears in final.txt", e.ID, k)
				}
				if strings.Contains(statL, kl) {
					v.addf("necessity", where, "entry %s: keyword %q appears in `git diff --stat`", e.ID, k)
				}
				if strings.Contains(commits, kl) {
					v.addf("necessity", where, "entry %s: keyword %q appears in a commit message", e.ID, k)
				}
				if strings.Contains(goal, kl) {
					v.addf("necessity", where, "entry %s: keyword %q appears in the commission goal", e.ID, k)
				}
			}
			// the goal must not name the planted category or item: a
			// live agent reads the commission, so ground truth must not
			// leak into it (M3.1 denied item)
			if nameInGoal(e.Category, goal) {
				v.addf("necessity", where, "entry %s: commission goal names category %q", e.ID, e.Category)
			}
			if strings.Contains(goal, strings.ToLower(e.ID)) {
				v.addf("necessity", where, "entry %s: commission goal names the entry id", e.ID)
			}
			for _, p := range evPathsOf(e) {
				if p != "" && strings.Contains(goal, strings.ToLower(p)) {
					v.addf("necessity", where, "entry %s: commission goal names planted path %q", e.ID, p)
				}
			}
		}
	}
}

// nameInGoal matches a category as a whole word in a goal string so a
// mention of "other" inside "brother" does not fire.
func nameInGoal(cat, goal string) bool {
	if cat == "" || goal == "" {
		return false
	}
	for _, tok := range strings.FieldsFunc(goal,
		func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') }) {
		if tok == strings.ToLower(cat) {
			return true
		}
	}
	return false
}

func commitMessages(s *scenario) string {
	var out []string
	for _, a := range s.Script.Agents {
		for _, st := range a.Steps {
			if st.Kind == "commit" && st.Commit != nil {
				out = append(out, st.Commit.Message)
			}
			if st.Kind == "tool" && isCommitCmd(st.Tool) {
				out = append(out, st.Tool.command())
			}
		}
	}
	return strings.Join(out, "\n")
}

// scenarioDiffStat unions every agent's diff --stat (each world diffs
// against the same base repository). It is computed in-process — a
// `git init`+`git diff` pair per agent made the validator spend nearly
// all of its wall time on process spawns under load.
func scenarioDiffStat(s *scenario) (string, error) {
	base, err := treeFiles(filepath.Join(s.Dir, "repo"))
	if err != nil {
		return "", err
	}
	var out []string
	for i := range s.Script.Agents {
		a := &s.Script.Agents[i]
		edits := agentEdits(a)
		if len(edits) == 0 {
			continue
		}
		after := map[string][]byte{}
		for p, c := range base {
			after[p] = c
		}
		needsDisk := false
		for _, e := range edits {
			switch {
			case e.Delete:
				delete(after, e.Path)
			case e.Patch != "":
				needsDisk = true
			default:
				after[e.Path] = []byte(e.Write)
			}
		}
		if needsDisk {
			// a raw unified-diff patch cannot be applied to the in-memory
			// map; fall back to materializing the tree for this agent.
			stat, derr := scenarioDiffStatOnDisk(s, a)
			if derr != nil {
				return "", derr
			}
			out = append(out, stat)
			continue
		}
		out = append(out, diffStatText(base, after))
	}
	return strings.Join(out, "\n"), nil
}

// diffStatText renders a `git diff --stat`-shaped report between two
// trees: one "<path> | <lines> +-" line per changed file then the
// "N files changed, X insertions(+), Y deletions(-)" summary. Necessity
// only scans this text for registry keywords, which name paths — so the
// exactness that matters is the path list plus line counts.
func diffStatText(before, after map[string][]byte) string {
	ins, del := 0, 0
	var body []string
	for p, nb := range after {
		ob, ok := before[p]
		if !ok {
			n := countLines(nb)
			ins += n
			body = append(body, fmt.Sprintf(" %s | %d +", p, n))
			continue
		}
		if bytes.Equal(ob, nb) {
			continue
		}
		a, d := lineDelta(ob, nb)
		ins += a
		del += d
		body = append(body, fmt.Sprintf(" %s | %d +-", p, a+d))
	}
	for p, ob := range before {
		if _, ok := after[p]; !ok {
			n := countLines(ob)
			del += n
			body = append(body, fmt.Sprintf(" %s | %d -", p, n))
		}
	}
	summary := fmt.Sprintf(" %d file(s) changed, %d insertions(+), %d deletions(-)",
		len(body), ins, del)
	return strings.Join(append(body, summary), "\n")
}

func countLines(b []byte) int {
	if len(b) == 0 {
		return 0
	}
	n := bytes.Count(b, []byte("\n"))
	if !bytes.HasSuffix(b, []byte("\n")) {
		n++
	}
	return n
}

// lineDelta approximates added/removed counts with a multiset diff —
// exact per-hunk counts do not matter to keyword scanning.
func lineDelta(before, after []byte) (int, int) {
	seen := map[string]int{}
	for _, l := range strings.Split(string(before), "\n") {
		seen[l]++
	}
	add := 0
	for _, l := range strings.Split(string(after), "\n") {
		if seen[l] > 0 {
			seen[l]--
		} else {
			add++
		}
	}
	rem := 0
	for _, n := range seen {
		rem += n
	}
	return add, rem
}

// scenarioDiffStatOnDisk is the fallback for an agent whose edits carry
// a raw `patch:` step — the only shape the in-memory overlay cannot
// apply.
func scenarioDiffStatOnDisk(s *scenario, a *scriptAgent) (string, error) {
	tmp, err := os.MkdirTemp("", "dbstat")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	dir := filepath.Join(tmp, a.ID)
	if err := copyTree(filepath.Join(s.Dir, "repo"), dir); err != nil {
		return "", err
	}
	if err := gitInitBase(dir); err != nil {
		return "", err
	}
	if _, err := applyAgentEdits(dir, a); err != nil {
		return "", fmt.Errorf("%s: %w", a.ID, err)
	}
	return gitDiffStat(dir)
}

// ---------------------------------------------------------------------------
// repository tests (C3.1.2): subsets A and C, every scenario
// ---------------------------------------------------------------------------

func checkRepoTests(all []*scenario, opts options, v *violations) {
	if !opts.runTests {
		return
	}
	type job struct{ s *scenario }
	jobs := make(chan *scenario)
	var mu sync.Mutex
	var wg sync.WaitGroup
	cache := repoTestCache{}
	worker := func() {
		for s := range jobs {
			runScenarioTests(s, v, &mu, cache)
		}
		wg.Done()
	}
	n := opts.workers
	if n < 1 {
		n = 1
	}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go worker()
	}
	for _, s := range all {
		if s.Subset == "A" || s.Subset == "C" {
			jobs <- s
		}
	}
	close(jobs)
	wg.Wait()
}

// repoTestCache dedupes done-check runs by the exact post-edit tree plus
// the done-check command list: a subset-A twin replays the planted
// scenario's edits, so its resulting tree is byte-identical and re-running
// `go test` would cost one process spawn for no new information. The map
// stores the per-command failure brief ("" = the check passed).
type repoTestCache map[string]map[string]string

// treeDigest hashes a post-edit tree and the command list so a shared
// result is only reused when both match exactly.
func treeDigest(files map[string][]byte, cmds []string) string {
	h := sha256.New()
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		h.Write([]byte(p))
		h.Write([]byte{0})
		h.Write(files[p])
		h.Write([]byte{1})
	}
	h.Write([]byte{2})
	for _, c := range cmds {
		h.Write([]byte(c))
		h.Write([]byte{3})
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// runScenarioTests copies repo/ once per editing agent, applies that
// agent's edits and runs every done-check in the resulting tree
// (spec/04 §2 rule 3: the suite still passes with the planted failure).
func runScenarioTests(s *scenario, v *violations, mu *sync.Mutex, cache repoTestCache) {
	where := s.Subset + "/" + s.ID
	if s.Script == nil || s.Comm == nil {
		return
	}
	for i := range s.Script.Agents {
		a := &s.Script.Agents[i]
		if len(agentEdits(a)) == 0 {
			continue
		}
		tmp, err := os.MkdirTemp("", "dbtest")
		if err != nil {
			mu.Lock()
			v.add("repo-tests", where, err.Error())
			mu.Unlock()
			return
		}
		dir := filepath.Join(tmp, "repo")
		err = copyTree(filepath.Join(s.Dir, "repo"), dir)
		if err == nil {
			_, err = applyAgentEdits(dir, a)
		}
		var key string
		var files map[string][]byte
		cmds := make([]string, 0, len(s.Comm.DoneChecks))
		for _, dc := range s.Comm.DoneChecks {
			cmds = append(cmds, dc.Cmd)
		}
		if err == nil {
			files, err = treeFiles(dir)
		}
		if err == nil {
			key = treeDigest(files, cmds)
			mu.Lock()
			cached, hit := cache[key]
			mu.Unlock()
			if hit {
				for _, dc := range s.Comm.DoneChecks {
					if brief := cached[dc.Cmd]; brief != "" {
						mu.Lock()
						v.addf("repo-tests", where, "agent %s: done-check %q failed post-edit: %s",
							a.ID, dc.Cmd, brief)
						mu.Unlock()
					}
				}
				os.RemoveAll(tmp)
				continue
			}
		}
		results := map[string]string{}
		for _, dc := range s.Comm.DoneChecks {
			if err != nil {
				break
			}
			res := runCmd(dir, dc.Cmd)
			if !res.ok {
				results[dc.Cmd] = res.brief()
				mu.Lock()
				v.addf("repo-tests", where, "agent %s: done-check %q failed post-edit: %s",
					a.ID, dc.Cmd, res.brief())
				mu.Unlock()
			} else {
				results[dc.Cmd] = ""
			}
		}
		if err == nil {
			mu.Lock()
			cache[key] = results
			mu.Unlock()
		}
		if err != nil {
			mu.Lock()
			v.addf("repo-tests", where, "agent %s: %s", a.ID, err.Error())
			mu.Unlock()
		}
		os.RemoveAll(tmp)
	}
}

type cmdResult struct {
	ok  bool
	out string
}

func (r cmdResult) brief() string {
	out := strings.TrimSpace(r.out)
	if len(out) > 300 {
		out = out[:300] + "…"
	}
	return out
}

// runCmd runs a done-check inside dir with the hermetic environment.
func runCmd(dir, cmd string) cmdResult {
	name, args := "sh", []string{"-c", cmd}
	if runtime.GOOS == "windows" {
		name, args = "cmd", []string{"/c", cmd}
	}
	c := exec.Command(name, args...)
	c.Dir = dir
	c.Env = append(os.Environ(),
		"GOPROXY=off", "GOFLAGS=-mod=mod", "CGO_ENABLED=0",
		"GOWORK=off",
	)
	out, err := c.CombinedOutput()
	res := cmdResult{ok: err == nil, out: string(out)}
	if err != nil && strings.TrimSpace(res.out) == "" {
		res.out = err.Error()
	}
	return res
}

// ---------------------------------------------------------------------------
// cross-scenario coverage (C3.1.1 sizes, C3.1.6 categories/variants,
// the five-mutation denied item)
// ---------------------------------------------------------------------------

func checkCoverage(all []*scenario, v *violations) {
	type scount struct{ planted, twins int }
	counts := map[string]*scount{"A": {}, "C": {}, "D": {}}
	catScen := map[string]map[string]bool{}
	variantN := map[string]int{}
	// the five-mutation cap (04 §2, denied): no template may be mutated
	// more than five times. The cap compares NORMALISED scripts — not
	// the template field, which is only an id — so two scenarios that
	// differ only in names and digits land in the same group.
	normGroups := map[string][]string{}
	for _, s := range all {
		c := counts[s.Subset]
		if c == nil {
			continue
		}
		if s.planted() {
			c.planted++
			if s.Script != nil {
				raw, err := os.ReadFile(filepath.Join(s.Dir, "script.yaml"))
				if err == nil {
					key := s.Subset + "/" + normaliseScript(string(raw))
					normGroups[key] = append(normGroups[key], s.ID)
				}
			}
		} else {
			c.twins++
		}
		if s.Subset == "A" && s.planted() {
			cats := map[string]bool{}
			for _, e := range s.Reg.Entries {
				cats[e.Category] = true
				if e.Variant != "" {
					variantN[e.Variant]++
				}
			}
			for c := range cats {
				if catScen[c] == nil {
					catScen[c] = map[string]bool{}
				}
				catScen[c][s.ID] = true
			}
		}
	}
	for sub, min := range subsetMin {
		c := counts[sub]
		if c.planted < min[0] {
			v.addf("coverage-count", sub, "planted scenarios %d < %d", c.planted, min[0])
		}
		if c.twins < min[1] {
			v.addf("coverage-count", sub, "clean twins %d < %d", c.twins, min[1])
		}
	}
	detCats := map[string]bool{}
	for _, c := range detectorCategory {
		detCats[c] = true
	}
	for c := range detCats {
		if n := len(catScen[c]); n < 4 {
			v.addf("coverage-category", "A", "category %s in %d planted scenarios (< 4)", c, n)
		}
	}
	for variant := range aVariants {
		if variantN[variant] < 8 {
			v.addf("coverage-variant", "A", "variant %s in %d planted scenarios (< 8)", variant, variantN[variant])
		}
	}
	for _, group := range normGroups {
		if len(group) > 5 {
			sort.Strings(group)
			v.addf("coverage-template", group[0],
				"%d scenarios share one normalised script (> 5): %v", len(group), group)
		}
	}
}

var normRunRe = regexp.MustCompile(`[@#]{2,}`)

// normaliseScript masks every identifier letter and every digit so that
// scripts differing only in names and numbers collapse to one form
// (spec/04 §2's five-mutation cap). What survives is structure: step
// kinds and counts, path depth, punctuation, token counts.
func normaliseScript(raw string) string {
	var b strings.Builder
	b.Grow(len(raw))
	for _, r := range raw {
		switch {
		case r >= '0' && r <= '9':
			b.WriteByte('#')
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_':
			b.WriteByte('@')
		default:
			b.WriteRune(r)
		}
	}
	return normRunRe.ReplaceAllString(b.String(), "@")
}

// strconv retained for id parsing helpers in future chunk checks.
var _ = strconv.Itoa
