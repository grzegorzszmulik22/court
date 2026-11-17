#!/usr/bin/env bash
# C3.1.6 M3.1 — subset A covers every detector-backed category in at
# least 4 planted scenarios and every variant of spec/04 §5 in at least
# 8; the fixture leg proves the coverage counters fire.
set -uo pipefail
go test ./bench/docketbench/validate -count=1 -timeout 300s -v \
    -run '^TestC3_1_6$' \
    || { echo "check failed; see go test output" >&2; exit 1; }
