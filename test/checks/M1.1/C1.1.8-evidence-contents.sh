#!/usr/bin/env bash
# C1.1.8 The evidence folder of a run contains the command line, exit code,
# duration, git commit and full output.
set -uo pipefail
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"; rm -rf evidence/X8.8' EXIT

mkdir -p "$tmp/checks/X8.8"
cat >"$tmp/checks/X8.8/C8.8.1-two-streams.sh" <<'EOF'
#!/usr/bin/env bash
echo "X8.8-stdout-marker"
echo "X8.8-stderr-marker" >&2
exit 0
EOF

COURT_CHECKS_DIR="$tmp/checks" tools/check.sh X8.8 >"$tmp/out.log" 2>"$tmp/err.log"
rc=$?
[ $rc -eq 0 ] || { echo "check run on a passing chunk exited $rc" >&2; exit 1; }

ev=$(ls -d evidence/X8.8/*/ 2>/dev/null | sort | tail -1)
[ -n "$ev" ] || { echo "no evidence folder written" >&2; exit 1; }

python3 - "$ev/result.json" <<'PYEOF' || { echo "result.json is missing required fields" >&2; exit 1; }
import json, os, subprocess, sys
r = json.load(open(sys.argv[1]))
assert r["command"] == "make check-X8.8", r
assert r["exit_code"] == 0, r
assert isinstance(r["duration_ms"], int) and r["duration_ms"] >= 0, r
# The runner records COURT_GIT_COMMIT when set (sabotage runs pin the outer
# commit), else HEAD — mirror that precedence here.
want = os.environ.get("COURT_GIT_COMMIT") or subprocess.run(
    ["git", "rev-parse", "HEAD"], capture_output=True, text=True).stdout.strip()
assert r["git_commit"] == want, (r["git_commit"], want)
PYEOF

grep -qx "CHECK C8.8.1 PASS" "$ev/output.log" \
    || { echo "output.log lacks the CHECK line" >&2; exit 1; }
grep -q "X8.8-stdout-marker" "$ev/checks/C8.8.1.log" \
    || { echo "check stdout missing from evidence" >&2; exit 1; }
grep -q "X8.8-stderr-marker" "$ev/checks/C8.8.1.log" \
    || { echo "check stderr missing from evidence" >&2; exit 1; }
