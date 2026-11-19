#!/usr/bin/env bash
# C2.1.5 Every default used is listed in commission.opened.defaults.
set -uo pipefail
d=${COURT_CHECK_DIR:?}
case "$d" in /*) ;; *) d="$PWD/$d" ;; esac
bin=$(bash test/checks/court-bin.sh "$d") || exit 1
go build -o "$bin" ./cmd/court || { echo "court build failed" >&2; exit 1; }
export COURT_BIN="$bin"
go test ./internal/commission -count=1 -timeout 120s -v -run '^TestC2_1_5$' \
    || { echo "defaults check failed; see go test output" >&2; exit 1; }
