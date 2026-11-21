#!/usr/bin/env bash
# C3.1.5 M3.1 (H11) — at the freeze commit there is no code under
# internal/detect/ or internal/reconcile/: ground truth lands before
# implementation.
set -uo pipefail

freeze=evidence/M3.1/freeze.txt
[ -f "$freeze" ] || { echo "missing $freeze" >&2; exit 1; }
recorded=$(grep -oE '^[0-9a-f]{40}' "$freeze" | head -1)
[ -n "$recorded" ] || { echo "$freeze records no commit id" >&2; exit 1; }
git merge-base --is-ancestor "$recorded" HEAD 2>/dev/null \
    || { echo "recorded freeze commit $recorded is not an ancestor of HEAD" >&2; exit 1; }

at_freeze=$(git ls-tree -r --name-only "$recorded" -- internal/detect internal/reconcile)
if [ -n "$at_freeze" ]; then
    echo "check failed: files existed at freeze commit $recorded:" >&2
    printf '%s\n' "$at_freeze" >&2
    exit 1
fi

prior=$(git log -1 --format=%H "$recorded" -- internal/detect internal/reconcile)
if [ -n "$prior" ]; then
    echo "check failed: $prior touched internal/detect or internal/reconcile at or before freeze $recorded" >&2
    exit 1
fi
exit 0
