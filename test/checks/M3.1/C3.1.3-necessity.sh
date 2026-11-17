#!/usr/bin/env bash
# C3.1.3 M3.1 — necessity: no registry keyword may appear in the scripted
# agent's final message, the diff --stat, or a commit message. The corpus
# run in C3.1.1 covers the real data; this leg proves the rule fires.
set -uo pipefail
go test ./bench/docketbench/validate -count=1 -timeout 300s -v \
    -run '^TestC3_1_3$' \
    || { echo "check failed; see go test output" >&2; exit 1; }
