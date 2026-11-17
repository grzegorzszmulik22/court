#!/usr/bin/env python3
"""Edit the newest event in a Court SQLite ledger and blank its hash column.

Court's reader (internal/ledger/verify.go:readDBLines -> Event.Line)
recomputes an empty hash instead of rejecting it, so the edited row is
reported valid. refledger, which keeps the stored hash, rejects the same
file ("envelope ... hash is not sha256 hex").

Usage: tamper.py <ledger.db>
"""

import json
import sqlite3
import sys


def edit_first_string(v, tag):
    if isinstance(v, str):
        return v + tag, True
    if isinstance(v, dict):
        for k, x in v.items():
            new, done = edit_first_string(x, tag)
            if done:
                v[k] = new
                return v, True
    if isinstance(v, list):
        for i, x in enumerate(v):
            new, done = edit_first_string(x, tag)
            if done:
                v[i] = new
                return v, True
    return v, False


def main():
    if len(sys.argv) != 2:
        sys.exit(__doc__)
    con = sqlite3.connect(sys.argv[1])
    seq, body = con.execute(
        "SELECT seq, body FROM ledger ORDER BY seq DESC LIMIT 1").fetchone()
    doc = json.loads(body)
    _, done = edit_first_string(doc, "-CANARY-EDITED")
    if not done:
        sys.exit("no string in the last body to edit")
    con.execute("UPDATE ledger SET body=?, hash='' WHERE seq=?",
                (json.dumps(doc, separators=(",", ":"), sort_keys=True), seq))
    con.commit()
    print("edited seq %d and blanked its hash: %s" %
          (seq, json.dumps(doc, separators=(",", ":"), sort_keys=True)))


if __name__ == "__main__":
    main()
