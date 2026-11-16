# Lane `ledger` — progress

Row format of `spec/05-agent-loop.md` §13: `| chunk | status | attempts | last evidence folder | note |`.

| chunk | status | attempts | last evidence folder | note |
|---|---|---|---|---|
| M1.2 | DONE | 3 | evidence/M1.2/2026-11-16T10-54-58Z | 6/6 PASS; 3 sabotages, 0 uncaught; C1.2.6 single measurement, all 5 samples + median + CV recorded; MM1.5 median 7538 ev/s (threshold 2000), CV 0.31 — marked UNSTABLE: variance is ambient host contention (parallel lanes), median still 3.7x over bar; quiet-window samples reach 10k; synchronous=NORMAL in WAL |
| M1.3 | DONE | 7 | evidence/M1.3/2026-11-16T10-30-43Z | 4/4 PASS; 6 sabotages, 0 uncaught (incl. lone-low-surrogate disables on each verifier); 24 mutant kinds incl. all four invalid-surrogate forms, 200 ledgers / 4,805 mutants, 0 class disagreements; 11 rejecting vectors pin every strict-decoding class in both verifiers; evidence = seeded manifest + summary |
