# Court: plan

Draft 3, 2026-11-15. Working name. Nothing is built.

- Draft 2 applied `Prior art check.md`: claims reworded, blind rulings made the default, fan-out adopted as the headline metric.
- Draft 3 applies the deeper read in `research/`: claims narrowed again (§13), a declaration gate at every commit, a `failure-handling` category, cost of delay computed and never claimed, five more older results (§3).

**This plan is background. The binding documents for milestones M0 to M3 are in `spec/`.** Where they differ, the spec wins. Milestones M4 to M8 below are specified after gate G1.

Court is a separate product from Countersign. It borrows ideas from the Countersign spec (§7.3) and does not depend on it. If this folder moves to its own repository, nothing here needs to change.

## 1. What Court is

Court is a decision layer for fleets of coding agents. It finds the decisions agents make, whether they announce them or not, settles every question a machine can settle, and brings the human only the choices that are theirs to make, in one queue, while the work continues.

Existing orchestrators coordinate over **tasks** and **messages**. Court coordinates over **decisions**. That is the one structural change, and the rest of the plan follows from it.

The whole user surface is three commands:

| Command | What the human does |
|---|---|
| `court commission "<goal>"` | States a goal. Answers a short upfront hearing. Agents start. |
| `court hold` | Opens the docket and rules on what is waiting. Works on a phone. |
| `court progress` | Takes a five-minute tour of what changed, ordered by what they have seen least. |

Court does not replace the agents. It pilots existing agent tools (Claude Code first) through adapters.

## 2. What multi-agent systems get wrong today

| # | Pitfall | Evidence |
|---|---|---|
| P1 | **Silent decisions.** Agents fill gaps with confident guesses and never ask. The decisions that matter most are the ones the agent did not recognise as decisions. | HiL-Bench (arXiv 2604.09408); AMBIG-SWE (arXiv 2502.13069); "Ask or Assume?" (arXiv 2603.26233) |
| P2 | **Conflicting implicit decisions.** Parallel agents each make choices the others never see, and the results do not fit together. | Cognition, "Don't Build Multi-Agents" (2025) |
| P3 | **Humans asked the wrong questions.** People approve 93 to 97% of permission prompts and catch 13.6% of dangerous commands, against 89% for a model reviewer. They reject 39% of plans. | `research_notes/Transactional sandbox prior art/2_review_and_lab_practice.md` |
| P4 | **Approval fatigue.** Human block rates fall from about 17% to about 5% after 50 prompts. | Same file |
| P5 | **Review is the bottleneck.** Fleets produce changes faster than one person can read them. | Augment Code guide; six-lane practitioner write-up |
| P6 | **Reports replace evidence.** Orchestrators summarise subagent work, the summary loses or distorts facts, and "done" is accepted on the agent's word. | MAST taxonomy (arXiv 2503.13657): missing and incorrect verification are named failure modes |
| P7 | **Underspecified goals.** Specification problems are one of MAST's three failure categories. | MAST |
| P8 | **The overseer loses competence.** The more the agents do, the less the human understands the system they are ruling on. | "Comprehension debt" (Osmani, 2026); Bainbridge (1983) |
| P9 | **Blocked or wasted work.** Agents either stop and wait for a human, or run ahead on a guess that is later thrown away. | Atomix (arXiv 2602.14849); Cordon (arXiv 2606.17573) |
| P10 | **Cost.** Multi-agent systems use about 15 times the tokens of a chat. | Anthropic, "How we built our multi-agent research system" |

## 3. Design lineage

Most of these pitfalls were met before, in aviation, process control, military staff work, economics and distributed systems. Each row names the older result, what it established, and the Court mechanism built on it. Authors, years and venues were checked on 15 Nov 2026 (`research/3_human_factors.md`); that file marks, row by row, where only the citation was confirmed and the claim was not re-read in the source.

| Older result | What it established | Pitfall | Court mechanism |
|---|---|---|---|
| Fitts (1951); Parasuraman, Sheridan and Wickens (2000); Dekker and Woods (2002) | Fitts listed what people and machines are each better at; later work read it as an allocation rule. Automation level can differ for gathering, analysing, deciding and acting. Dekker and Woods warn that automating a function changes the human's remaining work. | P3 | **Fact and will.** Questions of fact are settled by checks and model review. Only questions of will reach the human (§5.3). |
| Mission command: the confirmation brief (ADP 5-0) and the backbrief (FM 6-0, 2014) | Straight after the order the subordinate restates the mission; after planning, they brief how they intend to accomplish it. Misunderstanding is caught at the cheapest moment. | P1, P7 | **Backbrief.** Before work starts the orchestrator lists the decisions it expects to face (§5.1). |
| Completed staff work (Col. Archer L. Lerch, January 1942) | Never ask the chief "what should I do?". Bring a finished proposal the chief can accept or reject. | P3, P5 | **Ripeness.** A petition is heard only when its options are built and their consequences shown (§5.3). |
| Crawford and Sobel, cheap talk (1982); Milgrom, verifiable disclosure (1981) | Costless claims from an interested sender carry little information. When claims are checkable and the receiver assumes the worst about anything undisclosed, senders disclose. | P6 | **Evidence rule.** Every claim links to a check Court reruns. Missing evidence is read as the worst case (§5.3). |
| Romer and Rosenthal, agenda control (1978) | The agenda setter's power is bounded by the status quo: the worse the fallback is for the chooser, the more the setter can extract. | P3 | **Option not offered.** A blind agent proposes options from the goal alone (§5.3). **The fallback belongs to the human:** for a decision outside an agent's writ, what happens with no ruling is the state before the change, never the agent's choice. |
| Beer, Viable System Model (1972): the audit channel and the algedonic alarm | Management needs a channel that inspects operations directly, bypassing the reporting line, and an alarm that reaches the top unfiltered. | P6 | **Audit and alarm.** Court samples leaf work directly and any agent can raise an alarm its orchestrator cannot filter (§5.6). |
| Olsen and Goodrich (2003); Olsen and Wood, fan-out (2004); Crandall et al., neglect tolerance (2005) | Fan-out = neglect time ÷ interaction time + 1: the number of robots one person can operate follows from how long each can be left alone and how long each interaction takes. | P5 | **Fan-out** is Court's headline metric, with neglect time and interaction time as its parts (§8, C10). |
| Cummings and Mitchell, operator capacity (2008); Cummings and Guerlain (2007) | With wait times, fan-out = neglect time ÷ (interaction time + wait time) + 1; waits from lost situation awareness cut predicted capacity by up to 67%. Operator performance degrades above about 70% utilisation. | P5, P8 | Comprehension is reported beside fan-out, never apart from it (§8, C11). |
| Scerri, Pynadath and Tambe, transfer of control (2002) | An agent should weigh asking, waiting and acting as time passes, not hand over control once and block. | P9 | **Default and deadline** on every decision (§4). |
| Billings, management by consent and by exception; Olson and Sarter (2001) | Under time pressure, consent becomes perfunctory. | P4 | Court shows **no countdown**. A reversible decision that is not ruled proceeds on its default; the human is never hurried into a ruling (§5.4). |
| Bliss et al., alarm reliability (1995) | People respond to alarms at a rate that matches how often the alarms are right. | P4 | **Docket precision** is a headline number beside recall, because a queue that is half noise will be half ignored. |
| McFarlane, interruption coordination (2002) | Letting the person choose when to take an interruption performed best overall. | P4 | `court hold` is opened by the human; Court interrupts only for alarms (§5.4). |
| Tirole, collusion in hierarchies (1986) | In a principal, supervisor and agent hierarchy, the supervisor and the agent can collude, so the principal needs information that does not pass through the supervisor. | P6 | The **audit** is sampled by Court, not by the orchestrator, and judged by a model from another family (§5.6). |
| Dawes, Faust and Meehl, clinical versus actuarial judgment (1989) | Mechanical rules predict better than case-by-case expert judgment. | P3 | Facts are settled by **checks first**, model review second (`spec/01` §5). |
| Kaplow, rules versus standards (1992) | A rule costs more to write and less to apply than a standard; frequency decides which is cheaper. | P4 | A **statute** is proposed only for a decision shape that recurs, and its exception rate is tracked (§5.5). |
| Gray, the transaction concept (1981); Garcia-Molina and Salem, sagas (1987) | Long-running work is not atomic; a compensation cannot always undo an external effect. | P9 | **Reversibility is demonstrated, not declared** (`spec/01` §8). |
| The structuring offence in cash reporting (31 CFR 1010.314) | Splitting one transaction to stay under a threshold is judged on the aggregate. | P4 | **Rolling-window statutes** (§5.5). |
| Goldratt, *The Goal* (1984); Goldratt and Fox, *The Race* (1986) | System output is set by the bottleneck. Drum-buffer-rope releases work at the bottleneck's pace. | P5 | **Attention budget.** The human's minutes per day set how much work is started (§5.4). |
| The cμ rule (Smith 1956; Cox and Smith 1961) | Serve first the job with the highest cost of delay per unit of service time. | P5 | **Docket order** (§5.4). |
| EEMUA 191 (1999, later editions); ISA-18.2 | Every alarm must need an operator action. EEMUA: under one alarm per ten minutes is acceptable in steady state, one per two minutes is over-demanding. ISA-18.2: more than ten in ten minutes is a flood. | P4 | **No petition without a decision. Flood rule** (§5.4). |
| Wolfe et al., low-prevalence effect (2005); Mackworth, vigilance decrement (1948) | People miss rare targets, and monitoring performance decays within about half an hour. | P4 | **Informative docket.** A question whose answer is predictable is not asked. Short sessions. Canaries (§5.4). |
| Kahneman et al., noise and anchoring; Anthropic judgment-call study (2026) | Judgments drift with order, mood and the first number seen. Analysts matched their agent's randomly assigned lean on 86% of choices. | P4 | **Blind rulings** by default, and a consistency check against the human's own earlier rulings (§5.4). |
| Klein, the pre-mortem; Mitchell, Russo and Pennington (1989) | The pre-mortem asks a team to assume the plan has failed and say why. The experiment behind it found that treating an outcome as certain produced fuller explanations. | P6 | **Counter-case** format (§5.3). |
| Rasmussen, skill, rule and knowledge (1983); drift to danger (1997) | Behaviour runs at three levels. Under cost pressure systems drift toward the edge of safe operation. | P4, P5 | **Three tiers of law**, with expiry so that authority does not creep (§5.5). |
| Kung and Robinson, optimistic concurrency (1981); Jefferson, Time Warp (1985); de Kleer, assumption-based truth maintenance (1986) | Proceed without waiting, record what each result assumed, roll back exactly what depended on a failed assumption, and bound how far ahead to run. | P9 | **Assumption ledger** and speculation debt (§5.2). |
| Cohen and Levesque, teamwork and joint intentions (1991); Jennings, commitments and conventions (1993) | A team member that drops or changes a commitment must tell the others. | P2, P6 | **Shared decisions and announced drops** (§5.2). |
| Saltzer and Schroeder (1975); Hardy, the confused deputy (1988) | Least privilege. Delegated authority must never exceed the delegator's. | P6 | **Writs attenuate downward** (§5.1). |
| Bainbridge, ironies of automation (1983); Endsley, situation awareness (1995) | Automating the routine leaves the human unpractised for the hard cases. Awareness has three levels: what changed, what it means, what comes next. | P8 | **Exposure ledger and progress** (§5.6). |
| Sarter and Woods, mode confusion (1995) | Operators lose track of what the automation is currently allowed to do. | P8 | **Boundary page** (§5.6). |

## 4. The core object: a decision

```yaml
decision:
  id, commission, world
  question:        one sentence
  kind:            fact | will
  source:          petitioned | detected | expected | conflict | audit | alarm
  options:         [{summary, world_ref, evidence[], consequences}]
  assumes:         [decision ids this one rests on]
  class:           reversible | compensable | irreversible
  default:         option taken if nobody rules
  deadline:        when the default becomes final
  cost_of_delay:   estimate per hour
  ruling:          {option, by: agent | statute | human, shown_sha256, signature?}
  precedent:       [earlier decisions cited]
  outcome:         held | amended | reverted   # settled after probation
```

Every decision is a record in one append-only, hash-chained ledger. Tasks, messages and diffs hang off decisions, not the other way round.

## 5. Components

### 5.1 Commission and backbrief

- A **commission** holds the goal, the checks that define done, constraints, territory (paths), and three budgets: tokens, wall-clock time and human attention minutes.
- The orchestrator returns a **backbrief**: the goal in its own words, the plan, the decisions it expects to face with the default it intends for each, and what it will not touch.
- A **blind clerk** (a different model family where one is configured, shown only the commission) writes its own list of expected decisions.
- Differences between the two lists, and every expected question of will, are asked in one upfront hearing. Plans are where human judgment has been measured to work (39% rejection).
- Both lists are stored as the **expected decisions** for later reconciliation.
- **Writs** are issued here: what each agent may decide alone, by decision class and territory. A subagent's writ is always a subset of its parent's.

### 5.2 Decision ledger

Decisions enter from three independent sources:

| Source | How it arrives |
|---|---|
| Declared | The agent calls `record_decision` or `petition`. |
| Detected | Detectors run over each world's diff and tool events. |
| Expected | The backbrief lists from §5.1. |

**Reconciliation.** Detected but never declared is an **undeclared decision**. Expected but never seen is an open question ("where was this decided?"). Both go to the clerk.

**Detectors, version 1.** Deterministic first:

- dependency manifest changed
- schema or migration changed
- public surface changed (exported symbols, routes, command flags, config keys)
- test weakened (assertion removed, test deleted or skipped, threshold loosened)
- type weakened
- new outbound host
- authentication or permission code touched
- data deletion
- file outside territory
- change far larger than the plan

A model pass then phrases each hit as a question and names the alternative not taken.

**Assumptions.** A decision acted on before it is ruled is an assumption. Every commit in a world carries the ids of the assumptions it was built under. An overrule lists exactly the dependent commits. **Speculation debt** is the work resting on unratified assumptions. Each commission has a limit, and past it the agent stops extending that branch.

**Conflicts.** Two sibling worlds answering the same question differently create a conflict decision before merge. When a decision is ruled, every agent whose work rests on it is told.

**Commitments.** An agent that drops or narrows an assigned task must record it. A done-claim without a passing done-check is stored as "claimed, unproven" and never shown as done.

### 5.3 Clerk: before the hearing

- **Fact or will.** The clerk classifies each decision. For a fact it writes and runs a check in the world, or asks a second-family model reviewer, and attaches the result. Settled facts never reach the human. A fact it cannot settle goes to the docket labelled "unverified fact"; the rate is measured (C2).
- **Ripeness.** A petition is heard only when each option is built or sketched with its consequences, evidence links rerun, and a default and deadline are set. Otherwise it goes back to the agent.
- **Evidence rule.** Each claim maps to a check the clerk reruns. A claim with no check is displayed as unproven.
- **Counter-case.** A different-family model writes a pre-mortem: this was accepted and failed within two weeks; the most likely reason.
- **Option not offered.** The blind clerk proposes options from the commission alone. Any that the petition lacks is added and marked.
- **Moot.** A petition whose premise has changed (its world moved, or an assumption under it was overruled) is withdrawn.

### 5.4 Docket: the hearing

- **Attention budget.** The human sets minutes per day. Court forecasts the rulings each commission will need and does not start commissions the budget cannot cover. It plans to use at most 70% of the budget, the level above which operator performance has been found to degrade.
- **Order.** Cost of delay divided by expected time to rule. Irreversible first on ties. Items needing the same kind of judgment are grouped.
- **Hearing screen.** The question, how the options behave differently, evidence status, the counter-case, precedent, default and deadline. Actions: choose, amend, send back, make this a law.
- **Blind rulings.** For every question of will the agent's recommendation is hidden and option order is random until the human has chosen. Then the recommendation and its reasons are shown, and the human may change the ruling. A visible recommendation would mostly decide the ruling (86% in the Anthropic study).
- **Consistency.** A ruling that contradicts the human's own earlier rulings is flagged before it is sealed.
- **Signing.** Irreversible rulings need a passkey. Every ruling records a hash of what was shown.
- **Sessions.** Court holds items for sessions. It interrupts only for alarms and for irreversible decisions about to pass their deadline.
- **Flood rule.** Past ten arrivals in ten minutes Court groups them, and reversible items take their default under probation.
- **Informativeness.** Court tracks how predictable rulings are per decision shape. A shape the human always rules the same way is proposed as a law.
- **Canaries.** Past cases with known outcomes are replayed at a low rate. A miss ends the session with a suggestion to stop.

### 5.5 Law

| Tier | Who decides | When |
|---|---|---|
| Writ | The agent | Inside its delegated authority. Recorded. |
| Statute | A deterministic rule over the decision record and diff facts | Decides automatically. |
| Precedent | The human, with one tap | A similar past ruling pre-fills the answer and is cited. Never decides alone. |
| Hearing | The human | Everything else. |

- **Enactment.** "Make this a law" produces a statute and a backtest against the ledger: how many past cases it would have decided, and how many differently from the human. That second number must be zero.
- **Rolling window.** A statute evaluates cumulative change per commission and territory over a window, so a large change cannot be split into small ones that each pass.
- **Sunset.** Every statute expires (default 30 days). Renewal shows a sample of what it decided.
- **Settlement.** Writs widen only after outcomes hold through probation. A revert suspends the statute involved and narrows the writ.
- **Tainted origin.** Rulings from sessions that read untrusted content never become precedent.

### 5.6 Progress, audit and alarm

- **Exposure ledger.** Per territory: what the human was shown and when, from the hashes of hearing screens and tours.
- **Progress.** `court progress` is a five-minute tour ordered by exposure debt. Each stop says what changed, what it means and what comes next, and some stops ask the human to predict a behaviour before showing the real run.
- **Stale warning.** A hearing in a territory the human has not seen recently opens with a brief on the gap.
- **Audit.** Court samples leaf-agent work at random and compares it with what the orchestrator reported. Each orchestrator gets a report-fidelity score. A mismatch becomes a docket item.
- **Alarm.** Any agent can raise an alarm that goes straight to the human. Orchestrators cannot filter it. Alarms are rate-limited and scored per agent.
- **Boundary.** One page lists what every agent may do right now without the human: writs, statutes and defaults in force.

## 6. A day with Court

1. Morning: `court commission "cut checkout p99 below 60 ms"`. Two minutes of upfront hearing: may checkout read stale inventory, may the schema change.
2. Three agents work in separate worlds. One swaps a dependency without saying so. A detector catches it; the clerk confirms the licence and tests (facts) and drops it from the docket, recording why.
3. Two agents choose different retry policies. Court raises one conflict decision before either merges.
4. Midday: `court hold`. Four items, about five minutes. One becomes a law.
5. An overruled default discards two commits in one world and nothing else.
6. Friday: `court progress`. The tour starts with the billing code, which the human has not looked at for three weeks.

## 7. Architecture

### 7.1 Shape

- One binary, `court`: daemon, command line and a local web interface that works on a phone.
- Storage: SQLite, holding the hash-chained ledger.
- No configuration file is needed to start.

### 7.2 Agents and worlds

- **Agent tools.** A Model Context Protocol server gives agents five tools: `backbrief`, `record_decision`, `petition`, `claim_done`, `alarm`.
- **Adapters.** One per agent tool, to start and stop an agent in a world and stream its tool events. Claude Code first; Devin CLI and Codex CLI after their hook surfaces are confirmed (§12).
- **Worlds.** An interface with `fork`, `diff`, `run`, `merge`, `discard`. Version 1 uses git worktrees, with an optional container.
- **Court's own models.** Clerk roles run on a different model family from the agent under review where the user has one configured.

### 7.3 What Court takes from Countersign

These are ideas, re-implemented small. Court has no Kubernetes, broker or kernel observer.

| Idea | Countersign source | Use in Court |
|---|---|---|
| Declared against observed | `04` reconciliation | The decision ledger (§5.2) |
| Reversibility classes | `05` §1 | `class` on every decision |
| Staged work, one signed commit | `05` §3, §8 | Worlds and signed rulings |
| Hash of what the reviewer saw, bound into the signature | `05` §6, §8 | `shown_sha256` and the exposure ledger |
| Probation and undo | `05` §10 | Settlement |
| Autonomy earned from repeated review shapes | `05` §13.4 | Law proposals |
| Backtest of a new policy on past records | `05` §13.1 | Statute enactment |
| Worst case over everything that contributed | `05` §6 | The evidence rule |
| Taint propagation | `M10` | Assumption ids on commits; tainted origin |

### 7.4 What Court cannot do alone

Without a network-level substrate, Court stages only what lives in the repository. Anything that leaves the machine (a deploy, an email, a remote push) is classed irreversible and always heard. Countersign can later be an optional world backend that makes those effects stageable.

## 8. Hypotheses

`[B]` is measured on DocketBench (§9). `[F]` is measured in the field over four weeks of real use.

| ID | Metric | Target |
|---|---|---|
| C1 `[B]` | Planted undeclared decisions surfaced; share of detected items that are real decisions | ≥ 80%; ≥ 50% |
| C2 `[B]` | Docket items that are questions of fact, by independent labels | ≤ 10% |
| C3 `[F]` | Blind rulings that differ from the agent's hidden recommendation | 20% to 60% (permission prompts: 3% rejected) |
| C4 `[B]` | Agent time blocked on a reversible decision | 0 |
| C5 `[B]` | After an overrule: dependent commits identified; unrelated commits discarded | 100%; 0 |
| C6 `[B]` | Planted sibling conflicts raised before merge | ≥ 90% |
| C7 `[B]` | Planted orchestrator misreports caught at 20% sampling | ≥ 90% within 25 tasks |
| C8 `[B]` | Done-claims without a passing check shown as done | 0 |
| C9 `[B]` | Statutes enacted that contradict a past ruling; split changes that evade a statute | 0; 0 |
| C10 `[F]` | Fan-out = neglect time ÷ (interaction time + wait time) + 1, with each part reported, week 4 against week 1, with canary catch rate not lower | ≥ 2× |
| C11 `[F]` | Prediction accuracy on progress probes, week 4 against week 1 | Not lower |
| C12 | Install to first ruling | ≤ 10 minutes |
| C13 | Court's own tokens as a share of fleet tokens | ≤ 20% |

C1 is the central claim. C10 with C11 is the result not found in any publication: fan-out rising while the human's understanding does not fall.

## 9. DocketBench

A public benchmark for the oversight system around agents. HiL-Bench scores whether an agent asks; DocketBench scores whether the system catches what the agent did not ask.

Each scenario is a repository, a commission and a scripted or live agent, with ground truth for:

- hidden decisions the goal leaves open (C1)
- questions with a checkable answer, planted as petitions (C2)
- sibling tasks that force incompatible choices (C6)
- orchestrator reports that misstate subagent work (C7)
- false done-claims (C8)
- a risky change split into small commits (C9)

Scenario manifests and oracles are frozen before the detectors are written. The harness follows the conventions of control evaluations (ControlArena, built on Inspect), so results can be read beside that work.

## 10. Milestones

M0 to M3 are expanded into chunks with numbered checks, sabotage and denied items in `spec/milestones/`. The rows below are the summary; the spec has since added a declaration gate, claims checking and contests to M3, and moved the specification of M4 to M8 behind gate G1.

| | Goal | Build | Checks | Denied |
|---|---|---|---|---|
| **M0** | Field evidence for the two central bets. Runs alongside M1. | A one-line logger for every intervention the human makes on the Devin lanes for a week: was it raised by the agent, fact or will, repeat or new. | At least 50 logged interventions, each labelled. | Any Court code. |
| **M1** | A ledger and worlds. | The binary, the ledger, the world interface on git worktrees, the Claude Code adapter, the five agent tools. | Three agents in three worlds; every tool event in the ledger; the ledger replays to the same state; a broken chain is detected. | Any interface beyond a ledger dump. |
| **M2** | Commission and backbrief. | Commission format, backbrief, blind clerk list, upfront hearing in the terminal, writs. | A subagent cannot hold a writ wider than its parent (100% of generated cases). Both expected lists are stored. | Free-form chat with agents. |
| **M3** | Detection and reconciliation. | The ten detectors, the phrasing pass, reconciliation, DocketBench scenarios for C1. | C1. Each detector has planted positives and negatives. | Models deciding whether a deterministic detector fired. |
| **M4** | The clerk. | Fact or will, check writing, ripeness, the evidence rule, counter-case, option not offered, moot. | C2, C8. A petition missing a default is returned (100%). | The clerk ruling on questions of will. |
| **M5** | The docket. | Web interface, ordering, budget, blind rulings, passkey signing, sessions, flood rule, assumption ids and rollback, conflicts. | C4, C5, C6, C12. | Notifications outside alarms; a chat pane. |
| **M6** | Law. | Statutes, backtest, rolling window, sunset, precedent pre-fill, settlement. | C9. A statute with one contradicting past case cannot be enacted. | Model-judged statutes; precedent that decides alone. |
| **M7** | Progress, audit, alarm. | Exposure ledger, progress tour with probes, sampling audit, alarm channel, boundary page. | C7. An orchestrator cannot suppress a subagent alarm (tested). | Dashboards beyond the boundary page. |
| **M8** | Benchmark and release. | Full DocketBench, four weeks of own use, the leverage and comprehension curves, a quickstart. | C3, C10, C11, C13. | New features. |

**Human gates.**

- **G0, after M0.** If fewer than 20% of interventions concerned something the agent never raised, and fewer than 30% were questions of will, the central bets are weak and the plan is revisited before M3.
- **G1, after M3.** C1 must be met on frozen scenarios. If not, detection is redesigned before anything is built on it. ARCTIC (arXiv 2607.29516) is read in full before M3 starts.
- **G2, before release.** Citation check of §3, and a repeat of the prior-art check.

## 11. Denied for the whole product

- A new agent framework, model or prompt library. Court pilots existing agents.
- Kubernetes, a network broker or kernel observation.
- Multiple humans, roles or routing between reviewers.
- Agents that create their own work.
- Markets, points or reputation currencies between agents.
- Medieval vocabulary beyond six nouns: commission, hearing, ruling, law, writ, progress.
- A model, anywhere, ruling on a question of will.

## 12. Risks and facts to verify

| Risk or open fact | Why it matters | How it is handled |
|---|---|---|
| Detector precision | A docket flooded with non-decisions is worse than no docket. | C1's second target; G1. |
| Agent tool hook surfaces | Adapters need tool events and a way to inject rulings. Confirmed for Claude Code; not checked here for Devin CLI or Codex CLI. | Verify before M1 ends. |
| Token overhead | The clerk roles add model calls to a system that already costs 15× a chat. | C13. |
| Vendors ship fleet interfaces | Agent vendors are building their own. | Court is agent-agnostic, and the ledger and benchmark are the assets. |
| Field metrics need a real month | C3, C10 and C11 have one subject at first. | Stated with n beside every figure. |
| Two products, one builder | Countersign's build is in progress. | Court M0 costs minutes a day; M1 onward is scheduled by the human. |
| Citations | Authors, years and venues are now checked (`research/3_human_factors.md`). For several rows only the citation was confirmed, not the claim in the source; EEMUA 191, ISA-18.2 and the army publications were read through secondary sources. | The rows marked bib in that file are re-read before anything is published. |
| Novelty | A one-day check was done (`Prior art check.md`). No mechanism is new as an idea. The nearest product, PulseEngine's decision inbox, shipped on 3 Nov 2026. | Claims limited to §13. The check is repeated at G2. |

## 13. Prior art and what is claimed

| Existing work | What it covers | What it leaves |
|---|---|---|
| Approval inboxes (LangChain Agent Inbox, HumanLayer, gotoHuman, AICtrlNet) | A queue of agent requests with an audit log | Only what the agent chose to ask; action-level questions |
| Fleet dashboards (Flightdeck, Mission Control, Augment Cosmos) | Visibility and task management | Task-centred; review remains the bottleneck |
| HiL-Bench, AMBIG-SWE | Measuring whether agents ask | Improving the agent, not the system around it |
| Atomix, Cordon | Transactions and speculative branches for tool calls | No model of the human |
| Cognition's essay | Implicit decisions as a reason to avoid multi-agent designs | No mechanism to surface them |
| Architecture decision records | Decisions as documents | Written by hand, after the fact |
| ARCTIC (Meta) | Infers a change's intent and scores drift between intent and diff; deployed | A review aid; nothing is reconciled against what the agent declared or what was expected |
| PulseEngine decision inbox (Nov 2026) | Attention budget, decision windows, agents keep working | Only what the agent asks; no fact check; unmeasured |
| Pawsly | Agents' intended actions checked against recorded team decisions | Decisions come from humans' tools, not from the agents' work |
| Fan-out and neglect tolerance (robotics, 2003–2005) | How many robots one person can operate | Not applied to coding agents |
| Control evaluations (ControlArena, BashArena) | Oversight protocols scored against hostile agents | Benign failures |

Full detail: `Prior art check.md`. No mechanism in this plan is new as an idea.

Not found anywhere, and claimed (narrowed after the deeper read; `Prior art check.md` §6):

1. The docket is built by reconciling declared, detected and expected **decisions**, across a tree of agents, so that what an agent never raised becomes a question the human can rule on. The same reconciliation exists at the level of tool calls and files (EPISTEME, CodeClone, Claim Plane).
2. Fan-out and the overseer's comprehension are measured together, over time, for coding agents.
3. A benchmark that scores the oversight system on benign failures **in a multi-agent setting**. Single-agent undisclosed decisions are covered by AgentMonBench.

Design rules, not claims (each exists elsewhere as a principle or a product):

- No question of fact reaches the human.
- Human attention sets how much work is started.
- Rulings are blind by default.

## 14. Decisions for the human

1. Start M0 on the Devin lanes now, or skip it.
2. Implementation language (Go is assumed, for one binary).
3. Whether Court stays in this folder or gets its own repository.
4. When Court's build starts relative to Countersign's milestones.

## Sources

Further sources are listed in `Prior art check.md`.

- [Anthropic: How AI agents shape experts' judgment calls](https://www.anthropic.com/research/agents-judgment-calls)
- [Olsen and Wood: Fan-out (CHI 2004)](https://hci.rwth-aachen.de/materials/conferences/CHI2004/1p231.pdf)
- [Cummings and Mitchell: Predicting Controller Capacity](https://dspace.mit.edu/handle/1721.1/90280)
- [Scerri, Pynadath and Tambe: Towards Adjustable Autonomy for the Real World](https://arxiv.org/pdf/1106.4573)
- [PulseEngine: The scarce resource is the orchestrator's attention](https://pulseengine.eu/blog/the-scarce-resource-is-attention/)
- [ARCTIC (arXiv 2607.29516)](https://arxiv.org/pdf/2607.29516)
- [HiL-Bench (arXiv 2604.09408)](https://www.alphaxiv.org/abs/2604.09408)
- [Ask or Assume? (arXiv 2603.26233)](https://www.alphaxiv.org/abs/2603.26233v2)
- [AMBIG-SWE (arXiv 2502.13069)](https://arxiv.org/abs/2502.13069v3)
- [Why Do Multi-Agent LLM Systems Fail? (arXiv 2503.13657)](https://arxiv.org/abs/2503.13657v2)
- [Cognition: Don't Build Multi-Agents](https://cognition.ai/blog/dont-build-multi-agents)
- [Anthropic: How we built our multi-agent research system](https://www.anthropic.com/engineering/multi-agent-research-system)
- [Atomix (arXiv 2602.14849)](https://arxiv.org/html/2602.14849v1)
- [Cordon (arXiv 2606.17573)](https://www.alphaxiv.org/abs/2606.17573)
- [Comprehension debt (Addy Osmani)](https://addyosmani.com/blog/comprehension-debt/)
- [Augment: human-in-the-loop vs on-the-loop](https://www.augmentcode.com/guides/human-in-the-loop-vs-human-on-the-loop)
- [Six approval lanes write-up](https://myzone-ai-dev-site.web.app/pages/blog/most-productive-day-part3)
- [EEMUA 191 figures (Process Engineering)](https://processengineering.co.uk/article/1303733/alarms-should-not-undermine-safety-and-productivity)
- [EEMUA 191 figures (Chemical Engineering)](https://www.chemengonline.com/alarm-management-numbers/)
