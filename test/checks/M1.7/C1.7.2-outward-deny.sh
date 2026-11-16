#!/usr/bin/env bash
# C1.7.2 The deny decision is returned in the documented response shape
# for every outward-list entry and every data-directory path; an
# ordinary edit and `go test` abstain.
set -uo pipefail
d=${COURT_CHECK_DIR:?}
case "$d" in /*) ;; *) d="$PWD/$d" ;; esac
go build -o "$d/court" ./cmd/court || { echo "court build failed" >&2; exit 1; }
export COURT_BIN="$d/court"
go test ./internal/adapter/scripted -count=1 -timeout 120s -v -run '^TestC1_7_2$' \
    || { echo "outward deny check failed; see go test output" >&2; exit 1; }
# The deny-list property itself: every outward program and publish pair
# denied in every command position (nested -c, substitutions, subshells,
# groups, wrappers, paths).
go test ./internal/adapter/claude -count=1 -timeout 60s -v -run '^TestOutwardDeny_' \
    || { echo "outward deny table failed; see go test output" >&2; exit 1; }
