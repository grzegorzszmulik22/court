#!/usr/bin/env bash
# C1.2.2 A chain of 10,000 events: `seq` gap-free, every `prev` verifies,
# event 0 has the zero `prev`.
set -uo pipefail
d=${COURT_CHECK_DIR:-$(mktemp -d)}
mkdir -p "$d"
d=$(cd "$d" && pwd)   # go test runs in the package dir
go test ./internal/ledger/ -run 'TestChain10000|TestHashCoversEveryField' -count=1 -args "$d" \
    || { echo "10,000-event chain failed to build or verify" >&2; exit 1; }
