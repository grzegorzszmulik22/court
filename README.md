# Court

A decision layer for fleets of coding agents.

Orchestrators coordinate agents over tasks and messages. Court coordinates them over **decisions**: it finds the
decisions agents make, whether they announce them or not, settles every question a machine can settle, and brings the
human only the choices that are theirs, in one queue.

| Command | What the human does |
|---|---|
| `court commission "<goal>"` | States a goal and answers a short upfront hearing; agents start |
| `court hold` | Opens the docket and rules on what is waiting |
| `court progress` | A short tour of what changed, ordered by what they have seen least |

Court pilots existing agent tools (Claude Code first) through adapters; it does not replace them.

> **Status: under active construction.** Milestones M1 (ledger and worlds) and M2 (commission and backbrief) are
> merged. M3 (detecting and reconciling decisions) is in progress. The `court hold` docket and the report are not built
> yet.

## What works today

| Area | Built | Where |
|---|---|---|
| Ledger | Hash-chained event ledger with a Go/Python agreement suite and mutation tests | `internal/ledger`, `ref/python/refledger` |
| Worlds | Isolated git worktrees per agent that can be discarded without trace in the user's repository; Linux and Windows | `internal/world` |
| Commission | `court commission`, the daemon, the blind clerk's backbrief, writs and spawn budgets, the upfront hearing | `internal/commission`, `internal/daemon`, `internal/writ` |
| Agent adapter | A scripted agent and a Claude Code adapter that run inside worlds | `internal/adapter` |

## How it is verified

Specification-first, like its sibling project [Countersign](https://github.com/grzegorzszmulik22/countersign). Each
chunk in `spec/milestones/` lists its checks and **sabotages** (deliberate breakages the tests must catch). A chunk is
merged only after a supervisor review on a fresh clone, on Linux and Windows, catches sabotages the implementer never
saw. Expected results change only through recorded change requests.

The code is written by AI coding agents (Devin, and DeepSeek through opencode) in parallel lanes under that adversarial
review. `main` holds only reviewed merges; `spec/PROGRESS.md` records each one with its findings.

## Quick start

Linux or Windows (Git Bash). The Go toolchain is pinned and downloaded by `make tools`.

```sh
make tools
make regress    # every merged chunk's checks and sabotages
```

## Repository layout

| Path | Contents |
|---|---|
| `PLAN.md`, `Prior art check.md`, `research/` | Background: the plan, prior art, research notes |
| `spec/` | The binding specification and milestones M0–M3 |
| `cmd/court`, `internal/` | Go implementation |
| `ref/python/` | Independent Python reference implementation |
| `test/` | Check scripts, vectors and fixtures per chunk |
| `field/` | A tool for the human's intervention log (the log itself is never committed) |

## License

[Apache-2.0](LICENSE).
