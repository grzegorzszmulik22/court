#!/usr/bin/env bash
# C1.5.1 A script of 500 mixed steps yields exactly the expected tool.event
# and world.commit events, in order. The comparison is against the script
# (what it asked for), replayed by the scripted adapter through real hook
# posts and real commits reported by the generated post-commit hook.
# Also covers: denied calls record one tool.event with a reason, and a
# PostToolUse whose Pre was never parked still records.
set -uo pipefail
d=${COURT_CHECK_DIR:?}
case "$d" in /*) ;; *) d="$PWD/$d" ;; esac
go build -o "$d/court" ./cmd/court || { echo "court build failed" >&2; exit 1; }
export COURT_BIN="$d/court"
go test ./internal/adapter/scripted -count=1 -timeout 210s -v \
    -run '^TestC1_5_1$|^TestHook_DeniedRecordsReason$|^TestHook_UnpairedPosts$' \
    || { echo "mixed-script check failed; see go test output" >&2; exit 1; }
