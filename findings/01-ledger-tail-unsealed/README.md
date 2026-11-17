# 01 — The ledger's newest event is unsealed: edit, delete or insert is accepted

**Severity.** High for the chain's stated claim; the limitation is inherent
to a bare hash chain without an external anchor, but M1 claims more than
that.

**Property, as stated.**

- `spec/01-decision-model.md` §11.1: "Events are immutable. A correction is
  a new event. The chain detects corruption and out-of-band edits. It is
  not a signature: version 1 makes no claim against an attacker who can
  rewrite the whole file."
- `spec/milestones/M1-ledger-and-worlds.md`, Expected results: "Any
  single-byte change to a stored event is found by two independent
  verifiers."
- The brief asks for "an edit, insertion, deletion or reorder of events
  that `court verify` (or the ledger's own check) accepts, other than
  rewriting the whole file".

**What I did.** Generated a normal ledger with `court ledger gen` (28
events). The hash of event *i* covers only event *i*; nothing covers the
newest event's hash and nothing records the event count. I produced three
files that each change only the tail:

1. `edited.jsonl` — the newest event's body is edited and only that event's
   hash is recomputed;
2. `deleted.jsonl` — the newest event (here `commission.closed`) is removed;
3. `forged.jsonl` — one event is appended that Court never wrote: a
   `decision.ruled` with actor `human` and body `{"decision":"D1",
   "option":"o1"}`, chained off the real last hash.

The script is `repro.py` (standard library plus the repository's own
`ref/python/refledger` for RFC 8785 bytes, so no new code is trusted).

**What happened.** Both verifiers and replay accept all three files:

```
edited  go=ok py=ok replay=ok
deleted go=ok py=ok replay=ok
forged  go=ok py=ok replay=ok
```

`court ledger verify` exits 0, `ref/python/refledger` exits 0, and
`court replay` prints `PASS events=27` (or 29) with no stored-derived
comparison to object to. The same holds for the SQLite form: deleting the
newest row (and its derived row, when the event projects) leaves a shorter
ledger that `court ledger verify` and `court replay` both accept — see
`TestRT_LedgerTailDeletionAccepted` in
`internal/ledger/rt_ledger_tamper_test.go`.

**Why it works.** `prev` links an event to its predecessor; the first
event's `prev` is checked against 64 zeros. No value covers the last event
or the length of the chain. An edit to the last event therefore changes no
link; a deletion of the tail leaves a valid prefix; an appended event is a
valid extension by construction.

**What should have happened.** Under the milestone's "any single-byte
change" claim, each of these should be reported. With the §11.1 scope
(no claim against whole-file rewrite), the limitation should at least be
stated: an edit of the newest event, a deletion of trailing events and an
appended `human` ruling are all accepted. Closing it needs something
outside the file (a stored head/length, or the signatures deferred to
later), since the chain alone cannot know where the real ledger ends.

Note: this also means an agent with write access to the ledger can wait
until its action is the newest event, edit it and recompute its hash, and
the daemon's next append chains off the forged hash — the "hash chain
shows an edit to a past event" protection in `spec/02` §2 does not cover
the event being edited. `TestRT_LedgerTailEditAccepted` edits a
`tool.event` (which projects to nothing, so replay accepts too).

**How sure.** High. Reproduced twice from a fresh generated ledger; the Go
tests fail deterministically.

**Reproduction.**

```
go build -o .rt/court.exe ./cmd/court
.rt/court.exe ledger gen --out .rt/scratch/r01 --count 1
python findings/01-ledger-tail-unsealed/repro.py .rt/scratch/r01/gen-00000.jsonl .rt/scratch/r01
for f in edited deleted forged; do
  .rt/court.exe ledger verify .rt/scratch/r01/$f.jsonl; echo go=$?
  python ref/python/refledger .rt/scratch/r01/$f.jsonl; echo py=$?
  .rt/court.exe replay .rt/scratch/r01/$f.jsonl; echo replay=$?
done
```

Go test:

```
go test ./internal/ledger -run 'TestRT_LedgerTail(Edit|Deletion)Accepted' -count=1 -v
```

**Out of scope / correctly refused.** Editing or removing an event in the
middle, and reordering events, are detected (class `seq`/`prev`/`hash`);
an insertion in the middle requires recomputing every following `prev`,
which is the whole-suffix rewrite §11.1 excludes. See
`findings/TRIED.md`.
