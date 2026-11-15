#!/usr/bin/env bash
# C1.4.4 `git config --list` for the repository is byte-identical before a
# commission and after it. Per-world settings live in the environment
# (GIT_CONFIG_*), never in repository config.
set -uo pipefail
go test ./internal/world -run '^TestC1_4_4_ConfigIdentical$' -count=1 -timeout 60s -v \
    || { echo "config-identical check failed; see go test output" >&2; exit 1; }
