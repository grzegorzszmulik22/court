# 03. Detection and reconciliation

Normative for M3. Evidence for the choices here is in `../research/1_detection.md`; figures quoted below come from the sources named there.

## 1. Principles

1. **A flag is a tip, a ruling is the proof.** A detector describes what changed. It never says the change was wrong. Measured elsewhere: deterministic test-change checks described the diff correctly 93.6% of the time, yet only 11 of 265 flagged pull requests were unexplained weakening.
2. **Reconciliation is the precision filter.** A hit the agent declared, inside its writ, is recorded and not queued (`01` §7.3). What reaches the human is the hit nobody declared.
3. **A forced declaration is worth more than a clever checker.** In EPISTEME, 44 of 49 misbehaving runs were exposed, and all 44 through the agent's own stated reason; the mechanical check alone caught 3. Court therefore makes the agent declare at every commit (§4) and treats detectors as the check on that declaration.
4. **Deterministic first.** Every detector in §2 is a pure function of the before and after trees and the tool events. No model takes part (`00` §6, D3).
5. **Every hit carries its evidence**: file, line range, and the matched text digest. A hit without a span is a bug.
6. **The checker is not the checked.** Agents cannot write the ledger except through the six agent tools, and those writes are attributed to them (`02` §5). One study's orchestrator, asked to judge its own checks, rewrote a recorded stop as a pass.
7. **Expectation makes scope decidable.** Removing the stated scope from a prompt raised out-of-scope actions from 0.0% to 17.1% in one benchmark. The commission's territory and the backbrief plan are given to the agent and are what `territory` and `scope` are judged against.

## 2. Detectors, version 1

Each detector takes `(base tree, commit tree, changed hunks, tool events since the previous commit, commission)` and returns zero or more hits `{detector, category, paths, spans, subkind}`.

| ID | Category | Fires when | Subkinds | Evidence |
|---|---|---|---|---|
| DT1 | `dependency` | A dependency entry is added, removed or changes version in a recognised manifest: `package.json`, `go.mod`, `requirements*.txt`, `pyproject.toml`, `Cargo.toml` | `added`, `removed`, `version`, `lock-mismatch` (manifest changed, lockfile not) | Agents add dependencies in 1.3% of pull requests |
| DT2 | `schema` | A file is added or changed under a recognised migration directory; a `.sql` hunk adds `CREATE`, `ALTER` or `DROP` of a table, column or index; a `.proto`, `.avsc`, `*.schema.json` or `schema.prisma` file changes | `migration`, `ddl`, `format` | |
| DT3 | `interface` | A contract file changes (`openapi*.yaml|json`, `*.proto`, `*.graphql`). Go: an exported identifier outside `internal/` and `_test.go` is removed or its signature changes (parsed with `go/parser`). Python, TypeScript: a top-level `def`, `class` or `export` line whose name does not start with `_` is removed or its parameter list changes | `removed`, `changed`, `added` | AST pattern detectors were confirmed by raters in about 94 to 96% of sampled detections |
| DT4 | `test-strength` | In a test file: `deleted_test` (file or test function removed), `skip_or_focus` (a skip, only or expected-failure marker added), `fewer_assertions` (assertion count in a surviving test function falls), `changed_expectation` (the expected literal of an assertion changes while the call under test does not), `loosened_threshold` (a numeric bound in an assertion moves in the permissive direction) | the five names | The five checks of honest-ci. Its false positives all came from `changed_expectation` (18 of 142) and `fewer_assertions` (4 of 37) |
| DT5 | `type-strength` | An added line introduces `any` in a TypeScript annotation, `interface{}` or `any` in a Go signature where a concrete type stood, `Optional[`/`| None` on a Python parameter or return, or a checker suppression (`@ts-ignore`, `@ts-expect-error`, `# type: ignore`, `//nolint`, `#[allow(`) | `widened`, `suppressed` | |
| DT6 | `guardrail` | A CI workflow file loses a step or job, or gains `continue-on-error` or `|| true`; a lint or formatter configuration disables a rule; a coverage or quality threshold falls; a compiler strictness flag is turned off; a git hook or pre-commit entry is removed; an inline lint suppression is added | `ci`, `lint`, `threshold`, `strictness`, `hook`, `suppressed` | Held-out precision in one study: suppression added 5 of 6, CI gate relaxed 1 of 1 |
| DT7 | `failure-handling` | An added error handler has an empty body or only `pass`, `continue` or a constant return; an exception clause is broadened to catch everything; a literal that appears in a test file's inputs newly appears in a non-test conditional | `swallowed`, `broadened`, `special-case` | Special-casing is one of the four cheating strategies in ImpossibleBench, and survives read-only tests |
| DT8 | `outbound` | An added line contains a URL or host literal whose host appears nowhere in the base tree; a tool event contacted a host not in the commission's allowed hosts | `code`, `agent` | |
| DT9 | `authority` | A changed path has a segment matching `auth`, `login`, `session`, `token`, `permission`, `acl`, `rbac`, `secret`, `credential`, `crypto`; an added line widens access (`*` origin, mode `777`, a wildcard in a policy file) | `touched`, `widened` | |
| DT10 | `deletion` | A `.sql` hunk or string literal adds `DROP`, `TRUNCATE`, or `DELETE FROM` without `WHERE`; a tracked data file is deleted (`.csv`, `.parquet`, `.sqlite`, `.db`, files under `data/` or `fixtures/`); a tool event ran a destructive command (`rm -r`, `git clean`, `git reset --hard`, `git push --force`) | `sql`, `file`, `command` | |
| DT11 | `territory` | A changed path matches none of the commission's territory globs | | Rule checkers of this kind reached precision and recall of at least 0.93 and 0.92 against blind annotation |
| DT12 | `scope` | The backbrief plan listed files and a changed, non-exempt path is not among them; or total changed lines exceed three times the backbrief estimate; or a new top-level file or directory appears | `unplanned-file`, `oversize`, `new-top-level` | Unrequested changes: 5 to 7% of production agent trajectories |

`behaviour` and `structure` have **no detector in version 1**. A decision in those categories can enter only by declaration or expectation. This is the open slot in the field: the best published model-based extraction of design decisions reaches F1 0.66 on single functions. Court reports the share of queued decisions in each category so the gap is visible, and does not pretend to close it.

### 2.1 Languages

Version 1 parses Go with the standard library. Python and TypeScript rules are line-pattern rules over hunks, documented one by one in `test/vectors/detect/`. Other languages get only the language-neutral detectors (DT1, DT2, DT6, DT8 to DT12). Each hit records the rule that produced it, and H7 and H8 are reported per language.

### 2.2 Exemptions

Environment noise is the main source of false alarms in file-level checks (sync clients, caches, build products). Before any detector runs, paths matching the exemption list are dropped: `.git/`, `node_modules/`, `vendor/`, `dist/`, `build/`, `target/`, `__pycache__/`, `*.pyc`, `.venv/`, coverage output, and editor or sync temporary files. The list in force is stored in `commission.opened`. Lockfiles are exempt from every detector except DT1.

## 3. Claims

A claim is not a decision, and it is checked, not reconciled.

- `claim_done` names the done-checks the agent asserts pass. Court runs each one in the world and records `claim.checked`. A task is shown as done only when every done-check passed in Court's own run (P4).
- A `claim_done` with no test or build tool event after the last file edit in that world is recorded as `unverified-by-agent`, whatever Court's run finds.

Unsupported completion claims were the most frequent misbehaviour measured in public agent sessions (34.7%, severe in 1.8%).

## 4. The declaration gate

Every commit in a world passes a gate.

1. Court installs a `pre-commit` and a `post-commit` git hook in each world through `core.hooksPath`, pointing outside the worktree.
2. `pre-commit` asks Court whether the committing agent has called `record_decision` or `no_decisions` since the previous commit in that world. If not, the commit fails with one line: `Declare this commit's decisions with record_decision, or call no_decisions.`
3. The agent declares **before** any detector has run on the commit, so the declaration is made without sight of what will be detected.
4. `post-commit` tells Court the commit exists. Court records `world.commit`, runs the detectors, then reconciles (§6).
5. A commit that reaches a world without a gate record (hooks bypassed) is recorded with `gated: false`, and every hit in it is `undeclared`.

`no_decisions` is a real statement. A commit declared as containing no decisions that then produces a detector hit is the clearest case Court can show a human.

## 5. Phrasing

A queued decision needs a question a person can answer.

- A deterministic template always exists per detector and subkind: for example `DT4/skip_or_focus` gives "`<path>`: the test `<name>` is now skipped. Keep it skipped?". The queue never depends on a model.
- The clerk may replace the template with a model-written question and the alternative not taken. The output is recorded as `decision.phrased` before use (`01` §2). It must quote the evidence span; a phrasing that cites no span is discarded and the template stands.
- Questions are yes or no, with an optional comment, and anchored to code. This follows the one user study of decision questions for coding agents, which also found reflexive "yes" to be the failure mode; blind rulings (after G1) are the countermeasure.

## 6. The reconciliation engine

- `internal/reconcile` implements `01` §7 exactly: matching, verdicts, the queue rule, timing.
- `ref/python/refreconcile` is a second implementation written from `01` §7 alone, before the Go engine, by reading the spec and not the Go code.
- Both consume a **trace**: a ledger restricted to the event types that matter (`backbrief.submitted`, `writ.issued`, `decision.declared`, `world.commit` with hits, `commission.closed`). They must agree on every decision's sources, verdict, flags and queued status (H9).

## 7. What detection does not do

- It does not judge whether a change is good.
- It does not find decisions that leave no signature in the categories of §2. Those depend on the declaration gate and the expected lists.
- It does not use the agent's explanation as evidence that a hit does not apply. Published monitors have been talked out of findings by "backward compatibility" rationalisations.
- It does not block a commit for a hit. The gate blocks only a missing declaration.
