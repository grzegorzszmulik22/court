#!/usr/bin/env bash
# C3.1.1 M3.1 — corpus counts meet the M3.1 minimums; every planted
# scenario names a twin and the twin names it back. Also runs the
# sabotage-fixture legs (missing twin, broken back-ref, empty registry in
# a planted pair) plus the rule-level fixtures for the gate, contest
# raisability, net-diff planted-only, CR-003 guarantee twins, the
# normalised five-mutation cap and every count/coverage boundary, so a
# weakened validator cannot pass unnoticed.
set -uo pipefail
go test ./bench/docketbench/validate -count=1 -timeout 30m -v \
    -run '^(TestC3_1_1.*|TestC3_1_planted_empty_registry|TestGate_.*|TestContestRaisable_.*|TestPlantedOnly_.*|TestPlanted_.*|TestTwinEdits_.*|TestCoverageCount_.*|TestMutationCap_.*|TestReconcile_.*|TestVectors_.*|TestRootWrit_.*|TestEvidenceLines_.*|TestEdit_.*|TestSpawnBudget_.*|TestWorldDeclare_.*)$' \
    || { echo "check failed; see go test output" >&2; exit 1; }
