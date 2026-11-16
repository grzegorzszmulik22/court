#!/usr/bin/env bash
# C1.3.3 Replay of an M1.8-shaped run yields identical derived tables
# (row-for-row comparison). Also: tampering with a stored derived table and
# with a stored decision.reconciled verdict must make replay fail, so a
# replay that just echoes stored state cannot pass.
set -uo pipefail
d=${COURT_CHECK_DIR:-$(mktemp -d)}
mkdir -p "$d"
d=$(cd "$d" && pwd)
go build -o "$d/court" ./cmd/court || { echo "court build failed" >&2; exit 1; }
db="$d/run.sqlite"

"$d/court" ledger loadgen --agents 8 --tools 10000 --commits 500 --seed 7 "$db" \
    || { echo "load run failed" >&2; exit 1; }
"$d/court" replay "$db" || { echo "replay of a clean run failed" >&2; exit 1; }

# Export, rebuild through the JSONL path, replay again: the derived state
# must be rebuilt from events, not merely read back.
"$d/court" ledger export "$db" > "$d/run.jsonl" || { echo "export failed" >&2; exit 1; }
"$d/court" replay "$d/run.jsonl" || { echo "replay of exported ledger failed" >&2; exit 1; }

# Tamper 1: flip one stored derived row. Replay must fail.
python3 - "$db" <<'PYEOF'
import sqlite3, sys
c = sqlite3.connect(sys.argv[1])
row = c.execute("SELECT commission,id FROM decisions LIMIT 1").fetchone()
if row is None:
    sys.exit(2)
c.execute("UPDATE decisions SET recon='late' WHERE commission=? AND id=?", row)
c.commit(); c.close()
PYEOF
if "$d/court" replay "$db" >/dev/null 2>&1; then
    echo "replay accepted a tampered derived table" >&2; exit 1
fi
rm -f "$db" "$db-wal" "$db-shm"

# Tamper 2: a stored decision.reconciled that lies about its verdict.
"$d/court" ledger gen --out "$d/g" --count 1 --seed 99 --size 40 || exit 1
"$d/court" ledger build "$db" "$d/g/gen-00000.jsonl" || { echo "build failed" >&2; exit 1; }
python3 - "$db" <<'PYEOF'
import sqlite3, sys, json, hashlib
c = sqlite3.connect(sys.argv[1])
rows = c.execute("SELECT seq,prev,at,commission,actor,type,body,hash FROM ledger WHERE type='decision.reconciled'").fetchall()
if not rows:
    sys.exit(2)
seq,prev,at,com,act,typ,body,h = rows[0]
b = json.loads(body)
b['recon'] = 'undeclared'
body2 = json.dumps(b, sort_keys=True, separators=(',',':'))
# rebuild canonical envelope bytes for the new hash (same fields, new body)
ev = {"actor":act,"at":at,"body":b,"commission":com,"prev":prev,"seq":seq,"type":typ}
canon = json.dumps(ev, sort_keys=True, separators=(',',':'), ensure_ascii=False)
h2 = hashlib.sha256(canon.encode()).hexdigest()
c.execute("UPDATE ledger SET body=?, hash=? WHERE seq=?", (body2, h2, seq))
# fix every later prev/hash so only the reconciled lie remains
nxt = c.execute("SELECT seq,prev,at,commission,actor,type,body,hash FROM ledger WHERE seq>?", (seq,)).fetchall()
prevh = h2
for s2,p2,a2,c2,ac2,t2,b2,hh2 in nxt:
    b2j = json.loads(b2)
    ev = {"actor":ac2,"at":a2,"body":b2j,"commission":c2,"prev":prevh,"seq":s2,"type":t2}
    canon = json.dumps(ev, sort_keys=True, separators=(',',':'), ensure_ascii=False)
    nh = hashlib.sha256(canon.encode()).hexdigest()
    c.execute("UPDATE ledger SET prev=?, hash=? WHERE seq=?", (prevh, nh, s2))
    prevh = nh
c.commit(); c.close()
PYEOF
if "$d/court" replay "$db" >/dev/null 2>&1; then
    echo "replay accepted a lying decision.reconciled" >&2; exit 1
fi
