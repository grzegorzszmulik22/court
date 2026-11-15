# M1.1 plan — repository, toolchain, check tooling (supervisor review 1)

Review 1 found two tooling properties no check defended; both are now
implemented and tested.

## Property 1 — `make check-<chunk>` exit contract

Non-zero unless every check is PASS. `tools/check.sh` maps check exit 0 to
PASS, 77 to SKIP, 78 to BLOCKED, anything else (including `timeout` kill) to
FAIL, and prints exactly one `CHECK <id> <status>` line per check. A check
emitting a `CHECK` line naming its own id is a protocol violation -> FAIL
(the runner owns the CHECK namespace). Missing/empty checks dir or all
checks excluded -> non-zero, no CHECK lines.

Tested in `test/checks/M1.1/C1.1.2-*.sh` via temporary chunks
(`COURT_CHECKS_DIR`): FAIL capture, SKIP, BLOCKED, timeout, forged-line,
missing dir, empty dir, `--exclude`.

## Property 2 — `make sabotage-<chunk>` exit contract

Non-zero unless every patch applied cleanly AND produced >=1 non-PASS CHECK
line. `tools/sabotage.sh` uses `git apply --check`+`git apply` (no fuzz, no
partial application). Classification per patch:

- `SABOTAGE <name> caught by <ids>` — inner run printed a non-PASS CHECK line
- `UNCAUGHT <patch>` — inner run passed (suite stayed green)
- `SABOTAGE <name> ERROR <why>` — patch did not apply, scratch copy failed,
  or the inner run failed without any non-PASS CHECK line. An ERROR is never
  a catch; it is named and fails the run.

The inner run gets `COURT_CHECKS_DIR`, `COURT_SABOTAGE_DIR` and
`COURT_CHECK_EXCLUDE` unset, so no check can be excluded from evaluation.
`COURT_SABOTAGE_DIR`/`COURT_SABOTAGE_TIMEOUT` (600s) bound the run.

Tested in `test/checks/M1.1/C1.1.7-*.sh` via `COURT_SABOTAGE_DIR` temp dirs:
uncaught named (D9.2), non-applying patch -> ERROR, inner run with no CHECK
lines -> ERROR, empty sabotage dir -> error, env sanitisation. C1.1.3 also
asserts the three spec-named sabotage patches exist and are non-empty.

## Sabotages for each property (test/sabotage/M1.1/)

- `skip-counted-as-pass` — check runner: SKIP no longer fails the run -> caught by C1.1.2
- `apply-failure-counted-as-caught` — sabotage runner: non-applying patch reported caught -> caught by C1.1.7
- `inner-failure-counted-as-caught` — sabotage runner: no-CHECK-line inner failure reported caught -> caught by C1.1.7
- spec-required: `evidence-capture-drops-stderr` (C1.1.2,C1.1.8), `frozen-check-skips-missing-file` (C1.1.5), `sabotage-runner-always-green` (C1.1.7)

`fail-reported-as-pass` was removed: a FAIL->PASS mutation makes a non-PASS
CHECK line structurally impossible, so it can only ever be an ERROR, never a
catch — exactly what the new semantics report for that class.

## Results

- `make check-M1.1`: 8/8 PASS, twice consecutively from `make clean`
  (this folder is the second run; C1.1.1 re-verified the offline fresh clone).
- `make sabotage-M1.1`: 6/6 patches caught, `0 uncaught`.
- `make lint`, `make regress`: green.

## Upstream evidence (05 §8)

`upstream/` holds the recorded queries for the pinned versions: Go 1.27.4
(arm64 tarball sha256-pinned), modernc.org/sqlite v1.62.0, MCP Go SDK v1.8.0
(recorded, not implemented in M1.1), Claude Code 2.1.325 (recorded only).
