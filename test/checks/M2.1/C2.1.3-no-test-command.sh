#!/usr/bin/env bash
# C2.1.3 With no detectable test command the commission starts with an
# empty done-check list and one line says done-claims cannot be checked.
set -uo pipefail
d=${COURT_CHECK_DIR:?}
case "$d" in /*) ;; *) d="$PWD/$d" ;; esac
bin=$(bash test/checks/court-bin.sh "$d") || exit 1
go build -o "$bin" ./cmd/court || { echo "court build failed" >&2; exit 1; }
export COURT_BIN="$bin"
go test ./internal/commission -count=1 -timeout 120s -v -run '^TestC2_1_3$' \
    || { echo "no-test-command check failed; see go test output" >&2; exit 1; }
