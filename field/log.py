#!/usr/bin/env python3
"""Court M0.1: log one intervention on an agent lane.

Run it each time you correct, approve, reject, restart or redirect an agent:

    python log.py                 # asks six questions, appends one line
    python log.py --summary       # the three numbers gate G0 needs

One line per intervention goes to interventions.jsonl beside this file.
Spec: spec/milestones/M0-field-evidence.md.
"""
import json
import sys
from datetime import datetime, timezone
from pathlib import Path

LOG = Path(__file__).with_name("interventions.jsonl")


def read_lines():
    if not LOG.exists():
        return []
    return [json.loads(x) for x in LOG.read_text(encoding="utf-8").splitlines() if x.strip()]


def ask(prompt, choices=None, default=None, allow_empty=False):
    hint = ""
    if choices:
        hint = " [" + "/".join(choices) + "]"
    if default is not None:
        hint += f" (Enter = {default})"
    while True:
        answer = input(f"{prompt}{hint}: ").strip()
        if not answer and default is not None:
            return default
        if not answer and allow_empty:
            return ""
        if not answer:
            continue
        if choices:
            match = [c for c in choices if c.startswith(answer.lower())]
            if len(match) == 1:
                return match[0]
            print("  one of: " + ", ".join(choices))
            continue
        return answer


def add():
    lines = read_lines()
    print(f"Intervention #{len(lines) + 1}  (Ctrl-C to cancel)\n")
    last_lane = lines[-1]["lane"] if lines else None
    entry = {
        "n": len(lines) + 1,
        "at": datetime.now(timezone.utc).isoformat(timespec="seconds"),
        "lane": ask("Lane or agent", default=last_lane),
        "what": ask("What did you do or decide (one sentence)"),
        "raised_by_agent": ask("Did the agent raise it (asked or reported), or did you find it", ["yes", "no"]),
        "kind": ask(
            "Kind: fact = a test or reviewer could have settled it; will = a preference or an accepted risk",
            ["fact", "will", "unsure"],
        ),
    }
    repeat = ask("Repeat of an earlier intervention? Its number, or Enter for no", allow_empty=True)
    entry["repeat_of"] = int(repeat) if repeat.isdigit() else None
    minutes = ask("Minutes it took you", default="1")
    entry["minutes"] = float(minutes) if minutes.replace(".", "", 1).isdigit() else None
    entry["if_missed"] = ask("What would have happened had nobody intervened", allow_empty=True)
    with LOG.open("a", encoding="utf-8") as f:
        f.write(json.dumps(entry, ensure_ascii=False) + "\n")
    print(f"\nSaved as #{entry['n']} in {LOG.name}.")


def share(part, whole):
    return f"{part}/{whole} ({100 * part / whole:.0f}%)" if whole else "0/0"


def summary():
    lines = read_lines()
    n = len(lines)
    not_raised = sum(1 for x in lines if x["raised_by_agent"] == "no")
    will = sum(1 for x in lines if x["kind"] == "will")
    repeats = sum(1 for x in lines if x.get("repeat_of"))
    days = len({x["at"][:10] for x in lines})
    print(f"{n} interventions over {days} day(s); gate G0 wants at least 50 over 7 working days.")
    print(f"Not raised by the agent : {share(not_raised, n)}   supports the plan if >= 20%")
    print(f"Questions of will       : {share(will, n)}   supports the plan if >= 30%")
    print(f"Repeats of an earlier   : {share(repeats, n)}   bounds how far laws can shrink the queue")
    print(f"Minutes spent           : {sum(x.get('minutes') or 0 for x in lines):.0f}")


if __name__ == "__main__":
    try:
        summary() if "--summary" in sys.argv else add()
    except KeyboardInterrupt:
        print("\nCancelled; nothing saved.")
