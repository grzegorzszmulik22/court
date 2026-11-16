#!/usr/bin/env bash
# C1.6.1 Each tool validates its input against its schema; a bad
# category is rejected with the list of valid ones. Also asserts an
# unknown token gets no endpoint.
set -uo pipefail
d=${COURT_CHECK_DIR:?}
case "$d" in /*) ;; *) d="$PWD/$d" ;; esac
go build -o "$d/court" ./cmd/court || { echo "court build failed" >&2; exit 1; }
export COURT_BIN="$d/court"
go test ./internal/adapter/scripted -count=1 -timeout 120s -v -run '^TestC1_6_1$' \
    || { echo "tool validation check failed; see go test output" >&2; exit 1; }
