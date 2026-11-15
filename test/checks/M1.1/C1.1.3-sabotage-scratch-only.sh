#!/usr/bin/env bash
# C1.1.3 `make sabotage-M1.1` applies a patch to a scratch copy, never to the
# working tree (the tree's hash is unchanged afterwards).
#
# Runs `make sabotage-D9.1` (the dummy chunk with a real, caught sabotage) and
# compares a digest of the working tree before and after.
set -uo pipefail
d=${COURT_CHECK_DIR:-$(mktemp -d)}
mkdir -p "$d"

before=$(tools/treehash.sh)
make --no-print-directory sabotage-D9.1 >"$d/sab.out" 2>"$d/sab.err"
rc=$?
after=$(tools/treehash.sh)

[ "$before" = "$after" ] || { echo "working tree hash changed by a sabotage run" >&2; exit 1; }
grep -qF 'echo "$(( $1 + $2 ))"' test/fixtures/dummy_add.sh \
    || { echo "dummy fixture was modified" >&2; exit 1; }

# The spec-named sabotage patches must be present and non-empty; a sabotage
# that deletes them must be caught here, not by a silent 0-uncaught run.
for s in evidence-capture-drops-stderr frozen-check-skips-missing-file sabotage-runner-always-green; do
    [ -s "test/sabotage/M1.1/$s.patch" ] \
        || { echo "required sabotage test/sabotage/M1.1/$s.patch missing or empty" >&2; exit 1; }
done
[ "$(ls test/sabotage/M1.1/*.patch 2>/dev/null | wc -l)" -ge 3 ] \
    || { echo "fewer than 3 sabotage patches under test/sabotage/M1.1" >&2; exit 1; }
[ $rc -eq 0 ] || { echo "make sabotage-D9.1 unexpectedly failed" >&2; cat "$d/sab.out" "$d/sab.err" >&2; exit 1; }
tail -1 "$d/sab.out" | grep -qx "0 uncaught" \
    || { echo "sabotage-D9.1 did not end its output with '0 uncaught'" >&2; cat "$d/sab.out" >&2; exit 1; }
