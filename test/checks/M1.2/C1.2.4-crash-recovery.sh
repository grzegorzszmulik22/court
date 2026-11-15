#!/usr/bin/env bash
# C1.2.4 A crash injected between write and commit leaves a ledger that
# verifies, at each of 200 injection points.
set -uo pipefail
d=${COURT_CHECK_DIR:-$(mktemp -d)}
mkdir -p "$d"
d=$(cd "$d" && pwd)   # go test runs in the package dir
go test ./internal/ledger/ -run 'TestCrashRecovery' -count=1 -args "$d" \
    || { echo "a crash between write and commit left an unverifiable ledger" >&2; exit 1; }
