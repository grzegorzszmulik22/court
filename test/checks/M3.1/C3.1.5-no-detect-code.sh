#!/usr/bin/env bash
# C3.1.5 M3.1 (H11) — no file under internal/detect/ or internal/reconcile/
# exists at the freeze commit: ground truth lands before implementation.
set -uo pipefail

freeze=evidence/M3.1/freeze.txt
[ -f "$freeze" ] || { echo "missing $freeze" >&2; exit 1; }
recorded=$(grep -oE '^[0-9a-f]{40}' "$freeze" | head -1)
[ -n "$recorded" ] || { echo "$freeze records no commit id" >&2; exit 1; }
git merge-base --is-ancestor "$recorded" HEAD 2>/dev/null \
    || { echo "recorded freeze commit $recorded is not an ancestor of HEAD" >&2; exit 1; }

bad=$(git ls-tree -r --name-only "$recorded" -- internal/detect internal/reconcile)
if [ -n "$bad" ]; then
    echo "at freeze commit $recorded, detection/reconciliation code already existed:" >&2
    printf '%s\n' "$bad" >&2
    exit 1
fi
exit 0
