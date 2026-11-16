#!/usr/bin/env bash
# C1.3.4 The Python verifier runs with only the standard library.
# - every top-level import of refledger must be in sys.stdlib_module_names
# - it runs under python3 -I (isolated) and verifies a ledger there
# - its --selftest regenerates the deterministic ES6 number stream and
#   matches the published sha256 (no third-party JCS code)
set -uo pipefail
d=${COURT_CHECK_DIR:-$(mktemp -d)}
mkdir -p "$d"
d=$(cd "$d" && pwd)

python3 - ref/python/refledger <<'PYEOF' || { echo "non-stdlib import found" >&2; exit 1; }
import ast, sys
tree = ast.parse(open(sys.argv[1]).read())
mods = set()
for n in ast.walk(tree):
    if isinstance(n, ast.Import):
        for a in n.names: mods.add(a.name.split('.')[0])
    elif isinstance(n, ast.ImportFrom) and n.level == 0 and n.module:
        mods.add(n.module.split('.')[0])
stdlib = sys.stdlib_module_names
bad = {m for m in mods if m not in stdlib and m != '__main__'}
if bad:
    print("non-stdlib imports:", sorted(bad)); sys.exit(1)
print("imports:", " ".join(sorted(mods)))
PYEOF

# Runs isolated (no site-packages, no user env) and self-tests its number
# formatting against the published upstream checksum.
PYTHONPATH= python3 -I ref/python/refledger --selftest \
    || { echo "selftest failed" >&2; exit 1; }

# A real verification under -I too.
go build -o "$d/court" ./cmd/court || { echo "court build failed" >&2; exit 1; }
"$d/court" ledger gen --out "$d/g" --count 1 --seed 5 --size 50 || exit 1
PYTHONPATH= python3 -I ref/python/refledger "$d/g/gen-00000.jsonl" \
    || { echo "isolated run failed" >&2; exit 1; }
