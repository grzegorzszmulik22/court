# M0. Field evidence

**Goal.** Test Court's two central bets on real work before code depends on them: that agents make decisions they never raise, and that what the human is asked is mostly the wrong kind of question. M0 is **human work**, helped by an assistant session. The implementation agent does not run it. It runs alongside M1.

**Properties advanced.** None. M0 produces the evidence for gate G0 and the ground truth for H12.

**Expected results.**
- A week of the human's own interventions on the Countersign build lanes, each labelled.
- At least 30 agent-authored commits from those lanes with every decision in them labelled by a person.
- Three numbers and a go or no-go at G0.

**Milestone-wide denied.** Any Court code beyond the two small scripts below. Labelling by a model without a person confirming each label. Counting an intervention twice.

---

## M0.1 Intervention log

**Build.** One append-only file `field/interventions.jsonl` and a script `field/log.py` that asks six questions and appends one line:

| Field | Values |
|---|---|
| `at`, `lane` | timestamp, which agent lane |
| `what` | one sentence: what the human did or decided |
| `raised_by_agent` | `yes` (the agent asked or reported it), `no` (the human found it) |
| `kind` | `fact` (a test or reviewer could have settled it), `will` (a preference or an accepted risk), `unsure` |
| `repeat_of` | line number of an earlier intervention this repeats, or empty |
| `minutes`, `if_missed` | time spent; what would have happened had nobody intervened |

The human logs every intervention for seven consecutive working days: a correction, an approval, a rejected claim of done, a restart, a scope change.

**Checks.**
- C0.1.1 At least 50 lines, each with every field filled.
- C0.1.2 No two lines describe the same event (reviewed by reading).

**Denied.** Logging after the fact from memory at the end of the week. Categories beyond the fields above.

## M0.2 Decision labels on real agent history

**Build.** `field/history/`: at least 30 agent-authored commits or review rounds from the Countersign build lanes, chosen by date range, not by interest. For each, a file `<commit>.yaml` listing every decision a person finds in the diff:

```yaml
- question: "Pin the proxy library to an unreleased commit?"
  category: dependency            # 01 §4
  paths: [go.mod]
  lines: [12, 12]
  said_by_agent: no               # did the agent's own report or commit message state it
  would_have_wanted_to_rule: yes
```

An assistant session may propose labels. A person confirms or corrects each one, and the file records which labels were changed.

**Checks.**
- C0.2.1 At least 30 commits; the selection rule (date range) is written in `history/README.md`.
- C0.2.2 Every label has a category from `01` §4 and at least one path.
- C0.2.3 Ten of the commits are labelled a second time, blind, a week later or by a second person. Agreement on the set of decisions is reported (matching by category and overlapping lines).

**Denied.** Choosing commits known to be problematic. Using these files to tune a detector before M3.1 has frozen its vectors: the files are read by the scorer in M3.9 only.

## Gate G0

Computed from M0.1 and M0.2 and written to `field/G0.md`:

| Number | From | Supports the plan if |
|---|---|---|
| Share of interventions with `raised_by_agent: no` | M0.1 | ≥ 20% |
| Share of interventions with `kind: will` | M0.1 | ≥ 30% |
| Share of labelled decisions with `said_by_agent: no` and `would_have_wanted_to_rule: yes` | M0.2 | ≥ 15% |
| Share of interventions with `repeat_of` set | M0.1 | reported; it bounds how far laws can shrink the queue |

If the first and third both fall below their lines, agents on this work do raise what matters, and the central claim is weak here. The human then decides between stopping, and testing on a second body of agent work before M3.

One subject and one project is thin evidence. G0 says so in its first line.
