# Lane `ledger` — progress

Row format of `spec/05-agent-loop.md` §13: `| chunk | status | attempts | last evidence folder | note |`.

| chunk | status | attempts | last evidence folder | note |
|---|---|---|---|---|
| M1.2 | DONE | 2 | evidence/M1.2/2026-11-15T17-02-47Z | 6/6 PASS; 3 sabotages, 0 uncaught; MM1.5 median 6844 ev/s (threshold 2000, CV 0.05); synchronous=NORMAL in WAL (FULL measured ~220 ev/s, under the bar) |
| M1.3 | DONE | 4 | evidence/M1.3/2026-11-15T17-31-34Z | 4/4 PASS; 3 sabotages, 0 uncaught; C1.3.1 200 ledgers + 2600 mutants + 4 tail cases, Go/Python classes identical; replay checked 24 reconciled verdicts; refledger stdlib-only (audited + `python3 -I`) |
