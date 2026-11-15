#!/usr/bin/env bash
# C1.1.2 A check that exits non-zero leaves its output in the evidence folder
# and fails the target.
#
# Runs the check runner directly on temporary chunks and asserts, for every
# non-PASS exit path — FAIL, SKIP, BLOCKED, timeout, and a check forging its
# own CHECK line — that the runner exits non-zero while still printing exactly
# one CHECK line naming that check.
set -uo pipefail
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"; rm -rf evidence/X2.2 evidence/X2.3 evidence/X2.4 evidence/X2.5 evidence/X2.6 evidence/X2.7' EXIT

# --- FAIL: output captured, target fails -------------------------------------
mkdir -p "$tmp/checks/X2.2"
cat >"$tmp/checks/X2.2/C2.2.1-always-fails.sh" <<'EOF'
#!/usr/bin/env bash
echo "X2.2 stdout-marker"
echo "X2.2-BOOM stderr-marker" >&2
exit 1
EOF

COURT_CHECKS_DIR="$tmp/checks" tools/check.sh X2.2 >"$tmp/out.log" 2>"$tmp/err.log"
rc=$?
[ $rc -ne 0 ] || { echo "check runner exited 0 for a failing suite" >&2; exit 1; }
[ "$(grep -c '^CHECK ' "$tmp/out.log")" -eq 1 ] \
    || { echo "FAIL run did not print exactly one CHECK line" >&2; cat "$tmp/out.log" >&2; exit 1; }

ev=$(ls -d evidence/X2.2/*/ 2>/dev/null | sort | tail -1)
[ -n "$ev" ] || { echo "failing run left no evidence folder" >&2; exit 1; }

grep -q "X2.2-BOOM stderr-marker" "$ev/checks/C2.2.1.log" \
    || { echo "failing check's stderr not captured in $ev/checks/C2.2.1.log" >&2; exit 1; }
grep -q "X2.2 stdout-marker" "$ev/checks/C2.2.1.log" \
    || { echo "failing check's stdout not captured" >&2; exit 1; }
grep -q "X2.2-BOOM stderr-marker" "$ev/output.log" \
    || { echo "failing check's output not in run output.log" >&2; exit 1; }
grep -q "^CHECK C2.2.1 FAIL " "$ev/output.log" \
    || { echo "CHECK FAIL line missing from output.log" >&2; exit 1; }
grep -q "^CHECK C2.2.1 FAIL " "$tmp/out.log" \
    || { echo "CHECK FAIL line missing from runner stdout" >&2; exit 1; }

python3 - "$ev/result.json" <<'PYEOF' || { echo "result.json does not record the failed run" >&2; exit 1; }
import json, sys
r = json.load(open(sys.argv[1]))
assert r["command"] == "make check-X2.2", r
assert r["exit_code"] != 0, r
assert r["duration_ms"] >= 0, r
assert r["git_commit"], r
PYEOF

# --- SKIP: exit 77 fails the run with one CHECK SKIP line ---------------------
mkdir -p "$tmp/checks/X2.3"
cat >"$tmp/checks/X2.3/C2.3.1-always-skips.sh" <<'EOF'
#!/usr/bin/env bash
echo "X2.3 no frobnicate available" >&2
exit 77
EOF

COURT_CHECKS_DIR="$tmp/checks" tools/check.sh X2.3 >"$tmp/skip.log" 2>/dev/null
rc=$?
[ $rc -ne 0 ] || { echo "check runner exited 0 for a SKIP" >&2; exit 1; }
[ "$(grep -c '^CHECK ' "$tmp/skip.log")" -eq 1 ] \
    || { echo "SKIP run did not print exactly one CHECK line" >&2; cat "$tmp/skip.log" >&2; exit 1; }
grep -q "^CHECK C2.3.1 SKIP " "$tmp/skip.log" \
    || { echo "SKIP not reported as CHECK SKIP" >&2; cat "$tmp/skip.log" >&2; exit 1; }

# --- BLOCKED: exit 78 fails the run with one CHECK BLOCKED line ---------------
mkdir -p "$tmp/checks/X2.4"
cat >"$tmp/checks/X2.4/C2.4.1-always-blocked.sh" <<'EOF'
#!/usr/bin/env bash
echo "X2.4 awaiting human decision" >&2
exit 78
EOF

COURT_CHECKS_DIR="$tmp/checks" tools/check.sh X2.4 >"$tmp/blocked.log" 2>/dev/null
rc=$?
[ $rc -ne 0 ] || { echo "check runner exited 0 for a BLOCKED" >&2; exit 1; }
[ "$(grep -c '^CHECK ' "$tmp/blocked.log")" -eq 1 ] \
    || { echo "BLOCKED run did not print exactly one CHECK line" >&2; cat "$tmp/blocked.log" >&2; exit 1; }
grep -q "^CHECK C2.4.1 BLOCKED " "$tmp/blocked.log" \
    || { echo "BLOCKED not reported as CHECK BLOCKED" >&2; cat "$tmp/blocked.log" >&2; exit 1; }

# --- timeout: a killed check fails the run with one CHECK FAIL line -----------
mkdir -p "$tmp/checks/X2.5"
cat >"$tmp/checks/X2.5/C2.5.1-hangs.sh" <<'EOF'
#!/usr/bin/env bash
sleep 15
EOF

COURT_CHECK_TIMEOUT=2 COURT_CHECKS_DIR="$tmp/checks" tools/check.sh X2.5 >"$tmp/timeout.log" 2>/dev/null
rc=$?
[ $rc -ne 0 ] || { echo "check runner exited 0 for a timed-out check" >&2; exit 1; }
[ "$(grep -c '^CHECK ' "$tmp/timeout.log")" -eq 1 ] \
    || { echo "timeout run did not print exactly one CHECK line" >&2; cat "$tmp/timeout.log" >&2; exit 1; }
grep -q "^CHECK C2.5.1 FAIL .*timeout" "$tmp/timeout.log" \
    || { echo "timed-out check not reported as CHECK FAIL with a timeout reason" >&2; cat "$tmp/timeout.log" >&2; exit 1; }

# --- forged line: a check may not emit its own CHECK line ---------------------
mkdir -p "$tmp/checks/X2.6"
cat >"$tmp/checks/X2.6/C2.6.1-forges-result.sh" <<'EOF'
#!/usr/bin/env bash
echo "CHECK C2.6.1 PASS"
exit 0
EOF

COURT_CHECKS_DIR="$tmp/checks" tools/check.sh X2.6 >"$tmp/forged.log" 2>/dev/null
rc=$?
[ $rc -ne 0 ] || { echo "check runner exited 0 for a check forging its own CHECK line" >&2; exit 1; }
[ "$(grep -c '^CHECK ' "$tmp/forged.log")" -eq 1 ] \
    || { echo "forged-line run did not print exactly one CHECK line" >&2; cat "$tmp/forged.log" >&2; exit 1; }
grep -q "^CHECK C2.6.1 FAIL " "$tmp/forged.log" \
    || { echo "self-reported CHECK line not rejected as FAIL" >&2; cat "$tmp/forged.log" >&2; exit 1; }

# --- missing or empty checks dir is an error, not a pass ----------------------
tools/check.sh X9.9 >"$tmp/nodir.log" 2>/dev/null && \
    { echo "check runner exited 0 for a missing checks dir" >&2; exit 1; }
[ "$(grep -c '^CHECK ' "$tmp/nodir.log")" -eq 0 ] \
    || { echo "missing-dir run printed a CHECK line" >&2; cat "$tmp/nodir.log" >&2; exit 1; }

mkdir -p "$tmp/checks/X2.7"
COURT_CHECKS_DIR="$tmp/checks" tools/check.sh X2.7 >"$tmp/empty.log" 2>/dev/null && \
    { echo "check runner exited 0 for an empty checks dir" >&2; exit 1; }
[ "$(grep -c '^CHECK ' "$tmp/empty.log")" -eq 0 ] \
    || { echo "empty-dir run printed a CHECK line" >&2; cat "$tmp/empty.log" >&2; exit 1; }

# --- --exclude removes a check's line entirely --------------------------------
COURT_CHECKS_DIR="$tmp/checks" tools/check.sh X2.3 --exclude C2.3.1 >"$tmp/excl.log" 2>/dev/null
rc=$?
[ $rc -ne 0 ] || { echo "check runner exited 0 with every check excluded" >&2; exit 1; }
[ "$(grep -c '^CHECK ' "$tmp/excl.log")" -eq 0 ] \
    || { echo "an excluded check still printed a CHECK line" >&2; cat "$tmp/excl.log" >&2; exit 1; }
