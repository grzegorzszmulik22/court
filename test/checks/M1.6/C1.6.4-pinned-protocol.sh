#!/usr/bin/env bash
# C1.6.4 The endpoint works with a client speaking the pinned protocol
# revision without a session header (the transport is stateless).
set -uo pipefail
d=${COURT_CHECK_DIR:?}
case "$d" in /*) ;; *) d="$PWD/$d" ;; esac
go build -o "$d/court" ./cmd/court || { echo "court build failed" >&2; exit 1; }
export COURT_BIN="$d/court"
go test ./internal/adapter/scripted -count=1 -timeout 120s -v -run '^TestC1_6_4$' \
    || { echo "pinned protocol check failed; see go test output" >&2; exit 1; }
