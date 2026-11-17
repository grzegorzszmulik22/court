// Package paths implements the repository-glob semantics of spec/01:
// '*' matches inside a path segment, '**' crosses segments, '?' one
// character, '[...]' a class. Every glob compiles to an NFA; containment
// (L(child) ⊆ L(parent)) and overlap (L(a) ∩ L(b) ≠ ∅) are decided on
// the automata, never on the pattern text.
package paths

import (
	"fmt"
	"strings"
)

// frag is one path segment pattern: a run of atoms.
type frag []atom

type atom struct {
	// kind: 'l' literal rune, '?' any single rune except '/', '*' any
	// run within a segment, 'c' character class.
	kind  byte
	lit   rune
	class []rune // sorted; pairs are ranges (lo,hi,lo,hi,...)
	neg   bool
}

// parse splits a glob into segment frags and '**' markers.
func parse(g string) ([]any, error) {
	if g == "" {
		return nil, fmt.Errorf("empty glob")
	}
	var out []any
	for _, seg := range strings.Split(g, "/") {
		if seg == "**" {
			out = append(out, "**")
			continue
		}
		f, err := parseFrag(seg)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

func parseFrag(s string) (frag, error) {
	var f frag
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		switch rs[i] {
		case '*':
			f = append(f, atom{kind: '*'})
		case '?':
			f = append(f, atom{kind: '?'})
		case '[':
			j := i + 1
			neg := false
			if j < len(rs) && (rs[j] == '!' || rs[j] == '^') {
				neg = true
				j++
			}
			var class []rune
			for j < len(rs) && rs[j] != ']' {
				if j+2 < len(rs) && rs[j+1] == '-' && rs[j+2] != ']' {
					class = append(class, rs[j], rs[j+2])
					j += 3
				} else {
					class = append(class, rs[j], rs[j])
					j++
				}
			}
			if j >= len(rs) {
				return nil, fmt.Errorf("unterminated class in %q", s)
			}
			f = append(f, atom{kind: 'c', class: class, neg: neg})
			i = j
		case '\\':
			if i+1 < len(rs) {
				i++
			}
			f = append(f, atom{kind: 'l', lit: rs[i]})
		default:
			f = append(f, atom{kind: 'l', lit: rs[i]})
		}
	}
	return f, nil
}

// nfa is a nondeterministic automaton over path strings: states are ints,
// transitions rune ranges, 0 is start, accept is the last state.
type nfa struct {
	edges []map[string][]int // state -> sorted range-string -> next states
	eps   [][]int            // epsilon moves
	acc   int
}

func (n *nfa) add(from int, lo, hi rune, to int) {
	if n.edges[from] == nil {
		n.edges[from] = map[string][]int{}
	}
	n.edges[from][rng(lo, hi)] = append(n.edges[from][rng(lo, hi)], to)
}

func rng(lo, hi rune) string { return string([]rune{lo, hi}) }

const maxRune = 0x10FFFF

// compile builds the NFA for a glob. Segments are matched between '/'
// literals; '**' is "zero or more segments" — it may vanish (src/**/x
// matches src/x) and a trailing '**' matches zero or more trailing
// segments (src/** matches src/ and src/a/b — note src itself needs the
// segment separator to fire, so '**' also accepts the empty tail making
// src/** match src too when written as the last element).
func compile(elems []any) *nfa {
	n := &nfa{edges: []map[string][]int{{}}, eps: [][]int{{}}}
	state := 0
	next := func() int {
		n.edges = append(n.edges, map[string][]int{})
		n.eps = append(n.eps, nil)
		return len(n.edges) - 1
	}
	segChar := func(from int, a atom) int {
		// one new state per atom, transitions exclude '/' implicitly:
		// classes and '?'/'*' never list '/' in their range sets.
		to := next()
		switch a.kind {
		case 'l':
			n.add(from, a.lit, a.lit, to)
		case '?':
			n.add(from, 1, '/'-1, to)
			n.add(from, '/'+1, maxRune, to)
		case 'c':
			eachClassRange(a.class, func(lo, hi rune) {
				if a.neg {
					addComplement(n, from, lo, hi, to)
				} else {
					n.add(from, lo, hi, to)
				}
			})
		}
		return to
	}
	// '*' inside a segment: self-loop on the current segment-end state.
	star := func(st int) {
		n.add(st, 1, '/'-1, st)
		n.add(st, '/'+1, maxRune, st)
	}
	for i, el := range elems {
		switch v := el.(type) {
		case string: // "**" — zero or more whole segments
			mid := next()
			if i == 0 {
				// Leading '**': its segments carry no leading separator.
				n.add(state, 1, maxRune, mid)
			} else {
				// '**' after a segment must enter through a '/', so
				// src/** never matches src2/... (C2.4.1 vector).
				n.add(state, '/', '/', mid)
			}
			n.add(mid, 1, maxRune, mid) // any rune incl. '/'
			st2 := next()
			n.eps[mid] = append(n.eps[mid], st2)
			n.eps[state] = append(n.eps[state], st2)
			state = st2
			// '**' not last means a '/' already consumed by the loop
			// when segments exist; nothing more needed.
		case frag:
			if i > 0 {
				// segment separator, except after a '**' (the loop's '/'
				// is optional — ε-paths already connect state).
				if _, wasStar := elems[i-1].(string); !wasStar {
					to := next()
					n.add(state, '/', '/', to)
					state = to
				} else {
					// after '**': the next segment may follow directly
					// (ε path) or after consumed segments (loop '/'); add
					// a fresh '/' transition for the direct case.
					to := next()
					n.add(state, '/', '/', to)
					state = to
				}
			}
			for _, a := range v {
				if a.kind == '*' {
					star(state)
					continue
				}
				state = segChar(state, a)
			}
		}
	}
	// Trailing '**' also matches the bare prefix (src/** matches src):
	// handled because its ε-chain already reaches state.
	n.acc = state
	return n
}

func eachClassRange(class []rune, f func(lo, hi rune)) {
	for i := 0; i+1 < len(class); i += 2 {
		f(class[i], class[i+1])
	}
}

// addComplement adds transitions covering [1,maxRune] \ [lo,hi] minus '/'.
func addComplement(n *nfa, from int, lo, hi rune, to int) {
	for _, iv := range [][2]rune{{1, lo - 1}, {hi + 1, maxRune}} {
		a, b := iv[0], iv[1]
		if a > b {
			continue
		}
		// exclude '/'
		if a <= '/' && '/' <= b {
			if a < '/' {
				n.add(from, a, '/'-1, to)
			}
			if b > '/' {
				n.add(from, '/'+1, b, to)
			}
		} else {
			n.add(from, a, b, to)
		}
	}
}

// eclose returns the epsilon-closure of a state set.
func (n *nfa) eclose(set map[int]bool) map[int]bool {
	stack := []int{}
	for s := range set {
		stack = append(stack, s)
	}
	for len(stack) > 0 {
		s := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, t := range n.eps[s] {
			if !set[t] {
				set[t] = true
				stack = append(stack, t)
			}
		}
	}
	return set
}

// step moves a state set over one rune.
func (n *nfa) step(set map[int]bool, r rune) map[int]bool {
	out := map[int]bool{}
	for s := range set {
		for rg, tos := range n.edges[s] {
			rr := []rune(rg)
			if r >= rr[0] && r <= rr[1] {
				for _, t := range tos {
					out[t] = true
				}
			}
		}
	}
	return out
}

// Match reports whether glob g matches path p exactly.
func Match(g, p string) bool {
	elems, err := parse(g)
	if err != nil {
		return false
	}
	n := compile(elems)
	set := n.eclose(map[int]bool{0: true})
	for _, r := range p {
		set = n.eclose(n.step(set, r))
	}
	return set[n.acc]
}

// Contains reports whether every string glob child matches is also
// matched by parent — pattern containment by simulation, not by string
// prefix (C2.4.1, sabotage glob-containment-by-string-prefix).
func Contains(parent, child string) bool {
	pElems, err := parse(parent)
	if err != nil {
		return false
	}
	cElems, err := parse(child)
	if err != nil {
		return false
	}
	pn, cn := compile(pElems), compile(cElems)
	// Subset construction: track determinized child states paired with
	// the parent state set they imply. If a reachable child-accepting
	// configuration has no parent path to accept, containment fails.
	dfa := map[string]map[int]bool{}  // det child state-set key -> set
	pmap := map[string]map[int]bool{} // same key -> parent set
	var work []string
	start := cn.eclose(map[int]bool{0: true})
	sk := setKey(start) + "|" + setKey(pn.eclose(map[int]bool{0: true}))
	dfa[sk] = start
	pmap[sk] = pn.eclose(map[int]bool{0: true})
	work = append(work, sk)
	for len(work) > 0 {
		k := work[0]
		work = work[1:]
		cs, ps := dfa[k], pmap[k]
		if cs[cn.acc] && !ps[pn.acc] {
			return false // child accepts a string parent cannot
		}
		// Candidate alphabet: every range boundary appearing in either
		// automaton's outgoing edges.
		points := boundaryPoints(cn, cs, pn, ps)
		for _, r := range points {
			nc := cn.eclose(cn.step(cs, r))
			np := pn.eclose(pn.step(ps, r))
			nk := setKey(nc) + "|" + setKey(np)
			if _, seen := dfa[nk]; seen {
				continue
			}
			dfa[nk] = nc
			pmap[nk] = np
			work = append(work, nk)
		}
	}
	return true
}

func setKey(set map[int]bool) string {
	var b strings.Builder
	for i := 0; i < 4096; i++ {
		if set[i] {
			fmt.Fprintf(&b, "%d,", i)
		}
	}
	return b.String()
}

// boundaryPoints collects representative runes for every distinct
// character class reachable now: range endpoints and their successors.
func boundaryPoints(cn *nfa, cs map[int]bool, pn *nfa, ps map[int]bool) []rune {
	var pts []rune
	add := func(n *nfa, set map[int]bool) {
		for s := range set {
			for rg := range n.edges[s] {
				rr := []rune(rg)
				pts = append(pts, rr[0], rr[1])
				if rr[0] > 1 {
					pts = append(pts, rr[0]-1)
				}
				if rr[1] < maxRune {
					pts = append(pts, rr[1]+1)
				}
			}
		}
	}
	add(cn, cs)
	add(pn, ps)
	return pts
}

// Intersects reports whether some string matches both globs (used for
// expected-decision path overlap under §7.1).
func Intersects(a, b string) bool {
	// Cheap exact wins first.
	if a == b || Match(a, b) || Match(b, a) {
		return true
	}
	ae, err1 := parse(a)
	be, err2 := parse(b)
	if err1 != nil || err2 != nil {
		return false
	}
	an, bn := compile(ae), compile(be)
	// Product automaton BFS.
	seen := map[string]bool{}
	work := [][2]map[int]bool{
		{an.eclose(map[int]bool{0: true}), bn.eclose(map[int]bool{0: true})},
	}
	for len(work) > 0 {
		cur := work[0]
		work = work[1:]
		as, bs := cur[0], cur[1]
		k := setKey(as) + "|" + setKey(bs)
		if seen[k] {
			continue
		}
		seen[k] = true
		if as[an.acc] && bs[bn.acc] {
			return true
		}
		var pts []rune
		addPts := func(n *nfa, set map[int]bool) {
			for s := range set {
				for rg := range n.edges[s] {
					rr := []rune(rg)
					pts = append(pts, rr[0], rr[1])
					if rr[0] > 1 {
						pts = append(pts, rr[0]-1)
					}
					if rr[1] < maxRune {
						pts = append(pts, rr[1]+1)
					}
				}
			}
		}
		addPts(an, as)
		addPts(bn, bs)
		for _, r := range pts {
			na, nb := an.eclose(an.step(as, r)), bn.eclose(bn.step(bs, r))
			if len(na) > 0 && len(nb) > 0 {
				work = append(work, [2]map[int]bool{na, nb})
			}
		}
	}
	return false
}
