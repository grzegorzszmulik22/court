#!/usr/bin/env bash
# Dummy chunk (spec C1.1.7): a suite that ignores what its sabotage breaks —
# it only asserts a trivial, unrelated property.
set -uo pipefail
[ -f Makefile ] || { echo "Makefile missing" >&2; exit 1; }
