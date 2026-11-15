#!/usr/bin/env python3
"""lint_progress.py spec/progress/*.md

Lints the progress rows of spec/05-agent-loop.md section 13:
    | chunk | status | attempts | last evidence folder | note |

Checks: five fields, chunk id shape, status in the allowed set, attempts is a
non-negative integer when present, and a DONE row names a non-empty evidence
folder that exists and contains files. Exits 1 on the first invalid file list,
after printing every violation.
"""
import glob
import os
import re
import sys

STATUSES = {"TODO", "IN_PROGRESS", "DONE", "BLOCKED", "SKIPPED-ENV", "AWAITING-GATE"}
STATUS_RE = re.compile(r"^BLOCKED-CR-[0-9]+$")
CHUNK_RE = re.compile(r"^[MG][0-9]+(\.[0-9]+)?$")


def lint_file(path):
    errors = []
    for i, raw in enumerate(open(path, encoding="utf-8"), 1):
        s = raw.strip()
        if not s.startswith("|"):
            continue
        fields = [f.strip() for f in s.split("|")[1:-1]]
        if len(fields) < 5:
            errors.append(f"{path}:{i}: row has {len(fields)} fields, want 5")
            continue
        head = fields[0]
        if head.lower() == "chunk" or set(head) <= set("-: "):
            continue
        chunk, status, attempts, ev, note = fields[:5]
        if not CHUNK_RE.match(chunk):
            errors.append(f"{path}:{i}: bad chunk id {chunk!r}")
        if status not in STATUSES and not STATUS_RE.match(status):
            errors.append(f"{path}:{i}: bad status {status!r}")
        if attempts and not attempts.isdigit():
            errors.append(f"{path}:{i}: attempts {attempts!r} is not an integer")
        if status == "DONE":
            if not ev:
                errors.append(f"{path}:{i}: DONE row without an evidence folder")
            elif not os.path.isdir(ev) or not any(os.scandir(ev)):
                errors.append(f"{path}:{i}: evidence folder {ev!r} missing or empty")
    return errors


def main():
    bad = 0
    for pat in sys.argv[1:] or ["spec/progress/*.md"]:
        for path in sorted(glob.glob(pat)):
            for e in lint_file(path):
                print(f"lint-progress: {e}")
                bad += 1
    return 1 if bad else 0


if __name__ == "__main__":
    sys.exit(main())
