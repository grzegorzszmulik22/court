#!/usr/bin/env bash
# C3.1.1 M3.1 — corpus counts meet the M3.1 minimums; every planted
# scenario names a twin and the twin names it back. Also runs the
# sabotage-fixture legs (missing twin, broken back-ref, empty registry in
# a planted pair) so a weakened validator cannot pass unnoticed.
set -uo pipefail
go test ./bench/docketbench/validate -count=1 -timeout 30m -v \
    -run '^(TestC3_1_1|TestC3_1_1_missing_twin|TestC3_1_1_broken_backref|TestC3_1_1_vectors_counts|TestC3_1_1_reconcile_counts|TestC3_1_planted_empty_registry)$' \
    || { echo "check failed; see go test output" >&2; exit 1; }
