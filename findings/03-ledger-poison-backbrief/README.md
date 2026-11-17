# 03 — An agent's `backbrief` can make the commission's ledger unverifiable forever

**Severity.** High: an ordinary MCP tool call writes an event the
verifier's own JSON Schema rejects. From that point on `court ledger
verify` and `court replay` fail for the whole ledger, the daemon keeps
appending without noticing, and events are immutable so the record cannot
be repaired. This is a self-inflicted denial of the audit record (P1/H2),
triggerable by a first tool call with no special privilege.

**Property, as stated.**

- `spec/01-decision-model.md` §2: "Every model output enters the ledger as
  an event. Everything Court then does is a deterministic function of the
  ledger." §11.1: events are immutable; a correction is a new event.
- `spec/milestones/M1-ledger-and-worlds.md` M1.2: typed bodies, strict
  decoding, and C1.2.5 "every Go body type has a schema and every schema a
  Go type"; the verifier validates each body against
  `api/schema/<type>.schema.json`.
- The append path must not accept a body its verifier rejects; otherwise
  the ledger is not a record anything can check.

**What I did.** Called the `backbrief` tool through the scripted-agent
harness with `plan.lines = -1`:

```go
&scripted.Step{Backbrief: &scripted.BackbriefStep{
    Goal: "restate the goal",
    Plan: map[string]any{"files": []string{}, "lines": -1},
}}
```

`internal/tools/tools.go:backbrief` copies `plan.lines` into
`ledger.Plan` without a bounds check;
`internal/ledger/bodies.go:ValidateBody` checks the goal and the expected
decisions but never the plan; the append succeeds.
`api/schema/backbrief.submitted.schema.json` declares
`"lines": {"type": "integer", "minimum": 0}`, so the verifier rejects the
event.

**What happened.**

```
ledger written through the front door does not verify:
class=schema seq=3 $.body.plan.lines: below minimum
```

`court ledger verify` and `ref/python/refledger` both report the schema
error; `court replay` fails at verification. Appending a later, valid
event still succeeds and the ledger stays invalid at seq 3 (see
`repro.sh`).

The same mismatch is reachable without an agent through `court ledger
append`, which is useful as a minimal reproduction:

```
$ .rt/court.exe ledger append poison.db C1 agent:a1 backbrief.submitted <(cat body.json)
append exit=0
$ .rt/court.exe ledger verify poison.db
VERIFY poison.db schema seq=0 $.body.plan.lines: below minimum
```

**What should have happened.** `ValidateBody` (the append-time check) and
the JSON Schemas (the verify-time check) must agree, and `Append` must
reject the body before the durable write. A malformed `backbrief` should
return a tool error to the agent, not poison the ledger.

**Related, same class.** The append path validates bodies with hand-written
Go rules that are weaker than the schemas in several places. Confirmed with
`court ledger append` (operator-accessible, not agent-accessible):
`decision.contested` accepts `worlds: ["",""]` although the schema has
`minLength: 1` on each item; `backbrief.submitted` accepts `invalid:true`
with an `output` longer than 2000 characters. See `findings/TRIED.md`. The
agent-reachable one found here is `plan.lines`.

**How sure.** High. The harness test runs the actual MCP tool over the
actual daemon; reproduced twice. The CLI reproduction also runs twice.

**Reproduction (agent path).**

```
go test ./internal/adapter/scripted -run 'TestRT_BackbriefPoisonsLedger' -count=1 -v
```

**Reproduction (CLI).**

```
go build -o .rt/court.exe ./cmd/court
bash findings/03-ledger-poison-backbrief/repro.sh
```
