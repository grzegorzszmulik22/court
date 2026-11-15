# 05. How the implementation agent works

This is the operating protocol. It is binding in the same way as the specs. It follows the protocol used for Countersign, with the differences marked **(Court)**.

## 1. The loop

Repeat until every chunk in `PROGRESS.md` is `DONE` or the stop conditions in §5 apply.

1. **Read state.** `spec/PROGRESS.md`, `spec/BLOCKED.md`, and the last entry under `evidence/`.
2. **Select** the lowest-numbered chunk that is `TODO` or `IN_PROGRESS` and whose dependencies are `DONE`. Never work on two chunks at once.
3. **Re-read** that chunk's section, every spec section it cites, and the global denied list (`00` §6).
4. **Plan** in at most 15 lines, saved as `evidence/<chunk>/<UTC timestamp>/plan.md`: files to touch, the checks you expect to satisfy, the first test you will write.
5. **Write the checks first.** Tests and the `make check-<chunk>` target come before the implementation. Run them and confirm they fail for the right reason.
6. **Implement** the smallest change that can pass.
7. **Run** `make check-<chunk>`. Output is captured to the evidence folder automatically (M1.1).
8. **Run** `make sabotage-<chunk>` (§4).
9. **Run** `make regress` (all checks of all `DONE` chunks).
10. **Record** the outcome in `PROGRESS.md`. If all three passed twice in a row from `make clean`, mark `DONE` and commit (§9). Otherwise go to 6, or to §5 if stuck.

## 2. Definition of done for a chunk

All of the following, with evidence files to show for each:

- [ ] Every check listed for the chunk passes through `make check-<chunk>`, twice consecutively, starting from `make clean`.
- [ ] Every sabotage listed for the chunk turns at least one check red through `make sabotage-<chunk>`.
- [ ] `make regress` is green.
- [ ] `make lint` is green.
- [ ] Every measurement listed for the chunk is recorded in `evidence/<chunk>/…/measurements.json` with raw samples, and meets its target.
- [ ] Nothing on the chunk's denied list or the global denied list was built.
- [ ] No check, vector, threshold or expected verdict was modified (the frozen-file hashes of §7 still match).
- [ ] Public behaviour added by the chunk is documented in `docs/` in at most one page.

## 3. Definition of done for a milestone

- All its chunks are `DONE`.
- The milestone's **exit review** is written to `evidence/<milestone>/exit-review.md`. It answers, in order: (1) which properties from `00` §3 this milestone advanced and the measured evidence; (2) each headline number touched, target against achieved; (3) the two questions of `00` §5 answered honestly in three sentences each, naming the weakest point; (4) risks found that the spec did not anticipate.
- Where the milestone table says **human gate**, stop and wait (§10).

## 4. Sabotage checks

A test suite proves something only if it fails when the thing is broken. Each chunk lists sabotages: small, named, deliberate defects.

- Each sabotage is a patch file `test/sabotage/<chunk>/<name>.patch` against the implementation, never against tests.
- `make sabotage-<chunk>` applies each patch to a scratch copy, runs `make check-<chunk>`, and requires a non-zero exit. It then reports which check caught it.
- A sabotage that leaves the suite green means the suite is inadequate: strengthen the tests, never delete the sabotage.
- You may add sabotages. You may not remove or weaken the listed ones.

## 5. When stuck

A check is *stuck* when three consecutive iterations, each trying a materially different approach, fail on it.

1. Stop changing code for that check.
2. Append an entry to `spec/BLOCKED.md`: chunk, check ID, what was tried (three approaches, one line each), the exact failing output, your best hypothesis, and two or three options with their costs.
3. If another chunk is selectable under §1 step 2 without depending on the blocked one, continue there. Otherwise stop and report.

Stop immediately, without three attempts, when:

- passing would require changing a check, vector, threshold or denied item (file a change request, §6);
- a check needs something the environment cannot provide. Record `SKIPPED-ENV` with the probe output; a skipped check is not a pass and stays listed in the exit review;
- a spec statement contradicts another spec statement or an upstream fact you verified (§8).

## 6. Change requests

- File `spec/CHANGE_REQUESTS/NNN-<slug>.md` with: what you want changed, why (with data), the alternatives, and the effect on each property and headline number.
- Mark the affected chunk `BLOCKED-CR-NNN` and move on under §5.
- Only a human marks a change request `ACCEPTED`. Until then the spec stands.
- Targets marked [T] were set before measurement. A request to relax one must include at least five measurement runs and an explanation of where the time or error goes.

## 7. Frozen files

After the gate named for each, these are hash-pinned in `spec/FROZEN.sha256`, and `make regress` fails if any hash changes:

| Files | Frozen at |
|---|---|
| `test/vectors/ledger/**` (golden ledgers, mutation seeds) | M1 exit |
| `test/vectors/hooks/**` (recorded hook payloads) | M1 exit |
| `test/vectors/writ/**`, `test/vectors/clerk/**` (attenuation cases, recorded clerk outputs) | M2 exit |
| `test/vectors/detect/**` (per-detector positives and negatives) | Before M3.2 starts |
| `test/vectors/reconcile/**` (traces with expected reconciliation) | Before M3.4 starts |
| `bench/docketbench/scenarios/**` (manifests and oracles) | Before M3.2 starts |
| `spec/0*.md`, `spec/milestones/*.md` | From the start; changed only through §6 |

Adding new vectors is always allowed. Changing or deleting frozen ones is not.

**(Court) Vectors before detectors.** The detection vectors and the DocketBench scenarios for C1 are written and frozen in M3.1, before any detector exists (M3.2 onward). A detector written before its vectors are frozen is a failed chunk.

## 8. Facts to verify upstream

The specs name external tools and formats. `research/4_build_facts.md` records what was read on 15 Nov 2026. Before relying on any of them, check the upstream source at the version you pin and save the excerpt to `evidence/<chunk>/…/upstream/`:

- Claude Code: hook event names and payload fields, non-interactive mode flags, the streaming output format, session resume.
- Model Context Protocol: current revision, transports, how one server tells several client sessions apart; the Go SDK at the pinned version.
- `git worktree` at the pinned git version: locking, one branch per worktree, behaviour on Windows.
- The SQLite driver chosen in `02` §6.
- WebAuthn: assertion verification, allowed origins for a page served on a private network name (needed from M5, listed here so it is checked early).
- Inspect (the evaluation framework DocketBench follows): task and scorer interfaces at the pinned version.

If upstream differs from the spec in a way that matters, that is a contradiction: §5, then §6.

## 9. Commits

- One commit per chunk reaching `DONE`: `M<n>.<k>: <chunk title>`, body listing check IDs passed and the evidence folder.
- Work-in-progress commits are allowed on the working branch with prefix `wip:`.
- Never commit secrets. Fixture secrets match `CANARY_[A-Z0-9]{32}` and live under `test/`.
- Do not push to any remote, publish anything, open issues or pull requests, or post anywhere. Those are human actions.

## 10. Human gates

| Gate | After | What the human decides |
|---|---|---|
| G0 | M0 | Whether the field evidence supports the two central bets (`00` §4, C1 and C2). Kill criteria are in `M0` |
| G1 | M3 | Whether detection and reconciliation meet C1 on frozen scenarios. The specs for M4 to M8 are written only after this gate |

At a gate: write the exit review, set the milestone to `AWAITING-GATE` in `PROGRESS.md`, and stop.

## 11. Environment rules

- **(Court) Checks are hermetic and model-free.** `make check-*`, `make sabotage-*` and `make regress` make no network connection and call no model. Agents under test are the **scripted agent** (M1.4), which replays a fixed script of tool events and file edits. A check that needs a live model is a failed design.
- **(Court) Live runs are separate.** Targets named `make live-*` drive a real agent. They are never part of a chunk's definition of done. They need a human to supply credentials and a spending limit, and each run records its token count in the evidence folder.
- The build host and its rules are in `tools/host.md`. It is shared with the Countersign build: Court never touches the Countersign checkout, its kind cluster, its reviews or its processes.
- Tool versions are pinned in `tools/versions.env` at M1.1 and installed by `make tools` into `./.tools/` without root. Do not rely on globally installed versions.
- Checks pass on Linux (arm64 and amd64). Windows is checked from M1.3 for the world backend only, as `SKIPPED-ENV` where no Windows host is available.
- No real credentials are used in any check.

## 12. Measurement rules

- Every measurement states: command, machine description (`make sysinfo`), number of runs (minimum 5), all raw samples, median, and coefficient of variation. A result with variation above 10% is rerun on a quiet machine or reported as unstable.
- **(Court) Detection metrics** (recall, precision) are computed by `bench/docketbench/score` against frozen oracles, never by reading output by eye. Each figure is reported with its numerator and denominator.
- **(Court) Field metrics** (anything marked `[F]` in `00` §4) are reported with the number of subjects and days beside the figure, and never mixed with benchmark figures in one table.

## 13. Progress file

Progress is recorded on every loop iteration: status, attempt count, last evidence folder, a one-line note. Statuses: `TODO`, `IN_PROGRESS`, `DONE`, `BLOCKED`, `BLOCKED-CR-NNN`, `SKIPPED-ENV`, `AWAITING-GATE`.

Row format in a lane's progress file: `| chunk | status | attempts | last evidence folder | note |`.

## 14. Parallel sessions

Several sessions may run at once. The supervisor assigns each session a **lane**: a branch name and an explicit list of chunks.

- Work only on your lane's chunks, in order, on your lane's branch (`lane/<name>`). Never commit to `main`, never merge, never rebase onto another lane.
- Record progress in `spec/progress/<lane>.md` using the row format of §13. **Do not edit `spec/PROGRESS.md`**; the supervisor updates it when a lane is merged.
- If a chunk needs something from a chunk outside your lane that is not yet on `main`, stop that chunk and record `BLOCKED` with the missing dependency.
- Push your branch to `origin` after every `DONE` chunk and at least once an hour.
- The supervisor merges a lane chunk into `main` only after re-running its checks and sabotages on a fresh clone, with sabotages the lane has not seen.
- `field/` is not in the repository and is never to be requested, recreated or guessed at (M3 check C3.9.4).
