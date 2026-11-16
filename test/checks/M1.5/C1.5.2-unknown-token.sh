#!/usr/bin/env bash
# C1.5.2 A payload with an unknown token is refused and recorded as
# refused; nothing is attributed. Also covers delivery idempotency: a
# byte-identical repost records once (pre- and post-daemon-restart), while
# a distinct post records a new event.
set -uo pipefail
d=${COURT_CHECK_DIR:?}
case "$d" in /*) ;; *) d="$PWD/$d" ;; esac
go build -o "$d/court" ./cmd/court || { echo "court build failed" >&2; exit 1; }
export COURT_BIN="$d/court"
go test ./internal/adapter/scripted -count=1 -timeout 120s -v \
    -run '^TestC1_5_2$|^TestHook_IdempotentRetry$|^TestDaemon_RestartKeepsDedupe$|^TestHook_TokenIsIdentity$|^TestHook_RequestIdentityPerSession$|^TestHook_RequestIdentityRebuilt$' \
    || { echo "unknown-token/idempotency check failed; see go test output" >&2; exit 1; }
