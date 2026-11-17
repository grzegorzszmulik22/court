#!/usr/bin/env bash
# C2.5.2 M2.5 — see spec/milestones/M2-commission-and-backbrief.md.
set -uo pipefail
d=${COURT_CHECK_DIR:?}
case "$d" in /*) ;; *) d="$PWD/$d" ;; esac
go build -o "$d/court" ./cmd/court || { echo "court build failed" >&2; exit 1; }
export COURT_BIN="$d/court"
go test ./internal/commission -count=1 -timeout 240s -v -run "^TestC2_5_2$"     || { echo "check failed; see go test output" >&2; exit 1; }
