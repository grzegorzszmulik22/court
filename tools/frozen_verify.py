#!/usr/bin/env python3
"""frozen_verify.py <manifest> [root]

Verifies a FROZEN.sha256 manifest (spec/05-agent-loop.md section 7):

  * every listed file exists under <root> and its sha256 matches;
  * every file matching a `# frozen: <glob> ...` header line is listed.

Manifest format (sha256sum-compatible):
    # frozen: spec/0*.md spec/milestones/*.md
    <sha256>  <path relative to root>
    ...

Exits 0 when everything verifies; prints one line per violation and exits 1.
A missing file is a violation exactly like a changed one.
"""
import fnmatch
import glob
import hashlib
import os
import sys


def main() -> int:
    if len(sys.argv) < 2 or len(sys.argv) > 3:
        print("usage: frozen_verify.py <manifest> [root]", file=sys.stderr)
        return 2
    manifest, root = sys.argv[1], os.path.abspath(sys.argv[2] if len(sys.argv) == 3 else ".")
    bad = []
    listed = set()
    patterns = []
    try:
        lines = open(manifest, encoding="utf-8").read().splitlines()
    except OSError as e:
        print(f"frozen: cannot read {manifest}: {e}", file=sys.stderr)
        return 2
    for ln, line in enumerate(lines, 1):
        line = line.strip()
        if not line:
            continue
        if line.startswith("#"):
            words = line[1:].split()
            if words and words[0] == "frozen:":
                patterns.extend(words[1:])
            continue
        parts = line.split(None, 1)
        if len(parts) != 2:
            bad.append(f"line {ln}: malformed entry {line!r}")
            continue
        want, path = parts[0], parts[1].lstrip("* ")
        listed.add(path)
        full = os.path.join(root, path)
        if not os.path.isfile(full):
            bad.append(f"{path}: listed file is missing")
            continue
        got = hashlib.sha256(open(full, "rb").read()).hexdigest()
        if got != want:
            bad.append(f"{path}: sha256 differs ({got[:16]}… != {want[:16]}…)")
    for pat in patterns:
        for path in glob.glob(os.path.join(root, pat)):
            rel = os.path.relpath(path, root)
            if os.path.isfile(path) and rel not in listed:
                bad.append(f"{rel}: matches frozen pattern '{pat}' but is not listed")
    for b in bad:
        print(f"frozen: {b}")
    return 1 if bad else 0


if __name__ == "__main__":
    sys.exit(main())
