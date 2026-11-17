#!/usr/bin/env bash
# check.sh <chunk> [--exclude <id>[,<id>...]]
#
# Runs every check file under test/checks/<chunk>/ (or $COURT_CHECKS_DIR/<chunk>),
# in name order. The check id is the file name up to the first "-" or ".sh".
#
# Output convention (relied on by the supervisor's verifier):
#   stdout carries exactly one line per check:
#     CHECK <id> PASS | CHECK <id> FAIL <reason> | CHECK <id> BLOCKED <reason>
#     | CHECK <id> SKIP <reason>
#   Exit status is 0 only when every check passed; SKIP, BLOCKED, FAIL and
#   timeouts all make the run fail.
#
# Check file contract:
#   exit 0   -> PASS
#   exit 77  -> SKIP   (first stderr line is the reason)
#   exit 78  -> BLOCKED (first stderr line is the reason)
#   other    -> FAIL   (first stderr line is the reason, else "exit <rc>")
#   A check runs under `timeout` ($COURT_CHECK_TIMEOUT seconds, default 240;
#   900 on Windows, where scratch-tree I/O is slower);
#   a kill is a FAIL. A check's own scratch area is $COURT_CHECK_DIR; its
#   output is captured to checks/<id>.log in the run's evidence folder.
#   A check must never print a `CHECK <its own id> ...` line itself: the
#   runner owns the CHECK namespace, and a self-reported line is a protocol
#   violation reported as FAIL.
#
# Every run is captured under evidence/<chunk>/<UTC timestamp>/ with
# output.log (the full run output) and result.json (command line, exit code,
# duration, git commit).
set -uo pipefail
cd "$(dirname "$0")/.."

chunk=${1:?usage: check.sh <chunk> [--exclude <id>[,<id>...]]}
shift || true
exclude=" ${COURT_CHECK_EXCLUDE:-} "
exclude=" ${exclude//,/ } "
while [ $# -gt 0 ]; do
    case "$1" in
        --exclude) exclude="$exclude ${2//,/ } "; shift 2 ;;
        *) echo "check.sh: unknown argument $1" >&2; exit 2 ;;
    esac
done

dir=${COURT_CHECKS_DIR:-test/checks}/$chunk
if [ ! -d "$dir" ]; then
    echo "check.sh: no checks for chunk $chunk ($dir missing)" >&2
    exit 2
fi

# --- evidence folder ----------------------------------------------------------
ts=$(date -u +%Y-%m-%dT%H-%M-%SZ)
ev=evidence/$chunk/$ts
n=0
while [ -e "$ev" ]; do n=$((n+1)); ev=evidence/$chunk/$ts.$n; done
mkdir -p "$ev/checks"
export COURT_EVIDENCE_DIR="$PWD/$ev"
commit=${COURT_GIT_COMMIT:-$(git rev-parse HEAD 2>/dev/null || echo unknown)}
export COURT_CHECK_DEPTH=$(( ${COURT_CHECK_DEPTH:-0} + 1 ))
: >"$ev/output.log"

say() { echo "check.sh: $*" | tee -a "$ev/output.log" >&2; }

# A per-check hang guard, not a spec threshold. Windows I/O (Defender scanning
# the scratch trees C1.1.3/C1.1.7 create) makes a fresh-clone suite take
# minutes, so the guard is longer there; Linux keeps its original 240 s.
case "$(uname -s)" in
    MINGW*|MSYS*|CYGWIN*) default_timeout=900 ;;
    *) default_timeout=240 ;;
esac

start=$(date +%s%N)
ran=0; failed=0
for f in "$dir"/*; do
    [ -f "$f" ] || continue
    base=$(basename "$f")
    id=${base%.sh}; id=${id%%-*}
    case "$exclude" in *" $id "*) continue ;; esac
    ran=$((ran+1))
    d="$ev/checks/$id.d"; mkdir -p "$d"
    log="$ev/checks/$id.log"; err="$d/stderr.log"
    COURT_CHECK_DIR="$d" timeout "${COURT_CHECK_TIMEOUT:-$default_timeout}" bash "$f" >"$log" 2>"$err"
    rc=$?
    cat "$err" >>"$log"          # the check's log holds stdout then stderr
    cat "$log" >>"$ev/output.log"
    reason=$(head -1 "$err" 2>/dev/null | tr '\n' ' ' | cut -c1-200)
    [ "$rc" -eq 124 ] && reason="timeout after ${COURT_CHECK_TIMEOUT:-$default_timeout}s"
    # A check reporting its own CHECK line forges the result namespace; only
    # the runner may emit `CHECK <id>` lines for this run.
    forged=$(grep -cE "^CHECK ${id//./\\.}(\.[^ ]+)?( |$)" "$log" || true)
    if [ "$forged" -gt 0 ]; then
        line="CHECK $id FAIL check emitted its own CHECK line"
        failed=$((failed+1))
    else
        case $rc in
            0)  line="CHECK $id PASS" ;;
            77) line="CHECK $id SKIP ${reason:-skipped}";    failed=$((failed+1)) ;;
            78) line="CHECK $id BLOCKED ${reason:-blocked}"; failed=$((failed+1)) ;;
            *)  line="CHECK $id FAIL ${reason:-exit $rc}";   failed=$((failed+1)) ;;
        esac
    fi
    echo "$line"
    echo "$line" >>"$ev/output.log"
done
[ "$ran" -eq 0 ] && say "no check files in $dir"
rc_all=1
[ "$ran" -gt 0 ] && [ "$failed" -eq 0 ] && rc_all=0
dur=$(( ($(date +%s%N) - start) / 1000000 ))
cat >"$ev/result.json" <<EOF
{"command":"make check-$chunk","chunk":"$chunk","exit_code":$rc_all,"duration_ms":$dur,"git_commit":"$commit","started_at":"${ts}Z"}
EOF
say "evidence: $ev"
exit $rc_all
