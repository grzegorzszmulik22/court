# Lane `ledger` — progress

Row format of `spec/05-agent-loop.md` §13: `| chunk | status | attempts | last evidence folder | note |`.

| chunk | status | attempts | last evidence folder | note |
|---|---|---|---|---|
| M1.2 | DONE | 3 | evidence/M1.2/2026-11-15T21-12-53Z | 6/6 PASS; 3 sabotages, 0 uncaught; C1.2.6 retry reverted — single measurement, all 5 samples + median + CV recorded; MM1.5 median 6310 ev/s (threshold 2000, CV 0.05); synchronous=NORMAL in WAL |
| M1.3 | DONE | 7 | evidence/M1.3/2026-11-15T20-43-42Z | 4/4 PASS; 6 sabotages, 0 uncaught (incl. lone-low-surrogate disables on each verifier); 24 mutant kinds incl. all four invalid-surrogate forms, 200 ledgers / 4,805 mutants, 0 class disagreements; 11 rejecting vectors pin every strict-decoding class in both verifiers; evidence = seeded manifest + summary |
