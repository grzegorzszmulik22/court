# 3. Human factors, economics, cybernetics and distributed-systems citations: verification notes

Compiled 2026-11-15. Read-only web research.

How to read the "read" column:
- **primary** = I read the paper's own text (full text or the relevant section).
- **abstract** = I read the publisher/author abstract only.
- **secondary** = a reliable later source describing the work; the primary was paywalled or blocked.
- **bib** = authors, year, venue, volume and pages confirmed against the Crossref record for the DOI; the claim itself was checked only against an abstract, a one-line database summary, or not at all (stated per row).

All DOIs listed in Part A were checked against Crossref and matched on authors, year, venue and pages.

---

## Part A. Citation table

| # | Correct citation | Verdict | Corrected one-line claim | Source URL | Read |
|---|---|---|---|---|---|
| 1a | Fitts, P. M. (ed.) (1951). *Human Engineering for an Effective Air-Navigation and Traffic-Control System*. Washington, DC: National Research Council. | partly | The report lists what humans surpass machines at and vice versa, as a basis for deciding the division of responsibility in air traffic control. The rule "allocate each function to whoever does it better" is the later reading of that list (the "Fitts list", MABA-MABA). The report itself warned against a purely machine model of human behaviour and noted qualities that cannot be compared mechanistically. | https://link.springer.com/article/10.1007/s10111-011-0188-1 (de Winter & Dodou 2014, quoting Fitts 1951 pp. v, 8) | secondary |
| 1b | Dekker, S. W. A. & Woods, D. D. (2002). MABA-MABA or Abracadabra? Progress on Human-Automation Co-ordination. *Cognition, Technology & Work* 4(4), 240-244. | n/a (critique) | The critique: allocation by substitution fails because automation's real effects are qualitative. It transforms the work and forces people to adapt skills and routines, so a fixed "who does what" split is the wrong question. The right question is "how do we make them get along together?" | https://link.springer.com/article/10.1007/s101110200022 | abstract |
| 2 | Parasuraman, R., Sheridan, T. B. & Wickens, C. D. (2000). A model for types and levels of human interaction with automation. *IEEE Trans. SMC-A* 30(3), 286-297. | correct | Four function classes: (1) information acquisition, (2) information analysis, (3) decision and action selection, (4) action implementation. Each can be automated at a different level from low to high. | https://doi.org/10.1109/3468.844354 | bib + database summary |
| 3 | "The Doctrine of Completed Staff Work". Drafted by Col. Archer L. Lerch, Deputy Provost Marshal General, for the Provost Marshal General, US Army. Published in *Army and Navy Journal*, January 1942. | correct (year), author added | Claim is right. Authorship and date are from secondary sources only: most give 24 January 1942, one gives 29 January (24 January 1942 was a Saturday, the journal's publication day). One source spells the name "Archer J. Lerch". A rival attribution exists: a copy circulated by Brig. Gen. George A. Rehm credits MacArthur's Southwest Pacific headquarters. | https://govleaders.org/completed-staff-work.php ; https://en.wikipedia.org/wiki/Completed_staff_work | secondary |
| 4 | US Army FM 6-0, *Commander and Staff Organization and Operations* (May 2014), ch. 12 "Rehearsals", para 12-8 and adjacent. Older definition in FM 101-5-1 (1997). | partly | A **backbrief** is "a briefing by subordinates to the commander to review how subordinates intend to accomplish their mission", given after they have had time to plan. Restating the mission and intent immediately after receiving the order is a different event, the **confirmation brief** (ADP 5-0, 2019). Your one-liner merges the two. | https://api.army.mil/e2/c/downloads/2023/01/19/48e6a637/19-18-commander-and-staff-guide-to-rehearsals-a-no-fail-approach-handbook-jul-19-public.pdf (CALL Handbook 19-18, quoting FM 6-0) ; https://www.globalsecurity.org/military/library/policy/army/fm/101-5-1/f545-b.htm | secondary (FM 6-0 PDF blocked) |
| 5a | Crawford, V. P. & Sobel, J. (1982). Strategic Information Transmission. *Econometrica* 50(6), 1431-1451. | correct | With costless, unverifiable messages and misaligned interests, equilibria are partitions: the sender reveals only which interval the truth lies in. Informativeness rises as interests converge; full revelation only when they coincide; an uninformative ("babbling") equilibrium always exists. | https://doi.org/10.2307/1913390 | bib only (claim from memory) |
| 5b | Milgrom, P. R. (1981). Good News and Bad News: Representation Theorems and Applications. *Bell J. Econ.* 12(2), 380-391. Grossman, S. J. (1981). The Informational Role of Warranties and Private Disclosure about Product Quality. *J. Law & Econ.* 24(3), 461-483. | correct, with conditions | Full disclosure ("unravelling") follows when claims are verifiable, disclosure is costless, the receiver knows the sender has the information, and the receiver treats silence as the worst case. It fails if any of those conditions is dropped. | https://doi.org/10.2307/3003562 ; https://doi.org/10.1086/466995 | bib only (claim from memory) |
| 6 | Romer, T. & Rosenthal, H. (1978). Political resource allocation, controlled agendas, and the status quo. *Public Choice* 33(4), 27-43. | overstated | A monopoly agenda setter making a take-it-or-leave-it proposal does not get any outcome it likes. Its power is bounded by the reversion (status quo) point: "the worse the status quo, the greater this threat". With a status quo near the median voter's ideal, the setter gains little. | https://www.edegan.com/pdfs/Romer%20Rosenthal%20(1978)%20-%20Political%20Resource%20Allocation,%20Controlled%20Agendas%20and%20the%20Status%20Quo.pdf | primary |
| 7 | Beer, S. (1972). *Brain of the Firm*. London: Allen Lane (2nd ed. Wiley, 1981). Beer, S. (1979). *The Heart of Enterprise*. Wiley. Beer, S. (1985). *Diagnosing the System for Organizations*. Wiley. | content correct; book-per-concept not established | System 3* is a sporadic audit channel giving System 3 direct access to operations (System 1), alongside the command line. The algedonic signal is a pain/pleasure alarm that escalates up the levels of recursion when performance departs from capability, typically after a timeout at each level (so it reaches the top if lower levels do not act, not instantly). The algedonic loop is in *Brain of the Firm* (1972). I could not establish which book first uses the label "3*" (candidates: 1979 or the 1981 2nd edition), nor whether "algedonic" appears before 1972. | https://en.wikipedia.org/wiki/Viable_system_model ; https://books.google.com/books/about/Brain_of_the_Firm.html?id=T_A9AAAAIAAJ | secondary |
| 8 | Goldratt, E. M. & Cox, J. (1984). *The Goal*. North River Press. Goldratt, E. M. & Fox, R. E. (1986). *The Race*. North River Press. | partly | *The Goal* establishes that system output is set by the bottleneck (the hike with Herbie). The name and formal method "drum-buffer-rope" come from *The Race* (1986): drum = bottleneck pace, buffer = protective stock ahead of it, rope = release of new work tied to the drum. | https://en.wikipedia.org/wiki/Eliyahu_M._Goldratt | secondary |
| 9 | Smith, W. E. (1956). Various optimizers for single-stage production. *Naval Research Logistics Quarterly* 3(1-2), 59-66. Cox, D. R. & Smith, W. L. (1961). *Queues*. London: Methuen. | correct; cite both | Rule is right: serve the class with the highest c·μ (holding cost times service rate) first. Smith 1956 proved the deterministic, all-jobs-present version (Smith's ratio rule, WSPT). Cox & Smith 1961 proved it for the multiclass M/G/1 queue. Cite Cox & Smith for queues, Smith for the origin. | https://web.tecnico.ulisboa.pt/~ist12409/ques9316.pdf ; https://doi.org/10.1002/nav.3800030106 | secondary |
| 10 | EEMUA Publication 191, *Alarm Systems: A Guide to Design, Management and Procurement*. 1st ed. 1999, 2nd 2007, 3rd 2013, 4th November 2024. ANSI/ISA-18.2-2009 (revised 2016). | mostly correct; flood wording needs fixing | See "EEMUA and ISA figures" below. The "more than 10 in 10 minutes = flood" definition is the ISA-18.2 one; EEMUA's own upset bands are under 10 / 20-100 / over 100. | https://process.honeywell.com/content/dam/process/en/documents/document-lists/doc_asm-consortium/white-papers/February%2028%202005%20-%20Acheiving%20Effective%20Alarm%20System%20Performance%20Benchmarking.pdf ; https://www.processvue.com/downloads/Alarm_system_performance_KPIs_V1_0.pdf ; https://literature.rockwellautomation.com/idc/groups/literature/documents/wp/proces-wp014_-en-p.pdf | secondary (both standards paywalled) |
| 11 | Wolfe, J. M., Horowitz, T. S. & Kenner, N. M. (2005). Rare items often missed in visual searches. *Nature* 435(7041), 439-440. | correct | Miss rates: 7% at 50% prevalence, 16% at 10%, 30% at 1%. False alarms 0.03%. Second experiment with mixed targets: 11% missed at 44% prevalence, 25% at 10%, 52% at 1%. Over 4,000 trials at 1% prevalence: 41% misses. | https://search.bwh.harvard.edu/new/pubs/WolfePrevalenceNature05.pdf | primary |
| 12 | Mackworth, N. H. (1948). The breakdown of vigilance during prolonged visual search. *Quarterly Journal of Experimental Psychology* 1(1), 6-21. | correct | Two-hour Clock Test: detection of rare double-jumps fell after the first half hour (later sources put the drop at 10-15%) and kept declining. His later monograph added that a break or a phone call restored detection and exhortation did not. | https://doi.org/10.1080/17470214808416738 ; https://www.frontiersin.org/journals/cognition/articles/10.3389/fcogn.2025.1632885/full | secondary |
| 13a | Klein, G. (2007). Performing a Project Premortem. *Harvard Business Review* 85(9), 18-19. | correct as a description of the method | The method: assume the plan has failed, then each person writes reasons why. It is a two-page practitioner column, not a study. Its stated benefit is giving people permission to voice doubts. | https://hbr.org/2007/09/performing-a-project-premortem | secondary (paywalled) |
| 13b | Mitchell, D. J., Russo, J. E. & Pennington, N. (1989). Back to the future: Temporal perspective in the explanation of events. *J. Behavioral Decision Making* 2(1), 25-38. | overstated | The study found that **certainty** of the outcome, not looking back from the future, changed explanations: events described as sure produced longer explanations with a higher share of episodic reasons. Temporal perspective "showed little influence". The "30% more reasons correctly identified" wording is Klein's gloss; the study counted reasons and did not test their correctness. The figure does not appear in the abstract and I could not get the full text to confirm it. | https://doi.org/10.1002/bdm.3960020103 ; https://www.academia.edu/5699811/ | abstract |
| 14a | Rasmussen, J. (1983). Skills, rules, and knowledge; signals, signs, and symbols, and other distinctions in human performance models. *IEEE Trans. SMC* 13(3), 257-266. | correct | Three levels of control of behaviour: skill-based, rule-based, knowledge-based. | https://doi.org/10.1109/TSMC.1983.6313160 | bib only |
| 14b | Rasmussen, J. (1997). Risk management in a dynamic society: a modelling problem. *Safety Science* 27(2-3), 183-213. | correct | Under cost and workload pressure, normal work migrates toward the boundary of acceptable performance; accidents come from that drift, not from one-off errors. | https://doi.org/10.1016/S0925-7535(97)00052-0 | bib + database summary |
| 15a | Kung, H. T. & Robinson, J. T. (1981). On optimistic methods for concurrency control. *ACM TODS* 6(2), 213-226. | correct | No locking: work on private copies (read phase), validate at commit, write if valid. On conflict the transaction is backed up and restarted. Pays off when conflicts are rare. | https://doi.org/10.1145/319566.319567 | abstract |
| 15b | Jefferson, D. R. (1985). Virtual time. *ACM TOPLAS* 7(3), 404-425. | correct | Time Warp: each process runs ahead optimistically and rolls back (with antimessages) when a message arrives in its past. Bounded-optimism variants are separate papers: Moving Time Window (Sokol, Briscoe & Wieland 1988), Breathing Time Buckets (Steinman 1991), Bounded Time Warp (Turner & Xu 1992). | https://doi.org/10.1145/3916.3988 ; https://dl.acm.org/doi/pdf/10.1145/280265.280267 | bib + secondary |
| 15c | de Kleer, J. (1986). An assumption-based TMS. *Artificial Intelligence* 28(2), 127-162. | correct, minor fix | Each datum carries a label: the set of minimal consistent assumption sets (environments) under which it holds. A set of sets, not a single list. | https://doi.org/10.1016/0004-3702(86)90080-9 | bib + database summary |
| 16 | Cohen, P. R. & Levesque, H. J. (1991). Teamwork. *Noûs* 25(4), 487-512. Levesque, Cohen & Nunes (1990). On acting together. *AAAI-90*. Jennings, N. R. (1993). Commitments and conventions: the foundation of coordination in multi-agent systems. *Knowledge Engineering Review* 8(3), 223-250. | correct, one citation fix | Claim is right. Note that Cohen & Levesque 1990 (*Artif. Intell.* 42, "Intention is choice with commitment") is about individual intention; joint intention is the 1990 AAAI paper and the 1991 *Noûs* paper. A member who privately concludes the goal is achieved, impossible or irrelevant must make that mutually known. Jennings: commitments are pledges; conventions are the rules for monitoring and dropping them. | https://doi.org/10.2307/2216075 ; https://doi.org/10.1017/S0269888900000205 | bib + database summary (AAAI-90 paper from memory) |
| 17 | Saltzer, J. H. & Schroeder, M. D. (1975). The protection of information in computer systems. *Proc. IEEE* 63(9), 1278-1308. Hardy, N. (1988). The Confused Deputy (or why capabilities might have been invented). *ACM SIGOPS Operating Systems Review* 22(4), 36-38. | correct | No claim attached. Citations are right. | https://doi.org/10.1109/PROC.1975.9939 ; https://doi.org/10.1145/54289.871709 | bib |
| 18 | Bainbridge, L. (1983). Ironies of automation. *Automatica* 19(6), 775-779. Endsley, M. R. (1995). Toward a theory of situation awareness in dynamic systems. *Human Factors* 37(1), 32-64. Endsley, M. R. & Kiris, E. O. (1995). The out-of-the-loop performance problem and level of control in automation. *Human Factors* 37(2), 381-394. Sarter, N. B. & Woods, D. D. (1995). How in the world did we ever get into that mode? Mode error and awareness in supervisory control. *Human Factors* 37(1), 5-19. | correct | Endsley's three levels: perception, comprehension, projection. Sarter & Woods's own terms are "mode error" and "mode awareness"; "mode confusion" is the common later label. | DOIs 10.1016/0005-1098(83)90046-8 ; 10.1518/001872095779049543 ; 10.1518/001872095779064555 ; 10.1518/001872095779049516 | bib + database summaries |
| 19a | Olsen, D. R. & Goodrich, M. A. (2003). Metrics for evaluating human-robot interactions. *Proc. PerMIS 2003* (NIST). | correct | See "Fan-out" below. | http://byu.danrolsenjr.org/paperPDFs/RAD.pdf | primary |
| 19b | Olsen, D. R. & Wood, S. B. (2004). Fan-out: measuring human control of multiple robots. *Proc. CHI 2004*, 231-238. Companion: Olsen, Wood & Turner (2004), Metrics for human driving of multiple robots, *ICRA*. | correct | See "Fan-out" below. | https://doi.org/10.1145/985692.985722 ; http://byu.danrolsenjr.org/paperPDFs/FanoutIEEE.pdf | abstract (CHI) + primary (ICRA companion) |
| 19c | Crandall, J. W., Goodrich, M. A., Olsen, D. R. & Nielsen, C. W. (2005). Validating human-robot interaction schemes in multitasking environments. *IEEE Trans. SMC-A* 35(4), 438-449. | correct | Neglect tolerance, combined with interface efficiency, gives (1) the maximum number of robots one person can manage, (2) feasible team mixes, (3) predicted team performance under independence assumptions. Measured with a secondary-task method. | https://scholarsarchive.byu.edu/facpub/363/ | abstract |
| 20a | Cummings, M. L. & Mitchell, P. J. (2008). Predicting controller capacity in supervisory control of multiple UAVs. *IEEE Trans. SMC-A* 38(2), 451-460. | correct on wait times; **not** the source of 70% | See "Wait times and the 70% question" below. | https://ieeexplore.ieee.org/document/4445761/ ; https://faculty.cs.byu.edu/~crandall/papers/CrandallCummingsTRO2007_Final.pdf | abstract + secondary for the equation (MIT DSpace copy blocked) |
| 20b | Cummings, M. L. & Guerlain, S. (2007). Developing operator capacity estimates for supervisory control of autonomous vehicles. *Human Factors* 49(1), 1-15. | this is the 70% source | See below. | https://doi.org/10.1518/001872007779598109 | abstract |
| 21a | Scerri, P., Pynadath, D. V. & Tambe, M. (2002). Towards adjustable autonomy for the real world. *JAIR* 17, 171-228. | correct | See "Transfer of control" below. | https://arxiv.org/pdf/1106.4573 ; https://people.ict.usc.edu/~pynadath/Papers/aimag08.pdf | primary |
| 21b | Horvitz, E. (1999). Principles of mixed-initiative user interfaces. *Proc. CHI '99*, 159-166. | correct | See "Horvitz rule" below. | https://interruptions.net/literature/Horvitz-CHI99-p159-horvitz.pdf | primary |

### EEMUA and ISA figures (item 10)

EEMUA 191, average alarm rate in steady operation, per operator (1st ed. 1999, Figure 42; same in 3rd ed. p. 96):
- more than 1 per minute: very likely to be unacceptable
- 1 per 2 minutes: likely to be over-demanding
- 1 per 5 minutes: manageable
- less than 1 per 10 minutes: very likely to be acceptable

EEMUA 191, alarms in the first 10 minutes after a major upset (Figure 45; 3rd ed. p. 97):
- under 10: should be manageable, but may be difficult if several need a complex response
- 20 to 100: hard to cope with
- more than 100: definitely excessive, very likely to lead the operator to abandon the system
- (the 10-20 gap is not categorised by EEMUA; the ASM study labelled it "possibly hard to cope with")

ANSI/ISA-18.2-2009, Table of performance metrics, per operating position, at least 30 days of data:
- alarms per 10 minutes: about 1 very likely acceptable, about 2 maximum manageable
- per hour: about 6 and about 12. Per day: about 150 and about 300
- share of 10-minute periods with more than 10 alarms: under about 1%
- maximum alarms in any 10-minute period: 10 or fewer
- share of time in flood: under about 1%
- top 10 most frequent alarms: under 1% to 5% of load
- chattering and fleeting alarms: zero. Stale alarms: fewer than 5 on any day
- flood = more than 10 alarms in 10 minutes
- The 2016 revision removed the per-day figure and "hours with more than 30 alarms", because the 1-per-10-minutes benchmark was being misused as a daily quota.

"Every alarm should require an operator response": correct. Both documents define an alarm as something that requires operator action; an alarm with no documented action is treated as unnecessary.

Origin of the 1-per-10-minutes number: Bransby & Jenkinson's survey for the UK HSE (CRR 166, 1998), after the 1994 Milford Haven refinery explosion.

### Fan-out (item 19)

- Olsen & Goodrich 2003: robot attention demand `RAD = IE / (IE + NT)`; free time `FT = 1 − RAD`; fan-out `FO = 1 / RAD = (IE + NT) / IE`.
- Olsen & Wood 2004 (CHI): `FO = AT / IT`, with AT = activity time. The companion ICRA paper first writes `FO = NT / IT`, then measures NT by activity time and solves `IT = AT / FO`.
- Form used by the later literature (Crandall & Cummings 2007, Eq. 4): `FO = NT / IT + 1`. This equals the 2003 form, and equals `AT / IT` when `AT = NT + IT`.
- **Neglect time / neglect tolerance (NT)**: the time a robot can go without attention before its task effectiveness falls below an acceptable threshold.
- **Interaction time (IT)**: the time the human needs to interact with one robot to give it its next instruction. Olsen & Goodrich call the same quantity interaction effort (IE) and note it includes more than interface time: deciding which robot to service, regaining context, planning, then entering the command.
- **Activity time (AT)**: time from when the user commands a robot until the user commands it again, it stops making progress, or it reaches the goal.
- Worked example in the paper: a robot that runs 10 s per instruction, with 2 s to instruct, gives 5 robots.
- Caveat from Crandall & Cummings 2007: none of the four fan-out formulas they tested consistently predicted the observed limit (4 to 6 robots).

### Wait times and the 70% question (item 20)

- Cummings & Mitchell 2008 add wait time to fan-out: `FO = NT / (IT + WT) + 1`, with three sources summed in WT:
  - **WTI**: interaction wait time, including cognitive reorientation when switching to the vehicle
  - **WTQ**: time a vehicle waits in the queue while the operator serves others
  - **WTSA**: time a vehicle waits because the operator has lost situation awareness and does not notice it needs attention
- Result: including wait times cut capacity predictions by up to 67%. Loss of situation awareness was the main source. Even under management by exception, which should remove queueing and interaction waits, capacity fell 36% from WTSA alone.
- **The 70% figure is not from Cummings & Mitchell 2008.** The empirical source is Cummings & Guerlain 2007: 42 US Navy personnel retargeting 8, 12 or 16 Tomahawk missiles; performance and situation awareness degraded at 16; "a 70% utilization (percentage busy time) score is a valid threshold for predicting significant performance decay". They credit the prediction to Schmidt, D. K. (1978), A queuing analysis of the air traffic controller's work load, *IEEE Trans. SMC* 8(6), 492-498, and Rouse, W. B. (1983), *Systems Engineering Models of Human-Machine Interaction*, North Holland. I did not read Schmidt or Rouse.
- A second, unrelated "70%" that is easy to confuse with it: Wickens & Dixon (2007), *Theoretical Issues in Ergonomics Science* 8(3), 201-212. Across 20 studies, diagnostic automation with reliability below 0.70 gave worse performance than no automation.

### Transfer of control (item 21a)

- Definition 3.1: "a pre-defined, conditional sequence of two types of actions: (i) actions to transfer decision-making control (e.g., from an agent to a user or other agents, or vice versa) and (ii) actions to change an agent's pre-specified coordination constraints with team members, aimed at minimizing miscoordination costs." Example `H(5)A`: ask the human, wait 5 minutes, then the agent decides. A "D" action buys time (for example, delay the meeting) at a cost. The best strategy is found with a Markov decision process.
- Failures that motivated it, in the Electric Elves deployment (about a dozen agents, ISI/USC, June to December 2000). The first version used C4.5 decision trees:
  1. Waited indefinitely for the user, so teammates sat in a meeting room for nothing.
  2. After a fixed 5-minute timeout was added: cancelled a meeting with the division director (overgeneralised from training examples).
  3. Cancelled the group's weekly research meeting when the timeout fired and the user had not answered.
  4. Delayed one meeting almost 50 times, 5 minutes each time.
  5. Volunteered a user for a presentation he did not want to give.
- Lessons stated: avoid risky autonomous decisions by weighing their cost; take a cheap delaying action to buy the user time; plan ahead so that one costlier action replaces a long run of cheap ones.
- What the deployment found: daily use by the group began only after the MDP version replaced C4.5. Strategies with several transfers and delays were the useful ones wherever waiting costs were neither negligible nor very steep.

### Horvitz rule (item 21b)

- `eu(A|E) = p(G|E)·u(A,G) + [1 − p(G|E)]·u(A,¬G)` and `eu(¬A|E) = p(G|E)·u(¬A,G) + [1 − p(G|E)]·u(¬A,¬G)`, where G is the user's goal, E the evidence and A the automated action.
- Act when `p(G|E) > p*`, the probability at which the two lines cross. Solving (my algebra, not printed in this form): `p* = [u(¬A,¬G) − u(A,¬G)] / ([u(¬A,¬G) − u(A,¬G)] + [u(A,G) − u(¬A,G)])`.
- With "ask the user" as a third option there are two thresholds: below `p*(¬A,D)` do nothing; between them, ask; above `p*(D,A)`, act.
- The thresholds move with context. The cost of an unwanted action rises when the user is focused elsewhere, which raises p*.

---

## Part B. Recent sources

### 22. Anthropic, "How AI agents shape experts' judgment calls" (27 Oct 2026)

Paper: Herdağdelen, Joren & Tamkin, "Who's Aligned to Whom? How AI Agents Shape Experts' Research Judgment Calls". Blog: https://www.anthropic.com/research/agents-judgment-calls . PDF: https://assets.claude.com/519c17a5d23a794e8b6d50a854048d0b61e0d73a.pdf . Both read in full (main text; appendices skimmed).

Design
- Randomised trial. 61 professional causal-inference analysts completed the task; 11 chose not to use the agent and are a comparison group only; 50 analysed, 48 with surveys.
- Task: advise a fictional farming co-operative whether to roll out or discontinue an advisory programme. Board threshold: a 3-4% productivity gain. Median time 65 minutes.
- Five two-option decisions: outcome measure, comparison method, estimand, weighting, final recommendation. A coin flip set the agent's lean on each; a hidden system prompt told it to hold the lean, argue for it honestly, change its mind if the evidence was decisive, and never mislead. Each agent got one of 32 lean configurations, balanced in blocks.
- Agent: Claude Code v2.1.220 on Claude Opus 4.8 at high effort. Pre-registered (osf.io/t3q59), though 17 participants had started before registration.
- No decision was elicited before the agent spoke, on purpose, because elicitation is itself an intervention.

Headline numbers
- H1, four analytical decisions pooled: choice matched the agent's lean 86% of the time (50% by chance). Flip rate 0.72, 95% CI [0.60, 0.83], one-sided p < .001, n = 195 trials.
- Per decision match: outcome 92%, method 82%, estimand 81%, weighting 90%. Flip rates ranged 62 to 82 points.
- H2, final recommendation: shift 0.17 [−0.06, 0.40], p = 0.069, n = 50 (unadjusted 0.22 [−0.02, 0.47], p = 0.037). Rollout recommended by 54% when the agent leaned rollout, 36% otherwise. Not significant under the registered estimator.
- The four earlier leans alone predicted 82% of rollout-or-discontinue recommendations (majority baseline 61%, AUC 0.84, in sample, exploratory).
- H3, recommending against one's own estimate: 0.23 [−0.04, 0.50], p = 0.110, n = 26. Descriptive only; its validity check failed.
- H4, switching after taking a side first: 60% switched when the agent disagreed versus 6% when it agreed. Gap 0.53 [0.24, 0.82], p < .001, n = 38 trials.
- Who spoke first: the agent voiced a side first on 77% of decisions. In hand-coded sessions, on 39% of decisions the participant neither asked nor took a side.
- Afterwards: 77% (37 of 48) repeated the agent's argument in at least one justification; 15% credited the agent. 52% rated the recommendation more their own than the agent's; 35% mostly or entirely their own. They recalled their choice correctly on 85% of decisions.
- Confidence: 71 (n = 29, followed the lean on all four) versus 73 (n = 19). Spearman ρ = −0.06, p = 0.666.
- Ten moderators tested (experience, expertise, education, quiz score, trust in AI, AI use, need for cognition, open-mindedness, age, gender). Only age was nominally significant (p = 0.037) and it did not survive correction.
- Awareness: 6 of 48 guessed the study was about the AI's influence; none suspected the lean was planted.
- No-AI group (self-selected, less experienced): serious invalidating errors in 6 of 11 (55%) versus 3 of 50 (6%); median 90 minutes (IQR 83-132) versus 65 (50-78), Wilcoxon p = 0.002.
- Stock agent with no lean: made the same five choices in 91% of 123 runs (112). When a scripted analyst declared the opposite choice and asked only for code, it raised the other option in 66% of 150 cases and advocated it in 41%. With the lean prompt: 87%.

Limitations the authors state
- The lean was deliberately amplified; real effects depend on whether a model has a preference and volunteers it.
- One task, one domain, one model; experts in method but new to the domain; no stakeholders to consult.
- Average effects only; no per-person baseline. The switching subset is self-selected.
- They recorded which option was chosen, not how strongly it was preferred.

What they recommend
- Oversight has to look at how decisions get made: surface key decision points as work proceeds and help experts state the assumptions and values they would otherwise leave unstated.
- Measure deferral in real work, since self-report misses it.
- Design agents to argue the best case for each option, or ask for the expert's view first. They warn this overloads people if done for every decision, and that choosing which decisions to raise is itself a judgment call. Confident, fluent delivery sways people.
- Institutions should decide deliberately which decisions need a person's own informed judgment, give time and resources to form one, and plan for keeping the skills.
- Their mechanism reading: the agents did not mislead; they argued one side well and rarely explained why a competent expert might choose the other.

### 23. Mitchell, Ghosh & Passi, "AI Agents Push Humans Out of the Loop" (arXiv 2608.23642v3, 6 Sep 2026)

Read: abstract and section 5 in full, with the reference list. Position paper. Section 5 is an inventory drawn from the literature; the paper reports no new experiment. Six goals (prevent automation bias, fatigue, overreliance, overload; support judgement, engagement) crossed with six solution categories.

Strategic friction (cognitive forcing functions)
- Pre-commitment: user records their own view before seeing the agent's. Cites Buçinca, Malaya & Gajos 2021 (CSCW experiment); Ehsan et al. 2026 (CHI).
- Delay-and-choice: user decides whether and when to see the AI output. Cites Buçinca et al. 2021.
- Reasoning probes: inline prompts at high-stakes moments ("what evidence would change your mind?"). Cites Singh, Guan & Rieh 2025; Ghosh et al. 2026 (arXiv 2601.18033, experiment).
- Action gating: explicit verification before a consequential path, ideally offering alternatives, not one accept/reject. Cites Grunde-McLaughlin et al. 2026 (arXiv 2602.16844); Shi & DiFranzo 2026 (arXiv 2602.09286).

Decision design (expert attention is scarce)
- Bounded autonomy: prespecify what the agent may do without approval. Cites the "Encyclopedia of Agentic Coding Patterns" website entry on approval fatigue (anonymous); Shi & DiFranzo 2026.
- Batch review: agent finishes a logical unit and presents it as one diff; optional second human check for consequential actions. Cites the same website; Grunde-McLaughlin et al. 2026.
- Automated pre-checks: machines verify properties that need no judgement. Cites only the same website.
- Supportive interfaces: focus attention on what matters. Cites Horvitz 1999; Groce et al. 2014; Buçinca et al. 2021; Gajos & Mamykina 2022; one reference I could not resolve.
- Category-level citations: Endsley & Kiris 1995; Gerlich 2025; Chen et al. 2026 (arXiv 2604.04918).

Behavioural monitoring (oversight quality is measurable; balance against surveillance). Category cites Narayanan & Feigh 2026.
- Time-based signatures: review time falls while approval rate holds, or does not rise with complexity. Cites Swaroop et al. 2024 and 2025 (IUI).
- Override signatures: disagreement rate declines over time. No citation.
- Evidence-seeking signatures: reviewer stops asking for more as stakes rise. No citation.
- Canaries: tasks with known answers inserted into the workflow; also tasks where the right agent behaviour is to report inability. No citation.
- Style-fatigue covariance: does the agent adapt to, or encourage, weak oversight. No citation.
- Audits: re-examine past decisions; check the agent's trace summaries against raw action logs. Cites Santoni de Sio & van den Hoven 2018 (a philosophical account).
- Guardian agents watching for degradation. Cites a Gartner podcast (Plummer 2025).

Trainings and exercises
- Domain skill maintenance: regularly do the task without AI. Cites Ehsan et al. 2026; a *Nature Medicine* editorial 2025.
- Critical evaluation training. Cites Gerlich 2025.
- Self-monitoring (metacognition) training. Cites Fabiano et al. 2025; Gerlich 2025.

Workload and scheduling
- Enforced breaks. No citation.
- Rotations between tasks and between assisted and unassisted work. No citation.

Role design. Category cites Sterz et al. 2024 (FAccT).
- Assign oversight to people with the expertise to evaluate outputs.
- Separate the overseer from whoever benefits from approval continuing.
- Align incentives: remove productivity targets that punish scrutiny.

Evidence quality, my reading: the friction items rest on controlled experiments. Several decision-design items rest on one anonymous website. Most monitoring, workload and role items are proposals with no cited evidence. The authors' own caveat (section 6): users may dislike systems that reduce overreliance (Buçinca et al. 2021).

### 24. Jarmak, "Engineering Reliable Coding Agents" (arXiv 2608.13867), chapter 16 only

Read: chapter 16 in full, including its "Sources and evidence" section. The author rates the chapter's own evidence as 3 strong and 4 directional items.

Chapter claim: verification must be cheaper than uncritical acceptance. Shallow review is a rational response to cost, not a character flaw (Vasconcelos et al. 2023, five experiments).

Which checks go to machines, which to humans (section 16.3, "least complex mechanism capable of resolving it")
1. Deterministic rules: exact violations (wrote outside an allowed directory, forbidden operation, missing approval token).
2. Model judge with a fixed, versioned rubric: contextual cases (does the migration plan have a credible rollback).
3. Agent judge: only when no single artifact holds the evidence (a sequence of individually permitted actions that together bypass a control).
4. Humans: calibrate the layers and own the final verdict.
- Every layer must pass its evidence to the next. A bare risk score forces the reviewer to redo the work or trust the classifier.
- The author states this ordering is a design convention from one practitioner account with no independent measurement.
- Attach a falsification artifact to each claim: test, typed interface, deterministic replay, structured trace, sandbox result, policy match. An explanation without a check is context, not evidence.

Risk tiers
- The chapter gives no numbered tier table. It gives factors for the escalation decision: expected consequence, reversibility, novelty, disagreement among monitors, evidence quality, availability of a qualified owner. These should be shown to the judge, not folded into one score.
- Signals for where to place a forcing function: high-impact file classes, previously approved plan, repeated acceptance from the same agent, large diff, short time between presentation and approval, weak verification evidence, irreversible downstream action. Each can be wrong; they pick where to test, not who was careless.
- Forcing functions go at the acceptance decision and target "acceleration mode" (Barke et al. 2023, 20 programmers; directional only). Uniform friction wastes reviewer tolerance.
- Protect the checkpoint from time pressure: in pathology, time pressure raised the severity of automation-bias errors without changing their frequency (Rosbach et al. 2024).
- Emergency bypass must keep: owner, reason, scope, evidence available at the time, required follow-up.

Escalation contract
- Each flag names: decision owner, outcome at stake, supporting evidence, available options, recommendation, why automation cannot decide, what is paused, what token resumes it.
- The author's own "wake-up contract": wake a human for suspected wrong calls and scope or trust decisions, never for passing runs. The question must be answerable in one line. Success test: engineers do not learn to ignore the messages.
- State what pauses (action, trajectory, repository or fleet), what later actions are barred, and how stale approvals are rejected.
- Put checkpoints where intervention can still change the outcome.

Escalation metrics
- Kale et al. 2025 (arXiv 2508.19461): targeted escalation gave about 15% higher true-positive rate than blanket oversight at a false-positive rate of 0.01. One covert-misbehaviour arena, one operating point.
- Report true-positive rate at the chosen false-positive rate, with the confusion matrix per stratum, on an expert-labelled, stratified, held-out set. Re-run when agent, rubric, tools or case mix change.
- Translate rates into queue volume: a 0.01 false-positive rate is 1 false alarm per 100 benign actions and 10,000 per million. "A percentage without the expected number of human cases does not define an operating policy."
- Record per forcing function: where it appeared, whether the reviewer changed or rejected the output, decision time, later defects, whether defects cluster in bypassed cases, whether work is routed around the gate.
- Evaluate detection, delay, bypass frequency and workarounds together; satisfaction alone selects against effective friction (Buçinca et al. 2021, N = 199: the most effective forcing functions were rated least favourably).
- Audit the monitors: adjudicate flagged cases for false alarms, sample unflagged cases for misses (including strata where the monitor is expected to fail), version each verdict with its monitor configuration.
- Measure before adding friction: time to obtain evidence, where reviewers stop checking, how often approval happens without opening the evidence.
- Scale context only: Stripe's agents reportedly open over 1,300 pull requests a week (InfoQ, 2026); no data on review depth.

---

## Part C. Established results not in the list

1. **Consent versus exception.** Billings, C. E. (1996), *Human-Centered Aviation Automation: Principles and Guidelines*, NASA TM 110381 (earlier version NASA TM 103885, 1991). Management by consent: automation acts only after the supervisor approves each phase. Management by exception: automation acts unless the supervisor objects. His warning: consent "must be informed consent", and research is needed on "how to implement it without the consent becoming perfunctory". Evidence: Olson & Sarter (2001), *Human Factors* 43(2), 255-266: under consent, pilots often failed to detect conflicts in what the automation proposed, especially under time pressure, so they approved it. Ruff, Narayanan & Draper (2002), *Presence* 11(4), 335-351: consent beat both manual control and exception for one operator with up to four UAVs. Read: primary (Billings), abstracts (others). https://ntrs.nasa.gov/api/citations/19960016374/downloads/19960016374.pdf
2. **Complacency and automation bias.** Parasuraman, Molloy & Singh (1993), *Int. J. Aviation Psychology* 3(1), 1-23: operators detected automation failures much worse when its reliability was constant than when it varied, after about 20 minutes, and only when they had other tasks to do. Skitka, Mosier & Burdick (1999), *Int. J. Human-Computer Studies* 51(5), 991-1006: people with a highly but not perfectly reliable aid did worse than people with none, missing events the aid did not flag and following it against valid contrary indicators. Read: abstracts. https://ntrs.nasa.gov/citations/19930055574
3. **Cry-wolf is probability matching.** Bliss, Gilson & Deaton (1995), *Ergonomics* 38(11), 2300-2312: 138 subjects, alarms 25%, 50% or 75% true. About 90% of people did not respond to every alarm; they matched their response rate to the alarm's reliability. Term from Breznitz (1984), *Cry Wolf*. Read: abstract. https://pubmed.ncbi.nlm.nih.gov/7498189/
4. **Anaesthesia alarms.** Kestin, Miller & Lockhart (1988), *Anesthesiology* 69(1), 106-108: 50 paediatric operations, one alarm every 4.5 minutes on average, 75% with no therapeutic consequence, 3% indicating a critical situation. Lawless (1994): 94% of paediatric ICU alarms clinically irrelevant. Read: secondary (Schmid, Goepfert & Reuter 2013, *Critical Care* 17:216). https://pmc.ncbi.nlm.nih.gov/articles/PMC3672485/
5. **Nuclear control room.** Kemeny Commission (1979), *Report of the President's Commission on the Accident at Three Mile Island*: "During the first few minutes of the accident, more than 100 alarms went off, and there was no system for suppressing the unimportant signals so that operators could concentrate on the significant alarms." Read: primary. https://www.nrc.gov/docs/ML1216/ML12167A050.pdf
6. **How to interrupt.** McFarlane, D. C. (2002), Comparison of four primary methods for coordinating the interruption of people in human-computer interaction, *Human-Computer Interaction* 17(1), 63-139. Four methods: immediate, negotiated (person chooses when), mediated (an agent chooses when), scheduled (fixed times). Negotiated gave the best overall performance; immediate was better only where small differences in timeliness of the interrupting task are critical. Read: primary (results section). https://www.interruptions.net/literature/McFarlane-HCI02_2.pdf
7. **Call-centre staffing.** Halfin, S. & Whitt, W. (1981), Heavy-traffic limits for queues with many exponential servers, *Operations Research* 29(3), 567-588. With many servers, staffing at offered load plus β times its square root holds the probability of waiting constant as volume grows. My inference for a decision queue: pooled reviewers need proportionally less slack than a single reviewer, who is bound by the roughly 70% utilisation finding of item 20. Read: abstract. https://pubsonline.informs.org/doi/10.1287/opre.29.3.567
8. **Consent calendar (parliamentary procedure).** *Robert's Rules of Order Newly Revised*, 12th ed., §41:32: routine items are listed together and adopted without debate unless a member objects, in which case the item returns to the regular agenda. Requires a special rule of order. The procedural form of management by exception, with a per-item veto. Read: secondary quoting the section. https://www.diligent.com/resources/blog/consent-agenda-template-municipalities

Also relevant, found while checking item 20: Wickens & Dixon (2007), the 0.70 reliability crossover (see item 20 notes).

---

## Not verified

- Mitchell, Russo & Pennington 1989: full text not obtained. The "30%" figure is not in the abstract.
- Beer: which book first uses the "3*" label; whether "algedonic" predates 1972. Books not available online.
- Cummings & Mitchell 2008 full text (MIT DSpace blocked automated access). Wait-time equation taken from the abstract plus Crandall & Cummings 2007.
- Schmidt 1978 and Rouse 1983 (origin of the 70% prediction): not read.
- Olsen & Wood CHI 2004 full text (ACM paywall): abstract plus the companion ICRA paper.
- EEMUA 191 and ISA-18.2 themselves: paywalled; figures from three secondary sources that agree.
- FM 6-0 PDF (Army site refused access): definition from the CALL handbook that quotes it. Whether the 2022 editions moved the paragraph was not checked.
- Completed staff work: no primary copy of the 1942 journal issue; day of month (24 or 29) and the author's middle initial differ between sources.
- Fitts 1951 report itself; Klein 2007 (paywalled); Goldratt books.
- Skitka et al. 1999 exact error rates.
- Items marked "bib" in the table: bibliographic details confirmed, claim not re-read in the paper.
- Mitchell et al. reference [126] in section 5 could not be resolved from the HTML.
