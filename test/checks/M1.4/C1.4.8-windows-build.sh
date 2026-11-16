#!/usr/bin/env bash
# C1.4.8 On every host: `GOOS=windows go vet ./...` and
# `GOOS=windows go test -c -o /dev/null ./internal/world/` succeed, so a
# Windows build failure is caught on Linux before the Windows run (CR-001).
# Also runs the frozen_verify separator test: tools/frozen_verify.py is the
# one tools/ file this lane was allowed to fix for Windows, and the fix
# must carry a test.
set -uo pipefail
export GOOS=windows
go vet ./... || { echo "GOOS=windows go vet ./... failed" >&2; exit 1; }
go test -c -o /dev/null ./internal/world/ \
    || { echo "GOOS=windows go test -c ./internal/world/ failed" >&2; exit 1; }
python3 test/tools/frozen_verify_sep_test.py \
    || { echo "frozen_verify separator test failed" >&2; exit 1; }
