#!/usr/bin/env bash
# C1.1.7 A dummy chunk whose suite ignores the sabotage: `sabotage` exits
# non-zero, prints `1 uncaught` and names the patch.
#
# Also asserts the sabotage runner's error contract: a patch that does not
# apply, and an inner run that fails without any non-PASS CHECK line, are
# ERRORs named by patch — never catches and never silently skipped.
set -uo pipefail
d=${COURT_CHECK_DIR:-$(mktemp -d)}
mkdir -p "$d"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"; rm -rf evidence/X9.9' EXIT

make --no-print-directory sabotage-D9.2 >"$d/sab.log" 2>&1
rc=$?
[ $rc -ne 0 ] || { echo "make sabotage-D9.2 exited 0 for an uncaught sabotage" >&2; cat "$d/sab.log" >&2; exit 1; }
grep -qx "1 uncaught" "$d/sab.log" \
    || { echo "sabotage-D9.2 did not print '1 uncaught'" >&2; cat "$d/sab.log" >&2; exit 1; }
grep -q "UNCAUGHT test/sabotage/D9.2/breaks-dummy-add.patch" "$d/sab.log" \
    || { echo "sabotage-D9.2 did not name the uncaught patch" >&2; cat "$d/sab.log" >&2; exit 1; }

# --- a patch that does not apply is an ERROR, never a catch -------------------
mkdir -p "$tmp/sab-stale/D9.1"
cat >"$tmp/sab-stale/D9.1/stale-context.patch" <<'EOF'
--- a/test/fixtures/dummy_add.sh
+++ b/test/fixtures/dummy_add.sh
@@ -1,3 +1,3 @@
-#!/usr/bin/env zsh
-# Fixture "implementation" for the dummy chunk D9.1 (spec C1.1.6/C1.1.7).
-echo "$(( $1 + $2 ))"
+#!/usr/bin/env bash
EOF

COURT_SABOTAGE_DIR="$tmp/sab-stale" tools/sabotage.sh D9.1 >"$tmp/noapply.log" 2>&1
rc=$?
[ $rc -ne 0 ] \
    || { echo "sabotage runner exited 0 for a patch that does not apply" >&2; cat "$tmp/noapply.log" >&2; exit 1; }
grep -q "SABOTAGE stale-context ERROR patch does not apply" "$tmp/noapply.log" \
    || { echo "non-applying patch not reported as an ERROR naming it" >&2; cat "$tmp/noapply.log" >&2; exit 1; }
grep -q "SABOTAGE stale-context caught" "$tmp/noapply.log" \
    && { echo "non-applying patch was reported as caught" >&2; cat "$tmp/noapply.log" >&2; exit 1; }
grep -q "UNCAUGHT .*stale-context" "$tmp/noapply.log" \
    && { echo "non-applying patch was reported as uncaught" >&2; cat "$tmp/noapply.log" >&2; exit 1; }

# --- an inner run with no CHECK lines is an ERROR, never a catch --------------
mkdir -p "$tmp/sab-nolines/X9.9"
cat >"$tmp/sab-nolines/X9.9/noop-marker.patch" <<'EOF'
--- a/test/fixtures/dummy_add.sh
+++ b/test/fixtures/dummy_add.sh
@@ -1,3 +1,4 @@
 #!/usr/bin/env bash
+# sabotage-noop marker
 # Fixture "implementation" for the dummy chunk D9.1 (spec C1.1.6/C1.1.7).
 echo "$(( $1 + $2 ))"
EOF

COURT_SABOTAGE_DIR="$tmp/sab-nolines" tools/sabotage.sh X9.9 >"$tmp/nolines.log" 2>&1
rc=$?
[ $rc -ne 0 ] \
    || { echo "sabotage runner exited 0 for an inner run with no CHECK lines" >&2; cat "$tmp/nolines.log" >&2; exit 1; }
grep -q "SABOTAGE noop-marker ERROR inner check run" "$tmp/nolines.log" \
    || { echo "inner run without CHECK lines not reported as an ERROR naming the patch" >&2; cat "$tmp/nolines.log" >&2; exit 1; }
grep -q "SABOTAGE noop-marker caught" "$tmp/nolines.log" \
    && { echo "inner run without CHECK lines was reported as caught" >&2; cat "$tmp/nolines.log" >&2; exit 1; }

# --- no sabotage patches at all is an error, not a green run ------------------
mkdir -p "$tmp/sab-empty/X9.9"
COURT_SABOTAGE_DIR="$tmp/sab-empty" tools/sabotage.sh X9.9 >"$tmp/empty.log" 2>&1
rc=$?
[ $rc -ne 0 ] \
    || { echo "sabotage runner exited 0 for an empty sabotage dir" >&2; exit 1; }

# --- the inner evaluation ignores COURT_CHECK_EXCLUDE/COURT_CHECKS_DIR --------
# If either leaked in, the D9.1 sabotage would go unevaluated or be run against
# substitute checks and report UNCAUGHT instead of caught.
COURT_CHECK_EXCLUDE=C9.1.1 tools/sabotage.sh D9.1 >"$tmp/excl.out" 2>"$tmp/excl.err"
rc=$?
[ $rc -eq 0 ] || { echo "sabotage-D9.1 failed under COURT_CHECK_EXCLUDE (leaked into inner run)" >&2; cat "$tmp/excl.out" "$tmp/excl.err" >&2; exit 1; }
tail -1 "$tmp/excl.out" | grep -qx "0 uncaught" \
    || { echo "sabotage-D9.1 under COURT_CHECK_EXCLUDE did not report 0 uncaught" >&2; cat "$tmp/excl.out" >&2; exit 1; }

mkdir -p "$tmp/checks-green/D9.1"
cat >"$tmp/checks-green/D9.1/C9.1.1-always-pass.sh" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
COURT_CHECKS_DIR="$tmp/checks-green" tools/sabotage.sh D9.1 >"$tmp/subdir.out" 2>"$tmp/subdir.err"
rc=$?
[ $rc -eq 0 ] \
    || { echo "sabotage-D9.1 failed under COURT_CHECKS_DIR (leaked into inner run)" >&2; cat "$tmp/subdir.out" "$tmp/subdir.err" >&2; exit 1; }
tail -1 "$tmp/subdir.out" | grep -qx "0 uncaught" \
    || { echo "sabotage-D9.1 under COURT_CHECKS_DIR did not report 0 uncaught" >&2; cat "$tmp/subdir.out" >&2; exit 1; }
