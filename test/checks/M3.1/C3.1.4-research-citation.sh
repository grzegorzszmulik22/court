#!/usr/bin/env bash
# C3.1.4 M3.1 — every registry entry cites a row of
# research/1_detection.md in its source field.
set -uo pipefail
go test ./bench/docketbench/validate -count=1 -timeout 30m -v \
    -run '^TestC3_1_4$' \
    || { echo "check failed; see go test output" >&2; exit 1; }
