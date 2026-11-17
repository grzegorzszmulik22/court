# 02 — SQLite reader silently recomputes a blank `hash`; Go and refledger disagree

**Severity.** High: an edited newest event verifies in the Go tool, and a
full re-export launders the edit so both verifiers accept it.

**Property, as stated.**

- `spec/01-decision-model.md` §11.1: `prev` is the SHA-256 of the previous
  event's canonical bytes; events are immutable.
- `spec/02-architecture.md` §6: "One table is the ledger: `seq, prev, at,
  commission, actor, type, body, hash`… The canonical bytes of an event are
  RFC 8785 JSON of the envelope without `hash`."
- `spec/milestones/M1-ledger-and-worlds.md` M1.2: "the envelope and
  canonical bytes of `01` §11.1, strict decoding (unknown fields and
  duplicate keys rejected)". C1.3.1/MM1.2 require the Go and Python
  verifiers to report the same error class for every mutant (100%
  agreement).

**What I did.** Built a normal ledger DB with `court ledger build`, then
edited the newest event's body and set its stored `hash` column to the
empty string (`tamper.py`). The Go read path
(`internal/ledger/verify.go:readDBLines` → `Event.Line`) calls
`ComputeHash()` whenever `e.Hash == ""`, so the empty column is replaced by
a hash of whatever body is now stored. `refledger`'s `_read_db` keeps the
stored value, so the same row is an envelope error there.

A second step runs `court ledger export` on the tampered database. The
exported JSONL carries the recomputed hash; both verifiers accept it.

**What happened.**

```
== tampered ledger (last event edited, hash blanked) ==
VERIFY .../ledger.db ok
court verify exit=0
VERIFY .../ledger.db envelope seq=27 hash is not sha256 hex
refledger exit=1

== re-exported tampered ledger ==
VERIFY .../laundered.jsonl ok
court verify exit=0
VERIFY .../laundered.jsonl ok
refledger exit=0
```

The same test against a `tool.event` (which projects to no derived table)
also passes `court replay`:

```
VERIFY bh2.db ok            (Go)
VERIFY bh2.db envelope seq=1 hash is not sha256 hex   (refledger)
REPLAY bh2.db PASS events=2 ... derived-checked
```

(`TestRT_LedgerBlankHashAcceptsEdit` in
`internal/ledger/rt_ledger_tamper_test.go` and `repro.sh` here.)

**Why it works.** `Event.Line` is written for constructing new events, not
for reading stored ones:

```go
func (e *Event) Line() ([]byte, error) {
    if e.Hash == "" {
        if _, err := e.ComputeHash(); err != nil { ... }
    }
    ...
}
```

`readDBLines` re-serializes every row through `Line()`, so a missing hash
is "repaired" rather than rejected. Because the empty hash is only
observable on the row, this also means blanking the hash of *any* event is
accepted (the recomputed value equals the original when the body is
unchanged). Editing the body of a non-last event additionally needs the
following `prev` values re-tied, i.e. a suffix rewrite; the last-event case
is a single-row edit.

**What should have happened.** A stored `hash` that is missing, empty or
not 64 lowercase hex is corruption; the database read path must reject it
with class `envelope` (the same class refledger reports). The two
verifiers must agree on this file. `court ledger export` must not turn a
corrupt row into a valid line.

**How sure.** High. Reproduced twice; the Go test fails deterministically;
the Go/Python disagreement is on the same file.

**Reproduction.**

```
go build -o .rt/court.exe ./cmd/court
bash findings/02-ledger-db-empty-hash/repro.sh
```

Go test:

```
go test ./internal/ledger -run 'TestRT_LedgerBlankHashAcceptsEdit' -count=1 -v
```

**Not a finding by itself.** Editing the last event with a *correctly
recomputed* hash is accepted by both verifiers; that is finding 01. The
bug here is the extra leniency (an absent hash is trusted) and the
Go/Python disagreement.
