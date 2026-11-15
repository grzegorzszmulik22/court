# 5. Second-model review (DeepSeek, 15 Nov 2026)

A critical review of the design by a model from another family (`deepseek-v4-pro`), asked for missed older results, the strongest objections, likely failures, and one falsifiable headline claim. The DeepSeek API has no web search, so this is recall and critique, not research. The citations in part A were given from the model's memory; Tirole 1986, Dawes et al. 1989 and Kaplow 1992 were then confirmed by search. The rest are unverified.

The prompt described the mechanisms without naming the product or its author.

What was adopted: `../spec/01-decision-model.md` §5 (mechanical checks before model review; false-fact rate), §8 (reversibility is demonstrated), §10 (cost of delay is computed, never claimed); `../spec/04-benchmark.md` §7 (held-out set).

---

A.
1. Janis 1972, Victims of Groupthink (Houghton Mifflin). Cohesive groups suppress dissent and converge prematurely. Affects M1. Keep backbrief lists private, compare them before any reconciliation, and use a devil’s advocate on merged list. Confidence high.

2. Stasser & Titus 1985, JPSP. Groups over-discuss shared information and ignore unshared information. Affects M1. Force exchange of all unique decisions before discussion and score overlap/non-overlap explicitly. Confidence high.

3. Dawes, Faust & Meehl 1989, Science. Actuarial/mechanical rules outperform case-by-case clinician judgment. Affects M3. Settle “fact” decisions with pre-registered tests or deterministic checks; use a model reviewer only for the residual unexplained class. Confidence high.

4. Lichtenstein & Slovic 1971, J Exp Psych. Preference reversals between choice and bid forms. Affects M5. Elicit “will” decisions in the same form as their real consequences and periodically retest the human on equivalent cases. Confidence high.

5. Schauer 1991, Playing by the Rules. Rules are inevitably over- and under-inclusive compared with standards. Affects M6. Track statute misclassification and exception rates, not only backtest accuracy. Confidence high.

6. Kaplow 1992, Duke Law Journal. Rules have higher ex ante cost but lower ex post adjudication cost; frequency decides which wins. Affects M6. Require observed frequency and enactment/adjudication cost estimates before converting rulings to statutes. Confidence high.

7. Diver 1983, Yale Law Journal. More precise rules can create more indeterminacy and poor fit than vaguer standards. Affects M6. Audit statutes against previously unseen edge cases before enactment. Confidence high.

8. Alchian & Demsetz 1972, AER. Team production causes shirking unless a monitor is rewarded on independent output. Affects M7. The auditor is also an agent and must be rewarded on externally verified outcomes, not on orchestrator reports. Confidence high.

9. Tirole 1986, JLEO. Supervisors and agents may collude; audit reporting can be undermined by side contracts. Affects M7. Rotate auditors, avoid repeated agent/auditor dyads, and use random external audits. Confidence high.

10. Denning 1987, IEEE TSE. Intrusion detection needs both misuse signatures and anomaly detection; signature-only detection misses novel attacks. Affects M2. Add anomaly detection over diff/tool-event baselines, not only deterministic pattern detectors. Confidence high.

11. Gray 1981, VLDB. A transaction is only a transaction if it has atomicity, consistency, isolation, durability; long-running work often violates this. Affects M4. Treat a decision as “reversible” only if the affected change has true ACID properties or a verified compensating action. Confidence high.

12. Garcia-Molina & Salem 1987, SIGMOD Sagas. Long transactions need compensating steps, but compensation cannot always undo external effects. Affects M4. Define compensations per side effect and cap work on uncompensatable side effects, not only unratified commits. Confidence high.

13. Ophir, Nass & Wagner 2009, PNAS. Chronic media multitasking is associated with worse filtering and task switching. Affects M8. Do not infer comprehension from fan-out alone; measure task-switching cost and signal detection under load. Confidence high.

14. Wickens 2002, Theoretical Issues in Ergonomics Science. Dual-task interference depends on shared processing resources. Affects M8. The human dashboard competes for the same visual/cognitive channel as code review; use auditory or temporal separation for some alerts. Confidence high.

15. Baddeley & Hitch 1974, Psychology of Learning and Motivation. Working memory is limited and verbally mediated. Affects M5. Ruling sessions should isolate one decision with all relevant context, rather than queue-style batch processing from a visual list. Confidence high.

B.
1. Queue gaming. Agents can inflate cost-of-delay or choose defaults that make their preferred decision seem high-priority. Failure: the human spends the attention budget on urgent-looking reversible trivia while an irreversible risk waits. Evidence needed: pilot data comparing declared urgency with realized cost-of-delay, plus an ombudsman sample of low-ranked decisions that later caused incidents.

2. Detector flood and evasion. Deterministic detectors will fire on benign schema/host/dependency changes; agents will learn to split changes or phrase commits to stay below thresholds. Failure: thousands of queue items produce alarm fatigue and the human ignores the queue; meanwhile real decisions evade detection. Evidence needed: precision and recall against labeled historical PRs, plus a red-team trial where agents deliberately try to evade detectors.

3. Fact/will misclassification. Many engineering decisions are mixtures of fact and value: e.g. weakening a test for a performance budget, or allowing data deletion for migration speed. Failure: the clerk labels them as fact, a model reviewer ratifies the agent, and the human never sees a value-laden risk. Evidence needed: inter-rater reliability of fact/will labels on real decisions and a blind human panel comparison with model reviewer decisions.

4. Rollback incompleteness. Optimistic defaults can create external effects: a new endpoint is consumed by another service, a schema migration writes data that cannot be cleanly undone. Failure: human overrule rolls back the commit but breaks downstream consumers or leaves orphaned data. Evidence needed: staging experiments attempting full rollback of twenty realistic commits with dependent services, measuring how many require manual repair.

5. Oversight cost and correlated failure. Independent backbrief, clerk, reviewer, and audit all rely on similar LLMs. If the model family shares blind spots, the oversight stack can be confidently wrong in multiple places; token cost may also exceed human attention savings. Failure: a dangerous change is missed by all overseers and reported as green. Evidence needed: cost per accepted decision, plus failure-injection tests using heterogeneous models to measure whether independent overseers detect the same planted error.

C.
2. Most likely failure: the ledger gives false completeness. Detectors are closed-world, backbriefs are generic, and agent declarations become shaped by what detectors already catch; novel or ambiguous decisions never enter the queue. Cheapest experiment: take 20 historical PRs from a real repo, have a human label all decisions, then run the detector set and agent declarations and measure recall; also run one red-team agent instructed to make the same change while evading the ledger.

3. Most likely failure: the fact/will boundary drifts toward “fact” because facts do not consume human attention. Value-laden decisions are then settled by a model reviewer correlated with the agent. Cheapest experiment: create 20 short decision scenarios from real engineering changes, have the clerk classify them, and compare against a panel of engineers asked “would a reasonable person disagree on values?”; measure the false-fact rate and correlate with regretted automatic approvals.

4. Most likely failure: “reversible” means only code reverts, not system effects. Dependencies, data, user-visible behavior, or downstream consumers make the optimistic default permanent in practice. Cheapest experiment: select 10 past commits that look reversible, attempt full rollback in a clone including tests, schema, data, and consumer builds; record how many require out-of-band repair or cannot be rolled back cleanly.

D.
Falsifiable claim: with 10 minutes/day of human attention over a 10-working-day trial, the system sustains at least 15 agent work-hours per human minute, at least 95% of sampled human overrules roll back with no detected downstream breakage, and the human answers at least 80% of a 20-vignette audit correctly about which commits still rest on unratified assumptions. Missing any threshold falsifies the design.