# 00. North star

## 1. Thesis

> **Agents decide things without saying so. Court finds those decisions, settles what a machine can settle, and brings the human only the choices that are theirs, while the work continues.**

Every existing queue for agent oversight contains what the agent chose to ask. The costly decisions are the ones the agent did not recognise as decisions. Court's queue is built from observation: what the agent declared is reconciled against what detectors found in its work and against what was expected before it started.

The unit of coordination is the **decision**, for the human and for the agents alike. Tasks, messages and diffs hang off decisions.

## 2. What may be claimed

No mechanism in Court is new as an idea (`../Prior art check.md`). Three things were not found anywhere on 15 Nov 2026 and may be claimed once measured:

1. A queue built by reconciling declared, detected and expected **decisions**, across a tree of agents, so that what an agent never raised becomes a question the human can rule on. Checking what an agent declared against what it did already exists at the level of tool calls and files (EPISTEME, CodeClone's Patch Trail, Claim Plane); none of it works on decisions or produces questions.
2. Fan-out and the overseer's comprehension measured together, over time, for coding agents.
3. A benchmark that scores an oversight system on benign failures **in a multi-agent setting**: misreports between agents, contests between parallel agents, false done-claims with clean twins. Undisclosed decisions by a single agent are already benchmarked on the monitor side (AgentMonBench).

Everything else is a design rule with a cited source. Closest work, to be read in full before the chunk named: ARCTIC (arXiv 2607.29516), EPISTEME (arXiv 2611.06563) and AgentMonBench (arXiv 2610.06406) before M3.1; PulseEngine's decision inbox before the docket is specified.

## 3. Properties

A property is a statement that holds at all times once its milestone is done. Each has a headline number in §4.

| ID | Property | From |
|---|---|---|
| P1 | **Complete record.** Every tool event and every commit of every agent in a commission is in the ledger. | M1 |
| P2 | **Deterministic logic.** Every verdict Court shows is a function of the ledger; replay reproduces it with no model call. | M1 |
| P3 | **Isolation.** An agent's work is confined to its world. Discarding a world restores the repository bit for bit. | M1 |
| P4 | **Observation over report.** Nothing is shown as true on an agent's word alone. A done-claim is shown as done only after Court ran its checks. | M1, M3 |
| P5 | **Attenuation.** No agent holds authority its parent did not hold. | M2 |
| P6 | **Expectations are fixed first.** The expected decisions are in the ledger before the first agent tool event of the commission. | M2 |
| P7 | **Reconciliation.** Every detected decision without a prior declaration is marked undeclared. Every expected decision that leaves no trace is marked unanswered. | M3 |
| P8 | **Vectors precede detectors.** Detection vectors and benchmark oracles are frozen before the detector they test exists. | M3 |
| P9 | **Function allocation.** No question of fact reaches the human. | after G1 |
| P10 | **No blocking.** No agent waits on a reversible decision. An overrule discards exactly the commits that rest on it. | after G1 |
| P11 | **Blind rulings.** The human chooses before seeing the agent's recommendation. | after G1 |
| P12 | **Recorded exposure.** What the human was shown is recorded, per area, and their comprehension is measured. | after G1 |

## 4. Headline numbers

`[T]` targets were set before measurement. `[B]` is measured on DocketBench (`04`). `[F]` is measured in real use and reported with the number of subjects and days.

### Phase 1: record and expectation (M1, M2)

| ID | Metric | Target |
|---|---|---|
| H1 | Tool events and commits produced by scripted agents that are missing from the ledger | **0** of ≥ 10,000 (P1) |
| H2 | Ledgers whose replay reproduces every stored verdict; seeded ledger mutations detected | **100%** of ≥ 200; **100%** of ≥ 1,000 (P2) |
| H3 | Repository digest after discarding a world equals the digest before it was created | **100%** of ≥ 200 randomised sessions (P3) |
| H4 | Added latency per tool event from Court's hook | p99 ≤ **50 ms** [T] |
| H5 | Generated writs that break attenuation and are accepted | **0** of ≥ 5,000 (P5) |
| H6 | Commissions where an agent tool event precedes the stored expected decisions | **0** (P6) |

### Phase 2: detection (M3)

| ID | Metric | Target |
|---|---|---|
| H7 | Each detector on its frozen vectors: positives caught; negatives flagged | **100%**; **0** |
| H8 `[B]` | Planted undeclared decisions surfaced (recall); surfaced items that are planted or judged real decisions (precision) | ≥ **80%**; ≥ **50%** [T] (C1) |
| H9 | Go reconciliation engine against the Python reference: identical verdict for every decision | **100%** of ≥ 5,000 random traces (P7) |
| H10 `[B]` | Done-claims whose done-check fails that are shown as done | **0** (P4, C8) |
| H11 | Detectors whose first commit precedes the commit that froze their vectors | **0** (P8) |
| H12 `[F]` | Undeclared decisions per 100 agent commits on real agent history (the Countersign build lanes), and the share a human labeller confirms as real decisions | reported; ≥ **50%** [T] |

### After G1 (specified later; listed so M1 to M3 record what they need)

| ID | Metric | Target |
|---|---|---|
| C2 `[B]` | Docket items that are questions of fact | ≤ 10% |
| C3 `[F]` | Blind rulings that differ from the agent's hidden recommendation | 20% to 60% |
| C4 `[B]` | Agent time blocked on a reversible decision | 0 |
| C5 `[B]` | After an overrule: dependent commits identified; unrelated commits lost; downstream checks broken | 100%; 0; 0 of sampled |
| C6 `[B]` | Planted sibling conflicts raised before merge | ≥ 90% |
| C7 `[B]` | Planted orchestrator misreports caught at 20% sampling | ≥ 90% within 25 tasks |
| C9 `[B]` | Statutes enacted that contradict a past ruling; split changes that evade a statute | 0; 0 |
| C10 `[F]` | Fan-out, week 4 against week 1, with canary catch rate not lower | ≥ 2× |
| C11 `[F]` | Prediction accuracy on comprehension probes, week 4 against week 1 | not lower |
| C12 | Install to first ruling | ≤ 10 minutes |
| C13 | Court's own tokens as a share of fleet tokens | ≤ 20% |
| C14 `[F]` | Sampled fact-settled decisions the human says should have been theirs (false-fact rate) | ≤ 5% |

## 5. The bar

Every milestone exit review answers two questions in three sentences each, naming the weakest point.

1. **Would the technical lead of a large technology company or a frontier lab learn something from this milestone's measured result that an approval inbox could not show them?**
2. **Could a developer who has never heard of Court use what this milestone built within ten minutes, without a manual and without a configuration file?**

A milestone that advances the first at the cost of the second has failed. The user surface is three commands (`commission`, `hold`, `progress`); nothing a milestone adds may require a fourth.

## 6. Global denied list

Building any of these is a failure, not a bonus.

| # | Denied | Why |
|---|---|---|
| D1 | A new agent framework, prompt library or model wrapper. Court pilots existing agent tools | Not the contribution |
| D2 | Any logic whose result depends on a model call that is not recorded in the ledger | P2 |
| D3 | A model deciding whether a deterministic detector fired, or filtering detector hits before they are recorded | P7; filtering is the clerk's job, after G1, and is itself recorded |
| D4 | Matching decisions on free text, embeddings or similarity | `01` §7.1 is the only matcher |
| D5 | Agents stating urgency, priority or cost of delay | `01` §10 |
| D6 | Kubernetes, containers as a requirement, a network proxy, kernel observation | Countersign's ground; Court is one local binary |
| D7 | A server, account system, cloud sync or telemetry | Local-first |
| D8 | More than one human, roles, or routing between reviewers | Out of version 1 |
| D9 | Agents creating their own commissions | Raises the human's load |
| D10 | Points, reputations or markets between agents | No evidence they change model behaviour |
| D11 | Cryptographic signatures, Merkle trees or attestations in M1 to M3 | The chain of `01` §11 is enough until human rulings are signed |
| D12 | A chat pane, a kanban board, a task tracker | Court coordinates over decisions |
| D13 | Themed vocabulary beyond six nouns: commission, hearing, ruling, law, writ, progress | Simplicity |
| D14 | A configuration file required to start | §5, question 2 |
| D15 | Editing a frozen vector, oracle or threshold to make a check pass | `05` §7 |
