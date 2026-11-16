#!/usr/bin/env bash
# C1.6.3 An alarm from a child agent is in the ledger even when the
# parent's script drops every message it receives: the call goes from
# the child to the daemon directly and pairs with its PreToolUse.
set -uo pipefail
d=${COURT_CHECK_DIR:?}
case "$d" in /*) ;; *) d="$PWD/$d" ;; esac
go build -o "$d/court" ./cmd/court || { echo "court build failed" >&2; exit 1; }
export COURT_BIN="$d/court"
go test ./internal/adapter/scripted -count=1 -timeout 120s -v -run '^TestC1_6_3$' \
    || { echo "child alarm check failed; see go test output" >&2; exit 1; }
