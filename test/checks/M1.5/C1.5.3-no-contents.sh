#!/usr/bin/env bash
# C1.5.3 The stored tool.event contains no file contents and no command
# output for any fixture. Every fixture of the recorded corpus under
# test/vectors/hooks is posted with canary strings in every
# content-carrying field; every stored tool.event is then scanned for
# any canary, any content field name, or any output field name.
set -uo pipefail
d=${COURT_CHECK_DIR:?}
case "$d" in /*) ;; *) d="$PWD/$d" ;; esac
go build -o "$d/court" ./cmd/court || { echo "court build failed" >&2; exit 1; }
export COURT_BIN="$d/court"
go test ./internal/adapter/scripted -count=1 -timeout 120s -v -run '^(TestC1_5_3|TestToolEvent_CommandTruncatesRunes)$' \
    || { echo "no-contents check failed; see go test output" >&2; exit 1; }
