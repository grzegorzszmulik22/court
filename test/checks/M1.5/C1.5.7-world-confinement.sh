#!/usr/bin/env bash
# C1.5.7 World confinement and append-schema hardening (red-team B3-09,
# B3-03): scripted-agent writes stay inside the world — `..`, absolute
# paths, prefix-sharing siblings (<world>-x, <world>2) and symlinks are
# denied by whole path component, every path-writing tool (Write, Edit,
# MultiEdit, NotebookEdit) is confined by name, the attempt is recorded
# as a denied tool.event with a reason — and a tool call that would write
# a schema-invalid event (backbrief plan.lines = -1, hostile inputs) is
# rejected and writes nothing, so the ledger stays verifiable.
set -uo pipefail
d=${COURT_CHECK_DIR:?}
case "$d" in /*) ;; *) d="$PWD/$d" ;; esac
go build -o "$d/court" ./cmd/court || { echo "court build failed" >&2; exit 1; }
export COURT_BIN="$d/court"
go test ./internal/paths ./internal/adapter/scripted -count=1 -timeout 120s -v \
    -run 'TestC1_5_7' \
    || { echo "world-confinement/append-schema check failed; see go test output" >&2; exit 1; }
