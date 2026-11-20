#!/usr/bin/env bash
# sabotage.sh <chunk>
#
# Applies each patch under test/sabotage/<chunk>/ (or $COURT_SABOTAGE_DIR/<chunk>)
# to a scratch copy of the working tree (never the working tree itself), runs
# `make check-<chunk>` in the copy, and classifies the result:
#
#   SABOTAGE <name> caught by <check id ...>   >=1 check printed a non-PASS line
#   UNCAUGHT test/sabotage/<chunk>/<name>.patch inner run passed; suite stayed green
#   SABOTAGE <name> ERROR <why>                patch did not apply, scratch copy
#                                              failed, or the inner run failed
#                                              without any non-PASS CHECK line
#
# An ERROR is never a catch: the run exits non-zero unless every patch applied
# cleanly AND produced at least one non-PASS CHECK line. Output ends with one
# line `<n> uncaught` (uncaught only; errors are named above it).
#
# The scratch copy is the tracked plus untracked-nonignored working tree
# (minus evidence/ and .tools, which is symlinked), committed to a fresh git
# repo so checks that need git still work. The inner run gets a clean check
# environment: COURT_CHECKS_DIR and COURT_CHECK_EXCLUDE are unset so no check
# can be excluded from a sabotage evaluation.
set -uo pipefail
cd "$(dirname "$0")/.."

chunk=${1:?usage: sabotage.sh <chunk>}
sabdir=${COURT_SABOTAGE_DIR:-test/sabotage}/$chunk
if [ ! -d "$sabdir" ]; then
    echo "sabotage.sh: no sabotages for chunk $chunk ($sabdir missing)" >&2
    exit 2
fi
if ! ls "$sabdir"/*.patch >/dev/null 2>&1; then
    echo "sabotage.sh: no sabotage patches in $sabdir" >&2
    exit 2
fi

ts=$(date -u +%Y-%m-%dT%H-%M-%SZ)
ev=evidence/$chunk/$ts-sabotage
n=0
while [ -e "$ev" ]; do n=$((n+1)); ev=evidence/$chunk/$ts-sabotage.$n; done
mkdir -p "$ev/patches"
commit=$(git rev-parse HEAD 2>/dev/null || echo unknown)
orig=$PWD
: >"$ev/output.log"

report() { echo "$*" | tee -a "$ev/output.log"; }

# The inner suite runs under a hang guard; Windows needs longer because the
# scratch copy and its checks are slowed by filesystem scanning. Linux keeps
# its original 600 s.
case "$(uname -s)" in
    MINGW*|MSYS*|CYGWIN*) default_timeout=1800 ;;
    *) default_timeout=600 ;;
esac

start=$(date +%s%N)
uncaught=0; errors=0
for p in "$sabdir"/*.patch; do
    [ -f "$p" ] || continue
    name=$(basename "$p" .patch)
    plog="$ev/patches/$name.log"
    tmp=$(mktemp -d); scratch=$tmp/src; mkdir -p "$scratch"

    if ! git ls-files -co --exclude-standard -z \
            | grep -zv -e '^evidence/' -e '^\.lane/' \
            | tar --null -T - -cf - \
            | tar -xf - -C "$scratch" 2>"$plog"; then
        report "SABOTAGE $name ERROR scratch copy failed"
        errors=$((errors+1)); rm -rf "$tmp"; continue
    fi
    # checks that read git history (a check asserting freeze.txt records
    # the commit that last wrote FROZEN.sha256) need the real objects and
    # refs, not a single-commit repo — copy .git so the initialisation
    # below adds its "scratch" commit on top of the true history, and
    # restore the small tracked pointer files evidence/ keeps out.
    if ! cp -a .git "$scratch/.git" 2>>"$plog"; then
        report "SABOTAGE $name ERROR scratch git copy failed"
        errors=$((errors+1)); rm -rf "$tmp"; continue
    fi
    git ls-files | grep -E '^evidence/[^/]+/freeze\.txt$' | while IFS= read -r f; do
        mkdir -p "$scratch/$(dirname "$f")"
        cp "$f" "$scratch/$f"
    done
    (
        cd "$scratch" || exit 1
        git init -q && git add -A \
            && git -c user.email=court@local -c user.name=court commit -qm scratch >/dev/null \
            && ln -sfn "$orig/.tools" .tools
    ) >>"$plog" 2>&1 || { report "SABOTAGE $name ERROR git init failed"; errors=$((errors+1)); rm -rf "$tmp"; continue; }

    # "Applied cleanly" means git apply accepts the whole patch; fuzzed or
    # partial application is an error, never a catch.
    case $p in /*) patch_abs=$p ;; *) patch_abs=$orig/$p ;; esac
    if ! (cd "$scratch" && git apply --check "$patch_abs") >>"$plog" 2>&1 \
        || ! (cd "$scratch" && git apply "$patch_abs") >>"$plog" 2>&1; then
        report "SABOTAGE $name ERROR patch does not apply"
        errors=$((errors+1)); rm -rf "$tmp"; continue
    fi

    (cd "$scratch" && env -u COURT_CHECKS_DIR -u COURT_SABOTAGE_DIR -u COURT_CHECK_EXCLUDE \
        COURT_GIT_COMMIT="$commit" \
        COURT_TOOLS_DIR="$orig/.tools" \
        PATH="$orig/.tools/bin:$PATH" \
        timeout "${COURT_SABOTAGE_TIMEOUT:-$default_timeout}" \
        make --no-print-directory "check-$chunk") >>"$plog" 2>&1
    rc=$?
    caught=$(grep -oE '^CHECK [^ ]+ +(FAIL|BLOCKED|SKIP)' "$plog" | awk '{print $2}' | sort -u | tr '\n' ',' | sed 's/,$//')
    if [ -n "$caught" ]; then
        report "SABOTAGE $name caught by $caught"
    elif [ $rc -eq 0 ]; then
        uncaught=$((uncaught+1))
        report "UNCAUGHT $p"
    else
        errors=$((errors+1))
        report "SABOTAGE $name ERROR inner check run exited $rc without a non-PASS CHECK line"
    fi
    cat "$plog" >>"$ev/output.log"
    rm -rf "$tmp"
done

echo "$uncaught uncaught"
echo "$uncaught uncaught" >>"$ev/output.log"
[ $errors -gt 0 ] && echo "sabotage.sh: $errors patch(es) errored" | tee -a "$ev/output.log" >&2
rc_all=1
[ $uncaught -eq 0 ] && [ $errors -eq 0 ] && rc_all=0
dur=$(( ($(date +%s%N) - start) / 1000000 ))
cat >"$ev/result.json" <<EOF
{"command":"make sabotage-$chunk","chunk":"$chunk","exit_code":$rc_all,"duration_ms":$dur,"git_commit":"$commit","started_at":"${ts}Z"}
EOF
echo "sabotage.sh: evidence: $ev" >&2
exit $rc_all
