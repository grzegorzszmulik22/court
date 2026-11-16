#!/usr/bin/env bash
# C1.5.5 The endpoint binds to loopback only; a connection from another
# interface is refused.
set -uo pipefail
d=${COURT_CHECK_DIR:?}
case "$d" in /*) ;; *) d="$PWD/$d" ;; esac
go build -o "$d/court" ./cmd/court || { echo "court build failed" >&2; exit 1; }
export COURT_BIN="$d/court"
go test ./internal/adapter/scripted -count=1 -timeout 60s -v -run '^TestC1_5_5$' \
    || { echo "loopback check failed; see go test output" >&2; exit 1; }
