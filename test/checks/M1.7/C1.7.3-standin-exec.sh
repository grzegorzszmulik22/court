#!/usr/bin/env bash
# C1.7.3 The adapter starts a stand-in executable with the world as
# working directory and the documented flags, asserted from the
# double's own log.
set -uo pipefail
d=${COURT_CHECK_DIR:?}
case "$d" in /*) ;; *) d="$PWD/$d" ;; esac
go build -o "$d/court" ./cmd/court || { echo "court build failed" >&2; exit 1; }
export COURT_BIN="$d/court"
go test ./internal/adapter/scripted -count=1 -timeout 60s -v -run '^TestC1_7_3$' \
    || { echo "stand-in exec check failed; see go test output" >&2; exit 1; }
