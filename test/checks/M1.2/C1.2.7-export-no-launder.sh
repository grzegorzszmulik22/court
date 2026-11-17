#!/usr/bin/env bash
# C1.2.7 `court ledger export` never launders a tamper into a clean file
# (B3-02): a stored row that was rewritten, or whose hash was blanked, must
# abort the export — not produce a fresh-looking ledger. Also covers the
# kept-head flag at the CLI: --head accepts the true head and rejects a
# forged one.
set -uo pipefail
d=${COURT_CHECK_DIR:-$(mktemp -d)}
mkdir -p "$d"
d=$(cd "$d" && pwd)
go build -o "$d/court" ./cmd/court || { echo "court build failed" >&2; exit 1; }
COURT="$d/court"

"$COURT" ledger gen --out "$d/g" --count 1 --seed 77 --size 30 \
    || { echo "generator failed" >&2; exit 1; }
L="$d/g/gen-00000.jsonl"
"$COURT" ledger build "$d/run.sqlite" "$L" || { echo "build failed" >&2; exit 1; }

# clean export verifies
"$COURT" ledger export "$d/run.sqlite" >"$d/out.jsonl" \
    || { echo "clean export failed" >&2; exit 1; }
"$COURT" ledger verify "$d/out.jsonl" >/dev/null \
    || { echo "clean export does not verify" >&2; exit 1; }

# tamper one row's body and one row's hash in copies
python3 - "$d" <<'PYEOF'
import json, shutil, sqlite3, sys
d = sys.argv[1]
shutil.copy(d + "/run.sqlite", d + "/edited.sqlite")
c = sqlite3.connect(d + "/edited.sqlite")
row = c.execute("SELECT body FROM ledger WHERE seq=5").fetchone()[0]
b = json.loads(row); b["tampered"] = True
c.execute("UPDATE ledger SET body=? WHERE seq=5", (json.dumps(b, separators=(",",":")),))
c.commit(); c.close()
shutil.copy(d + "/run.sqlite", d + "/blank.sqlite")
c = sqlite3.connect(d + "/blank.sqlite")
c.execute("UPDATE ledger SET hash='' WHERE seq=5")
c.commit(); c.close()
PYEOF

for f in edited blank; do
    if "$COURT" ledger export "$d/$f.sqlite" >"$d/$f.jsonl" 2>/dev/null; then
        echo "export of tampered $f.sqlite succeeded" >&2; exit 1
    fi
done

# --head at the CLI: the true head passes, a forged one fails.
H=$("$COURT" ledger verify "$d/out.jsonl" | sed -n 's/.*ok head //p')
SEQ=${H#seq=}; SEQ=${SEQ%% *}; HASH=${H##*hash=}
"$COURT" ledger verify --head "$SEQ:$HASH" "$d/out.jsonl" >"$d/head.out" \
    || { echo "--head rejected the true head" >&2; exit 1; }
grep -q "^HEAD $d/out.jsonl ok" "$d/head.out" \
    || { echo "no HEAD ok line: $(cat "$d/head.out")" >&2; exit 1; }
BADH="ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
if "$COURT" ledger verify --head "$SEQ:$BADH" "$d/out.jsonl" >"$d/badhead.out" 2>&1; then
    echo "--head accepted a forged head" >&2; exit 1
fi
grep -q "^HEAD $d/out.jsonl FAIL" "$d/badhead.out" \
    || { echo "forged head not reported: $(cat "$d/badhead.out")" >&2; exit 1; }
