"""Redact a recorded fixture set: the Windows username in paths, and the user's own setup lists.

Usage: python redact.py <recording dir> <out dir> <username>
Hook posts keep every field; only the username inside strings changes.
The stream keeps every message and key; list values that enumerate the
recording user's installed plugins, skills, agents and commands are emptied.
"""
import glob
import json
import os
import shutil
import sys

SRC, OUT, USER = sys.argv[1], sys.argv[2], sys.argv[3]
SETUP_LISTS = {"slash_commands", "terminal_slash_commands", "agents", "skills", "plugins",
               "commands", "added", "removed"}


def scrub(v):
    if isinstance(v, str):
        return v.replace(USER, "fixture")
    if isinstance(v, list):
        return [scrub(x) for x in v]
    if isinstance(v, dict):
        return {k: scrub(x) for k, x in v.items()}
    return v


os.makedirs(os.path.join(OUT, "hooks"), exist_ok=True)
for p in sorted(glob.glob(os.path.join(SRC, "hooks", "*.json"))):
    r = json.load(open(p, encoding="utf-8"))
    r = scrub(r)  # raw_body is a string: only the username inside it changes
    with open(os.path.join(OUT, "hooks", os.path.basename(p)), "w", encoding="utf-8", newline="\n") as f:
        json.dump(r, f, indent=1, ensure_ascii=False)
        f.write("\n")

emptied = set()
with open(os.path.join(OUT, "stream.jsonl"), "w", encoding="utf-8", newline="\n") as f:
    for line in open(os.path.join(SRC, "stream.jsonl"), encoding="utf-8"):
        m = json.loads(line)
        if m.get("type") == "system" and m.get("subtype") in ("init", "commands_changed"):
            for k in list(m):
                if k in SETUP_LISTS and isinstance(m[k], list):
                    m[k] = []
                    emptied.add(f"{m['subtype']}.{k}")
            if isinstance(m.get("memory_paths"), dict):
                m["memory_paths"] = {}
                emptied.add("init.memory_paths")
        f.write(json.dumps(scrub(m), ensure_ascii=False, separators=(",", ":")) + "\n")

for name in ("settings.json", "mcp.json", "input.jsonl", "recorder.py", "redact.py", "session-id"):
    shutil.copy(os.path.join(SRC, name), os.path.join(OUT, name))
print("emptied:", ", ".join(sorted(emptied)))
