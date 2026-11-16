#!/usr/bin/env bash
# C1.4.7 (restated by CR-001) On Windows: a world whose root path is at most
# 200 characters holds, commits, diffs and discards a file whose full path is
# longer than 260 characters; the same check run with core.longpaths disabled
# fails, proving the setting is what makes it pass. Opening a commission
# whose data directory would put a world root above 200 characters fails with
# a one-sentence error naming the limit. C1.4.6 also passes on Windows.
#
# SKIPPED-ENV where no Windows host exists. The tests run under plain
# `go test` so the identical command works on a Windows PC:
#   go test ./internal/world -run 'TestC1_4_6|TestC1_4_7' -count=1 -v
set -uo pipefail
s=$(uname -s 2>/dev/null || echo unknown)
case "$s" in
    MINGW*|MSYS*|CYGWIN*|Windows_NT)
        go test ./internal/world -run '^TestC1_4_6_RunTimeout$|^TestC1_4_7_WindowsLongPaths$' \
            -count=1 -v \
            || { echo "C1.4.7 windows run failed" >&2; exit 1; }
        ;;
    *)
        {
            echo "no Windows host available: uname -s reports '$s' (go env GOOS: $(go env GOOS 2>/dev/null || echo unknown))"
            echo "probe: uname -a: $(uname -a 2>/dev/null)"
            echo "windows run: go test ./internal/world -run '^TestC1_4_6_RunTimeout$|^TestC1_4_7_WindowsLongPaths$' -count=1 -v"
        } >&2
        exit 77
        ;;
esac
