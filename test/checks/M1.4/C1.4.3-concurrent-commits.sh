#!/usr/bin/env bash
# C1.4.3 Eight worlds committing concurrently for 60 seconds: no lost
# commit, no ref-lock failure surfaced to an agent (retries are internal and
# counted). Kept to the spec duration on this shared two-core host.
set -uo pipefail
go test ./internal/world -run '^TestC1_4_3_ConcurrentCommits$' -count=1 -timeout 200s -v \
    || { echo "concurrent-commits check failed; see go test output" >&2; exit 1; }
