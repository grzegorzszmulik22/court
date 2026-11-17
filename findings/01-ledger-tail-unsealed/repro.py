#!/usr/bin/env python3
"""Produce three tampered copies of a JSONL Court ledger.

The chain is unsealed at its newest event: the last event can be edited,
deleted or followed by a forged event, and no earlier hash changes. Each
output file verifies with both `court ledger verify` and refledger, and
`court replay` passes (a JSONL export keeps no derived state to compare).

Usage: repro.py <ledger.jsonl> <outdir>
"""

import hashlib
import importlib.machinery
import importlib.util
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(os.path.dirname(HERE))


def refledger():
    path = os.path.join(ROOT, "ref", "python", "refledger")
    loader = importlib.machinery.SourceFileLoader("refledger_repro", path)
    spec = importlib.util.spec_from_loader("refledger_repro", loader)
    mod = importlib.util.module_from_spec(spec)
    loader.exec_module(mod)
    return mod


def canon_line(m, ev):
    return m.canon(m.Obj([
        ("seq", m.Num(str(ev["seq_i"]))), ("prev", ev["prev"]),
        ("at", ev["at"]), ("commission", ev["commission"]),
        ("actor", ev["actor"]), ("type", ev["type"]),
        ("body", ev["body"]), ("hash", ev["hash"]),
    ]))


def edit_first_string(m, v, tag):
    if isinstance(v, str):
        return v + tag, True
    if isinstance(v, m.Obj):
        for i, (k, x) in enumerate(v.pairs):
            new, done = edit_first_string(m, x, tag)
            if done:
                v.pairs[i] = (k, new)
                return v, True
    if isinstance(v, m.Arr):
        for i, x in enumerate(v.items):
            new, done = edit_first_string(m, x, tag)
            if done:
                v.items[i] = new
                return v, True
    return v, False


def main():
    if len(sys.argv) != 3:
        sys.exit(__doc__)
    src, outdir = sys.argv[1], sys.argv[2]
    m = refledger()
    os.makedirs(outdir, exist_ok=True)
    lines = open(src, "rb").read().splitlines()
    last = m.decode_envelope(lines[-1])

    # 1. Edit the newest event's body and recompute only its own hash.
    ev = dict(last)
    ev["body"] = m.decode_envelope(lines[-1])["body"]
    _, done = edit_first_string(m, ev["body"], "-CANARY-EDITED")
    if not done:
        sys.exit("no string in the last body to edit")
    ev["hash"] = hashlib.sha256(m.canon_of(ev)).hexdigest()
    edited = lines[:-1] + [canon_line(m, ev)]
    open(os.path.join(outdir, "edited.jsonl"), "wb").write(b"\n".join(edited) + b"\n")

    # 2. Delete the newest event.
    deleted = lines[:-1]
    open(os.path.join(outdir, "deleted.jsonl"), "wb").write(b"\n".join(deleted) + b"\n")

    # 3. Append an event nobody wrote: a human ruling on D1.
    n = last["seq_i"] + 1
    forged = {
        "seq_i": n, "prev": last["hash"], "at": "2026-11-15T00:00:09.999Z",
        "commission": last["commission"], "actor": "human",
        "type": "decision.ruled",
        "body": m.Obj([("decision", "D1"), ("option", "o1")]),
        "hash": "",
    }
    forged["hash"] = hashlib.sha256(m.canon_of(forged)).hexdigest()
    open(os.path.join(outdir, "forged.jsonl"), "wb").write(
        b"\n".join(lines + [canon_line(m, forged)]) + b"\n")

    for name in ("edited.jsonl", "deleted.jsonl", "forged.jsonl"):
        print(os.path.join(outdir, name))


if __name__ == "__main__":
    main()
