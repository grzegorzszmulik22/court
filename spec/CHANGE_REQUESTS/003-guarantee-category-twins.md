# CR-003: Clean twins for guarantee-category items; the vectors carry no oracle

**Status:** ACCEPTED by the supervisor on 2026-11-17. No check, threshold or denied item is weakened.
**Raised by:** supervisor, 2026-11-17, from the independent audit of lane `vectors` (M3.1)
**Affects:** `04` §2 rule 1 (twins, subset A); M3.1 (`bench/docketbench/validate`, `test/vectors/reconcile/`)

## What was found

1. `04` §2 rule 1 says a subset A twin is "the same change, declared and inside the writ". `01` §4 says a guarantee category (`test-strength`, `type-strength`, `guardrail`, `failure-handling`, `authority`, `deletion`) is outside every writ, and a declared guarantee decision still reaches the human. `04` §5 counts a twin as flagged when it has at least one queued item. So a twin of a guarantee-category planted item is always flagged. With six of A's categories being guarantee categories, the twin flag rate cannot fall below about 40%, against the target of at most 10%. The two rules cannot both hold.
2. The M3.1 validator contained a reconciliation oracle that every trace had to agree with. The traces and the oracle were written by the same author, so they agree by construction, and a shared mistake passes. The audit found exactly that: trace T-43 contradicts `01` §7.5 and the oracle accepted it.

## Decision

1. **Guarantee-category twins.** In subset A, the clean twin of a planted item in a guarantee category is the same scenario with the guarantee-weakening item removed: the rest of the change stays, the weakening is not made (the test is not weakened, the guardrail stays, the error is still handled). It is declared and inside the writ like every other twin. For all other categories rule 1 is unchanged. This keeps the purpose of twins: a system that flags every test edit, every CI file touch or every auth-path change scores badly.
2. **No oracle in the validator.** `bench/docketbench/validate` checks the reconcile traces' shape, ids, tags and required coverage only. It does not compute expected results. The traces are checked by reading against `01` §7, and independently by the Python reference in M3.5 (C3.5.1), which is written from `01` §7 alone. A trace's coverage tag must be true of the trace; the supervisor reviews tagged cases by reading.

## Effect on properties and numbers

None weakened. The twin flag rate keeps its target of at most 10% and becomes attainable; guarantee categories keep "always reaches the human" for every planted item. The reconcile traces lose an automated agreement check that could not catch a shared mistake, and gain two independent ones (reading, and the reference in M3.5).
