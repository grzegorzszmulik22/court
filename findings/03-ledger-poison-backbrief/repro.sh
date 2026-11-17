#!/usr/bin/env bash
# Reproduce: the append path accepts a body the verifier's schema rejects.
# Run from the repository root.
set -u
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
BIN=${COURT_BIN:-$ROOT/.rt/court.exe}
WORK=$ROOT/.rt/scratch/repro03
rm -rf "$WORK" && mkdir -p "$WORK"
DB=$WORK/poison.db
"$BIN" ledger append "$DB" C1 agent:a1 backbrief.submitted "$(dirname "$0")/body.json" > /dev/null
echo "append exit=$?"
"$BIN" ledger verify "$DB"; echo "court verify exit=$?"
python "$ROOT/ref/python/refledger" "$DB"; echo "refledger exit=$?"
"$BIN" replay "$DB"; echo "court replay exit=$?"
# The daemon keeps appending to the invalid ledger; nothing notices.
printf '{"text":"after the poison","paths":[]}\n' > "$WORK/alarm.json"
"$BIN" ledger append "$DB" C1 agent:a1 alarm.raised "$WORK/alarm.json" > /dev/null
echo "second append exit=$?"
"$BIN" ledger verify "$DB"; echo "court verify after more events exit=$?"
