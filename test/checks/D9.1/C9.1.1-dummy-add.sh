#!/usr/bin/env bash
# Dummy chunk (spec C1.1.6): a passing suite over the "implementation"
# test/fixtures/dummy_add.sh.
set -uo pipefail
got=$(test/fixtures/dummy_add.sh 2 2) || { echo "dummy_add failed to run" >&2; exit 1; }
[ "$got" = "4" ] || { echo "dummy_add 2 2 = $got, want 4" >&2; exit 1; }
