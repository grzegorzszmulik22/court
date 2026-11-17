#!/usr/bin/env bash
# M2.3 — the backbrief.submitted body is this chunk's ledger body (the
# clerk's output is recorded under it). TestBodyFieldShapes asserts the
# body-type <-> schema field-name bijection for every event type, so
# schema drift like an undeclared Go field fails this suite.
set -uo pipefail
go test ./internal/ledger -count=1 -timeout 120s -v -run '^TestBodyFieldShapes$' \
    || { echo "check failed; see go test output" >&2; exit 1; }
