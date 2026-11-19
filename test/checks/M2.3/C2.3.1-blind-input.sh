#!/usr/bin/env bash
# C2.3.1 M2.3 — see spec/milestones/M2-commission-and-backbrief.md.
set -uo pipefail
d=${COURT_CHECK_DIR:?}
case "$d" in /*) ;; *) d="$PWD/$d" ;; esac
bin=$(bash test/checks/court-bin.sh "$d") || exit 1
go build -o "$bin" ./cmd/court || { echo "court build failed" >&2; exit 1; }
export COURT_BIN="$bin"
go test ./internal/commission -count=1 -timeout 240s -v -run '^TestC2_3_1$'     || { echo "check failed; see go test output" >&2; exit 1; }
