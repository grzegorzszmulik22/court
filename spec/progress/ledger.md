# Lane `ledger` — progress

Row format of `spec/05-agent-loop.md` §13: `| chunk | status | attempts | last evidence folder | note |`.

| chunk | status | attempts | last evidence folder | note |
|---|---|---|---|---|
| M1.2 | DONE | 2 | evidence/M1.2/2026-11-15T17-02-47Z | 6/6 PASS; 3 sabotages, 0 uncaught; MM1.5 median 6844 ev/s (threshold 2000, CV 0.05); synchronous=NORMAL in WAL (FULL measured ~220 ev/s, under the bar) |
| M1.3 | DONE | 5 | evidence/M1.3/2026-11-15T18-25-24Z | 4/4 PASS; 4 sabotages, 0 uncaught (incl. refledger dup-key disable); 21 mutant kinds, 200 ledgers / 4,200 mutants, 0 class disagreements; 8 rejecting golden vectors pin every strict-decoding class in both verifiers; evidence = seeded manifest + summary |
