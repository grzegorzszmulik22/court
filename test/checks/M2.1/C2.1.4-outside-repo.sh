#!/usr/bin/env bash
# C2.1.4 Outside a git repository the command prints one sentence and
# exits non-zero.
set -uo pipefail
d=${COURT_CHECK_DIR:?}
case "$d" in /*) ;; *) d="$PWD/$d" ;; esac
bin=$(bash test/checks/court-bin.sh "$d") || exit 1
go build -o "$bin" ./cmd/court || { echo "court build failed" >&2; exit 1; }
# The check dir lives inside the checkout, so the not-a-repository
# fixture must sit outside it entirely.
norepo=$(mktemp -d "${TMPDIR:-/tmp}/court-norepo.XXXXXX")
out=$(cd "$norepo" && "$d/court" commission "x" 2>&1)
rc=$?
if [ $rc -eq 0 ]; then
    echo "commission outside a repo exited 0" >&2
    exit 1
fi
lines=$(printf '%s\n' "$out" | grep -c .)
if [ "$lines" -ne 1 ]; then
    echo "expected one output line, got $lines: $out" >&2
    exit 1
fi
case "$out" in
    *git*repository*|*git*repo*) ;;
    *) echo "the one sentence does not name the repository: $out" >&2; exit 1 ;;
esac
