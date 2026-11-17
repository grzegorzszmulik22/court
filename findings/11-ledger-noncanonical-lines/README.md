# 11 — Stored bytes need not be canonical: whitespace and CRLF rewrites verify

**Severity.** Low / informational. The event content is unchanged; this
does not let an attacker alter what the ledger says. It does contradict
the milestone's "single-byte change" claim and shows the verifier checks
re-canonicalized bytes rather than the stored bytes.

**Property, as stated.**

- `spec/milestones/M1-ledger-and-worlds.md`, Expected results: "Any
  single-byte change to a stored event is found by two independent
  verifiers."
- `spec/02-architecture.md` §6: "The canonical bytes of an event are RFC
  8785 JSON of the envelope without `hash`." The stored form is described
  as those bytes plus `hash` (`internal/ledger/event.go:Line`).

**What I did.**

1. Inserted one space after the opening `{` of the first line of a
   generated ledger (`{"seq"` → `{ "seq"`).
2. Converted every line ending in the file from `\n` to `\r\n`.

Both are byte changes to stored events; the JSON still decodes to the same
values, so `CanonicalBytes()` produces the same bytes and the same hash.

**What happened.** `court ledger verify` and `ref/python/refledger` both
report `ok`; `court replay` passes. Exactly the same happens with a
reordered object member, an escaped string written with `\uXXXX` instead
of raw UTF-8 (where lengths allow), or any other non-canonical spelling of
the same value.

`TestRT_LedgerNonCanonicalLineAccepted` fails on current code for the
whitespace case.

**What should have happened.** If the claim is about stored events, the
verifier should require the line to equal `Event.Line()` / `canon_of(ev)`
byte for byte (or keep a separate digest of the stored line in the chain).
If the claim is only about semantic changes, the milestone should say so.

**How sure.** High for the acceptance; the "finding" status is a judgement
call because no semantic value changes.

**Reproduction.**

```
go build -o .rt/court.exe ./cmd/court
.rt/court.exe ledger gen --out .rt/scratch/r11 --count 1
python - <<'PY'
p = ".rt/scratch/r11/gen-00000.jsonl"
d = open(p, "rb").read()
open(".rt/scratch/r11/space.jsonl", "wb").write(b"{ " + d[1:])
open(".rt/scratch/r11/crlf.jsonl", "wb").write(d.replace(b"\n", b"\r\n"))
PY
.rt/court.exe ledger verify .rt/scratch/r11/space.jsonl; echo go=$?
python ref/python/refledger .rt/scratch/r11/space.jsonl; echo py=$?
.rt/court.exe ledger verify .rt/scratch/r11/crlf.jsonl; echo go=$?
python ref/python/refledger .rt/scratch/r11/crlf.jsonl; echo py=$?
```

Go test:

```
go test ./internal/ledger -run 'TestRT_LedgerNonCanonicalLineAccepted' -count=1 -v
```
