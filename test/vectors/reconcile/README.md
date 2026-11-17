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
                    "world","commit","petition"}],
    "writs":    {"<world>": {"categories":[...],"territory":[...]}},
    "bypassed": [{"world","commits":[...]}],
    "contested": [["<id>","<id>"]]
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
- `contested` lists cross-world decision pairs with differing evidence
  (`01` §7.5): both are queued.
- `verdict` is the `recon` of `01` §7.2: `confirmed`, `declared-only`,
  `undeclared`, `late`, `unanswered`. `unforeseen` is a flag; it combines
  with the first four.
- `flags` ⊆ {`late`, `unforeseen`, `outside-writ`, `ruled`, `contested`}.
- `queued` follows `01` §7.3 exactly.
- `events` are the `decision.reconciled` events (`01` §11.2): one per
  merged decision, member input ids listed, seq increasing.

The validator `bench/docketbench/validate` replays each trace through an
independent implementation of §7 and requires agreement on every field.
