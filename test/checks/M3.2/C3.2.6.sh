#!/usr/bin/env bash
# C3.2.6 M3.2 — see spec/milestones/M3-detection-and-reconciliation.md.
set -uo pipefail
d=${COURT_CHECK_DIR:?}
case "$d" in /*) ;; *) d="$PWD/$d" ;; esac
go build -o "$d/court" ./cmd/court || { echo "court build failed" >&2; exit 1; }
export COURT_BIN="$d/court"
go test ./internal/gate -count=1 -timeout 240s -v -run '^TestC3_2_6'     || { echo "check failed; see go test output" >&2; exit 1; }
