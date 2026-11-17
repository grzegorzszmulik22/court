package main

// Reconciliation-trace validation (M3.1 build item 2, check C3.1.5).
//
// Every trace in test/vectors/reconcile/ is replayed through an
// independent implementation of the spec/01 §7 rules written here, from
// the spec text alone. The trace is valid when:
//   - every input entry appears in exactly one merged expectation,
//   - the merged decision's sources, verdict, flags and queued status
//     equal the oracle's,
//   - the listed decision.reconciled events cover every expectation with
//     strictly increasing seqs.
// This is not the production reconciler: that is built in M3.5 against
// these vectors, in the pattern M2.3 used for backbrief conformance.

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
		Expected  []rExpected      `json:"expected"`
		Decisions []rDecision      `json:"decisions"`
		Writs     map[string]rWrit `json:"writs"`
		Bypassed  []rBypass        `json:"bypassed"`
		Contested [][]string       `json:"contested"`
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
// (spec/03 §4.5): their detector hits are undeclared.
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
	flagCount := map[string]int{}
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
		checkTrace(tr, v)
		if seen[tr.ID] {
			v.addf("reconcile-id", root, "duplicate trace id %q", tr.ID)
		}
		seen[tr.ID] = true
		for _, t := range tr.Tags {
			tags[t]++
		}
		for _, e := range tr.Expect {
			verdictCount[e.Verdict]++
			for _, fl := range e.Flags {
				flagCount[fl+":"+e.Verdict]++
			}
		}
	}
	if n < 60 {
		v.addf("reconcile-count", root, "%d traces < 60", n)
	}
	// required coverage (M3.1 build text)
	for _, verdict := range []string{"confirmed", "declared-only", "undeclared", "late", "unanswered"} {
		if verdictCount[verdict] == 0 {
			v.addf("reconcile-coverage", root, "verdict %q never expected", verdict)
		}
	}
	for _, t := range []string{
		"late-one-commit",
		"unforeseen+confirmed", "unforeseen+declared-only",
		"unforeseen+undeclared", "unforeseen+late",
		"empty-paths", "empty-paths-competing",
		"other-overlap", "other-no-overlap",
		"outside-writ-guarantee", "expected-ruled", "gate-bypassed",
	} {
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

func checkTrace(tr *rtrace, v *violations) {
	where := "reconcile/" + tr.ID
	if tr.ID == "" {
		v.add("reconcile-id", where, "missing id")
	}
	for _, d := range tr.Inputs.Decisions {
		if d.Source != "declared" && d.Source != "detected" {
			v.addf("reconcile-source", where, "source %q must be declared or detected", d.Source)
		}
		if !categories[d.Category] {
			v.addf("reconcile-category", where, "category %q unknown", d.Category)
		}
		if d.Commit < 1 {
			v.addf("reconcile-commit", where, "decision %s: commit must be ≥ 1", d.ID)
		}
	}
	for _, e := range tr.Inputs.Expected {
		if !categories[e.Category] {
			v.addf("reconcile-category", where, "expected %s: category %q unknown", e.ID, e.Category)
		}
	}
	for _, e := range tr.Expect {
		if !reconVerdicts[e.Verdict] {
			v.addf("reconcile-verdict", where, "verdict %q unknown", e.Verdict)
		}
		for _, f := range e.Flags {
			if !reconFlags[f] {
				v.addf("reconcile-flag", where, "flag %q unknown", f)
			}
		}
		for _, s := range e.Sources {
			if !sourceSet[s] {
				v.addf("reconcile-source", where, "sources element %q unknown", s)
			}
		}
	}
	// events: seqs strictly increasing, ids disjoint, ids are input ids
	inIDs := map[string]bool{}
	for _, d := range tr.Inputs.Decisions {
		inIDs[d.ID] = true
	}
	for _, e := range tr.Inputs.Expected {
		inIDs[e.ID] = true
	}
	prev := 0
	evIDs := map[string]int{}
	for _, ev := range tr.Events {
		if ev.Seq <= prev {
			v.addf("reconcile-events", where, "event seqs not strictly increasing (%d)", ev.Seq)
		}
		prev = ev.Seq
		for _, id := range ev.IDs {
			if !inIDs[id] {
				v.addf("reconcile-events", where, "event names %q which is not an input", id)
			}
			if evIDs[id] != 0 {
				v.addf("reconcile-events", where, "%q appears in two reconciled events", id)
			}
			evIDs[id] = ev.Seq
		}
	}
	// expectations: disjoint coverage of all input ids
	expIDs := map[string]bool{}
	for _, e := range tr.Expect {
		if len(e.IDs) == 0 {
			v.addf("reconcile-expect", where, "an expectation has no member ids")
		}
		for _, id := range e.IDs {
			if !inIDs[id] {
				v.addf("reconcile-expect", where, "expected member %q is not an input", id)
			}
			if expIDs[id] {
				v.addf("reconcile-expect", where, "%q appears in two expectations", id)
			}
			expIDs[id] = true
			if evIDs[id] == 0 {
				v.addf("reconcile-events", where, "%q in no decision.reconciled event", id)
			}
		}
	}
	for id := range inIDs {
		if !expIDs[id] {
			v.addf("reconcile-expect", where, "input %q has no expectation", id)
		}
	}
	// replay through the spec oracle
	got := oracle(tr)
	for _, e := range tr.Expect {
		key := expectKey(e.IDs)
		g, ok := got[key]
		if !ok {
			v.addf("reconcile-oracle", where, "no expectation covers members %v (oracle groups differ)", e.IDs)
			continue
		}
		if !sameSet(g.Sources, e.Sources) {
			v.addf("reconcile-oracle", where, "%v: sources %v != oracle %v", e.IDs, e.Sources, g.Sources)
		}
		if g.Verdict != e.Verdict {
			v.addf("reconcile-oracle", where, "%v: verdict %q != oracle %q", e.IDs, e.Verdict, g.Verdict)
		}
		if !sameSet(g.Flags, e.Flags) {
			v.addf("reconcile-oracle", where, "%v: flags %v != oracle %v", e.IDs, e.Flags, g.Flags)
		}
		if g.Queued != e.Queued {
			v.addf("reconcile-oracle", where, "%v: queued %v != oracle %v", e.IDs, e.Queued, g.Queued)
		}
	}
	if len(got) != len(tr.Expect) {
		v.addf("reconcile-oracle", where, "oracle formed %d merged decisions, trace expects %d", len(got), len(tr.Expect))
	}
}

func expectKey(ids []string) string {
	s := append([]string{}, ids...)
	sort.Strings(s)
	return strings.Join(s, "+")
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	as, bs := append([]string{}, a...), append([]string{}, b...)
	sort.Strings(as)
	sort.Strings(bs)
	for i := range as {
		if as[i] != bs[i] {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// the spec/01 §7 oracle
// ---------------------------------------------------------------------------

// oracle replays a trace's inputs through §7 and returns one expectation
// per merged decision, keyed by sorted member ids.
func oracle(tr *rtrace) map[string]rExpect {
	// entries per world
	type entry struct {
		id       string
		source   string // declared | detected | expected
		category string
		paths    []string
		commit   int
		petition bool
		ruled    bool
	}
	worlds := map[string][]entry{}
	var worldOrder []string
	add := func(w string, e entry) {
		if _, ok := worlds[w]; !ok {
			worldOrder = append(worldOrder, w)
		}
		worlds[w] = append(worlds[w], e)
	}
	for _, d := range tr.Inputs.Decisions {
		w := d.World
		if w == "" {
			w = "w1"
		}
		add(w, entry{d.ID, d.Source, d.Category, d.Paths, d.Commit, d.Petition, false})
	}
	for _, e := range tr.Inputs.Expected {
		w := e.World
		if w == "" {
			w = "w1"
		}
		add(w, entry{e.ID, "expected", e.Category, e.Paths, 0, false, e.Ruled})
	}
	sort.Strings(worldOrder)

	bypassed := map[string]map[int]bool{}
	for _, b := range tr.Inputs.Bypassed {
		if bypassed[b.World] == nil {
			bypassed[b.World] = map[int]bool{}
		}
		for _, c := range b.Commits {
			bypassed[b.World][c] = true
		}
	}

	out := map[string]rExpect{}
	memberWorld := map[string]string{} // merged group key -> world
	for _, w := range worldOrder {
		ents := worlds[w]
		// union-find over entries that match under §7.1
		parent := make([]int, len(ents))
		for i := range parent {
			parent[i] = i
		}
		var find func(int) int
		find = func(i int) int {
			for parent[i] != i {
				parent[i] = parent[parent[i]]
				i = parent[i]
			}
			return i
		}
		union := func(a, b int) {
			parent[find(a)] = find(b)
		}
		match := func(a, b entry) bool {
			if a.category != b.category {
				return false
			}
			if rPathsOverlap(a.paths, b.paths) {
				return true
			}
			if a.category == "other" {
				return false // other needs overlapping paths, never empty-match
			}
			if len(a.paths) == 0 || len(b.paths) == 0 {
				// empty-paths clause: no competing entry of this category
				// with overlapping paths in the same world
				nonEmpty := a.paths
				if len(nonEmpty) == 0 {
					nonEmpty = b.paths
				}
				for _, c := range ents {
					if c.id == a.id || c.id == b.id || c.category != a.category {
						continue
					}
					if len(c.paths) > 0 && rPathsOverlap(c.paths, nonEmpty) {
						return false
					}
				}
				return true
			}
			return false
		}
		for i := range ents {
			for j := i + 1; j < len(ents); j++ {
				if match(ents[i], ents[j]) {
					union(i, j)
				}
			}
		}
		groups := map[int][]entry{}
		for i, e := range ents {
			groups[find(i)] = append(groups[find(i)], e)
		}
		for _, g := range groups {
			var ids, srcs []string
			srcSet := map[string]bool{}
			var exps []entry
			var decls, dets []entry
			for _, m := range g {
				ids = append(ids, m.id)
				if !srcSet[m.source] {
					srcSet[m.source] = true
					srcs = append(srcs, m.source)
				}
				switch m.source {
				case "expected":
					exps = append(exps, m)
				case "declared":
					decls = append(decls, m)
				case "detected":
					dets = append(dets, m)
				}
			}
			sort.Strings(ids)
			sort.Strings(srcs)
			e := rExpect{IDs: ids, Sources: srcs}
			flags := map[string]bool{}

			// verdict (§7.2)
			switch {
			case len(dets) == 0 && len(decls) == 0:
				e.Verdict = "unanswered"
			case len(dets) == 0:
				e.Verdict = "declared-only"
			default:
				// any hit on a bypassed commit is undeclared (03 §4.5)
				onBypass := false
				earliestHit := 1 << 30
				for _, h := range dets {
					if bypassed[w][h.commit] {
						onBypass = true
					}
					if h.commit < earliestHit {
						earliestHit = h.commit
					}
				}
				timely := false
				var latestTimely int
				for _, d := range decls {
					if d.commit <= earliestHit {
						timely = true
						if d.commit > latestTimely {
							latestTimely = d.commit
						}
					}
				}
				switch {
				case onBypass:
					e.Verdict = "undeclared"
				case len(decls) == 0:
					e.Verdict = "undeclared"
				case timely:
					e.Verdict = "confirmed"
				default:
					e.Verdict = "late"
					flags["late"] = true
				}
			}

			// flags (§11.2): unforeseen, outside-writ, ruled
			if len(exps) == 0 {
				flags["unforeseen"] = true
			}
			// ruled: an expected member settled in the hearing passes its
			// ruling to the decision, unless the category is a guarantee
			// (01 §6)
			ruled := false
			for _, x := range exps {
				if x.ruled {
					ruled = true
				}
			}
			cat := g[0].category
			if ruled && !guaranteeCategories[cat] {
				flags["ruled"] = true
			}
			// outside-writ: category or any path outside the world's writ
			wr, hasWrit := tr.Inputs.Writs[w]
			outside := false
			if !hasWrit {
				outside = guaranteeCategories[cat]
			} else {
				outside = guaranteeCategories[cat] || !contains(wr.Categories, cat)
				if !outside {
					for _, p := range g[0].paths {
						if !writTerritoryCovers(wr.Territory, p) {
							outside = true
						}
					}
				}
			}
			if outside {
				flags["outside-writ"] = true
			}

			// queued (§7.3)
			e.Queued = e.Verdict == "undeclared" || e.Verdict == "late" ||
				e.Verdict == "unanswered" || flags["outside-writ"]
			for _, d := range decls {
				if d.petition {
					e.Queued = true
				}
			}

			for f := range flags {
				e.Flags = append(e.Flags, f)
			}
			sort.Strings(e.Flags)
			key := expectKey(ids)
			out[key] = e
			memberWorld[key] = w
		}
	}

	// contested pairs (§7.5): both members queued and flagged
	for _, pair := range tr.Inputs.Contested {
		if len(pair) != 2 {
			continue
		}
		for _, id := range pair {
			for key, e := range out {
				for _, mid := range e.IDs {
					if mid == id {
						e.Queued = true
						e.Flags = append(e.Flags, "contested")
						sort.Strings(e.Flags)
						out[key] = e
					}
				}
			}
		}
	}
	_ = memberWorld
	return out
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
