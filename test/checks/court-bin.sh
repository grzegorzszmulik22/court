#!/usr/bin/env bash
# court-bin.sh <check-dir> — prints the path to build the court binary at.
#
# Windows requires an executable to carry an extension: `go build -o
# .../court` writes a file with no extension, and CreateProcess refuses
# it ("executable file not found"). Checks that build the binary and
# pass it to tests through COURT_BIN ask this helper instead; on Linux
# the name stays exactly `court` (CR-002).
set -u
d=${1:?usage: court-bin.sh <check-dir>}
case "$(uname -s)" in
    MINGW*|MSYS*|CYGWIN*) echo "$d/court.exe" ;;
    *) echo "$d/court" ;;
esac
