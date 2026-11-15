#!/usr/bin/env bash
# C1.4.1 MM1.3: random sequences of fork, edit, commit, run and discard
# across up to 8 worlds. The repository digest (every ref and every object
# id) after discarding a world equals the digest before it was created, in
# 200 seeded sessions.
set -uo pipefail
d=${COURT_CHECK_DIR:?}
# go test runs inside the package directory; the output path must be absolute.
case "$d" in /*) ;; *) d="$PWD/$d" ;; esac
export COURT_MM13_OUT="$d/mm13.json"

go test ./internal/world -run '^TestC1_4_1_MM13$' -count=1 -timeout 210s -v \
    || { echo "MM1.3 sessions failed; see go test output" >&2; exit 1; }

# MM1.3 is a measurement (05 section 12): record it with raw samples.
python3 - "$d/mm13.json" "${COURT_EVIDENCE_DIR:-$d}/measurements.json" <<'PY' \
    || { echo "could not write measurements.json" >&2; exit 1; }
import json, subprocess, sys
rec = json.load(open(sys.argv[1]))
try:
    machine = subprocess.run(["tools/sysinfo.sh"], capture_output=True,
                           text=True).stdout.strip()
except Exception as e:
    machine = f"sysinfo unavailable: {e}"
rec["command"] = "make check-M1.4"
rec["machine"] = machine
json.dump(rec, open(sys.argv[2], "w"), indent=2)
PY
