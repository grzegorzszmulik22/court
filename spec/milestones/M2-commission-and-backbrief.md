# M2. Commission and backbrief

**Goal.** Fix what is expected before any work starts. The human states a goal in one command. The orchestrator restates it and lists the decisions it expects to face. An independent clerk, shown only the goal, writes its own list. The human answers the differences once. Authority is handed down as writs that can only narrow.

**Properties advanced.** P5 (attenuation), P6 (expectations are fixed first). Headlines H5, H6.

**Normative references.** `01` §4, §6, §9; `02` §5, §7.

**Why.** Mission command has two briefs. In the confirmation brief the subordinate restates the mission straight after receiving it; in the backbrief, after planning, they say how they intend to accomplish it. Court asks for both in one step: the restated goal, then the plan and the decisions the agent expects to face. Misunderstanding is caught at the cheapest moment. Plans are also where human judgment has been measured to work: people reject 39% of plans and 3% of individual permission prompts. And an expectation set in advance is what later makes "undeclared" and "out of scope" decidable (`03` §1, principle 7).

**Expected results.**
- `court commission "<goal>"` in any git repository starts work with no configuration file.
- The expected decisions are in the ledger before the first agent tool event.
- Decisions that only one of the two lists foresaw are shown to the human, once, before work starts.
- No agent can give a child more authority than it has.

**Measurements [T].**

| ID | Metric | Target |
|---|---|---|
| MM2.1 | Generated writs breaking attenuation that are accepted | 0 of ≥ 5,000 (H5) |
| MM2.2 | Commissions in which an agent tool event precedes the stored expected decisions | 0 of ≥ 500 scripted commissions (H6) |
| MM2.3 | From `court commission` to the first agent step, scripted, excluding model time | ≤ 3 s |
| MM2.4 | Keystrokes the human needs beyond the goal sentence to get agents working | at most 1 (Enter at the upfront hearing) |

**Milestone-wide denied.** A configuration file needed to start (`00` D14). Free-form chat with agents. Editing the agent's plan in place. More than one human. Templates or presets for commissions. Matching the two lists by text (`00` D4).

---

## M2.1 The commission

**Build.** `internal/commission` and `court commission "<goal>"`. Flags exist for everything and are needed for nothing: `--done "<command>"` (repeatable), `--territory <glob>` (repeatable), `--tokens`, `--minutes`, `--attention <minutes per day>`. Defaults: done-checks from the repository's detected test command (Go, Node, Python: one rule each, stated in the output), territory the whole repository, budgets from a built-in table. `commission.opened` records the values used and which were defaults.

**Checks.**
- C2.1.1 In a fresh fixture repository, `court commission "x"` with no flags records a commission, starts the daemon, creates a world and starts the scripted orchestrator.
- C2.1.2 MM2.4 and MM2.3.
- C2.1.3 With no detectable test command, the commission starts with an empty done-check list and prints one line saying that done-claims cannot be checked.
- C2.1.4 Outside a git repository the command prints one sentence and exits non-zero.
- C2.1.5 Every default used is listed in `commission.opened` under `defaults`.

**Sabotage.** `default-territory-empty`; `defaults-not-recorded`.

**Denied.** Interactive wizards. Reading settings from the repository.

## M2.2 Backbrief and ordering

**Build.** The `backbrief` tool (`02` §5). The ordering rule: until an agent's `backbrief.submitted` is in the ledger, every other tool call from that agent is denied at `PreToolUse` with the reason "Submit your backbrief first", except read-only tools (file read, search, listing), which an agent needs to write a plan. The list of read-only tools is fixed in the adapter.

**Checks.**
- C2.2.1 MM2.2: scripted orchestrators that try to edit, run or commit before `backbrief` are denied every time, and the ledger order holds.
- C2.2.2 A backbrief with an expected decision lacking a `category` is rejected with the valid list.
- C2.2.3 Read-only tool events before the backbrief are recorded and allowed.
- C2.2.4 A second `backbrief` from the same agent is recorded as a revision; the first stays, and expected decisions from both are kept.
- C2.2.5 The plan's file list and line estimate are stored in the shape DT12 reads (M3.4).

**Sabotage.** `backbrief-not-required`; `write-tool-classed-read-only`; `revision-overwrites-first`.

**Denied.** Judging the quality of a plan. Requiring a minimum number of expected decisions.

## M2.3 The blind clerk

**Build.** The clerk call of `02` §7: a tool-less one-shot session given only the commission, asked for expected decisions in the same schema. The output is recorded as `backbrief.submitted` with actor `clerk`. Then the comparison, using the matcher of `01` §7.1: decisions on both lists, only the orchestrator's, only the clerk's.

**Checks (hermetic, with recorded clerk outputs in `test/vectors/clerk/`).**
- C2.3.1 The clerk's input contains the commission and nothing from the orchestrator's backbrief or the repository's files (asserted on the recorded request).
- C2.3.2 The comparison over 40 fixture pairs equals hand-written expected results.
- C2.3.3 A clerk output that is not valid against the schema is recorded as invalid and the commission continues on the orchestrator's list, with one line telling the human.
- C2.3.4 With no model configured, C2.3.3's path is taken without error.
- C2.3.5 The clerk is recorded before the orchestrator's first write tool event (P6 covers both lists).

**Checks (live, not part of done).**
- L2.3.1 `make live-clerk`: on five fixture commissions, the clerk returns a valid list. Tokens recorded.

**Sabotage.** `clerk-sees-orchestrator-list`; `comparison-by-question-text`.

**Denied.** The clerk reading the repository. Merging lists by similarity. The clerk ruling on anything.

## M2.4 Writs and spawn

**Build.** Writs per `01` §9; the `spawn` tool: attenuation check, world creation, session start with the child's token, writ and task; `writ.issued` or `writ.refused`. Marking of `outside-writ` on declared decisions (detected decisions are marked in M3). The human's root writ: every non-guarantee category, class reversible, the commission's territory and budgets.

**Checks.**
- C2.4.1 MM2.1 over generated parent and child writs, including glob containment cases frozen in `test/vectors/writ/` (`src/**` contains `src/api/*.go`; `src/*` does not contain `src/api/x.go`; `**` contains everything).
- C2.4.2 A guarantee category in any requested writ is refused.
- C2.4.3 A child's budget is deducted from the parent's remaining budget; two spawns that together exceed it: the second is refused.
- C2.4.4 A refused spawn creates no world and no process.
- C2.4.5 A declared decision in a category outside the agent's writ is marked `outside-writ`.
- C2.4.6 Depth: a chain of five nested spawns narrows correctly at every level.

**Sabotage.** `glob-containment-by-string-prefix`; `budget-not-deducted`; `refused-spawn-leaves-world`; `guarantee-category-grantable`.

**Denied.** Widening a writ after issue. Writs that expire or are earned (after G1). An agent reading another agent's writ.

## M2.5 The upfront hearing

**Build.** After both lists are in, `court commission` shows, in the terminal: the orchestrator's restated goal in at most three lines; then the expected decisions that need the human now, at most seven, in this order: those in guarantee categories, those only one list foresaw, the rest. Each is a yes or no question. The agent's intended default is **not** shown until the human has answered that question; it is then shown beside the answer, and the human may change it. Enter with no answer skips: the work starts, the skipped decisions stay expected and unruled, and agents proceed on their intended defaults as assumptions. Only an actual answer is recorded as `decision.ruled`. Later matching decisions inherit the ruling (`01` §6; checked in M3.5).

A skipped question is never recorded as an accepted default. A ruling the human did not consider would enter the record as their judgment and later be used as precedent.

**Checks.**
- C2.5.1 Pressing Enter once starts the work with nothing ruled: the ledger has no `decision.ruled` event, and every expected decision is still open.
- C2.5.6 The intended default of a question does not appear in the terminal output before that question's answer is read (asserted on the output stream order).
- C2.5.2 With more than seven candidates, seven are shown and the rest are recorded as `open`, with a count printed.
- C2.5.3 With standard input not a terminal, the hearing is skipped, nothing is ruled, and the output says so.
- C2.5.4 The restated goal shown is the orchestrator's text from the ledger, unchanged.
- C2.5.5 The hearing output for a fixture commission is identical on replay.

**Sabotage.** `enter-records-defaults-as-rulings`; `default-printed-before-answer`; `hearing-shows-clerk-text-as-orchestrator`.

**Denied.** A text box for free answers. Showing more than seven questions. Any recommendation, ranking or highlighting of an option before the human answers: analysts shown an agent's lean matched it on 86% of choices.
