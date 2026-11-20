#!/usr/bin/env bash
# C3.2.7 (extra, never a substitute for C3.2.1-C3.2.6): the daemon-leak
# barrier tests itself on every OS; the Windows process/handle fixes are
# proven where they exist.
set -uo pipefail
d=${COURT_CHECK_DIR:?}
case "$d" in /*) ;; *) d="$PWD/$d" ;; esac
bin=$(bash test/checks/court-bin.sh "$d") || exit 1
go build -o "$bin" ./cmd/court || { echo "court build failed" >&2; exit 1; }
export COURT_BIN="$bin"
go test ./internal/commission -count=1 -timeout 240s -v \
    -run '^TestLeakBarrierReportsLiveDaemon$' \
    || { echo "leak-barrier self-test failed; see go test output" >&2; exit 1; }
case "$(uname -s)" in
    MINGW*|MSYS*|CYGWIN*)
        go test ./internal/commission -count=1 -timeout 240s -v \
            -run '^(TestProcessAliveAccessDeniedIsAlive|TestDetachClearsStandardHandleInheritance)$' \
            || { echo "Windows process/handle tests failed; see go test output" >&2; exit 1; } ;;
esac
