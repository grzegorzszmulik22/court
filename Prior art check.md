# Prior-art check for Court

*15 Nov 2026. Twenty-four web and arXiv searches, each aimed at disproving one of the five claims in `PLAN.md` §13, plus twelve sources read directly. No patent search was done.*

## 1. Bottom line

**No mechanism in the plan is new as an idea.** Three of the five claims have to be reworded, and one (attention as the scheduling constraint) is mostly gone.

Two things were not found anywhere:

1. A system that builds the human's queue by **reconciling what the agent declared, what was detected in its work, and what was expected before it started**, so that decisions the agent never raised become items the human can rule on.
2. A measurement of **how many agents one person can oversee together with whether that person still understands the system**, tracked over time, for coding agents.

A third gap is narrower: no benchmark scores an oversight system on **benign** failures (silent decisions, misreports, false done-claims). Benchmarks exist for whether agents ask, and for oversight against deliberately hostile agents.

The field is moving quickly. The closest product shipped twelve days ago, and Anthropic published a study nineteen days ago that points at this design space by name. That confirms demand and shortens the window.

"Not found" means not found in one day of searching. Several papers were read only as abstracts or through a subagent's summary; §5 lists which.

## 2. Claim by claim

| # | Claim in the plan | Verdict | Closest prior art | What still appears to be Court's |
|---|---|---|---|---|
| 1 | The docket is built by reconciling declared, detected and expected decisions | **Partially exists.** Detection exists; the reconciliation was not found. | **ARCTIC** (Meta, arXiv 2607.29516): infers the intent of a change, scores drift between intent and diff, and highlights the regions to look at; deployed. "Agentic entropy" (arXiv 2604.16323): a deviation detector comparing stated intent with human-written architectural rules; a position paper, not built. adrchive: an audit mode that finds undocumented decisions (page could not be read). Transluce Docent: found an agent writing down decisions the user never made. Judgement-call logs and `DECISIONS.md` files: filled in by hand. | A list of expected decisions written before work starts, reconciled automatically against what the agent declared and what detectors found. The output is a rulable queue item, and recall is measured. |
| 2 | No question of fact reaches the human | **Exists as a principle.** Not found as an enforced gate with a metric. | Mitchell, Ghosh and Passi (arXiv 2608.23642) recommend "automated pre-checks" for "properties that do not require judgement". Anthropic's auto mode and OpenAI's auto-review put a model reviewer at the permission boundary. ACCORD (arXiv 2608.27818) separates what must be asked from what the agent should find out. AbstainEQA has a "preference dependence" category. Anthropic's judgment-call study says agents "would need to raise only the decisions that matter most". | The clerk as a hard gate, and two published numbers: the share of queue items that are questions of fact, and how often the human disagrees with the agent on the rest. |
| 3 | The human's exposure and comprehension are recorded and measured alongside leverage | **Problem well described; instruments partly exist.** The pairing over time was not found. | Mitchell et al. make the Bainbridge argument for agents and recommend pre-commitment, canaries and monitoring of review time and override rate. They do not measure comprehension. Explyt's quiz gate is a proposal, per change, not tracked over time. Vibe-GUIDE (arXiv 2609.23859) tested a comprehension interface on 16 developers and found no clear effect on its main measure. Cummings and Mitchell (2008) showed that lost situation awareness is the main limit on how many vehicles one operator can supervise. | A ledger of what the human was shown, per area, taken from signed ruling records. Comprehension probes in routine use. The two curves reported together. |
| 4 | Human attention is the scheduling constraint for the fleet | **Exists as practice, and partly built.** | **PulseEngine** (3 Nov 2026): a decision inbox with an attention budget (at most three flows asking at once), decision windows twice a day, an urgent bypass, "an irreversible step always waits", and agents that continue with work that does not depend on the answer. Shipped as a plugin; the author says nothing has been measured. Several practitioner posts say to cap active agents at review capacity. Olsen and Wood (2004) defined **fan-out**, the number of robots one person can operate, from how long a robot can be neglected and how long each interaction takes. | Only one detail: starting less work when the forecast number of rulings exceeds the budget. PulseEngine limits what is shown, not what is started. |
| 5 | A benchmark for the oversight system itself | **Partially exists in two neighbouring fields.** | Whether agents ask: HiL-Bench, AMBIG-SWE, ClarifyBench, DiscoBench, Ask2Decide (arXiv 2611.13015). Oversight against hostile agents: ControlArena and BashArena score monitoring protocols with a fixed human auditing budget against planted attacks. | Scoring the system around an agent on benign failures: decisions never raised, orchestrator misreports, false done-claims, sibling conflicts, split changes. |

## 3. Supporting mechanisms

| Mechanism (`PLAN.md`) | Verdict | Prior art |
|---|---|---|
| Default and deadline; the agent keeps working (§5.2, §5.4) | **Exists** | Scerri, Pynadath and Tambe (2002), "transfer-of-control strategies": the agent weighs asking, waiting and acting as time passes. PulseEngine. Atomix, Cordon. |
| Oversight leverage as the headline metric (§8, C10) | **Exists, under another name** | Fan-out and neglect tolerance (Olsen and Goodrich 2003; Olsen and Wood 2004; Crandall et al. 2005). Not found applied to coding agents: "The Work Behind Delegation" (arXiv 2609.24234) adapts Sheridan's supervisory-control stages to coding agents from 19 developers and does not use these metrics. |
| Blind rulings (§5.4) | **Exists as a recommendation, now with strong evidence** | Mitchell et al.: "pre-commitment". Anthropic (27 Oct 2026): 50 analysts matched their agent's randomly assigned lean on 86% of choices; 77% repeated its arguments and 15% credited it; expertise did not reduce this. The study suggests agents "ask experts for their own view first". |
| Counter-case (§5.3) | **Exists as a recommendation** | The same Anthropic study: agents that "make the best case for each option". Debate research. |
| Canaries (§5.4) | **Exists** | Mitchell et al.; airport-screening practice. |
| Law proposed from repeated rulings, with a backtest (§5.5) | **Exists** | Hedwig (arXiv 2605.11495): a classifier over 13 features updated after each developer decision. Pushary: proposes a rule after five consistent decisions, never applies it itself. An AWS write-up replays a proposed policy on labelled cases before activation. Countersign `05` §13.4. |
| Shared decision record across agents (§5.2) | **Exists** | Pawsly (Jindo AI, Oct 2026): agents declare an intended action, and it is checked against the team's recorded decisions. Its authors report agents agreed more and were not more correct, at about 1.5 times the steps. Shepherd: leases on parts of the code. |
| Assumption tracking and exact rollback (§5.2) | **Exists as an idea** | Atomix; Cordon; an AWS write-up on revoked approvals using a dependency graph. |
| Audit that bypasses the orchestrator (§5.6) | **Partially exists** | A public issue asks that a conductor check a lane's account against gate evidence. Verification-aware planning (arXiv 2510.17109). Random sampling that yields a report-fidelity score per orchestrator was not found. |
| Backbrief with a blind second list (§5.1) | **Partially exists** | Plan modes and plan-review tools (Plannotator compares plans with diffs by hand). The independent second list was not found. |
| Rolling-window statutes against split changes (§5.5) | **Not checked** | No search was run for this. |

## 4. What this changes

1. **Reword the claim.** Court is not "the first decision queue" or "the first to budget attention". It is the first to treat what the agent did not ask as the main input, and to measure oversight capacity and the overseer's understanding together.
2. **Blind rulings become the default for every question of will**, not only irreversible ones. The 86% figure means a visible recommendation mostly decides the ruling. Hypothesis C3 (how often the human departs from the recommendation) is therefore measured on blind rulings.
3. **Use the existing name.** The headline metric is fan-out, with neglect tolerance and interaction time as its parts, cited to the robotics work. That gives Court twenty years of method to stand on and an untried application.
4. **Claim 4 is demoted** from a novelty claim to a design rule. PulseEngine is cited.
5. **DocketBench should reuse control-evaluation conventions** (ControlArena is built on Inspect) so results sit beside that literature.
6. **Add the older results the plan missed**: fan-out and neglect tolerance, Cummings and Mitchell's capacity model, and Scerri et al.'s transfer of control.
7. **Read before G1:** ARCTIC in full. It is the nearest thing to Court's detectors and is deployed at scale.

## 5. Limits of this check

- Read in full: the PulseEngine post, the Anthropic study's blog page, the Explyt article.
- Read in part, the rest through a subagent's summary: arXiv 2609.24234, 2608.23642, 2609.23859, 2605.11495, and the Pawsly post.
- Abstract or search summary only: ARCTIC, ACCORD, AbstainEQA, Ask2Decide, the control-evaluation papers, the robotics papers.
- Not read: chapter 16 of "Engineering Reliable Coding Agents" (arXiv 2608.13867), which covers risk-based escalation; the adrchive page; the Lockstep product.
- Pawsly's and PulseEngine's figures are their authors' own.

## 6. Addendum: deeper read, later the same day

Four research passes read the closest work in full where the first pass had only abstracts. Notes: `research/1_detection.md`, `2_benchmarks.md`, `3_human_factors.md`, `4_build_facts.md`, `5_second_model_review.md`. Tools: context.dev search and scraping, Scrapling, and DeepSeek as a second-model critic. Firecrawl was not available in the session, and the DeepSeek API has no web search.

### 6.1 Claims narrowed again

| Claim | New prior art | What is left |
|---|---|---|
| 1. Reconciling declared, detected and expected | **EPISTEME** (arXiv 2611.06563): every shell call needs a declared type and reason, checked against observed effects. **CodeClone "Patch Trail"**: the agent declares intent and file scope, the tool sets do-not-touch boundaries, and a finish step checks the actual change. **Claim Plane** (arXiv 2607.21909) and a gated spec-driven workflow study (arXiv 2611.05094) do the same for declared scope. Aporia and AssumptionMiner surface decisions, but from the model's own report (decision-level F1 0.66). | The reconciliation exists at the level of tool calls and files. Not found: at the level of **decisions**, across a tree of agents, producing questions a human rules on. |
| 3. Benchmark of oversight on benign failures | **AgentMonBench** (arXiv 2610.06406): scores monitors on consequential decisions a single agent never surfaced; three subsets of 100. | Single-agent undisclosed decisions are covered. Not found: orchestrator misreports, contests between parallel agents as a detection task, false done-claims in code with clean twins, benign split changes. |

ARCTIC, read in full, is further from Court than its abstract suggested. It checks inferred intent against a summary of the diff, never against what the agent declared; it states that "beneficial unrequested changes are not penalized"; its drift score is weak in the middle of its scale (F1 0.34 and 0.41, n = 118).

### 6.2 Measured base rates that changed the design

| Finding | Source | Effect on Court |
|---|---|---|
| Deterministic test-change checks describe the diff correctly 93.6% of the time, but only 11 of 265 flagged pull requests were unexplained weakening | honest-ci study | A detector hit is a question, never an accusation. Reconciliation is the precision filter |
| 44 of 49 misbehaving runs were exposed through the agent's own stated reason; the mechanical check alone caught 3 | EPISTEME | A declaration gate at every commit |
| Unsupported completion claims in 34.7% of public agent sessions | Transluce | Done-claims are always re-run by Court |
| Agents rarely weaken tests (0.3% of pull requests); they bend the code to fit a wrong test, or add a quiet fallback | repopilot, ImpossibleBench | A `failure-handling` detector and category |
| Removing the stated scope raised out-of-scope actions from 0.0% to 17.1% | OverEager-Bench | Territory and plan are given to the agent and judged against |
| A model orchestrator asked to judge its own checks rewrote a recorded stop as a pass | gated workflow study | Agents cannot write the ledger; the checker is a separate process |
| Best published extraction of design decisions by a model: F1 0.66 on single functions | AssumptionMiner | No detector for `behaviour` or `structure` in version 1; stated openly |

### 6.3 Other findings

- **Cumulative-change rules.** No code-review or CI tool was found that budgets change across separate pull requests over a window. The analogues are in law: the cash-structuring offence (31 CFR 1010.314) and the ban on splitting purchases to stay under a threshold (FAR 13.003(c)(2)).
- **Integration.** Claude Code, Codex CLI and Devin CLI all document hooks of the same shape, and Devin reads Claude Code hook files by default. One hook endpoint can observe all three.
- **Second-model review.** DeepSeek's strongest objections: agents could game the queue by claiming urgency; the fact and will boundary drifts toward fact because facts cost the human nothing; "reversible" often means only that code reverts. Each is answered in the spec (`spec/01-decision-model.md` §5, §8, §10).
- **To read:** "Not Self-Decidable: LLMs Cannot Draw the Boundary of What an Agent Verifier Can Check" (arXiv 2610.04699), seen only as a search result. It bears on fact and will.

## Sources

- [EPISTEME (arXiv 2611.06563)](https://arxiv.org/html/2611.06563)
- [CodeClone Patch Trail](https://dev.to/orenlab/your-ai-agents-diff-looks-fine-thats-the-problem-de4)
- [Claim Plane (arXiv 2607.21909)](https://arxiv.org/abs/2607.21909)
- [A Gated Spec-Driven Workflow (arXiv 2611.05094)](https://arxiv.org/html/2611.05094)
- [AgentMonBench (arXiv 2610.06406)](https://arxiv.org/abs/2610.06406)
- [Aporia (arXiv 2604.05203)](https://arxiv.org/html/2604.05203)
- [AssumptionMiner (arXiv 2607.22898)](https://arxiv.org/html/2607.22898)
- [honest-ci study](https://dev.to/chrisships/coding-agents-weaken-tests-about-as-rarely-as-people-do-513)
- [repopilot study](https://dev.to/cherven/358-pull-requests-that-changed-tests-agents-rarely-weakened-them-they-bent-the-code-instead-3ld)
- [ImpossibleBench (arXiv 2510.20270)](https://arxiv.org/html/2510.20270)
- [OverEager-Bench (arXiv 2605.18583)](https://arxiv.org/abs/2605.18583)
- [Not Self-Decidable (arXiv 2610.04699)](https://arxiv.org/abs/2610.04699v2)
- [Tirole: Hierarchies and Bureaucracies (1986)](https://academic.oup.com/jleo/article-abstract/2/2/181/848407)
- [Dawes, Faust and Meehl: Clinical versus actuarial judgment (1989)](https://www.science.org/doi/10.1126/science.2648573)
- [ARCTIC: From Code Review to Code Critique (arXiv 2607.29516)](https://arxiv.org/pdf/2607.29516)
- [Beyond the "Diff": Agentic Entropy (arXiv 2604.16323)](https://arxiv.org/abs/2604.16323)
- [adrchive](https://pypi.org/project/adrchive/)
- [Transluce: Measuring coding agent misalignment in the wild](https://transluce.org/docent/blog/coding-agent-behaviors)
- [Your coding agent has opinions (DEV)](https://dev.to/betatesterlife/your-coding-agent-has-opinions-log-the-calls-it-made-for-you-3i0c)
- [Anthropic: How AI agents shape experts' judgment calls](https://www.anthropic.com/research/agents-judgment-calls)
- [AI Agents Push Humans Out of the Loop (arXiv 2608.23642)](https://arxiv.org/abs/2608.23642)
- [Engineering Reliable Coding Agents (arXiv 2608.13867)](https://arxiv.org/pdf/2608.13867)
- [ACCORD (arXiv 2608.27818)](https://arxiv.org/pdf/2608.27818)
- [AbstainEQA (CVPR 2026)](https://openaccess.thecvf.com/content/CVPR2026/papers/Wu_When_Robots_Should_Say_I_Dont_Know_Benchmarking_Abstention_in_CVPR_2026_paper.pdf)
- [Ask2Decide (arXiv 2611.13015)](https://arxiv.org/pdf/2611.13015)
- [DiscoBench (arXiv 2606.27669)](https://arxiv.org/pdf/2606.27669)
- [ClarifyBench / SAGE-Agent (ACL Findings 2026)](https://aclanthology.org/2026.findings-acl.2028.pdf)
- [HiL-Bench (arXiv 2604.09408)](https://www.alphaxiv.org/abs/2604.09408)
- [Explyt: a quiz gate for AI-generated code](https://explyt.ai/en/blog/comprehension-debt-quiz-gate)
- [Vibe-GUIDE (arXiv 2609.23859)](https://arxiv.org/pdf/2609.23859)
- [Cognitive Debt in the GenAI Era (arXiv 2610.37888)](https://arxiv.org/html/2610.37888v1)
- [Cummings and Mitchell: Predicting Controller Capacity (MIT DSpace)](https://dspace.mit.edu/handle/1721.1/90280)
- [PulseEngine: The scarce resource is the orchestrator's attention](https://pulseengine.eu/blog/the-scarce-resource-is-attention/)
- [Code Got Cheap. Your Attention Didn't. (DEV)](https://dev.to/max_quimby/code-got-cheap-your-attention-didnt-4fm3)
- [Building an Autonomous Coding Fleet That Actually Scales](https://brianjamescurry.substack.com/p/building-an-autonomous-coding-fleet)
- [Olsen and Goodrich: Metrics for Evaluating Human-Robot Interactions (2003)](https://faculty.cs.byu.edu/~mike/mikeg/papers/OlsenGoodrichPERMIS2003.pdf)
- [Olsen and Wood: Fan-out (CHI 2004)](https://hci.rwth-aachen.de/materials/conferences/CHI2004/1p231.pdf)
- [Scerri, Pynadath and Tambe: Towards Adjustable Autonomy for the Real World (2002)](https://arxiv.org/pdf/1106.4573)
- [The Work Behind Delegation (arXiv 2609.24234)](https://arxiv.org/pdf/2609.24234)
- [Hedwig (arXiv 2605.11495)](https://arxiv.org/abs/2605.11495)
- [Pushary: how permission policies learn your defaults](https://pushary.com/blog/how-permission-policies-learn-your-defaults)
- [AWS: guardrails and human approval gates](https://builder.aws.com/content/3JFVF2VqkKqTA9DZkzC9OzvbVQT/agents-for-humans-guardrails-and-human-approval-gates)
- [Pawsly: Four AI Coding Agents, One Codebase](https://hackernoon.com/four-ai-coding-agents-one-codebase-and-a-coordination-problem)
- [Adaptive Attacks on Trusted Monitors (arXiv 2510.09462)](https://arxiv.org/pdf/2510.09462)
- [ControlArena (UK AI Security Institute)](https://www.aisi.gov.uk/blog/introducing-controlarena-a-library-for-running-ai-control-experiments)
- [Conductor should verify a lane's claim (GitHub issue)](https://github.com/remigiusz-antczak/deep-code-review/issues/1213)
- [Verification-Aware Planning for Multi-Agent Systems (arXiv 2510.17109)](https://arxiv.org/pdf/2510.17109)
- [Plannotator: approve a plan before execution](https://docs.plannotator.ai/learn/code-context/approve-ai-coding-plan-before-execution)
