package main

// Reconciliation-trace validation (M3.1 build item 2, check C3.1.5).
//
// Per CR-003 (spec/CHANGE_REQUESTS/003) this validator contains NO
// reconciliation oracle and computes NO expected result: verdicts,
// flags and queued status in test/vectors/reconcile/ were written by
// reasoning from spec/01 §7, are reviewed independently, and are
// checked again by the independent M3.5 implementation. What this file
// enforces is trace SHAPE:
//   - file and id naming, vocabulary of categories/verdicts/flags/tags;
//   - entry kinds: a detected entry carries real paths and hit text
//     digests (03 §1.5); declared/expected entries carry neither;
//   - every input entry appears in exactly one merged expectation and
//     the expectation's sources equal the union of its members';
//   - decision.reconciled events: strictly increasing seqs, one event
//     per expectation listing exactly its member ids;
//   - every tag is a known name whose truth can be read from the
//     trace's own inputs and expectation rows;
//   - corpus coverage: >= 60 traces, every verdict and every required
//     case exercised.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ---------------------------------------------------------------------------
// trace schema (documented in test/vectors/reconcile/README.md)
// ---------------------------------------------------------------------------

type rtrace struct {
	ID      string `json:"id"`
	Comment string `json:"comment"`
	Inputs  struct {
		Expected    []rExpected      `json:"expected"`
		Decisions   []rDecision      `json:"decisions"`
		Writs       map[string]rWrit `json:"writs"`
		Bypassed    []rBypass        `json:"bypassed"`
		NoDecisions []rBypass        `json:"no_decisions"`
	} `json:"inputs"`
	Events []rEvent  `json:"events"`
	Expect []rExpect `json:"expect"`
	Tags   []string  `json:"tags"`
}

// rExpected is a backbrief.expected entry.
type rExpected struct {
	ID       string   `json:"id"`
	Category string   `json:"category"`
	Paths    []string `json:"paths"`
	World    string   `json:"world"`
	Ruled    bool     `json:"ruled"` // settled in the upfront hearing (01 §6)
}

// rDecision is a declared or detected entry. Detected entries carry
// `digests` — the sha256 text digests of the hits they stand for —
// which are the evidence 01 §7.5 compares to derive contests.
type rDecision struct {
	ID       string   `json:"id"`
	Source   string   `json:"source"` // declared | detected
	Category string   `json:"category"`
	Paths    []string `json:"paths"`
	World    string   `json:"world"`
	Commit   int      `json:"commit"`   // 1-based commit index in its world
	Petition bool     `json:"petition"` // declared with petition: true
	Digests  []string `json:"digests"`  // hit text digests (detected only)
}

type rWrit struct {
	Categories []string `json:"categories"`
	Territory  []string `json:"territory"`
}

// rBypass marks commits that reached the world with no gate record
// (spec/03 §4.5): their detector hits are undeclared. The same shape is
// reused for inputs.no_decisions: commits where the agent recorded a
// no_decisions statement (a gate fact, not a decision entry).
type rBypass struct {
	World   string `json:"world"`
	Commits []int  `json:"commits"`
}

// rEvent is a decision.reconciled event: the ids of the input entries
// that merged into the decision it describes.
type rEvent struct {
	Seq int      `json:"seq"`
	IDs []string `json:"decision_ids"`
}

type rExpect struct {
	IDs     []string `json:"ids"`     // member input ids, one merged decision
	Sources []string `json:"sources"` // sorted union
	Verdict string   `json:"verdict"`
	Flags   []string `json:"flags"`
	Queued  bool     `json:"queued"`
	Note    string   `json:"note"`
}

var traceFileRe = regexp.MustCompile(`^T-\d+\.json$`)

var reconVerdicts = map[string]bool{
	"confirmed": true, "declared-only": true, "undeclared": true,
	"late": true, "unanswered": true,
}

var reconFlags = map[string]bool{
	"late": true, "unforeseen": true, "outside-writ": true, "ruled": true,
	"contested": true,
}

var sourceSet = map[string]bool{"declared": true, "detected": true, "expected": true}

// tags whose truth the validator can check from the trace's inputs and
// expectation rows. Any tag outside this vocabulary is a violation.
var knownTags = map[string]bool{
	"verdict-confirmed": true, "verdict-declared-only": true,
	"verdict-undeclared": true, "verdict-late": true,
	"verdict-unanswered":   true,
	"unforeseen+confirmed": true, "unforeseen+declared-only": true,
	"unforeseen+undeclared": true, "unforeseen+late": true,
	"foreseen-confirmed": true,
	"empty-paths":        true, "empty-paths-competing": true,
	"other-overlap": true, "other-no-overlap": true,
	"outside-writ": true, "outside-writ-guarantee": true,
	"inside-writ": true, "expected-ruled": true,
	"expected-declared": true, "expected-detected": true,
	"gate-bypassed": true, "no-decisions": true,
	"contested": true, "no-contest": true, "petition": true,
	"timing": true, "late-one-commit": true,
	"merge": true, "three-way": true, "per-world": true,
	"glob": true, "dir-containment": true, "disjoint-paths": true,
	"no-cross-category": true, "no-detector-category": true,
	"multi-commit": true, "multi-category": true,
}

// requiredReconcileCases are the corpus-coverage tags/verdicts M3.1's
// build text requires. Every one must appear at least once.
var requiredReconcileTags = []string{
	"late-one-commit",
	"unforeseen+confirmed", "unforeseen+declared-only",
	"unforeseen+undeclared", "unforeseen+late",
	"empty-paths", "empty-paths-competing",
	"other-overlap", "other-no-overlap",
	"outside-writ-guarantee", "expected-ruled", "gate-bypassed",
}

func validateReconcile(root string, v *violations) {
	fi, err := os.ReadDir(root)
	if err != nil {
		v.add("reconcile-missing", root, err.Error())
		return
	}
	n := 0
	seen := map[string]bool{}
	tags := map[string]int{}
	verdictCount := map[string]int{}
	for _, f := range fi {
		if !traceFileRe.MatchString(f.Name()) {
			if strings.HasSuffix(f.Name(), ".json") {
				v.addf("reconcile-name", root, "file %s is not T-NN.json", f.Name())
			}
			continue
		}
		n++
		tr := loadTrace(filepath.Join(root, f.Name()), v)
		if tr == nil {
			continue
		}
		checkTrace(tr, f.Name(), v)
		if seen[tr.ID] {
			v.addf("reconcile-id", root, "duplicate trace id %q", tr.ID)
		}
		seen[tr.ID] = true
		for _, t := range tr.Tags {
			tags[t]++
		}
		for _, e := range tr.Expect {
			verdictCount[e.Verdict]++
		}
	}
	if n < 60 {
		v.addf("reconcile-count", root, "%d traces < 60", n)
	}
	for _, verdict := range []string{"confirmed", "declared-only", "undeclared", "late", "unanswered"} {
		if verdictCount[verdict] == 0 {
			v.addf("reconcile-coverage", root, "verdict %q never expected", verdict)
		}
	}
	for _, t := range requiredReconcileTags {
		if tags[t] == 0 {
			v.addf("reconcile-coverage", root, "required case %q never exercised", t)
		}
	}
}

func loadTrace(path string, v *violations) *rtrace {
	raw, err := os.ReadFile(path)
	if err != nil {
		v.add("reconcile-read", path, err.Error())
		return nil
	}
	var tr rtrace
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&tr); err != nil {
		v.add("reconcile-parse", filepath.Base(path), err.Error())
		return nil
	}
	return &tr
}

// ---------------------------------------------------------------------------
// per-trace checks
// ---------------------------------------------------------------------------

var digestRe = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// traceMember is one input entry resolved for checking an expectation.
type traceMember struct {
	id       string
	source   string
	category string
	paths    []string
	world    string
	commit   int
	petition bool
	ruled    bool
	digests  []string
}

func checkTrace(tr *rtrace, fname string, v *violations) {
	where := "reconcile/" + tr.ID
	if tr.ID == "" {
		v.add("reconcile-id", where, "missing id")
	} else if strings.TrimSuffix(fname, ".json") != tr.ID {
		v.addf("reconcile-id", where, "file %s does not match id %q", fname, tr.ID)
	}
	members := map[string]traceMember{}
	for _, d := range tr.Inputs.Decisions {
		w := d.World
		if w == "" {
			w = "w1"
		}
		if d.Source != "declared" && d.Source != "detected" {
			v.addf("reconcile-source", where, "source %q must be declared or detected", d.Source)
		}
		if !categories[d.Category] {
			v.addf("reconcile-category", where, "category %q unknown", d.Category)
		}
		if d.Commit < 1 {
			v.addf("reconcile-commit", where, "decision %s: commit must be ≥ 1", d.ID)
		}
		// 03 §1.5: a hit carries file, line range and the matched text
		// digest — a detected entry therefore names real paths and at
		// least one sha256 digest; a declaration has no hit and may not
		// carry digests.
		if d.Source == "detected" {
			if len(d.Paths) == 0 {
				v.addf("reconcile-detected-paths", where,
					"detected entry %s has empty paths (a hit always has a span)", d.ID)
			}
			if len(d.Digests) == 0 {
				v.addf("reconcile-digest", where,
					"detected entry %s carries no hit text digest", d.ID)
			}
		} else if len(d.Digests) != 0 {
			v.addf("reconcile-digest", where, "entry %s is not detected but carries digests", d.ID)
		}
		for _, dg := range d.Digests {
			if !digestRe.MatchString(dg) {
				v.addf("reconcile-digest", where, "entry %s: digest %q is not sha256:<64-hex>", d.ID, dg)
			}
		}
		if _, dup := members[d.ID]; dup {
			v.addf("reconcile-input-id", where, "input id %q used twice", d.ID)
		}
		members[d.ID] = traceMember{d.ID, d.Source, d.Category, d.Paths, w, d.Commit, d.Petition, false, d.Digests}
	}
	for _, e := range tr.Inputs.Expected {
		w := e.World
		if w == "" {
			w = "w1"
		}
		if !categories[e.Category] {
			v.addf("reconcile-category", where, "expected %s: category %q unknown", e.ID, e.Category)
		}
		if _, dup := members[e.ID]; dup {
			v.addf("reconcile-input-id", where, "input id %q used twice", e.ID)
		}
		members[e.ID] = traceMember{e.ID, "expected", e.Category, e.Paths, w, 0, false, e.Ruled, nil}
	}
	for _, g := range tr.Inputs.Bypassed {
		for _, c := range g.Commits {
			if c < 1 {
				v.addf("reconcile-bypass", where, "bypassed commit %d < 1", c)
			}
		}
	}
	for _, g := range tr.Inputs.NoDecisions {
		for _, c := range g.Commits {
			if c < 1 {
				v.addf("reconcile-no-decisions", where, "no_decisions commit %d < 1", c)
			}
		}
	}
	for wname, wr := range tr.Inputs.Writs {
		for _, c := range wr.Categories {
			if !categories[c] {
				v.addf("reconcile-writ", where, "writ %s names unknown category %q", wname, c)
			}
		}
		if len(wr.Territory) == 0 {
			v.addf("reconcile-writ", where, "writ %s has empty territory", wname)
		}
	}

	// expectations: disjoint coverage of every input id
	expIDs := map[string]bool{}
	for _, e := range tr.Expect {
		if len(e.IDs) == 0 {
			v.addf("reconcile-expect", where, "an expectation has no member ids")
		}
		for _, id := range e.IDs {
			if _, ok := members[id]; !ok {
				v.addf("reconcile-expect", where, "expected member %q is not an input", id)
				continue
			}
			if expIDs[id] {
				v.addf("reconcile-expect", where, "%q appears in two expectations", id)
			}
			expIDs[id] = true
		}
		checkExpectRow(tr, e, members, where, v)
	}
	for id := range members {
		if !expIDs[id] {
			v.addf("reconcile-expect", where, "input %q has no expectation", id)
		}
	}

	// events: one decision.reconciled per expectation, member ids equal
	evSets := map[string]int{}
	prev := 0
	for _, ev := range tr.Events {
		if ev.Seq <= prev {
			v.addf("reconcile-events", where, "event seqs not strictly increasing (%d)", ev.Seq)
		}
		prev = ev.Seq
		key := idSetKey(ev.IDs)
		if evSets[key] != 0 {
			v.addf("reconcile-events", where, "two events carry the same member set %v", ev.IDs)
		}
		evSets[key] = ev.Seq
		for _, id := range ev.IDs {
			if _, ok := members[id]; !ok {
				v.addf("reconcile-events", where, "event names %q which is not an input", id)
			}
		}
	}
	expSets := map[string]bool{}
	for _, e := range tr.Expect {
		expSets[idSetKey(e.IDs)] = true
	}
	for _, ev := range tr.Events {
		if !expSets[idSetKey(ev.IDs)] {
			v.addf("reconcile-events", where, "event members %v are no expectation's member set", ev.IDs)
		}
	}
	for key := range expSets {
		if evSets[key] == 0 {
			v.addf("reconcile-events", where, "no decision.reconciled event for members %s", key)
		}
	}

	checkTraceTags(tr, members, where, v)
}

// checkExpectRow validates one merged decision's SHAPE only — member
// ids resolve, members share one category and one world (a merged
// decision is a single-category, single-world object by construction),
// the row's denormalised sources field equals the member union, and
// verdict/flag words come from the §7 vocabulary. It computes NO
// expected result: whether the verdict, flags or queued status are
// right is ground truth written by reasoning from §7 and checked by
// the independent M3.5 implementation (CR-003).
func checkExpectRow(tr *rtrace, e rExpect, members map[string]traceMember, where string, v *violations) {
	var ms []traceMember
	srcSet := map[string]bool{}
	for _, id := range e.IDs {
		m, ok := members[id]
		if !ok {
			continue
		}
		ms = append(ms, m)
		srcSet[m.source] = true
	}
	if len(ms) == 0 {
		return // ids errors already reported
	}
	cat := ms[0].category
	for _, m := range ms[1:] {
		if m.category != cat {
			v.addf("reconcile-merge", where, "%v merges categories %q and %q", e.IDs, cat, m.category)
		}
		if m.world != ms[0].world {
			v.addf("reconcile-merge", where, "%v merges worlds %q and %q", e.IDs, ms[0].world, m.world)
		}
	}
	// sources must equal the union of member sources, sorted — a
	// denormalised field of the merged row, not a derived result.
	var wantSrc []string
	for _, s := range []string{"declared", "detected", "expected"} {
		if srcSet[s] {
			wantSrc = append(wantSrc, s)
		}
	}
	if !sameStrings(wantSrc, e.Sources) {
		v.addf("reconcile-sources", where, "%v: sources %v, member union is %v", e.IDs, e.Sources, wantSrc)
	}
	if !reconVerdicts[e.Verdict] {
		v.addf("reconcile-verdict", where, "verdict %q unknown", e.Verdict)
	}
	for _, f := range e.Flags {
		if !reconFlags[f] {
			v.addf("reconcile-flag", where, "flag %q unknown", f)
		}
	}
}

// ---------------------------------------------------------------------------
// tag truthfulness: every tag must be a known name that is true of the
// trace's own inputs and expectation rows
// ---------------------------------------------------------------------------

func checkTraceTags(tr *rtrace, members map[string]traceMember, where string, v *violations) {
	var ins []traceMember
	for _, m := range members {
		ins = append(ins, m)
	}
	hasExp := func(pred func(rExpect) bool) bool {
		for _, e := range tr.Expect {
			if pred(e) {
				return true
			}
		}
		return false
	}
	expFlag := func(name string) func(rExpect) bool {
		return func(e rExpect) bool {
			for _, f := range e.Flags {
				if f == name {
					return true
				}
			}
			return false
		}
	}
	expVerdict := func(name string) func(rExpect) bool {
		return func(e rExpect) bool { return e.Verdict == name }
	}
	expBoth := func(a, b func(rExpect) bool) func(rExpect) bool {
		return func(e rExpect) bool { return a(e) && b(e) }
	}
	memberCat := func(e rExpect) string {
		for _, id := range e.IDs {
			if m, ok := members[id]; ok {
				return m.category
			}
		}
		return ""
	}
	truth := map[string]func() bool{
		"verdict-confirmed":        func() bool { return hasExp(expVerdict("confirmed")) },
		"verdict-declared-only":    func() bool { return hasExp(expVerdict("declared-only")) },
		"verdict-undeclared":       func() bool { return hasExp(expVerdict("undeclared")) },
		"verdict-late":             func() bool { return hasExp(expVerdict("late")) },
		"verdict-unanswered":       func() bool { return hasExp(expVerdict("unanswered")) },
		"unforeseen+confirmed":     func() bool { return hasExp(expBoth(expVerdict("confirmed"), expFlag("unforeseen"))) },
		"unforeseen+declared-only": func() bool { return hasExp(expBoth(expVerdict("declared-only"), expFlag("unforeseen"))) },
		"unforeseen+undeclared":    func() bool { return hasExp(expBoth(expVerdict("undeclared"), expFlag("unforeseen"))) },
		"unforeseen+late":          func() bool { return hasExp(expBoth(expVerdict("late"), expFlag("unforeseen"))) },
		"foreseen-confirmed": func() bool {
			return hasExp(func(e rExpect) bool {
				if e.Verdict != "confirmed" {
					return false
				}
				for _, f := range e.Flags {
					if f == "unforeseen" {
						return false
					}
				}
				return true
			})
		},
		"empty-paths": func() bool {
			for _, m := range ins {
				if len(m.paths) == 0 {
					return true
				}
			}
			return false
		},
		"empty-paths-competing": func() bool {
			for _, m := range ins {
				if len(m.paths) != 0 {
					continue
				}
				others := 0
				for _, o := range ins {
					if o.id != m.id && o.category == m.category && o.world == m.world {
						others++
					}
				}
				if others >= 2 {
					return true
				}
			}
			return false
		},
		"other-overlap": func() bool {
			return hasExp(func(e rExpect) bool {
				return memberCat(e) == "other" && len(e.IDs) > 1
			})
		},
		"other-no-overlap": func() bool {
			// two "other" inputs in the same world that did not merge
			seen := map[string]int{}
			for _, e := range tr.Expect {
				for _, id := range e.IDs {
					if m, ok := members[id]; ok && m.category == "other" {
						seen[m.world]++
					}
				}
			}
			for _, c := range seen {
				if c >= 2 {
					return true
				}
			}
			return false
		},
		"outside-writ": func() bool { return hasExp(expFlag("outside-writ")) },
		"outside-writ-guarantee": func() bool {
			// The tag is about §6/§7.4 guarantees being ruled in their
			// own hearing: a guarantee-category row lands outside the
			// writ AND a declared decision exists somewhere in the
			// trace — a detected-only trace exercises the outside-writ
			// path but not the guarantee declaration.
			declared := false
			for _, m := range ins {
				if m.source == "declared" {
					declared = true
				}
			}
			return declared && hasExp(func(e rExpect) bool {
				if !expFlag("outside-writ")(e) {
					return false
				}
				return guaranteeCategories[memberCat(e)]
			})
		},
		"inside-writ": func() bool {
			return hasExp(func(e rExpect) bool {
				for _, f := range e.Flags {
					if f == "outside-writ" {
						return false
					}
				}
				return true
			})
		},
		"expected-ruled": func() bool {
			for _, m := range ins {
				if m.ruled {
					return true
				}
			}
			return false
		},
		"expected-declared": func() bool {
			return hasExp(func(e rExpect) bool {
				var d, x bool
				for _, s := range e.Sources {
					if s == "declared" {
						d = true
					}
					if s == "expected" {
						x = true
					}
				}
				return d && x
			})
		},
		"expected-detected": func() bool {
			return hasExp(func(e rExpect) bool {
				var d, x bool
				for _, s := range e.Sources {
					if s == "detected" {
						d = true
					}
					if s == "expected" {
						x = true
					}
				}
				return d && x
			})
		},
		"gate-bypassed": func() bool { return len(tr.Inputs.Bypassed) > 0 },
		"no-decisions":  func() bool { return len(tr.Inputs.NoDecisions) > 0 },
		"contested": func() bool {
			// The tag claims the trace exercises a contest: a flag row
			// exists AND the inputs contain a contest-capable pair —
			// two same-category decisions in different worlds whose
			// evidence differs through hits (different digest sets, or
			// a hit against a bare declaration). Two declaration-only
			// decisions can never contest (01 §7.5).
			if !hasExp(expFlag("contested")) {
				return false
			}
			return hasPair(ins, func(a, b traceMember) bool {
				return a.world != b.world && a.category == b.category && contestingEvidence(a, b)
			})
		},
		"no-contest": func() bool {
			// True when the trace has a cross-world same-category pair
			// that CANNOT contest — both declared, or detected with
			// identical hit digests — and no row carries the flag.
			if hasExp(expFlag("contested")) {
				return false
			}
			return hasPair(ins, func(a, b traceMember) bool {
				if a.world == b.world || a.category != b.category ||
					a.source == "expected" || b.source == "expected" {
					return false
				}
				return !contestingEvidence(a, b)
			})
		},
		"petition": func() bool {
			for _, m := range ins {
				if m.petition {
					return true
				}
			}
			return false
		},
		"timing": func() bool {
			return hasExp(func(e rExpect) bool {
				var d, h bool
				for _, s := range e.Sources {
					if s == "declared" {
						d = true
					}
					if s == "detected" {
						h = true
					}
				}
				return d && h
			})
		},
		"late-one-commit": func() bool {
			return hasExp(func(e rExpect) bool {
				if e.Verdict != "late" {
					return false
				}
				var ds, hs []traceMember
				for _, id := range e.IDs {
					if m, ok := members[id]; ok {
						if m.source == "declared" {
							ds = append(ds, m)
						}
						if m.source == "detected" {
							hs = append(hs, m)
						}
					}
				}
				for _, d := range ds {
					for _, h := range hs {
						if d.world == h.world && d.commit == h.commit+1 {
							return true
						}
					}
				}
				return false
			})
		},
		"merge": func() bool {
			return hasExp(func(e rExpect) bool { return len(e.IDs) >= 2 })
		},
		"three-way": func() bool {
			return hasExp(func(e rExpect) bool { return len(e.IDs) >= 3 })
		},
		"per-world": func() bool {
			w := map[string]bool{}
			for _, m := range ins {
				w[m.world] = true
			}
			return len(w) >= 2
		},
		"glob": func() bool {
			for _, m := range ins {
				for _, p := range m.paths {
					if strings.Contains(p, "*") {
						return true
					}
				}
			}
			return false
		},
		"dir-containment": func() bool {
			for _, m := range ins {
				for _, o := range ins {
					if m.id == o.id || m.category != o.category || m.world != o.world {
						continue
					}
					for _, p := range m.paths {
						for _, q := range o.paths {
							if dirContains(p, q) || dirContains(q, p) {
								return true
							}
						}
					}
				}
			}
			return false
		},
		"disjoint-paths": func() bool {
			for _, m := range ins {
				for _, o := range ins {
					if m.id == o.id || m.category != o.category || m.world != o.world {
						continue
					}
					if len(m.paths) > 0 && len(o.paths) > 0 && !rPathsOverlap(m.paths, o.paths) {
						return true
					}
				}
			}
			return false
		},
		"no-cross-category": func() bool {
			c := map[string]bool{}
			for _, m := range ins {
				c[m.category] = true
			}
			return len(c) >= 2
		},
		"no-detector-category": func() bool {
			// Categories with no detector in 03 §2: a decision in one
			// can only ever be declared or expected.
			return hasExp(func(e rExpect) bool {
				c := memberCat(e)
				return c == "behaviour" || c == "structure" || c == "other"
			})
		},
		"multi-commit": func() bool {
			seen := map[string]map[int]bool{}
			for _, m := range ins {
				if m.source == "expected" {
					continue
				}
				if seen[m.world] == nil {
					seen[m.world] = map[int]bool{}
				}
				seen[m.world][m.commit] = true
			}
			for _, cs := range seen {
				if len(cs) >= 2 {
					return true
				}
			}
			return false
		},
		"multi-category": func() bool {
			c := map[string]bool{}
			n := 0
			for _, m := range ins {
				if m.source == "expected" {
					continue
				}
				c[m.category] = true
				n++
			}
			return n >= 2 && len(c) >= 2
		},
	}
	for _, tag := range tr.Tags {
		if !knownTags[tag] {
			v.addf("reconcile-tag", where, "tag %q is not a known coverage tag", tag)
			continue
		}
		if ok := truth[tag]; ok != nil && !ok() {
			v.addf("reconcile-tag", where, "tag %q is not true of this trace", tag)
		}
	}
}

// hasPair reports whether any two members satisfy pred (order-free).
func hasPair(ins []traceMember, pred func(a, b traceMember) bool) bool {
	for i := range ins {
		for j := i + 1; j < len(ins); j++ {
			if pred(ins[i], ins[j]) || pred(ins[j], ins[i]) {
				return true
			}
		}
	}
	return false
}

// contestingEvidence: 01 §7.5 — two decisions contest only when their
// evidence differs through hits: detected-vs-detected with different
// digest sets, or a detected hit on one side against a bare
// declaration on the other. Declaration-vs-declaration never contests.
func contestingEvidence(a, b traceMember) bool {
	da, db := a.source == "detected", b.source == "detected"
	if da != db {
		return true
	}
	if !da {
		return false
	}
	return !sameStrings(a.digests, b.digests)
}

func idSetKey(ids []string) string {
	s := append([]string{}, ids...)
	sort.Strings(s)
	return strings.Join(s, "+")
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// rPathsOverlap: §7.1 — any common path, directory containment, or glob
// intersection. Two empty path lists do NOT overlap; the empty clause is
// evaluated separately in the matcher.
func rPathsOverlap(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y || dirContains(x, y) || dirContains(y, x) || globsIntersect(x, y) {
				return true
			}
		}
	}
	return false
}
