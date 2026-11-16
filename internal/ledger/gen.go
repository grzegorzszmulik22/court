package ledger

import (
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
)

// This file generates deterministic, schema-valid ledgers for the mutation
// suite, the golden vectors, and the replay check (an M1.8-shaped run).
// Timestamps are derived from the seq so output is reproducible per seed.

type genEvent struct {
	commission string
	actor      string
	typ        string
	body       any
}

type genState struct {
	r        *rand.Rand
	at       string // next timestamp
	atN      int64
	seq      int64
	events   []*Event
	pending  []genEvent
	decCount map[string]int // commission -> decisions issued so far
	closed   map[string]bool
	cats     []string
	agents   map[string]string // agent id -> world id (per scenario)
	agentW   map[string]string // agent id -> writ id
	base     string
}

func (g *genState) ts() string {
	g.atN++
	// 2026-11-15T00:00:00.000Z + atN milliseconds, computed without time zones.
	sec := g.atN / 1000
	ms := g.atN % 1000
	return fmt.Sprintf("2026-11-15T%02d:%02d:%02d.%03dZ",
		(sec/3600)%24, (sec/60)%60, sec%60, ms)
}

func (g *genState) emit(commission, actor, typ string, body any) {
	bv, err := BodyFrom(body)
	if err != nil {
		panic(err)
	}
	prev := ZeroPrev
	if len(g.events) > 0 {
		prev = g.events[len(g.events)-1].Hash
	}
	ev := &Event{Seq: g.seq, Prev: prev, At: g.ts(), Commission: commission,
		Actor: actor, Type: typ, Body: bv}
	if _, err := ev.ComputeHash(); err != nil {
		panic(err)
	}
	g.seq++
	g.events = append(g.events, ev)
}

func (g *genState) decisionID(commission string) string {
	g.decCount[commission]++
	return "D" + strconv.Itoa(g.decCount[commission])
}

func (g *genState) cat() string { return g.cats[g.r.Intn(len(g.cats))] }

func pickAgent(g *genState, commission string) string {
	var ids []string
	for a := range g.agents {
		ids = append(ids, a)
	}
	if len(ids) == 0 {
		return "a0"
	}
	return ids[g.r.Intn(len(ids))]
}

// hexOf deterministically fabricates lowercase hex.
func (g *genState) hexOf(n int) string {
	const d = "0123456789abcdef"
	var sb strings.Builder
	for i := 0; i < n; i++ {
		sb.WriteByte(d[g.r.Intn(16)])
	}
	return sb.String()
}

// Generate builds a deterministic ledger of about n events covering every
// event type, and returns the canonical lines.
func Generate(seed int64, n int) ([][]byte, error) {
	g := &genState{
		r: rand.New(rand.NewSource(seed)), decCount: map[string]int{},
		closed: map[string]bool{}, agents: map[string]string{},
		agentW: map[string]string{},
		cats: []string{"dependency", "interface", "behaviour", "structure",
			"failure-handling", "scope", "other"},
	}
	commission := "C1"
	g.emit(commission, "human", "commission.opened", &CommissionOpened{
		Goal:       "generated ledger " + strconv.FormatInt(seed, 10),
		DoneChecks: []string{"go test ./..."},
		Territory:  []string{"**"},
		Budgets:    Budgets{Tokens: 100000, Minutes: 60, Speculation: 500},
		Attention:  30, Defaults: []string{"territory", "budgets"},
		Exemptions: []string{"vendor/**"},
	})
	nAgents := 1 + g.r.Intn(4)
	for i := 0; i < nAgents; i++ {
		ag := fmt.Sprintf("a%d", i)
		w := fmt.Sprintf("w%d", i)
		wr := fmt.Sprintf("W%d", i)
		g.agentW[ag] = wr
		g.emit(commission, "court", "writ.issued", &WritIssued{
			ID: wr, Agent: ag, Parent: "human",
			Categories: []string{"dependency", "interface", "behaviour", "structure", "scope", "other"},
			Class:      "reversible", Territory: []string{"**"},
			Budgets: Budgets{Tokens: 50000, Minutes: 30, Speculation: 200},
		})
		g.emit(commission, "court", "world.created", &WorldCreated{
			World: w, Base: g.hexOf(40), Branch: "court/C1/" + ag})
		g.emit(commission, "court", "agent.started", &AgentStarted{
			Agent: ag, Parent: "human", Tool: "scripted", Version: "1.0",
			World: w, Writ: wr})
		g.agents[ag] = w
		g.emit(commission, "agent:"+ag, "backbrief.submitted", &BackbriefSubmitted{
			Goal: "restated goal",
			Plan: Plan{Files: []string{"main.go", "lib.go"}, Lines: 120},
			ExpectedDecisions: []Expected{{
				Question: "Which parser do we use?", Category: "dependency",
				Paths: []string{"lib.go"}, IntendedDefault: "o1"}},
			WillNotTouch: []string{"vendor/**"},
		})
		g.decisionID(commission) // the expected decision row
	}
	// A writ refusal exercises the type.
	g.emit(commission, "court", "writ.refused", &WritRefused{
		Agent: "a0", Parent: "a1", Categories: []string{"authority"},
		Class: "reversible", Territory: []string{"**"},
		Budgets: Budgets{Tokens: 1, Minutes: 1, Speculation: 1},
		Rule:    "guarantee-category", Detail: "authority is a guarantee category",
	})

	body := func() {
		ag := pickAgent(g, commission)
		switch g.r.Intn(8) {
		case 0, 1, 2: // tool events dominate a real ledger
			outcome := []string{"ok", "ok", "ok", "error", "denied"}[g.r.Intn(5)]
			te := &ToolEvent{
				Tool:        []string{"Read", "Edit", "Bash", "Grep", "Write"}[g.r.Intn(5)],
				ToolUseID:   "tu_" + g.hexOf(12),
				InputDigest: g.hexOf(64),
				Fields: ToolFields{
					Paths:   []string{fmt.Sprintf("src/f%d.go", g.r.Intn(20))},
					Command: "go test ./pkg/" + strconv.Itoa(g.r.Intn(5)),
					Hosts:   []string{},
				},
				Outcome: outcome,
			}
			if outcome == "denied" {
				te.Reason = "outward action denied"
			}
			g.emit(commission, "agent:"+ag, "tool.event", te)
		case 3:
			g.emit(commission, "court", "world.commit", &WorldCommit{
				World: g.agents[ag], Commit: g.hexOf(40),
				Parents: []string{g.hexOf(40)},
				Changes: []PathChange{{
					Path:  fmt.Sprintf("src/f%d.go", g.r.Intn(20)),
					Added: int64(g.r.Intn(40)), Deleted: int64(g.r.Intn(10)),
				}},
				Gated: true, Assumes: []string{},
			})
		case 4:
			g.emit(commission, "agent:"+ag, "decision.declared", &DecisionDeclared{
				Question: "Keep the flag at " + strconv.Itoa(g.r.Intn(3)) + "?",
				Category: g.cat(), Paths: []string{fmt.Sprintf("src/f%d.go", g.r.Intn(20))},
				Options: []Option{{ID: "o1", Summary: "yes"}, {ID: "o2", Summary: "no"}},
				Chosen:  "o1", Petition: g.r.Intn(8) == 0,
			})
			g.decisionID(commission)
		case 5:
			g.emit(commission, "court", "decision.detected", &DecisionDetected{
				Detector: "DT" + strconv.Itoa(1+g.r.Intn(12)),
				Category: g.cat(), Paths: []string{fmt.Sprintf("src/f%d.go", g.r.Intn(20))},
				Commit: g.hexOf(40),
				Evidence: []Evidence{{
					File: "src/f0.go", StartLine: 1 + int64(g.r.Intn(90)),
					EndLine: 100 + int64(g.r.Intn(40)), Digest: g.hexOf(64),
				}},
			})
			g.decisionID(commission)
		case 6:
			g.emit(commission, "agent:"+ag, "claim.done", &ClaimDone{
				Task:   "task-" + strconv.Itoa(g.r.Intn(4)),
				Checks: []string{"go test ./..."},
			})
		case 7:
			g.emit(commission, "agent:"+ag, "alarm.raised", &AlarmRaised{
				Text:  "suspicious diff in f" + strconv.Itoa(g.r.Intn(20)),
				Paths: []string{"src/f0.go"},
			})
		}
	}
	for g.seq < int64(n)-14 {
		body()
	}
	// Tail: decision lifecycle events.
	ag0 := "a0"
	if len(g.agents) > 0 {
		ag0 = pickAgent(g, commission)
	}
	g.emit(commission, "agent:"+ag0, "decision.none", &DecisionNone{})
	g.emit(commission, "agent:"+ag0, "decision.declared", &DecisionDeclared{
		Question: "Name the new module x?", Category: "structure",
		Paths: []string{"x/"}, Options: []Option{{ID: "o1", Summary: "call it x"}},
		Chosen: "o1", Petition: false,
	})
	dec := g.decisionID(commission)
	g.emit(commission, "court", "decision.reconciled", &DecisionReconciled{
		Decision: dec, Sources: []string{"declared"}, Recon: "declared-only",
		Flags: []string{},
	})
	g.emit(commission, "clerk", "decision.phrased", &DecisionPhrased{
		Decision: dec, Question: "Name the new module x?",
		Alternative: "leave it in the old package",
	})
	g.emit(commission, "human", "decision.ruled", &DecisionRuled{
		Decision: dec, Option: "o1",
	})
	// A contested pair across two worlds.
	ca := g.decisionID(commission)
	g.emit(commission, "agent:"+ag0, "decision.declared", &DecisionDeclared{
		Question: "Retry on timeout?", Category: "behaviour",
		Paths: []string{"net/"}, Options: []Option{{ID: "o1", Summary: "retry"}},
		Chosen: "o1", Petition: false,
	})
	cb := g.decisionID(commission)
	g.emit(commission, "court", "decision.detected", &DecisionDetected{
		Detector: "DT11", Category: "behaviour", Paths: []string{"net/"},
		Commit: g.hexOf(40),
		Evidence: []Evidence{{File: "net/r.go", StartLine: 1, EndLine: 3,
			Digest: g.hexOf(64)}},
	})
	g.emit(commission, "court", "decision.contested", &DecisionContested{
		Decisions: []string{ca, cb}, Worlds: []string{"w0", "w1"},
		Evidence: []string{g.hexOf(64), g.hexOf(64)},
	})
	g.emit(commission, "court", "claim.checked", &ClaimChecked{
		Task:              "task-0",
		Results:           []CheckResult{{Check: "go test ./...", Result: "pass"}},
		UnverifiedByAgent: false,
	})
	for _, a := range sortedKeys(g.agents) {
		g.emit(commission, "court", "agent.stopped", &AgentStopped{Agent: a, Reason: "done"})
	}
	g.emit(commission, "court", "commission.closed", &CommissionClosed{
		Outcome: "merged",
		Counts:  ReconCounts{Confirmed: 2, DeclaredOnly: 3, Undeclared: 1},
	})
	var out [][]byte
	for _, ev := range g.events {
		l, err := ev.Line()
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, nil
}

func sortedKeys(m map[string]string) []string {
	var k []string
	for x := range m {
		k = append(k, x)
	}
	for i := 0; i < len(k); i++ {
		for j := i + 1; j < len(k); j++ {
			if k[j] < k[i] {
				k[i], k[j] = k[j], k[i]
			}
		}
	}
	return k
}

// GenerateLoadToStore appends an M1.8-shaped run: agents, backbriefs, >= nTool
// tool events, >= nCommit commits, and the lifecycle tail, into a store.
func GenerateLoadToStore(s *Store, seed int64, nAgents, nTool, nCommit int) (int, error) {
	g := &genState{
		r: rand.New(rand.NewSource(seed)), decCount: map[string]int{},
		closed: map[string]bool{}, agents: map[string]string{},
		agentW: map[string]string{},
		cats:   []string{"dependency", "interface", "behaviour", "structure", "scope", "other"},
	}
	commission := "C1"
	app := func(actor, typ string, body any) error {
		_, err := s.Append(commission, actor, typ, body)
		return err
	}
	if err := app("human", "commission.opened", &CommissionOpened{
		Goal: "load run", DoneChecks: []string{"go test ./..."},
		Territory: []string{"**"},
		Budgets:   Budgets{Tokens: 1 << 20, Minutes: 600, Speculation: 50000},
		Attention: 60, Defaults: []string{"territory", "budgets"},
		Exemptions: []string{},
	}); err != nil {
		return 0, err
	}
	for i := 0; i < nAgents; i++ {
		ag, w, wr := fmt.Sprintf("a%d", i), fmt.Sprintf("w%d", i), fmt.Sprintf("W%d", i)
		g.agentW[ag], g.agents[ag] = wr, w
		for _, e := range []struct {
			actor, typ string
			body       any
		}{
			{"court", "writ.issued", &WritIssued{ID: wr, Agent: ag, Parent: "human",
				Categories: g.cats, Class: "reversible", Territory: []string{"**"},
				Budgets: Budgets{Tokens: 1 << 18, Minutes: 300, Speculation: 10000}}},
			{"court", "world.created", &WorldCreated{World: w, Base: g.hexOf(40),
				Branch: "court/C1/" + ag}},
			{"court", "agent.started", &AgentStarted{Agent: ag, Parent: "human",
				Tool: "scripted", Version: "1.0", World: w, Writ: wr}},
			{"agent:" + ag, "backbrief.submitted", &BackbriefSubmitted{
				Goal: "work", Plan: Plan{Files: []string{"f.go"}, Lines: 10},
				ExpectedDecisions: []Expected{{Question: "log to stderr?",
					Category: "behaviour", Paths: []string{"f.go"},
					IntendedDefault: "yes"}},
				WillNotTouch: []string{}}},
		} {
			if err := app(e.actor, e.typ, e.body); err != nil {
				return 0, err
			}
		}
	}
	var ags []string
	for a := range g.agents {
		ags = append(ags, a)
	}
	sort.Strings(ags)
	// Decision ids follow the projector's nextDecisionID (D1, D2, ...).
	// Each agent contributes one expected, one declared and one detected row.
	expID := map[string]string{}
	decID := map[string]string{}
	detID := map[string]string{}
	dn := 0
	for _, a := range ags {
		dn++
		expID[a] = fmt.Sprintf("D%d", dn)
	}
	for _, a := range ags {
		dn++
		decID[a] = fmt.Sprintf("D%d", dn)
		dn++
		detID[a] = fmt.Sprintf("D%d", dn)
	}
	for i := 0; i < nTool; i++ {
		ag := ags[g.r.Intn(len(ags))]
		if err := app("agent:"+ag, "tool.event", &ToolEvent{
			Tool: "Bash", ToolUseID: fmt.Sprintf("tu_%d", i), InputDigest: g.hexOf(64),
			Fields:  ToolFields{Paths: []string{"f.go"}, Command: "go test", Hosts: []string{}},
			Outcome: "ok",
		}); err != nil {
			return 0, err
		}
	}
	commitOf := map[string]string{}
	for i := 0; i < nCommit; i++ {
		ag := ags[g.r.Intn(len(ags))]
		cid := g.hexOf(40)
		commitOf[ag] = cid
		if err := app("court", "world.commit", &WorldCommit{
			World: g.agents[ag], Commit: cid, Parents: []string{g.hexOf(40)},
			Changes: []PathChange{{Path: "f.go", Added: 3, Deleted: 1}},
			Gated:   true, Assumes: []string{},
		}); err != nil {
			return 0, err
		}
	}
	// Declared and detected decisions, then claims and an alarm.
	for _, a := range ags {
		if err := app("agent:"+a, "decision.declared", &DecisionDeclared{
			Question: "Retry on timeout?", Category: "behaviour",
			Paths: []string{"f.go"}, Options: []Option{{ID: "o1", Summary: "retry"}},
			Chosen: "o1", Petition: false,
		}); err != nil {
			return 0, err
		}
		if err := app("court", "decision.detected", &DecisionDetected{
			Detector: "DT11", Category: "behaviour", Paths: []string{"f.go"},
			Commit: commitOf[a],
			Evidence: []Evidence{{File: "f.go", StartLine: 1, EndLine: 3,
				Digest: g.hexOf(64)}},
		}); err != nil {
			return 0, err
		}
	}
	for _, a := range ags {
		for _, e := range []struct {
			actor, typ string
			body       any
		}{
			{"agent:" + a, "claim.done", &ClaimDone{
				Task: "task-" + a, Checks: []string{"go test ./..."}}},
			{"court", "claim.checked", &ClaimChecked{
				Task:              "task-" + a,
				Results:           []CheckResult{{Check: "go test ./...", Result: "pass"}},
				UnverifiedByAgent: false}},
			{"agent:" + a, "alarm.raised", &AlarmRaised{
				Text: "coverage thin in f.go", Paths: []string{"f.go"}}},
		} {
			if err := app(e.actor, e.typ, e.body); err != nil {
				return 0, err
			}
		}
	}
	// Reconciliation verdicts must equal what replay recomputes: a declared
	// decision is declared-only, a detected one undeclared, and an expected
	// one unanswered once the commission closes (spec/01 section 7).
	for _, a := range ags {
		for _, r := range []struct {
			id      string
			sources []string
			recon   string
		}{
			{expID[a], []string{"expected"}, "unanswered"},
			{decID[a], []string{"declared"}, "declared-only"},
			{detID[a], []string{"detected"}, "undeclared"},
		} {
			if err := app("court", "decision.reconciled", &DecisionReconciled{
				Decision: r.id, Sources: r.sources, Recon: r.recon,
				Flags: []string{},
			}); err != nil {
				return 0, err
			}
		}
	}
	for _, a := range ags {
		if err := app("clerk", "decision.phrased", &DecisionPhrased{
			Decision: decID[a], Question: "Retry on timeout?",
			Alternative: "fail fast",
		}); err != nil {
			return 0, err
		}
		if err := app("human", "decision.ruled", &DecisionRuled{
			Decision: decID[a], Option: "o1",
		}); err != nil {
			return 0, err
		}
	}
	for _, a := range ags {
		if err := app("court", "agent.stopped", &AgentStopped{Agent: a, Reason: "done"}); err != nil {
			return 0, err
		}
	}
	if err := app("court", "commission.closed", &CommissionClosed{
		Outcome: "done", Counts: ReconCounts{}}); err != nil {
		return 0, err
	}
	return 1 + 5*nAgents + nTool + nCommit + 5*nAgents + 5*nAgents + nAgents + 1, nil
}
