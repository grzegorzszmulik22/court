#!/usr/bin/env bash
# C1.3.2 Golden ledgers in test/vectors/ledger/ verify in both verifiers.
set -uo pipefail
d=${COURT_CHECK_DIR:-$(mktemp -d)}
mkdir -p "$d"
d=$(cd "$d" && pwd)
vec=test/vectors/ledger
ls "$vec"/*.jsonl >/dev/null 2>&1 || { echo "no golden ledgers" >&2; exit 1; }
go build -o "$d/court" ./cmd/court || { echo "court build failed" >&2; exit 1; }
"$d/court" ledger verify "$vec"/golden-*.jsonl \
    || { echo "go verifier rejected a golden" >&2; exit 1; }
python3 ref/python/refledger "$vec"/golden-*.jsonl \
    || { echo "python verifier rejected a golden" >&2; exit 1; }

# A SQLite rendering of a golden must verify in both too (built in scratch:
# the frozen set holds the stable .jsonl form only).
"$d/court" ledger build "$d/golden-mixed.sqlite" "$vec/golden-mixed.jsonl" \
    || { echo "sqlite build failed" >&2; exit 1; }
"$d/court" ledger verify "$d/golden-mixed.sqlite" \
    || { echo "go verifier rejected the sqlite golden" >&2; exit 1; }
python3 ref/python/refledger "$d/golden-mixed.sqlite" \
    || { echo "python verifier rejected the sqlite golden" >&2; exit 1; }

# Rejecting vectors: each strict-decoding rule of spec/01 section 11 has a
# {"name","class","b64"} record; BOTH verifiers must report the recorded
# class. A verifier that skips a rule reports ok or a different class.
[ -f "$vec/rejects.jsonl" ] || { echo "missing rejects.jsonl" >&2; exit 1; }
python3 - "$d" "$vec/rejects.jsonl" <<'PYEOF' || exit 1
import base64, json, os, sys
d, rej = sys.argv[1], sys.argv[2]
os.makedirs(os.path.join(d, 'rej'), exist_ok=True)
cls = {}
for line in open(rej):
    r = json.loads(line)
    p = os.path.join(d, 'rej', r['name'] + '.jsonl')
    open(p, 'wb').write(base64.b64decode(r['b64']))
    cls[p] = r['class']
json.dump(cls, open(os.path.join(d, 'rej-classes.json'), 'w'))
print('reject vectors:', len(cls))
PYEOF
"$d/court" ledger verify "$d"/rej/*.jsonl 2>/dev/null | awk '{print $2, $3}' > "$d/go-rej.out" || true
python3 ref/python/refledger "$d"/rej/*.jsonl 2>/dev/null | awk '{print $2, $3}' > "$d/py-rej.out" || true
python3 - "$d" <<'PYEOF' || { echo "rejecting vectors: disagreement" >&2; exit 1; }
import json, os, sys
d = sys.argv[1]
def norm(p):
    # Windows: os.path.join writes '\' while the verifiers print the path they
    # were given ('/'). Normalise separators only; the compared class is not
    # changed.
    return p.replace('\\', '/')
want = {norm(p): c for p, c in json.load(open(os.path.join(d, 'rej-classes.json'))).items()}
got = {}
for which in ('go-rej.out', 'py-rej.out'):
    for line in open(os.path.join(d, which)):
        path, cls = line.rstrip('\r\n').split()
        got.setdefault(norm(path), {})[which[:2]] = cls
bad = []
for p, wantcls in want.items():
    g = got.get(p, {})
    if g.get('go') != wantcls:
        bad.append(f"{os.path.basename(p)}: go={g.get('go')} want {wantcls}")
    if g.get('py') != wantcls:
        bad.append(f"{os.path.basename(p)}: python={g.get('py')} want {wantcls}")
if bad:
    print('\n'.join(bad)); sys.exit(1)
print(f"rejects: {len(want)} vectors, both verifiers report the expected class")
PYEOF
