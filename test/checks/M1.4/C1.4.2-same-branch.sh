#!/usr/bin/env bash
# C1.4.2 Two worlds cannot hold the same branch; the error is reported, not
# retried.
set -uo pipefail
go test ./internal/world -run '^TestC1_4_2_SameBranch$' -count=1 -timeout 60s -v \
    || { echo "same-branch check failed; see go test output" >&2; exit 1; }
