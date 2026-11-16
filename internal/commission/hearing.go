package commission

// M2.5 — the upfront hearing. Once both expected-decision lists are in
// the ledger, `court commission` shows the orchestrator's restated goal
// and asks the human, yes or no, about the expected decisions that need
// a ruling now — at most seven, guarantee categories first, then the
// decisions only one list foresaw, then the rest. The intended default
// is printed only after the human answers that question. An empty
// answer skips: the decision stays expected and open, and no
// decision.ruled is ever emitted for it.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"court/internal/ledger"
	"court/internal/recon"

	"github.com/mattn/go-isatty"
)

// terminalIn reports whether the hearing may run: standard input must
// be a real terminal (C2.5.3 — piped input skips the hearing).
func terminalIn(r io.Reader) bool {
	f, ok := r.(*os.File)
	return ok && isatty.IsTerminal(f.Fd())
}

// HearingCap is the at-most-seven bound of spec/milestones M2.5.
const HearingCap = 7

// Candidate is one expected decision that may go before the human.
// ID is its projected D<n> decision id; Sources counts how many lists
// foresaw it (1 or 2); Guarantee marks the categories that order first.
type Candidate struct {
	Exp       ledger.Expected
	ID        string
	Sources   int
	Guarantee bool
}

// HearingCandidates orders the union of both expected lists for the
// hearing: guarantee categories first, then entries only one list
// foresaw, then entries both foresaw — list order inside each group.
// orchBase and clerkBase are the numbers of decisions the projection
// allocated before each list's event, so each candidate carries the
// same D<n> the ledger gave it.
func HearingCandidates(orch, clerk []ledger.Expected, orchBase, clerkBase int) []Candidate {
	cmp := recon.Compare(orch, clerk)
	both := make(map[int]bool, len(cmp.Both))
	for _, pr := range cmp.Both {
		both[pr[0]] = true
	}
	var guarantee, single, rest []Candidate
	for i, e := range orch {
		c := Candidate{
			Exp: e, ID: fmt.Sprintf("D%d", orchBase+i+1), Sources: 1,
			Guarantee: ledger.IsGuarantee(e.Category),
		}
		if both[i] {
			c.Sources = 2
		}
		switch {
		case c.Guarantee:
			guarantee = append(guarantee, c)
		case c.Sources == 1:
			single = append(single, c)
		default:
			rest = append(rest, c)
		}
	}
	for _, j := range cmp.OnlyClerk {
		e := clerk[j]
		c := Candidate{
			Exp: e, ID: fmt.Sprintf("D%d", clerkBase+j+1), Sources: 1,
			Guarantee: ledger.IsGuarantee(e.Category),
		}
		if c.Guarantee {
			guarantee = append(guarantee, c)
		} else {
			single = append(single, c)
		}
	}
	return append(append(guarantee, single...), rest...)
}

// DecisionBases replays the projection's D<n> allocation over the
// event stream: every backbrief's expected list takes that many ids in
// order, then each declared or detected decision takes one.
func DecisionBases(evs []*ledger.Event, orchSeq int64) (orchBase, clerkBase int) {
	n := 0
	for _, e := range evs {
		var bb *ledger.BackbriefSubmitted
		if e.Type == "backbrief.submitted" {
			bb = briefBody(e)
		}
		if e.Seq == orchSeq {
			orchBase = n
		}
		if e.Type == "backbrief.submitted" && e.Actor == "clerk" &&
			bb != nil && !bb.Invalid {
			clerkBase = n
		}
		if bb != nil {
			n += len(bb.ExpectedDecisions)
		}
		if e.Type == "decision.declared" || e.Type == "decision.detected" {
			n++
		}
	}
	return orchBase, clerkBase
}

// briefBody decodes a backbrief.submitted event body.
func briefBody(e *ledger.Event) *ledger.BackbriefSubmitted {
	if e == nil || e.Body == nil {
		return nil
	}
	raw, err := ledger.Canonical(e.Body)
	if err != nil {
		return nil
	}
	var b ledger.BackbriefSubmitted
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil
	}
	return &b
}

// Hearing runs the upfront exchange over in/out. rule records a
// decision.ruled for an actual answer; it is never called for a
// skipped question. Output is a deterministic function of the inputs
// and the answer lines read, so a replay over the same events and
// answers is byte-identical (C2.5.5).
func Hearing(in io.Reader, out io.Writer, goal string,
	cands []Candidate,
	rule func(decision, option string) error) error {

	shown := cands
	if len(shown) > HearingCap {
		shown = shown[:HearingCap]
	}
	open := len(cands) - len(shown)

	fmt.Fprintln(out, "the orchestrator restates the goal:")
	for _, line := range strings.Split(strings.TrimRight(goal, "\n"), "\n") {
		fmt.Fprintf(out, "  %s\n", line)
	}
	if len(cands) == 0 {
		fmt.Fprintln(out, "hearing: no expected decisions need a ruling")
		return nil
	}
	fmt.Fprintf(out, "%d expected decision(s) need a ruling now:\n", len(cands))

	r := bufio.NewReader(in)
	ruled, skipped := 0, 0
	for i, c := range shown {
		fmt.Fprintf(out, "  %d. %s [y/n] ", i+1, c.Exp.Question)
		answer, _ := r.ReadString('\n')
		answer = strings.ToLower(strings.TrimSpace(answer))
		var option string
		switch answer {
		case "y", "yes":
			option = "yes"
		case "n", "no":
			option = "no"
		default:
			skipped++
			fmt.Fprintln(out, "skipped — stays open")
			continue
		}
		// Only now may the intended default appear (C2.5.6, sabotage
		// default-printed-before-answer): shown beside the answer,
		// and the human may change their answer after seeing it.
		fmt.Fprintf(out, "\n     your answer: %s | intended default: %s\n     change? [y/n] ",
			option, c.Exp.IntendedDefault)
		chg, _ := r.ReadString('\n')
		chg = strings.ToLower(strings.TrimSpace(chg))
		if chg == "y" || chg == "yes" {
			if option == "yes" {
				option = "no"
			} else {
				option = "yes"
			}
			fmt.Fprintf(out, "changed to %s\n", option)
		} else {
			fmt.Fprintln(out, "kept")
		}
		if err := rule(c.ID, option); err != nil {
			return err
		}
		ruled++
	}
	if open > 0 {
		fmt.Fprintf(out, "  … %d more stay open\n", open)
	}
	fmt.Fprintf(out, "hearing: %d ruled, %d open\n", ruled, skipped+open)
	return nil
}
