# Court: specification set (phase 1 and 2)

*Working name. Renaming is one substitution (`02-architecture.md` §9).*

> **Agents decide things without saying so. Court finds those decisions, settles what a machine can settle, and brings the human only the choices that are theirs, while the work continues.**

This folder is the brief for an implementation agent working in a loop. It covers milestones M0 to M3, up to gate G1. The specs for the clerk, the docket, law, and progress are written after G1, from what M3 measures. That order is deliberate: everything later rests on detection, and detection is the part most likely to be wrong.

Read in this order:

| # | File | What it settles |
|---|---|---|
| 1 | `00-north-star.md` | The thesis, what may be claimed, twelve properties, the headline numbers, the bar, the **global denied list** |
| 2 | `01-decision-model.md` | The decision record, categories, fact and will, states, reconciliation, contests, writs, the ledger |
| 3 | `02-architecture.md` | One binary, the daemon, worlds, adapters, the six agent tools, storage, repository layout |
| 4 | `03-detection.md` | Principles, the twelve detectors, claims, the declaration gate, phrasing, the reconciliation engine |
| 5 | `04-benchmark.md` | DocketBench: what exists already, scenario format, twins, metrics, the held-out set |
| 6 | `05-agent-loop.md` | **How to work**: the loop, definition of done, sabotage checks, stuck protocol, change requests, gates |
| 7 | `milestones/M0…M3` | Chunks, each with build instructions, checks, sabotage and denied items |
| 8 | `PROGRESS.md` | The tracker the loop reads and updates |

Background, not normative: `../PLAN.md` (the whole product), `../Prior art check.md`, `../research/` (four sets of notes read on 15 Nov 2026).

## Rules that override everything else

1. **A check is never edited to make it pass.** Thresholds, vectors and expected verdicts change only through `CHANGE_REQUESTS/` and a human decision (`05` §6).
2. **"Denied" sections are binding.** They list attractive improvements that are out of scope. Building them is a failure.
3. **Every chunk has sabotage checks.** A suite that stays green when the implementation is deliberately broken does not count.
4. **Vectors and scenarios come before the code they test** (`05` §7). A detector written before its vectors are frozen is a failed chunk.
5. **No model in any check.** Checks are hermetic. Model outputs enter only as recorded events (`01` §2).
6. **Numbers are recorded raw**, with numerator and denominator, in `evidence/`.

## Phases

| Phase | Milestones | What exists at the end |
|---|---|---|
| **0. Evidence** | M0 (human) | A week of real interventions and thirty labelled agent commits; a go or no-go on the central bets |
| **1. Record and expectation** | M1, M2 | Every agent action is in one ledger; agents work in worlds that can be discarded exactly; expected decisions are fixed before work starts; authority only narrows |
| **2. Detection** | M3 | Every commit passes a declaration gate; twelve detectors describe what changed; declared, detected and expected are reconciled; done-claims are checked; contests are raised |
| *3. Judgment* | *after G1* | *The clerk (fact and will), the docket (blind rulings, attention budget), assumptions and rollback* |
| *4. Law and awareness* | *after G1* | *Statutes, audit, alarm routing, exposure ledger, progress, fan-out and comprehension measured together* |

## Milestone map

| ID | Name | Produces | Depends on |
|---|---|---|---|
| M0 | Field evidence | Intervention log, labelled agent history, gate G0 | none (human; runs beside M1) |
| M1 | Ledger and worlds | Hash-chained ledger with two verifiers, git-worktree worlds, scripted agent, hook endpoint, first agent tools, Claude Code adapter | none |
| M2 | Commission and backbrief | `court commission`, backbrief, the blind clerk's list, writs and `spawn`, the upfront hearing | M1 |
| M3 | Detection and reconciliation | Frozen vectors and DocketBench A, C, D; the declaration gate; detectors; the reference and Go reconcilers; claims; contests; `court report` | M2; M0.2 for one measurement |
| **G1** | **Human gate** | Held-out run; decision to specify phases 3 and 4 | M3 |

**Where the step forward is.** M1 and M2 are plumbing that others have. The step is M3: a queue built from what the agent did not say, measured for recall on planted decisions and on real agent history. Do not polish M1 and M2 beyond their checks.

**What may be claimed.** No single mechanism here is new as an idea (`00` §2). ARCTIC and AgentMonBench are the closest work and must be read before M3.1.
