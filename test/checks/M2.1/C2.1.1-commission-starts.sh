#!/usr/bin/env bash
# C2.1.1 In a fresh fixture repository, `court commission "x"` with no
# commission-shaping flags records a commission, starts the daemon,
# creates a world and starts the scripted orchestrator.
set -uo pipefail
d=${COURT_CHECK_DIR:?}
case "$d" in /*) ;; *) d="$PWD/$d" ;; esac
go build -o "$d/court" ./cmd/court || { echo "court build failed" >&2; exit 1; }
export COURT_BIN="$d/court"
go test ./internal/commission -count=1 -timeout 120s -v -run '^TestC2_1_1$' \
    || { echo "commission-start check failed; see go test output" >&2; exit 1; }
