#!/usr/bin/env bash
# C2.1.2 MM2.3 (commission -> first agent step <= 3s, >=5 runs) and
# MM2.4 (at most one keystroke; non-terminal stdin needs zero).
set -uo pipefail
d=${COURT_CHECK_DIR:?}
case "$d" in /*) ;; *) d="$PWD/$d" ;; esac
bin=$(bash test/checks/court-bin.sh "$d") || exit 1
go build -o "$bin" ./cmd/court || { echo "court build failed" >&2; exit 1; }
export COURT_BIN="$bin"
export COURT_CHECK_DIR="$d"
COURT_SYSINFO="$(tools/sysinfo.sh 2>/dev/null || true)"
export COURT_SYSINFO
go test ./internal/commission -count=1 -timeout 240s -v -run '^TestC2_1_2$' \
    || { echo "timing check failed; see go test output" >&2; exit 1; }
[ -f "$d/measurements.json" ] || { echo "no measurements.json produced" >&2; exit 1; }
