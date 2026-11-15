#!/usr/bin/env bash
# C1.4.5 A repository with a submodule is refused with one sentence and no
# world is created.
set -uo pipefail
go test ./internal/world -run '^TestC1_4_5_SubmoduleRefusal$' -count=1 -timeout 60s -v \
    || { echo "submodule-refusal check failed; see go test output" >&2; exit 1; }
