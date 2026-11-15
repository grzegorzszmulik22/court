#!/usr/bin/env bash
# C1.1.6 A dummy chunk with a passing suite and a real sabotage: `check`
# prints its `CHECK … PASS` lines and exits 0; `sabotage` prints `0 uncaught`.
#
# D9.1 checks the fixture test/fixtures/dummy_add.sh; its sabotage makes the
# addition wrong, and the suite must catch it.
set -uo pipefail
d=${COURT_CHECK_DIR:-$(mktemp -d)}
mkdir -p "$d"

make --no-print-directory check-D9.1 >"$d/check.log" 2>&1
rc=$?
[ $rc -eq 0 ] || { echo "make check-D9.1 exited $rc" >&2; cat "$d/check.log" >&2; exit 1; }
grep -qx "CHECK C9.1.1 PASS" "$d/check.log" \
    || { echo "check-D9.1 did not print 'CHECK C9.1.1 PASS'" >&2; cat "$d/check.log" >&2; exit 1; }

make --no-print-directory sabotage-D9.1 >"$d/sab.out" 2>"$d/sab.err"
rc=$?
[ $rc -eq 0 ] || { echo "make sabotage-D9.1 exited $rc" >&2; cat "$d/sab.out" "$d/sab.err" >&2; exit 1; }
tail -1 "$d/sab.out" | grep -qx "0 uncaught" \
    || { echo "sabotage-D9.1 did not end its output with '0 uncaught'" >&2; cat "$d/sab.out" >&2; exit 1; }
