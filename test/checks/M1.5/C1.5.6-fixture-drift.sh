#!/usr/bin/env bash
# C1.5.6 Fixture drift: the field names of each fixture equal the field
# list in tools/hook-fields.txt. The authoritative fixtures are recorded
# once from a real Claude Code session at the pinned version
# (spec/02 section 4.2) and land on main at test/vectors/hooks/recorded/.
# This host cannot run that session; until the recorded corpus exists the
# check is BLOCKED with the reason the lane note specifies. It does not
# substitute the documented stand-ins: those are generated from the same
# field table the file is checked against, so comparing them proves
# nothing.
set -uo pipefail
if [ ! -d test/vectors/hooks/recorded/hooks ] \
    || ! ls test/vectors/hooks/recorded/hooks/*.json >/dev/null 2>&1; then
    echo "recorded hook fixtures not on main yet" >&2
    exit 78
fi
go test ./internal/adapter/scripted -count=1 -timeout 60s -v -run '^TestC1_5_6' \
    || { echo "fixture drift check failed; see go test output" >&2; exit 1; }
