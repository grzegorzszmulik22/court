#!/usr/bin/env bash
# C3.1.7 freeze integrity — spec/milestones/M3-detection-and-reconciliation.md
# gate: "spec/FROZEN.sha256 updated with all of the above in one commit,
# whose id is recorded in evidence/M3.1/freeze.txt". The commit that last
# touched FROZEN.sha256 must BE the recorded one; a later rehash with no
# matching freeze.txt update leaves the metadata stale.
set -uo pipefail

freeze=evidence/M3.1/freeze.txt
[ -f spec/FROZEN.sha256 ] || { echo "spec/FROZEN.sha256 missing" >&2; exit 1; }
[ -f "$freeze" ] || { echo "missing $freeze" >&2; exit 1; }

recorded=$(grep -oE '^[0-9a-f]{40}' "$freeze" | head -1)
[ -n "$recorded" ] || { echo "$freeze records no commit id" >&2; exit 1; }
git merge-base --is-ancestor "$recorded" HEAD 2>/dev/null \
    || { echo "recorded freeze commit $recorded is not an ancestor of HEAD" >&2; exit 1; }

last=$(git log -1 --format=%H -- spec/FROZEN.sha256)
if [ "$last" != "$recorded" ]; then
    echo "spec/FROZEN.sha256 last changed in $last; $freeze records $recorded" >&2
    exit 1
fi
git diff --quiet HEAD -- spec/FROZEN.sha256 \
    || { echo "spec/FROZEN.sha256 has uncommitted changes after the recorded freeze" >&2; exit 1; }
