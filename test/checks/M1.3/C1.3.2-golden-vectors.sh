#!/usr/bin/env bash
# C1.3.2 Golden ledgers in test/vectors/ledger/ verify in both verifiers.
set -uo pipefail
d=${COURT_CHECK_DIR:-$(mktemp -d)}
mkdir -p "$d"
d=$(cd "$d" && pwd)
vec=test/vectors/ledger
ls "$vec"/*.jsonl >/dev/null 2>&1 || { echo "no golden ledgers" >&2; exit 1; }
go build -o "$d/court" ./cmd/court || { echo "court build failed" >&2; exit 1; }
"$d/court" ledger verify "$vec"/*.jsonl || { echo "go verifier rejected a golden" >&2; exit 1; }
python3 ref/python/refledger "$vec"/*.jsonl || { echo "python verifier rejected a golden" >&2; exit 1; }

# A SQLite rendering of a golden must verify in both too (built in scratch:
# the frozen set holds the stable .jsonl form only).
"$d/court" ledger build "$d/golden-mixed.sqlite" "$vec/golden-mixed.jsonl" \
    || { echo "sqlite build failed" >&2; exit 1; }
"$d/court" ledger verify "$d/golden-mixed.sqlite" \
    || { echo "go verifier rejected the sqlite golden" >&2; exit 1; }
python3 ref/python/refledger "$d/golden-mixed.sqlite" \
    || { echo "python verifier rejected the sqlite golden" >&2; exit 1; }
