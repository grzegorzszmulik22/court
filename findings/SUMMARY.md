# B3 red-team summary — Court M1 ledger, worlds, adapters

Scope: `internal/ledger`, `court ledger verify` / `court replay`,
`internal/world`, and the adapter paths (`internal/adapter`,
including the daemon's PreToolUse policy and `/gate`). Host: Windows 11,
Go 1.27.4, git for Windows. Everything ran inside this clone; the only
processes started were `court`, `git`, `ping` (5 s, self-terminating) and
PowerShell for one path query; all have exited.

## Counts

- Recorded attempts/checks: **66** in `findings/TRIED.md` (many cover
  several inputs or variants).
- Findings: **11** (10 behavioural, 1 informational).
- All reproductions were run at least twice from a clean state; each
  finding folder has the failing `rt_*_test.go` and/or script and the exact
  command.

## Findings

| # | Finding | Folder |
|---|---|---|
| 01 | The newest event is unsealed: edit + rehash, tail deletion, and a forged `human` ruling appended to the tail all pass both verifiers and `court replay` | `01-ledger-tail-unsealed/` |
| 02 | SQLite reader silently recomputes a blank `hash`; Go accepts an edited last event, refledger reports `envelope`; `court ledger export` launders the edit | `02-ledger-db-empty-hash/` |
| 03 | An agent's `backbrief` with `plan.lines = -1` is accepted by the append path and rejected by the verifier's schema: the commission's ledger is permanently unverifiable, and appends continue | `03-ledger-poison-backbrief/` |
| 04 | `Discard` leaves `git config --local` changes and files in `.git/hooks` made from a world; `Digest` cannot see either | `04-world-discard-config-hooks/` |
| 05 | A ref/tag created inside a world survives `Discard`; `Discard` returns nil while the H3 digest changes | `05-world-discard-extra-ref/` |
| 06 | A world can write into the repository's main worktree; the file survives `Discard` and `Digest` is unchanged | `06-world-main-worktree-write/` |
| 07 | `World.Run`'s timeout is not a wall-clock bound when a detached descendant (`cmd /c start /b …`) holds the output pipes: 5.1 s for a 0.5 s limit, unbounded in principle | `07-world-run-detached-hang/` |
| 08 | The data-directory deny rule is a case-sensitive string prefix: case variants, `\\?\` paths and a junction inside the world reach the real data directory; the file is written | `08-adapter-datadir-aliases/` |
| 09 | The scripted adapter's Edit writes `../../../outside/escape.txt` outside the world; Court's policy does not confine edits to the world (trust caveat in the README) | `09-scripted-edit-escape/` |
| 10 | Unauthenticated `/gate` records a sibling world's commit as a `world.commit` of the poster's world; `court verify` accepts it | `10-gate-foreign-commit/` |
| 11 | Stored lines need not be canonical: one-byte whitespace insertion and whole-file CRLF conversion verify in both verifiers (`court replay` too); no semantic change | `11-ledger-noncanonical-lines/` |

Severity ordering for triage: 03 (agent-triggerable, permanent loss of
verifiability), 01/02 (accepted ledger edits/deletions; 02 is a Go/Python
disagreement), 04/05/06 (H3 false for shared state), 07 (availability),
08/09 (adapter confinement), 10 (record attribution), 11 (informational).

## What I did not get to

- The full C1.2.3/C1.2.4 stress: kill -9 mid-append and all fault points
  were not rerun. Multi-writer was tested: 50 rounds of two concurrent
  appends (100/100 events, no error) and one burst of 100 (99 events, one
  `SQLITE_BUSY` to that caller, chain valid).
- A live Claude Code session (`L1.7.1`): whether a real agent with
  user-level edit auto-approval can use the case-variant/junction path
  (finding 08) without a shell, and whether Bash can POST `/gate`
  (finding 10) under the default permission mode.
- Direct hook-endpoint fuzzing: duplicate `tool_use_id`s, JSON-RPC id
  reuse, `agent_id` with an actor-invalid shape (see `NOTES.md`).
- Windows symlinks requiring developer mode/admin; 8.3 names (disabled on
  this volume); ADS; UNC paths; device names.
- `git gc --prune=now` from one world against a sibling's unreferenced
  objects; branch/worktree moves making `Discard` fail noisily.
- M2/M3 (commission, docket, detectors, reconciliation) beyond the
  `backbrief` tool that produced finding 03.
- Go/Python differential fuzzing of the JSON canonicalizer beyond the
  published vectors and the hand-picked numeric edge cases.

## What I would try next

1. Property-based differential testing: random bodies and envelopes through
   `ledger.ValidateBody`/`Append` and through both schemas, asserting
   “anything Append accepts, both verifiers accept” (finding 03 is one
   instance; the class is likely larger).
2. An external head anchor for findings 01/02 (a stored `(seq, hash)` or a
   periodic seal event), then re-run the mutation suite with tail cases.
3. Include the main worktree, `.git/config`, `.git/hooks` and reflogs in
   `Digest`, and assert the digest after every `Discard` in C1.4.1.
4. A Windows job object for `Run` and a C1.4.6 variant with a detached
   child and a strict (limit + 1 s) assertion.
5. Resolve file identity in `Policy` (`GetFinalPathNameByHandle`, or
   `EvalSymlinks` plus case-folding) and test junctions, 8.3 names on a
   volume that has them, and verbatim/UNC spellings.
6. Authenticate `/gate` with the session token and check the posted commit
   is the session world's own tip/ancestry.

## Files added

Finding material:

- `findings/01-ledger-tail-unsealed/` (README, `repro.py`)
- `findings/02-ledger-db-empty-hash/` (README, `repro.sh`, `tamper.py`)
- `findings/03-ledger-poison-backbrief/` (README, `repro.sh`, `body.json`)
- `findings/04-world-discard-config-hooks/` … `findings/11-ledger-noncanonical-lines/` (READMEs)
- `findings/TRIED.md`, `findings/NOTES.md`, this file

Failing `rt_*_test.go` reproductions (all start with `rt_`, product code
untouched):

- `internal/ledger/rt_ledger_tamper_test.go` (01, 02, 11)
- `internal/world/rt_world_test.go` (04, 05, 06, 07; also the two passing
  “refused” cases)
- `internal/adapter/claude/rt_policy_test.go` (08)
- `internal/adapter/scripted/rt_poison_test.go` (03)
- `internal/adapter/scripted/rt_escape_test.go` (09)
- `internal/adapter/scripted/rt_gate_test.go` (10)

`go build ./...`, `go vet` on the touched packages and the existing
C1.2.5/C1.4.2/C1.4.4/C1.4.5/C1.4.6/C1.4.7 checks still pass; the `TestRT_*`
tests fail on purpose. Build artifacts and scratch output were removed;
`.rt/court.exe` (git-excluded) is the binary the reproduction commands use
and can be rebuilt with `go build -o .rt/court.exe ./cmd/court`.
