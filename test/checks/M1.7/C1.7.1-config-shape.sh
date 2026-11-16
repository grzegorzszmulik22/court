#!/usr/bin/env bash
# C1.7.1 Generated settings and MCP configuration validate against the
# documented shapes (http hooks for every consumed event,
# allowedHttpHookUrls, a single court MCP server), written under the
# data directory, never the repository.
set -uo pipefail
d=${COURT_CHECK_DIR:?}
case "$d" in /*) ;; *) d="$PWD/$d" ;; esac
go build -o "$d/court" ./cmd/court || { echo "court build failed" >&2; exit 1; }
export COURT_BIN="$d/court"
go test ./internal/adapter/scripted -count=1 -timeout 60s -v -run '^TestC1_7_1$' \
    || { echo "config shape check failed; see go test output" >&2; exit 1; }
