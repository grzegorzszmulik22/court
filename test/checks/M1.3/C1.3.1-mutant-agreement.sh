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
"$COURT" ledger gen --out "$d/gen" --count 200 --seed 20261115 --size 40 \
    || { echo "generator failed" >&2; exit 1; }

# 2. >= 1000 mutants (one per kind per ledger across every MutantKinds).
# indir mode mutates ledger i with seed 4242+i; running one --in call per
# ledger with --seed $((4242+i)) produces the identical corpus while letting
# several ledgers mutate in parallel (the serial pass cost ~30 s under load,
# which pushed this check past its 240 s timeout on a contended host).
gens=("$d"/gen/*.jsonl)
mut_jobs=${#gens[@]}
[ "$mut_jobs" -gt 8 ] && mut_jobs=8
i=0
pids=()
for f in "${gens[@]}"; do
    base=$(basename "$f" .jsonl)
    idx=${base##*-}; idx=$((10#$idx))
    "$COURT" ledger mutate --in "$f" --out "$d/mut" --seed $((4242 + idx)) &
    pids+=($!)
    i=$((i + 1))
    if [ $((i % mut_jobs)) -eq 0 ]; then
        for p in "${pids[@]}"; do wait "$p" || { echo "mutate failed" >&2; exit 1; }; done
        pids=()
    fi
done
for p in "${pids[@]}"; do wait "$p" || { echo "mutate failed" >&2; exit 1; }; done
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

# 4. Verify with both, in parallel (independent passes over the corpus).
# Windows caps one command line at 32,767 characters and the corpus is over
# 5,000 paths, so each verifier is invoked in deterministic batches and its
# per-file lines are appended in corpus order. Every file is still verified
# exactly once by each verifier; the agreement pass below sees the same output
# it sees on Linux.
# The corpus is split into strided shards verified concurrently: two serial
# passes over 1,200+ ledgers can exceed the 240 s check timeout on a contended
# host (seen 2026-11-20, regress replay timing out at load ~12). Each shard
# keeps corpus order internally, so the concatenated output is still a
# deterministic list in which every corpus file appears exactly once.
files=("$d"/gen/*.jsonl "$d"/mut/*.jsonl "$d"/edge/*.jsonl)
shards=6
i=0
for f in "${files[@]}"; do
    printf '%s\n' "$f" >>"$d/shard.$((i % shards)).files"
    i=$((i + 1))
done
run_shard() { # <shard-index> <out-prefix> <verifier command...>
    local s=$1 out=$2
    shift 2
    local batch=() f
    : >"$out"
    : >"$out.err"
    while IFS= read -r f; do
        batch+=("$f")
        # 200 paths * ~140 chars stays under the Windows 32,767-char limit.
        if [ "${#batch[@]}" -ge 200 ]; then
            "$@" "${batch[@]}" >>"$out" 2>>"$out.err" || true
            batch=()
        fi
    done <"$d/shard.$s.files"
    if [ "${#batch[@]}" -gt 0 ]; then
        "$@" "${batch[@]}" >>"$out" 2>>"$out.err" || true
    fi
}
pids=()
for i in $(seq 0 $((shards - 1))); do
    run_shard "$i" "$d/go.$i.out" "$COURT" ledger verify &
    pids+=($!)
    run_shard "$i" "$d/py.$i.out" python3 "$REFLEDGER" &
    pids+=($!)
done
wait "${pids[@]}" 2>/dev/null || true
cat "$d"/go.*.out >"$d/go.out"
cat "$d"/py.*.out >"$d/py.out"
cat "$d"/go.*.out.err >"$d/go.err"
cat "$d"/py.*.out.err >"$d/py.err"

python3 - "$d" <<'PYEOF' || { echo "verifier agreement failed" >&2; exit 1; }
import sys, os, re
d = sys.argv[1]
def norm(p):
    # Windows: verifier output and Python path joins may use either separator.
    # Normalise separators for the key; line endings for the read. The class
    # compared is untouched.
    return p.replace('\\', '/')
def classes(path):
    out = {}
    for line in open(path):
        m = re.match(r'^VERIFY (\S+) (\S+)', line.rstrip('\r\n'))
        if m:
            out[norm(m.group(1))] = m.group(2)
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
# The verifiers run in batches, so a crashed batch would drop its files from
# both outputs at once. Every corpus file must have a result from both.
# Keyed by <subdir>/<name>: on Windows the two sides may spell the scratch
# root differently (/tmp/... versus C:/...).
def key(p):
    return '/'.join(norm(p).split('/')[-2:])
reported = {key(p) for p in go} & {key(p) for p in py}
for sub in ('gen', 'mut', 'edge'):
    for f in sorted(os.listdir(os.path.join(d, sub))):
        if f.endswith('.jsonl') and f"{sub}/{f}" not in reported:
            bad.append(f"{sub}/{f}: not reported by both verifiers")
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
    "gen":  {"cmd": "court ledger gen", "seed": 20261115, "count": 200, "size": 40},
    "mut":  {"cmd": "court ledger mutate --indir gen --seed 4242 "
                    "(per-ledger seed = 4242 + index i=0..199)",
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
rm -rf "$d/gen" "$d/mut" "$d/edge" "$d/rej" "$d/court" "$d"/shard.*.files \
    "$d"/go.*.out "$d"/py.*.out "$d"/go.*.out.err "$d"/py.*.out.err \
    "$d/go.out" "$d/py.out" "$d/go.err" "$d/py.err"
