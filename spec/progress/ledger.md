# Lane `ledger` — progress

Row format of `spec/05-agent-loop.md` §13: `| chunk | status | attempts | last evidence folder | note |`.

| chunk | status | attempts | last evidence folder | note |
|---|---|---|---|---|
| M1.2 | IN_PROGRESS | 2 | evidence/M1.2/2026-11-15T17-02-47Z | supervisor: C1.2.6 retries a measurement until it passes, which M1.1 denies; see lane note |
| M1.3 | DONE | 7 | evidence/M1.3/2026-11-15T20-43-42Z | 4/4 PASS; 6 sabotages, 0 uncaught (incl. lone-low-surrogate disables on each verifier); 24 mutant kinds incl. all four invalid-surrogate forms, 200 ledgers / 4,805 mutants, 0 class disagreements; 11 rejecting vectors pin every strict-decoding class in both verifiers; evidence = seeded manifest + summary |
