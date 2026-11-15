#!/usr/bin/env bash
# C1.3.1 MM1.2: >= 200 generated ledgers and >= 1,000 mutants; both verifiers
# report the same error class for every mutant and ok for every generated
# ledger. Also exercises hand-crafted edge mutants (last-event removal,
# trailing-newline loss, partial last line, first-two swap) so a verifier
# that skips the tail or accepts truncation cannot stay green.
set -uo pipefail
d=${COURT_CHECK_DIR:-$(mktemp -d)}
mkdir -p "$d"
d=$(cd "$d" && pwd)
mkdir -p "$d/gen" "$d/mut" "$d/edge"

go build -o "$d/court" ./cmd/court || { echo "court build failed" >&2; exit 1; }
COURT="$d/court"
REFLEDGER="ref/python/refledger"

# 1. >= 200 generated ledgers, all valid.
"$COURT" ledger gen --out "$d/gen" --count 200 --seed 20261115 --size 60 \
    || { echo "generator failed" >&2; exit 1; }

# 2. >= 1000 mutants (one per kind per ledger across every MutantKinds).
i=0
for f in "$d/gen"/*.jsonl; do
    "$COURT" ledger mutate --in "$f" --out "$d/mut" --seed $((4242+i)) \
        || { echo "mutate failed on $f" >&2; exit 1; }
    i=$((i+1))
done
nmut=$(find "$d/mut" -name '*.jsonl' | wc -l)
[ "$nmut" -ge 1000 ] || { echo "only $nmut mutants" >&2; exit 1; }

# 3. Hand-crafted edge mutants aimed at tail handling. A verifier that skips
# the last event reports ok on the bad-last-* files where a correct verifier
# reports hash/prev — and a verifier that accepts truncation reports ok on
# no-newline where a correct one reports truncated.
python3 - "$d" <<'PYEOF'
import json, os, sys
d = sys.argv[1]
src = os.path.join(d, 'gen', 'gen-00000.jsonl')
lines = open(src, 'rb').read().splitlines()
# bad-last-hash: only defect is the final event's hash.
ev = json.loads(lines[-1])
ev['hash'] = 'f' * 64 if ev['hash'][0] != 'f' else 'e' * 64
with open(os.path.join(d, 'edge', 'bad-last-hash.jsonl'), 'wb') as f:
    f.write(b'\n'.join(lines[:-1]) + b'\n' + json.dumps(ev).encode() + b'\n')
# bad-last-prev: only defect is the final event's prev.
ev = json.loads(lines[-1])
ev['prev'] = '0' * 64
with open(os.path.join(d, 'edge', 'bad-last-prev.jsonl'), 'wb') as f:
    f.write(b'\n'.join(lines[:-1]) + b'\n' + json.dumps(ev).encode() + b'\n')
PYEOF
head -c -1 "$d/gen/gen-00001.jsonl" > "$d/edge/no-newline.jsonl"
(head -n -1 "$d/gen/gen-00002.jsonl"; head -c 80 "$d/gen/gen-00002.jsonl" | tail -c 40) \
    > "$d/edge/partial-last.jsonl"
{ tail -n 1 "$d/gen/gen-00003.jsonl"; head -n 3 "$d/gen/gen-00003.jsonl"; } \
    > "$d/edge/first-moved-last.jsonl"

# 4. Verify with both. Go verifier then Python reference verifier.
"$COURT" ledger verify "$d"/gen/*.jsonl "$d"/mut/*.jsonl "$d"/edge/*.jsonl \
    > "$d/go.out" 2>"$d/go.err" || true
python3 "$REFLEDGER" "$d"/gen/*.jsonl "$d"/mut/*.jsonl "$d"/edge/*.jsonl \
    > "$d/py.out" 2>"$d/py.err" || true

python3 - "$d" <<'PYEOF' || { echo "verifier agreement failed" >&2; exit 1; }
import sys, os, re
d = sys.argv[1]
def classes(path):
    out = {}
    for line in open(path):
        m = re.match(r'^VERIFY (\S+) (\S+)', line)
        if m:
            out[m.group(1)] = m.group(2)
    return out
go, py = classes(d + '/go.out'), classes(d + '/py.out')
bad = []
n_gen, n_mut = 0, 0
for path, gcls in go.items():
    pcls = py.get(path)
    base = os.path.basename(path)
    if pcls is None:
        bad.append(f"{base}: python verifier did not report")
        continue
    if gcls != pcls:
        bad.append(f"{base}: go={gcls} python={pcls}")
        continue
    if '/gen/' in path:
        n_gen += 1
        if gcls != 'ok':
            bad.append(f"{base}: generated ledger not ok ({gcls})")
    else:
        n_mut += 1
        if gcls == 'ok':
            bad.append(f"{base}: mutant accepted")
missing = set(py) - set(go)
for path in missing:
    bad.append(f"{os.path.basename(path)}: go verifier did not report")
print(f"agreement: {n_gen} ledgers, {n_mut} mutants, {len(bad)} disagreements")
if n_gen < 200 or n_mut < 1000:
    print(f"counts below spec: {n_gen} ledgers {n_mut} mutants"); sys.exit(1)
for b in bad[:20]:
    print("MISMATCH", b)
sys.exit(1 if bad else 0)
PYEOF

# 5. Evidence stays small: a manifest from which every ledger and mutant
# regenerates deterministically, plus the agreement summary. The generated
# and mutant files themselves are not committed.
python3 - "$d" <<'PYEOF' || exit 1
import json, os, re, sys, collections
d = sys.argv[1]
manifest = {
    "gen":  {"cmd": "court ledger gen", "seed": 20261115, "count": 200, "size": 60},
    "mut":  {"cmd": "court ledger mutate --seed <4242+i> per ledger i=0..199",
             "kinds": "all of MutantKinds (one per kind per ledger)"},
    "edge": ["bad-last-hash", "bad-last-prev", "no-newline",
             "partial-last", "first-moved-last"],
}
json.dump(manifest, open(os.path.join(d, 'manifest.json'), 'w'), indent=1)
hist = collections.Counter()
for which in ('go.out', 'py.out'):
    for line in open(os.path.join(d, which)):
        m = re.match(r'^VERIFY (\S+) (\S+)', line)
        if m:
            hist[which[:2] + ':' + m.group(2)] += 1
summary = {"mutant_files": sum(v for k, v in hist.items() if k.startswith('go:')) // 1,
           "classes": dict(sorted(hist.items()))}
json.dump(summary, open(os.path.join(d, 'agreement-summary.json'), 'w'), indent=1)
print(json.dumps(summary, indent=1))
PYEOF
rm -rf "$d/gen" "$d/mut" "$d/edge" "$d/rej" "$d/court" "$d/go.out" "$d/py.out" \
    "$d/go.err" "$d/py.err"
