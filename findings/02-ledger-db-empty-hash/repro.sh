#!/usr/bin/env bash
# Reproduce: Go accepts, refledger rejects, and export launders the edit.
# Run from the repository root.
set -u
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
BIN=${COURT_BIN:-$ROOT/.rt/court.exe}
WORK=$ROOT/.rt/scratch/repro02
rm -rf "$WORK" && mkdir -p "$WORK"

"$BIN" ledger gen --out "$WORK" --count 1 || exit 1
"$BIN" ledger build "$WORK/ledger.db" "$WORK/gen-00000.jsonl" || exit 1
echo "== clean ledger =="
"$BIN" ledger verify "$WORK/ledger.db"
python "$ROOT/ref/python/refledger" "$WORK/ledger.db"

python "$(dirname "$0")/tamper.py" "$WORK/ledger.db" || exit 1
echo "== tampered ledger (last event edited, hash blanked) =="
"$BIN" ledger verify "$WORK/ledger.db"; echo "court verify exit=$?"
python "$ROOT/ref/python/refledger" "$WORK/ledger.db"; echo "refledger exit=$?"

"$BIN" ledger export "$WORK/ledger.db" > "$WORK/laundered.jsonl"
echo "== re-exported tampered ledger =="
"$BIN" ledger verify "$WORK/laundered.jsonl"; echo "court verify exit=$?"
python "$ROOT/ref/python/refledger" "$WORK/laundered.jsonl"; echo "refledger exit=$?"
