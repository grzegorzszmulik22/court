#!/usr/bin/env bash
# C1.3.5 The kept-head check and the stored-bytes rule agree between the
# two verifiers (CR-004, B3-01/B3-02/B3-11):
#   - both print an identical `head seq=<n> hash=<hex>` on a clean ledger;
#   - `court replay` prints the head too;
#   - `--head <seq>:<hash>` passes on the clean ledger and reports the
#     events appended since that head, identically in both;
#   - a ledger cut back below the kept head fails in both;
#   - a ledger with a different event at the kept seq fails in both;
#   - a SQLite row whose stored hash is blank, or whose stored body is not
#     the canonical bytes, is an envelope error in both (readers never
#     recompute a hash they were not given).
set -uo pipefail
d=${COURT_CHECK_DIR:-$(mktemp -d)}
mkdir -p "$d"
d=$(cd "$d" && pwd)
go build -o "$d/court" ./cmd/court || { echo "court build failed" >&2; exit 1; }
COURT="$d/court"
REF="ref/python/refledger"

"$COURT" ledger gen --out "$d/g" --count 1 --seed 314 --size 60 \
    || { echo "generator failed" >&2; exit 1; }
L="$d/g/gen-00000.jsonl"

# --- 1. identical head on a clean ledger; replay prints it too ----------
"$COURT" ledger verify "$L" | grep -E '^VERIFY .* ok head seq=[0-9]+ hash=[0-9a-f]{64}$' >"$d/go.head" \
    || { echo "go verifier printed no head" >&2; exit 1; }
python3 "$REF" "$L" | grep -E '^VERIFY .* ok head seq=[0-9]+ hash=[0-9a-f]{64}$' >"$d/py.head" \
    || { echo "python verifier printed no head" >&2; exit 1; }
gh=$(sed 's/.* head //' "$d/go.head"); ph=$(sed 's/.* head //' "$d/py.head")
[ "$gh" = "$ph" ] || { echo "heads differ: go=$gh python=$ph" >&2; exit 1; }

"$COURT" ledger build "$d/run.sqlite" "$L" || { echo "build failed" >&2; exit 1; }
"$COURT" replay "$d/run.sqlite" | grep -E ' head seq=[0-9]+ hash=[0-9a-f]{64}$' \
    || { echo "replay printed no head" >&2; exit 1; }

# --- 2. --head agreement -------------------------------------------------
headat() { python3 -c "import json,sys; ls=open(sys.argv[1]).read().splitlines(); e=json.loads(ls[int(sys.argv[2])]); print('%s:%s'%(e['seq'],e['hash']))" "$L" "$1"; }
H10=$(headat 10)
# kept head at seq 10 -> both verifiers emit the same verdict stream,
# byte for byte (same VERIFY ok head, HEAD ok appended=N, APPENDED lines).
"$COURT" ledger verify --head "$H10" "$L" >"$d/go.h10" || { echo "go --head failed on clean ledger" >&2; exit 1; }
python3 "$REF" --head "$H10" "$L" >"$d/py.h10" || { echo "python --head failed on clean ledger" >&2; exit 1; }
diff "$d/go.h10" "$d/py.h10" \
    || { echo "--head streams differ (go above, python below)" >&2; exit 1; }
grep -q "HEAD $L ok seq=10 appended=46" "$d/go.h10" \
    || { echo "expected appended=46 in $(cat "$d/go.h10")" >&2; exit 1; }
# cut back below the kept head: both fail with HEAD ... FAIL.
head -5 "$L" >"$d/cut.jsonl"
for v in "$COURT ledger verify" "python3 $REF"; do
    if $v --head "$H10" "$d/cut.jsonl" >"$d/cut.out" 2>&1; then
        echo "$v accepted a cut-back ledger" >&2; exit 1
    fi
    grep -q "HEAD $d/cut.jsonl FAIL seq=10" "$d/cut.out" \
        || { echo "$v did not report the cut-back head: $(cat "$d/cut.out")" >&2; exit 1; }
done
# a different event at the kept seq: a second, valid ledger verifies yet
# holds a different event at seq 10 — the kept head must fail in both.
"$COURT" ledger gen --out "$d/g2" --count 1 --seed 999 --size 60 || exit 1
L2="$d/g2/gen-00000.jsonl"
"$COURT" ledger verify "$L2" >/dev/null || { echo "second ledger does not verify" >&2; exit 1; }
H10B=$(headat() { python3 -c "import json,sys; ls=open(sys.argv[1]).read().splitlines(); e=json.loads(ls[10]); print(e['hash'])" "$L2"; }; headat)
[ "$H10B" != "${H10#*:}" ] || { echo "second ledger collides at seq 10 (bad seed choice)" >&2; exit 1; }
for v in "$COURT ledger verify" "python3 $REF"; do
    if $v --head "$H10" "$L2" >"$d/edit.out" 2>&1; then
        echo "$v accepted a different event at the kept seq" >&2; exit 1
    fi
    grep -q "HEAD $L2 FAIL seq=10" "$d/edit.out" \
        || { echo "$v did not report the seq-10 mismatch: $(cat "$d/edit.out")" >&2; exit 1; }
done

# --- 3. sqlite stored rows: blank hash, non-canonical body ---------------
python3 - "$d" <<'PYEOF'
import shutil, sqlite3, sys
d = sys.argv[1]
shutil.copy(d + "/run.sqlite", d + "/blankhash.sqlite")
c = sqlite3.connect(d + "/blankhash.sqlite")
c.execute("UPDATE ledger SET hash='' WHERE seq=7")
c.commit(); c.close()
shutil.copy(d + "/run.sqlite", d + "/noncanon.sqlite")
c = sqlite3.connect(d + "/noncanon.sqlite")
row = c.execute("SELECT body FROM ledger WHERE seq=7").fetchone()[0]
b = row[:1] + " " + row[1:]  # '{ ...' -> '{  ...' : valid JSON, not canonical
c.execute("UPDATE ledger SET body=? WHERE seq=7", (b,))
c.commit(); c.close()
PYEOF
for f in blankhash noncanon; do
    gocls=$("$COURT" ledger verify "$d/$f.sqlite" 2>/dev/null | awk '{print $3}')
    pycls=$(python3 "$REF" "$d/$f.sqlite" 2>/dev/null | awk '{print $3}')
    [ "$gocls" = "$pycls" ] || { echo "$f: go=$gocls python=$pycls" >&2; exit 1; }
    [ "$gocls" = "envelope" ] || { echo "$f: class $gocls, want envelope" >&2; exit 1; }
done
