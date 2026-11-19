#!/usr/bin/env bash
# C2.1.6: the daemon process lifecycle under which every M2.1 check runs.
# /admin/shutdown must end the process, and a daemon whose discovery file
# (daemon.json) is gone must exit on its own within its documented bound.
# Both are asserted on the process itself, then the fixture's TestMain
# leak barrier proves the run left no court daemon alive.
set -uo pipefail
d=${COURT_CHECK_DIR:?}
case "$d" in /*) ;; *) d="$PWD/$d" ;; esac
bin=$(bash test/checks/court-bin.sh "$d") || exit 1
go build -o "$bin" ./cmd/court || { echo "court build failed" >&2; exit 1; }
export COURT_BIN="$bin"
go test ./internal/commission -count=1 -timeout 120s -v -run '^TestDaemonExits' \
    || { echo "daemon lifecycle check failed; see go test output" >&2; exit 1; }
