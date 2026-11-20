#!/usr/bin/env bash
# C3.1.3 M3.1 — necessity: no registry keyword may appear in the scripted
# agent's final message, the diff --stat, a commit message, or the
# commission goal. The corpus run in C3.1.1 covers the real data; the
# fixture legs prove each scanned surface fires, case-insensitively.
set -uo pipefail
go test ./bench/docketbench/validate -count=1 -timeout 30m -v \
    -run '^TestC3_1_3.*$' \
    || { echo "check failed; see go test output" >&2; exit 1; }
