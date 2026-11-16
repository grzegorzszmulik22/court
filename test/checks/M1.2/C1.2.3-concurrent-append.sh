#!/usr/bin/env bash
# C1.2.3 Two processes appending at once: one succeeds per `seq`, none is
# lost, the chain holds.
set -uo pipefail
d=${COURT_CHECK_DIR:-$(mktemp -d)}
mkdir -p "$d"
d=$(cd "$d" && pwd)   # go test runs in the package dir
go test ./internal/ledger/ -run 'TestConcurrentAppend' -count=1 -args "$d" \
    || { echo "concurrent append lost events or broke the chain" >&2; exit 1; }
