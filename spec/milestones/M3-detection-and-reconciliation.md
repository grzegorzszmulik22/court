# M3. Detection and reconciliation

**Goal.** Find the decisions agents made without saying so. Every commit passes a declaration gate; detectors describe what the commit changed; the engine reconciles declared, detected and expected; claims of done are checked; contests between worlds are raised. The output of M3 is a report in the terminal, not yet a docket.

**Properties advanced.** P4 (observation over report), P7 (reconciliation), P8 (vectors precede detectors). Headlines H7 to H12.

**Normative references.** `01` §4, §7, §11; `03` in full; `04` §1 to §7.

**Read before starting.** `../research/1_detection.md` and `../research/2_benchmarks.md` in full. ARCTIC (arXiv 2607.29516) sections on drift and its limitations.

**Expected results.**
- A commit declared as "no decisions" that skips a test, adds a dependency or swallows an error produces a queued item naming the file and line.
- The same change, declared inside the agent's writ, produces none.
- Two agents that choose differently on the same ground are told apart before either merges.
- A task is never shown as done on the agent's word.
- Two independent implementations agree on every verdict.

**Measurements [T].**

| ID | Metric | Target |
|---|---|---|
| MM3.1 | Each detector on its frozen vectors: positives caught; negatives flagged | 100%; 0 (H7) |
| MM3.2 | DocketBench A: recall; precision; twin flag rate | ≥ 80%; ≥ 50%; ≤ 10% (H8) |
| MM3.3 | Go engine against Python reference on random traces | identical on 100% of ≥ 5,000 (H9) |
| MM3.4 | DocketBench D: failing or unrun done-checks shown as done; clean twins shown as not done | 0; 0 (H10) |
| MM3.5 | DocketBench C: contests raised before merge; twins flagged | ≥ 90%; ≤ 10% |
| MM3.6 | Detector time per commit on a 2,000-line diff | p99 ≤ 500 ms |
| MM3.7 | Field history (M0.2): undeclared decisions per 100 commits; share matching a human label; human-labelled decisions found | reported; ≥ 50%; reported (H12) |

**Milestone-wide denied.** A model anywhere in detection, reconciliation or scoring (`00` D2, D3, D4). Detectors for `behaviour` or `structure`. Blocking a commit for a detector hit. Any web interface. Editing a frozen vector or scenario (`05` §7). Tuning a detector on the M0.2 history or on the held-out set. Severity or risk scores on hits.

---

## M3.1 Vectors and scenarios first

**Build.** Nothing executable except validators.

- `test/vectors/detect/DT<n>/`: for each detector, at least 20 positive and 20 negative cases per supported language. A case is a before tree, an after tree, optional tool events, and the expected hits (detector, category, path, lines, subkind). Negatives include the near misses named in `03` §2: an assertion moved, not removed; a threshold tightened; a dependency reordered; a comment containing a host name.
- `test/vectors/reconcile/`: at least 60 hand-written traces with expected sources, verdict, flags and queued status per decision, derived by reasoning from `01` §7 and reviewed by reading. Required coverage: each verdict; `late` by one commit; `unforeseen` combined with each of the first four; empty `paths` with and without a competing entry; `other` with and without overlapping paths; `outside-writ` for a guarantee category that was declared; an expected decision ruled in the upfront hearing; a bypassed gate.
- `bench/docketbench/scenarios/{A,C,D}/` at the M3 sizes of `04` §5, with twins.
- `bench/docketbench/validate`: checks the rules of `04` §2.
- `spec/FROZEN.sha256` updated with all of the above in one commit, whose id is recorded in `evidence/M3.1/freeze.txt`.

**Checks.**
- C3.1.1 Counts meet the minimums above; every planted scenario names a twin and the twin names it back.
- C3.1.2 Every scenario's repository test suite passes with the script applied (subsets A and C), run by the validator.
- C3.1.3 Necessity: for every planted scenario, the scripted agent's final message and the `git diff --stat` do not contain the registry entry's `description` keywords (a fixed keyword list per entry in the registry).
- C3.1.4 Every registry entry cites a row of `research/1_detection.md` in a `source` field.
- C3.1.5 No file under `internal/detect/` or `internal/reconcile/` exists at the freeze commit (H11).
- C3.1.6 Category coverage in subset A: every category that has a detector appears in at least 4 planted scenarios; each variant of `04` §5 in at least 8.

**Sabotage.** `validator-ignores-missing-twin`; `validator-skips-repo-tests`; `validator-accepts-empty-registry-in-planted`.

**Denied.** Generating scenarios by mutating one template more than five times. Scenarios in which the planted item is the only change.

## M3.2 The declaration gate

**Build.** `internal/gate`: installation of `pre-commit` and `post-commit` hooks through `core.hooksPath` to a directory outside the world; the `no_decisions` agent tool; the gate query (has this agent declared since the previous commit in this world); `gated: false` marking for commits that arrive without a gate record.

**Checks.**
- C3.2.1 A scripted commit with no declaration fails with exactly the message of `03` §4, and the world has no new commit.
- C3.2.2 After `record_decision` or `no_decisions`, the same commit succeeds, and `world.commit` follows the declaration in the ledger.
- C3.2.3 A commit made with hooks bypassed is recorded with `gated: false` within one second of the next ledger write, and its hits are `undeclared`.
- C3.2.4 A declaration made for one world does not open the gate in another.
- C3.2.5 The hooks directory is not inside any world and is not writable through any agent tool path the adapter allows (tested with the scripted agent attempting an edit).
- C3.2.6 Hooks already present in the repository still run, after Court's.

**Sabotage.** `gate-always-open`; `gate-shared-across-worlds`; `ungated-commit-not-marked`.

**Denied.** Rejecting a commit for its content. Rewriting the agent's commit message.

## M3.3 Detectors DT1 to DT6

**Build.** `internal/detect`: the detector interface of `03` §2, the exemption filter of `03` §2.2, and detectors DT1 (dependency), DT2 (schema), DT3 (interface), DT4 (test-strength), DT5 (type-strength), DT6 (guardrail). Go parsing with the standard library; Python and TypeScript by the documented line rules.

**Checks.**
- C3.3.1 MM3.1 for DT1 to DT6, per language.
- C3.3.2 Every hit has a path, a line range inside the changed hunks, and a text digest.
- C3.3.3 Detectors are pure: the same inputs give byte-identical hits across 100 runs and across two machines' evidence.
- C3.3.4 An exempt path produces no hit from any detector except lockfiles for DT1.
- C3.3.5 The detector package imports no network, process or model package (import-graph test).
- C3.3.6 DT4's two line-pairing checks (`changed_expectation`, `fewer_assertions`) each have at least 10 negative vectors drawn from the false-positive shapes reported for honest-ci.

**Sabotage.** `dt1-ignores-version-change`; `dt4-counts-assertions-file-wide` (not per test function); `dt6-misses-continue-on-error`; `exemptions-not-applied`; `hit-without-span`.

**Denied.** Language support beyond `03` §2.1. Configurable detector rules. A plug-in interface.

## M3.4 Detectors DT7 to DT12

**Build.** DT7 (failure-handling), DT8 (outbound), DT9 (authority), DT10 (deletion), DT11 (territory), DT12 (scope). DT8 and DT10 also read tool events; DT11 and DT12 read the commission and the backbrief plan.

**Checks.**
- C3.4.1 MM3.1 for DT7 to DT12, per language.
- C3.4.2 DT7 `special-case`: a literal taken from a test file's inputs and added to a non-test conditional is caught; the same literal added to another test is not.
- C3.4.3 DT8: a host that already appears anywhere in the base tree produces no hit; a host seen only in a tool event produces subkind `agent`.
- C3.4.4 DT11 and DT12 produce no hit when the commission has no territory and the backbrief listed no files, and say so in the report ("no expectation to judge against").
- C3.4.5 MM3.6.

**Sabotage.** `dt7-ignores-broadened-except`; `dt8-compares-full-url-not-host`; `dt11-treats-glob-as-prefix`; `dt12-oversize-uses-added-lines-only`.

**Denied.** Secret scanning. Vulnerability lookups for dependencies. Reading file contents of deleted data files.

## M3.5 Reconciliation: reference, then engine

**Build.** First `ref/python/refreconcile`, written from `01` §7 alone. Then `internal/reconcile` in Go. A random trace generator producing declarations, hits, expectations, writs and commit orderings.

**Checks.**
- C3.5.1 The Python reference passes every trace in `test/vectors/reconcile/` before any Go reconciliation code exists (commit order recorded in evidence).
- C3.5.2 MM3.3.
- C3.5.3 `court replay` over ledgers produced by the DocketBench scenarios reproduces every `decision.reconciled` event.
- C3.5.4 A decision is queued exactly when `01` §7.3 says so: a property test over ≥ 5,000 generated decisions checks both directions.
- C3.5.5 An expected decision ruled in the upfront hearing (M2.5) is inherited by the matching later decision, which is then `ruled` and not queued, unless it is in a guarantee category.

**Sabotage.** `match-ignores-paths`; `late-treated-as-confirmed`; `unanswered-never-emitted`; `guarantee-category-inside-writ`; `other-matches-other-without-paths`.

**Denied.** Any matching rule beyond `01` §7.1. A shared library between the Go engine and the Python reference.

## M3.6 Claims

**Build.** `internal/claims` per `03` §3: on `claim_done`, run each named done-check in the world, record `claim.checked`; compute `unverified-by-agent` from tool events.

**Checks.**
- C3.6.1 MM3.4 on DocketBench D.
- C3.6.2 A done-check that passes in Court's run but that the agent never ran after its last edit yields done **and** `unverified-by-agent`.
- C3.6.3 A done-check that hangs is stopped at the commission's time budget and recorded as failed.
- C3.6.4 Done-checks run with the same outward-action denial as the agent (`02` §4).

**Sabotage.** `claim-trusted-without-run`; `check-run-in-main-repo-not-world`; `timeout-recorded-as-pass`.

**Denied.** Parsing the agent's prose for claims. Scoring partial completion.

## M3.7 Contests

**Build.** Cross-world comparison per `01` §7.5, evaluated at each observed commit.

**Checks.**
- C3.7.1 MM3.5 on DocketBench C.
- C3.7.2 Two worlds making the byte-identical change produce no contest.
- C3.7.3 A contest is recorded before any merge of either world (ledger order test).
- C3.7.4 Three worlds contesting one ground produce one queued item naming all three.

**Sabotage.** `contest-only-at-merge`; `identical-change-contested`; `contest-per-pair-not-per-ground`.

**Denied.** Judging compatibility of the two choices. Automatic merging or resolution.

## M3.8 Phrasing and the report

**Build.** The deterministic question template for every detector and subkind (`03` §5); intake of a recorded `decision.phrased` event with the span-citation rule; `court report <commission>`: a terminal listing of queued decisions grouped by verdict, each with question, world, agent, file and lines, and the counts by `recon` and category.

**Checks.**
- C3.8.1 Every detector and subkind has a template; a test enumerates them against the detector registry.
- C3.8.2 A recorded phrasing that quotes no evidence span is discarded and the template is shown.
- C3.8.3 `court report` output is identical on replay of the same ledger.
- C3.8.4 The report fits a decision in at most four lines at 100 columns, and shows categories with no detector as "declared or expected only".
- C3.8.5 With no model configured, the report is complete.

**Sabotage.** `phrasing-accepted-without-span`; `report-reads-live-state-not-ledger`.

**Denied.** Colour themes, interactive selection, or ruling from the report (rulings arrive with the docket, after G1).

## M3.9 Scoring and measurements

**Build.** `bench/docketbench/score` per `04` §6; the runner that plays every scenario through Court on the scripted agent; the field run over `field/history/` (M0.2), which the human copies in at that point, which replays each labelled commit through the detectors with an empty declaration and compares hits with the human labels.

**Checks.**
- C3.9.1 MM3.2, MM3.4, MM3.5, each with numerator and denominator, per category and per language.
- C3.9.2 The scorer run on a system stub that queues everything gets precision below 20% and twin flag rate 100%; on a stub that queues nothing, recall 0. Both are recorded.
- C3.9.3 MM3.7, reported with the number of commits and the agreement figure from M0.2.
- C3.9.4 The M0.2 files were first read by a process after the freeze commit of M3.1 and after M3.4 was `DONE` (file access recorded by the runner).
- C3.9.5 The exit review lists the ten lowest-precision detector subkinds with one example each.

**Sabotage.** `scorer-matches-on-category-only`; `scorer-drops-twins`; `score-counts-duplicates-as-hits`.

**Denied.** Changing a detector after seeing MM3.7 without a change request. Reporting an aggregate without its per-category table.

---

## Gate G1

The human supplies the held-out set (`04` §7) and runs `make live-heldout`. The exit review states, for in-repository and held-out sets side by side: recall, precision, twin flag rate, and the categories where they differ most.

G1 passes when the held-out set meets H8. If it does not, detection is redesigned through change requests before anything is specified on top of it. The specs for the clerk, the docket, law, and progress are written after this gate, from what M3 measured.
