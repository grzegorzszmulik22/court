# Reconciliation traces

Hand-written traces derived by reasoning from `spec/01` §7 and reviewed
by reading. These are the ground truth `internal/reconcile` (Go) and
`ref/python/refreconcile` are built against in M3.5, in the pattern M2.3
used for backbrief conformance.

## Schema

```json
{
  "id": "T-NN",
  "comment": "the reasoning, required on non-obvious cases",
  "inputs": {
    "expected": [{"id","category","paths","world","ruled"}],
    "decisions": [{"id","source":"declared|detected","category","paths",
                    "world","commit","petition","digests"}],
    "writs":    {"<world>": {"categories":[...],"territory":[...]}},
    "bypassed": [{"world","commits":[...]}],
    "no_decisions": [{"world","commits":[...]}]
  },
  "events": [{"seq","decision_ids":[<input ids that merged>]}, ...],
  "expect": [{"ids":[...],"sources":[...],"verdict","flags":[...],"queued","note"}],
  "tags": [...]
}
```

- `expected` entries come from `backbrief.submitted`; `ruled` marks a
  decision settled in the upfront hearing (M2.5, `01` §6).
- `commit` is the 1-based commit index in the entry's world. Timing rule
  `01` §7.4: a declaration is timely for hits at commits >= its own.
- `bypassed` lists commits with `gated: false` (`03` §4.5): hits in them
  are `undeclared` regardless of declarations.
- `digests` is required on `detected` entries and forbidden elsewhere:
  it carries the `sha256:<hex>` text digest of each hit the entry
  stands for (`01` §7.5's contest evidence; `03` §1.5 requires the
  matched text digest on every hit). Contest is DERIVED from the
  digests — different digests, or a hit against only a declaration —
  never supplied as an input. A detected entry must also name real
  `paths` (`03` §1.5: a hit without a span is a bug); only declared and
  expected entries may have empty paths.
- `no_decisions` lists commits where the agent stated no_decisions; the
  record is a gate fact, not a decision entry, and can never match a hit.
- `verdict` is the `recon` of `01` §7.2: `confirmed`, `declared-only`,
  `undeclared`, `late`, `unanswered`. `unforeseen` is a flag; it combines
  with the first four.
- `flags` ⊆ {`late`, `unforeseen`, `outside-writ`, `ruled`, `contested`}.
- `queued` follows `01` §7.3 exactly.
- `events` are the `decision.reconciled` events (`01` §11.2): one per
  merged decision, member input ids listed, seq increasing.

The validator `bench/docketbench/validate` checks each trace's shape:
id vocabulary, per-row internal consistency (verdict/flag/queued
relations from `01` §7.2 and §7.3, event coverage), tag truthfulness,
and corpus coverage. Per CR-003 it does NOT replay the traces through a
second implementation of §7; the expected results were written by
reasoning from `01` §7 and are reviewed independently, then checked
again by the independent M3.5 implementation.
