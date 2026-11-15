#!/usr/bin/env bash
# C1.4.7 On Windows: a world path longer than 260 characters works.
# SKIPPED-ENV where no Windows host exists. This lane runs on Linux
# (tools/host.md); the probe below records that fact as the skip reason.
set -uo pipefail
s=$(uname -s 2>/dev/null || echo unknown)
case "$s" in
    MINGW*|MSYS*|CYGWIN*|Windows_NT)
        echo "windows host detected but no longpath probe exists on this lane" >&2
        exit 78
        ;;
    *)
        {
            echo "no Windows host available: uname -s reports '$s' (go env GOOS: $(go env GOOS 2>/dev/null || echo unknown))"
            echo "probe: uname -a: $(uname -a 2>/dev/null)"
        } >&2
        exit 77
        ;;
esac
