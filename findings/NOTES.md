# Unconfirmed, by-design or low-value observations

Nothing here is claimed as a finding. Each line says what was seen and why
it is not written up.

1. **Derived tables are not authenticated; `court ledger verify` accepts
   them.** Editing `decisions.state` in SQLite leaves
   `court ledger verify` green; `court replay` reports
   `table decisions differs`. `spec/02` §6 says every other table is
   derived and can be rebuilt, so this is arguably by design, but any
   runtime consumer of the derived tables (queue, gating) sees the tamper
   until someone runs replay. Reproduce:
   `python -c 'import sqlite3;c=sqlite3.connect("db");c.execute("update decisions set state=\"ruled\"");c.commit()'`
   then `court ledger verify db` (ok) and `court replay db` (diverges).

2. **`Store.Append` does not validate envelope fields.** An actor such as
   `agent:evil name` is accepted and stored; the verifier's `reActor` then
   rejects it (`envelope` class). Reached only by code that calls Append
   with an arbitrary actor, or by a direct hook post whose `agent_id`
   contains characters the regex forbids. The agent tools cannot set the
   actor directly, and `spawn` agent names pass through `world.Fork`'s
   `nameRe` (which excludes spaces), so no agent path was confirmed.

3. **Idempotency-key suppression.** `AppendIdem` deduplicates on
   caller-supplied identities (`hook|<token>|<tool_use_id>`,
   `mcp|<token>|<json-rpc id>`). An agent that can post hooks directly and
   knows a future request id could pre-seed the key and suppress an event.
   Not attempted; would need direct hook POSTs, which the ledger threat
   model already treats as “an agent with a shell could reach it”.

4. **Scripted `Bash` has no timeout.** `internal/adapter/scripted/client.go:runBash`
   uses `CombinedOutput` with no context; a non-terminating script step
   hangs the check. Scripts are Court-owned test input, so this is a test
   robustness issue, not an attack; the `World.Run` hang that *is* reachable
   from product code is finding 07.

5. **`court replay` on a live database.** `ReadLedgerFile` reads the event
   rows and `dumpDerivedIfDB` reads derived tables in separate statements;
   a concurrent append between them should produce a spurious mismatch.
   Not reproduced (it needs a writer racing the verifier), and the verifier
   is described as a check-time command.

6. **`hasSubmodules` scans HEAD only.** A gitlink or `.gitmodules` present
   in another commit/branch is not seen. Commission worlds fork the
   repository's HEAD, so the normal path is covered; not attacked.

7. **8.3 short names.** This volume has 8.3 generation disabled, so the
   policy's short-name bypass (finding 08 covers case, verbatim and
   junction) cannot be demonstrated here. If enabled, `insideDataDir` would
   not match the short spelling for the same reason it misses case.

8. **Device names.** `NUL`/`CON` paths inside a world produce no file (the
   write goes to the device); no escape or hang was observed. Not pursued.

9. **`court commission` / M2 and the detector/reconciliation layers** were
   not attacked: the brief targets M1 and says M2 checks may not yet run
   here. The only M2 surface touched is the `backbrief` tool, which is what
   produced finding 03.

10. **A large burst of writers can surface `SQLITE_BUSY`.** 100 simultaneous
    `court ledger append` processes produced 99 events and one
    `database is locked (5)` error after the internal retries. Two
    processes at once (the C1.2.3 shape) showed no loss over 50 rounds.
    The production daemon is the only writer and serializes in-process, so
    this is an operational limit of the CLI, not a chain defect; the
    caller sees the error rather than a silent loss.

11. **Ledger tail findings are inherent to a bare chain.** Findings 01 and
    the append half of 02 cannot be fixed inside the file format without an
    external anchor; they are reported because M1 states a stronger claim
    (“any single-byte change”, “chain detects out-of-band edits”) and the
    brief asks for edits/insertions/deletions accepted other than a
    whole-file rewrite. Signatures are explicitly deferred.
