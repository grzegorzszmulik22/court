#!/usr/bin/env bash
# C3.1.2 M3.1 — every scenario's repository test suite passes with the
# script applied (subsets A and C), run by the validator. The fixture leg
# proves the rule fires on a planted failing suite.
set -uo pipefail
go test ./bench/docketbench/validate -count=1 -timeout 300s -v \
    -run '^(TestC3_1_2|TestC3_1_2_failing_repo)$' \
    || { echo "check failed; see go test output" >&2; exit 1; }
