#!/usr/bin/env bash
# C1.6.2 A call on one token is never attributed to another agent:
# 1,000 interleaved alarm calls from 8 scripted agents.
set -uo pipefail
d=${COURT_CHECK_DIR:?}
case "$d" in /*) ;; *) d="$PWD/$d" ;; esac
go build -o "$d/court" ./cmd/court || { echo "court build failed" >&2; exit 1; }
export COURT_BIN="$d/court"
go test ./internal/adapter/scripted -count=1 -timeout 300s -v -run '^TestC1_6_2$' \
    || { echo "token attribution check failed; see go test output" >&2; exit 1; }
