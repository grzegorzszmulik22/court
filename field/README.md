# Field evidence (M0)

Human work. Spec: `../spec/milestones/M0-field-evidence.md`.

## M0.1 Intervention log

Each time you correct, approve, reject, restart or redirect an agent lane:

```bash
python log.py
```

Six questions, about 30 seconds. Log at the moment, not from memory at the end of the day. After seven working days and at least 50 entries:

```bash
python log.py --summary
```

## M0.2 Decision labels

`history/` holds one YAML file per agent-authored commit, in the format given in the milestone file. Choose commits by date range, not by interest, and write the range here before labelling:

- Date range: *to be filled in*
- Repository and lanes: *to be filled in*

## Kept out of the build repository

`interventions.jsonl` and `history/` are in `.gitignore`. The implementation agent must not see the labelled history before M3.9 (`M3` check C3.9.4), so these files stay on this machine.
