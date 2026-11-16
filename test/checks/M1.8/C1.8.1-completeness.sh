#!/usr/bin/env bash
# C1.8.1 (MM1.1) The load run: 8 scripted agents, >=10,000 tool events
# and >=500 commits, randomly interleaved; every scripted step has
# exactly one ledger event, compared against the scripts.
set -uo pipefail
d=${COURT_CHECK_DIR:?}
case "$d" in /*) ;; *) d="$PWD/$d" ;; esac
go build -o "$d/court" ./cmd/court || { echo "court build failed" >&2; exit 1; }
export COURT_BIN="$d/court"
go test ./internal/adapter/scripted -count=1 -timeout 600s -v -run '^TestC1_8_1$' \
    || { echo "completeness check failed; see go test output" >&2; exit 1; }
