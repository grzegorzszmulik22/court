#!/usr/bin/env bash
# C1.5.4 A subagent payload (agent_id set on the same session token) is
# recorded under a child agent id whose parent is the session's agent.
set -uo pipefail
d=${COURT_CHECK_DIR:?}
case "$d" in /*) ;; *) d="$PWD/$d" ;; esac
go build -o "$d/court" ./cmd/court || { echo "court build failed" >&2; exit 1; }
export COURT_BIN="$d/court"
go test ./internal/adapter/scripted -count=1 -timeout 120s -v -run '^TestC1_5_4$' \
    || { echo "subagent check failed; see go test output" >&2; exit 1; }
