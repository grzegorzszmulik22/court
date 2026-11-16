# Lane `worlds` — progress

Row format of `spec/05-agent-loop.md` §13: `| chunk | status | attempts | last evidence folder | note |`.

| chunk | status | attempts | last evidence folder | note |
|---|---|---|---|---|
| M1.4 | SKIPPED-ENV | 7 | evidence/M1.4/2026-11-16T13-44-56Z | review 3: Windows-path bug fixed — C1.4.4's core.hooksPath assertion now compares filepath.Clean paths against the data dir's own hooks dir (<data>/hooks), not a '/hooks' suffix; swept world tests for other '/'-joined path expectations — none remain (all path ops already filepath/ToSlash); GOOS=windows vet+test-build green (C1.4.8 PASS); MM1.3 200/200, 6/6 sabotages caught, lint+regress green; C1.4.7 SKIPPED-ENV: no Windows host |
