#!/usr/bin/env bash
# C1.8.2 (MM1.4) Paired hook round trip against a no-op endpoint,
# p99 <= 50 ms.
set -uo pipefail
d=${COURT_CHECK_DIR:?}
case "$d" in /*) ;; *) d="$PWD/$d" ;; esac
go build -o "$d/court" ./cmd/court || { echo "court build failed" >&2; exit 1; }
export COURT_BIN="$d/court"
go test ./internal/adapter/scripted -count=1 -timeout 300s -v -run '^TestC1_8_2$' \
    || { echo "latency check failed; see go test output" >&2; exit 1; }
