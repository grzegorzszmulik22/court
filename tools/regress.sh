#!/usr/bin/env bash
# regress.sh — make regress.
#   1. verify spec/FROZEN.sha256 (spec/05-agent-loop.md section 7)
#   2. run `make check-<chunk>` for every chunk marked DONE in spec/PROGRESS.md
#      or spec/progress/*.md
set -uo pipefail
cd "$(dirname "$0")/.."

fail=0

if [ -f spec/FROZEN.sha256 ]; then
    echo "regress: verifying spec/FROZEN.sha256"
    python3 tools/frozen_verify.py spec/FROZEN.sha256 . || fail=1
fi

chunks=$(python3 - <<'PYEOF'
import glob, re
seen = set()
for path in sorted(glob.glob("spec/PROGRESS.md") + glob.glob("spec/progress/*.md")):
    for line in open(path, encoding="utf-8"):
        s = line.strip()
        if not s.startswith("|"):
            continue
        fields = [f.strip() for f in s.split("|")[1:-1]]
        if len(fields) < 3 or fields[0].lower() == "chunk":
            continue
        if "DONE" in fields and re.match(r"^[MG][0-9]+(\.[0-9]+)?$", fields[0]):
            seen.add(fields[0])
for c in sorted(seen):
    print(c)
PYEOF
)

os=$(uname -s)
for c in $chunks; do
    echo "regress: check-$c"
    out=$(make --no-print-directory "check-$c" 2>&1); rc=$?
    printf '%s\n' "$out"
    [ "$rc" -eq 0 ] && continue
    # A failing run is accepted only when every non-PASS check is a SKIP that
    # tools/env-skips.txt allows on this OS (and the run did check something).
    if printf '%s\n' "$out" | python3 tools/env_skip_ok.py tools/env-skips.txt "$os"; then
        continue
    fi
    fail=1
done

[ -z "$chunks" ] && echo "regress: no DONE chunks"
exit $fail
