#!/usr/bin/env bash
# C1.6.5 Tool descriptions are each one sentence of at most 200
# characters, and tools/list offers exactly the spec'd set (the M1.6 three plus M2 backbrief and spawn).
set -uo pipefail
d=${COURT_CHECK_DIR:?}
case "$d" in /*) ;; *) d="$PWD/$d" ;; esac
go build -o "$d/court" ./cmd/court || { echo "court build failed" >&2; exit 1; }
export COURT_BIN="$d/court"
go test ./internal/adapter/scripted -count=1 -timeout 120s -v -run '^TestC1_6_5$' \
    || { echo "tool description check failed; see go test output" >&2; exit 1; }
