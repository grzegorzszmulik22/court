#!/usr/bin/env bash
# C1.2.6 MM1.5: ledger append throughput, single writer, >= 2,000 events/s.
# Writes measurements.json (command, sysinfo, runs, raw samples, median, CV)
# into the run's evidence folder.
set -uo pipefail
d=${COURT_CHECK_DIR:-$(mktemp -d)}
mkdir -p "$d"
d=$(cd "$d" && pwd)   # go test runs in the package dir
out="${COURT_EVIDENCE_DIR:-$d}/measurements.json"
COURT_MEASUREMENTS="$out" go test ./internal/ledger/ -run 'TestAppendThroughput' -count=1 -args "$d" \
    || { echo "append throughput below 2,000 events/s (see measurements.json)" >&2; exit 1; }
[ -s "$out" ] || { echo "measurements.json not written" >&2; exit 1; }
