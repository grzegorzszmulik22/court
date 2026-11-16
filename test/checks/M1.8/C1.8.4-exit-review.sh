#!/usr/bin/env bash
# C1.8.4 The M1 exit review answers the two questions of spec/00 §5 and
# names the weakest point (docs/M1-exit-review.md).
set -uo pipefail
d=${COURT_CHECK_DIR:?}
case "$d" in /*) ;; *) d="$PWD/$d" ;; esac
go test ./internal/adapter/scripted -count=1 -timeout 60s -v -run '^TestC1_8_4$' \
    || { echo "exit review check failed; see go test output" >&2; exit 1; }
