#!/usr/bin/env bash
# C1.8.3 Killing the daemon mid-run and restarting loses no
# acknowledged event; failed hook posts retry; no step records twice.
set -uo pipefail
d=${COURT_CHECK_DIR:?}
case "$d" in /*) ;; *) d="$PWD/$d" ;; esac
go build -o "$d/court" ./cmd/court || { echo "court build failed" >&2; exit 1; }
export COURT_BIN="$d/court"
go test ./internal/adapter/scripted -count=1 -timeout 300s -v -run '^TestC1_8_3$' \
    || { echo "restart check failed; see go test output" >&2; exit 1; }
