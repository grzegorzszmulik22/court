package main

// Reconciliation-trace validation (M3.1 build item 2, check C3.1.5).
//
// Per CR-003 (spec/CHANGE_REQUESTS/003) this validator contains NO
// reconciliation oracle: the expected results in
// test/vectors/reconcile/ were written by reasoning from spec/01 §7 and
// are reviewed independently, then checked again by the independent
// M3.5 implementation. What this file enforces is trace shape:
//   - file and id naming, vocabulary of categories/verdicts/flags/tags;
//   - every input entry appears in exactly one merged expectation and
//     the expectation's sources equal the union of its members';
//   - per-row internal consistency derivable from §7.2/§7.3 without
//     re-running matching: verdict vs member sources, flags vs inputs,
//     queued vs verdict+flags+petition;
//   - decision.reconciled events: strictly increasing seqs, one event
//     per expectation listing exactly its member ids;
//   - every tag is a known name that is true of its trace;
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
		Contested   [][]string       `json:"contested"`
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

// rDecision is a declared or detected entry.
type rDecision struct {
	ID       string   `json:"id"`
	Source   string   `json:"source"` // declared | detected
	Category string   `json:"category"`
	Paths    []string `json:"paths"`
	World    string   `json:"world"`
	Commit   int      `json:"commit"`   // 1-based commit index in its world
	Petition bool     `json:"petition"` // declared with petition: true
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
		if _, dup := members[d.ID]; dup {
			v.addf("reconcile-input-id", where, "input id %q used twice", d.ID)
		}
		members[d.ID] = traceMember{d.ID, d.Source, d.Category, d.Paths, w, d.Commit, d.Petition, false}
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
		members[e.ID] = traceMember{e.ID, "expected", e.Category, e.Paths, w, 0, false, e.Ruled}
	}
	for _, pair := range tr.Inputs.Contested {
		if len(pair) != 2 {
			v.addf("reconcile-contested", where, "contested pair %v must name exactly two ids", pair)
			continue
		}
		a, aok := members[pair[0]]
		b, bok := members[pair[1]]
		if !aok || !bok {
			v.addf("reconcile-contested", where, "contested pair %v names a non-input id", pair)
			continue
		}
		if a.world == b.world {
			v.addf("reconcile-contested", where, "contested pair %v is in one world (%s)", pair, a.world)
		}
		if a.category != b.category {
			v.addf("reconcile-contested", where, "contested pair %v crosses categories", pair)
		}
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

// checkExpectRow enforces everything §7.2/§7.3 fixes about one merged
// decision WITHOUT re-running the matcher's grouping: verdict vs member
// sources, flags vs inputs, queued vs the §7.3 clause list.
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
	// sources must equal the union of member sources, sorted
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
	// verdict vs member sources (§7.2 table)
	hasDecl, hasDet, hasExp := srcSet["declared"], srcSet["detected"], srcSet["expected"]
	switch {
	case !hasDet && !hasDecl:
		if e.Verdict != "unanswered" {
			v.addf("reconcile-verdict", where, "%v: expected-only members must be unanswered, not %q", e.IDs, e.Verdict)
		}
	case hasDet && !hasDecl:
		if e.Verdict != "undeclared" {
			v.addf("reconcile-verdict", where, "%v: detected without declaration must be undeclared, not %q", e.IDs, e.Verdict)
		}
	case !hasDet && hasDecl:
		if e.Verdict != "declared-only" {
			v.addf("reconcile-verdict", where, "%v: declared without detection must be declared-only, not %q", e.IDs, e.Verdict)
		}
	default: // declared + detected present
		if e.Verdict != "confirmed" && e.Verdict != "undeclared" && e.Verdict != "late" {
			v.addf("reconcile-verdict", where, "%v: declared+detected must be confirmed/undeclared/late, not %q", e.IDs, e.Verdict)
		}
	}
	flag := map[string]bool{}
	for _, f := range e.Flags {
		if !reconFlags[f] {
			v.addf("reconcile-flag", where, "flag %q unknown", f)
		}
		flag[f] = true
	}
	// late flag iff late verdict
	if flag["late"] != (e.Verdict == "late") {
		v.addf("reconcile-flag", where, "%v: late flag %v disagrees with verdict %q", e.IDs, flag["late"], e.Verdict)
	}
	// unforeseen flag iff no expected member
	if flag["unforeseen"] == hasExp {
		v.addf("reconcile-flag", where, "%v: unforeseen %v disagrees with expected membership", e.IDs, flag["unforeseen"])
	}
	// contested flag iff a member is named in an inputs.contested pair
	inContest := map[string]bool{}
	for _, pair := range tr.Inputs.Contested {
		for _, id := range pair {
			inContest[id] = true
		}
	}
	memberContested := false
	for _, m := range ms {
		if inContest[m.id] {
			memberContested = true
		}
	}
	if flag["contested"] != memberContested {
		v.addf("reconcile-flag", where, "%v: contested %v disagrees with inputs.contested", e.IDs, flag["contested"])
	}
	// outside-writ flag iff the merged decision is outside its writ:
	// guarantee categories always, else the member world's writ must
	// list the category and cover every member path
	outside := guaranteeCategories[cat]
	if !outside {
		if wr, ok := tr.Inputs.Writs[ms[0].world]; ok {
			if !contains(wr.Categories, cat) {
				outside = true
			} else {
				for _, m := range ms {
					for _, p := range m.paths {
						if !writTerritoryCovers(wr.Territory, p) {
							outside = true
						}
					}
				}
			}
		}
	}
	if flag["outside-writ"] != outside {
		v.addf("reconcile-flag", where, "%v: outside-writ %v disagrees with category/writ inputs", e.IDs, flag["outside-writ"])
	}
	// ruled flag iff a ruled expected member merged and the category is
	// not a guarantee category (guarantees always get their own ruling)
	ruled := false
	for _, m := range ms {
		if m.ruled {
			ruled = true
		}
	}
	if flag["ruled"] != (ruled && !guaranteeCategories[cat]) {
		v.addf("reconcile-flag", where, "%v: ruled %v disagrees with expected member rulings", e.IDs, flag["ruled"])
	}
	// queued iff any §7.3 clause holds
	queued := e.Verdict == "undeclared" || e.Verdict == "late" || e.Verdict == "unanswered" ||
		flag["outside-writ"] || flag["contested"]
	for _, m := range ms {
		if m.petition {
			queued = true
		}
	}
	if e.Queued != queued {
		v.addf("reconcile-queued", where, "%v: queued %v, §7.3 requires %v", e.IDs, e.Queued, queued)
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
			return hasExp(func(e rExpect) bool {
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
		"contested":     func() bool { return hasExp(expFlag("contested")) },
		"no-contest": func() bool {
			return len(tr.Inputs.Contested) == 0 && !hasExp(expFlag("contested"))
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
			return hasExp(func(e rExpect) bool {
				c := memberCat(e)
				return c == "behaviour" || c == "structure"
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
