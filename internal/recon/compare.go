// Package recon holds the deterministic comparison of two
// expected-decision lists under spec/01 §7.1: entries match on category
// and paths, never on question text.
package recon

import (
	"strings"

	"court/internal/ledger"
	"court/internal/paths"
)

// Comparison is the diff of two expected-decision lists: pairs matched
// under §7.1, then the entries only one list foresaw.
type Comparison struct {
	Both      [][2]int // (orchestrator index, clerk index) in list order
	OnlyOrch  []int
	OnlyClerk []int
}

// Compare matches the orchestrator's and the clerk's expected decisions
// deterministically: one-to-one, orchestrator order first, earliest
// matching counterpart wins.
func Compare(orch, clerk []ledger.Expected) Comparison {
	var c Comparison
	usedC := make([]bool, len(clerk))
	matchedO := make([]bool, len(orch))
	// Pass 1: entries whose paths overlap pair up. 'other' only ever
	// matches through paths, so this pass carries it entirely.
	for i, o := range orch {
		for j, k := range clerk {
			if usedC[j] || o.Category != k.Category {
				continue
			}
			if pathsOverlap(o.Paths, k.Paths) {
				c.Both = append(c.Both, [2]int{i, j})
				usedC[j], matchedO[i] = true, true
				break
			}
		}
	}
	// Pass 2: same-category pairs where at least one side has empty
	// paths merge — but only when no path-overlapping alternative was
	// taken (pass 1 consumed those). 'other' never matches by category.
	for i, o := range orch {
		if matchedO[i] || o.Category == "other" {
			continue
		}
		for j, k := range clerk {
			if usedC[j] || o.Category != k.Category {
				continue
			}
			if len(o.Paths) == 0 || len(k.Paths) == 0 {
				c.Both = append(c.Both, [2]int{i, j})
				usedC[j], matchedO[i] = true, true
				break
			}
		}
	}
	for i := range orch {
		if !matchedO[i] {
			c.OnlyOrch = append(c.OnlyOrch, i)
		}
	}
	for j := range clerk {
		if !usedC[j] {
			c.OnlyClerk = append(c.OnlyClerk, j)
		}
	}
	return c
}

// pathsOverlap: two path lists overlap when any path in one equals,
// contains (as a directory prefix), or intersects (as globs) any path in
// the other (§7.1).
func pathsOverlap(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y || dirContains(x, y) || dirContains(y, x) ||
				paths.Intersects(x, y) {
				return true
			}
		}
	}
	return false
}

// dirContains: d contains p when p lies under directory d.
func dirContains(d, p string) bool {
	return strings.HasPrefix(p, strings.TrimSuffix(d, "/")+"/")
}
