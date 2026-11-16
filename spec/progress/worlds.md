# Lane `worlds` — progress

Row format of `spec/05-agent-loop.md` §13: `| chunk | status | attempts | last evidence folder | note |`.

| chunk | status | attempts | last evidence folder | note |
|---|---|---|---|---|
| M1.4 | SKIPPED-ENV | 6 | evidence/M1.4/2026-11-16T13-17-02Z | review 2: Discard test made discriminating — git shim locks .git read-only right after branch -D so ONLY the packed-refs removal fails; verified it FAILS with the fix reverted (Discard returned nil) and PASSes with it (reports packed-refs residue error); MM1.3 200/200, 6/6 sabotages caught, lint+regress green; C1.4.7 SKIPPED-ENV: no Windows host |
