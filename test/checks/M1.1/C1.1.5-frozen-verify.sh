#!/usr/bin/env bash
# C1.1.5 FROZEN.sha256 verification fails when a listed file changes — and also
# when a listed file is missing, or a file matching a frozen pattern is not
# listed. The real manifest must verify clean.
set -uo pipefail
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

mkdir -p "$tmp/spec/milestones"
printf 'alpha\n' >"$tmp/spec/00-a.md"
printf 'beta\n'  >"$tmp/spec/01-b.md"
printf 'mile\n'  >"$tmp/spec/milestones/M1-x.md"
(
    cd "$tmp" || exit 1
    {
        echo "# frozen: spec/0*.md spec/milestones/*.md"
        sha256sum spec/00-a.md spec/01-b.md spec/milestones/M1-x.md
    } > spec/FROZEN.sha256
)

run() { python3 tools/frozen_verify.py "$tmp/spec/FROZEN.sha256" "$tmp" >/dev/null 2>&1; }

run || { echo "frozen verify fails on an unchanged tree" >&2; exit 1; }

printf 'tampered\n' >>"$tmp/spec/01-b.md"
run && { echo "frozen verify passed after a listed file changed" >&2; exit 1; }
printf 'beta\n' >"$tmp/spec/01-b.md"

rm "$tmp/spec/milestones/M1-x.md"
run && { echo "frozen verify passed with a listed file missing" >&2; exit 1; }
printf 'mile\n' >"$tmp/spec/milestones/M1-x.md"

printf 'unlisted\n' >"$tmp/spec/02-c.md"
run && { echo "frozen verify passed with an unlisted file matching a pattern" >&2; exit 1; }
rm "$tmp/spec/02-c.md"

run || { echo "frozen verify does not pass after restore" >&2; exit 1; }

# The repository's own manifest must verify clean.
python3 tools/frozen_verify.py spec/FROZEN.sha256 . \
    || { echo "spec/FROZEN.sha256 does not verify in the working tree" >&2; exit 1; }
