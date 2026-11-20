#!/usr/bin/env bash
# C3.1.5 M3.1 (H11) — at the freeze commit there is no code under
# internal/detect/ or internal/reconcile/: ground truth lands before
# implementation.
set -uo pipefail
for d in internal/detect internal/reconcile; do
    if [ -d "$d" ]; then
        echo "check failed: $d exists before its milestone" >&2
        exit 1
    fi
done
exit 0
