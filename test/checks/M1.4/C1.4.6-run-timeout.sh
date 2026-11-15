#!/usr/bin/env bash
# C1.4.6 `Run` enforces its time limit and reports a timeout distinctly from
# a non-zero exit.
set -uo pipefail
go test ./internal/world -run '^TestC1_4_6_RunTimeout$' -count=1 -timeout 60s -v \
    || { echo "run-timeout check failed; see go test output" >&2; exit 1; }
