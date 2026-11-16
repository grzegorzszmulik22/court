# Lane `agents` — progress

Row format of `spec/05-agent-loop.md` §13: `| chunk | status | attempts | last evidence folder | note |`.

| chunk | status | attempts | last evidence folder | note |
|---|---|---|---|---|
| M1.5 | BLOCKED | 1 | evidence/M1.5/2026-11-16T16-13-43Z | 5/6 PASS (C1.5.1 mixed script 500 events in order, C1.5.2 unknown token + cross-session + idempotent retry + restart dedupe, C1.5.3 no contents/output over all fixtures, C1.5.4 subagent child attribution, C1.5.5 loopback-only); C1.5.6 BLOCKED: recorded hook fixtures not on main yet — this host cannot run a real Claude Code 2.1.325 session, so test/vectors/hooks/documented/ holds generated documented-shape stand-ins and the drift check awaits supervisor-recorded fixtures; 3/3 sabotages caught (0 uncaught) |
| M1.6 | DONE | 1 | evidence/M1.6/2026-11-16T16-29-08Z | 5/5 PASS (C1.6.1 validation incl. bad-category list, C1.6.2 1,000 interleaved calls / 8 agents zero cross-attribution, C1.6.3 child alarm direct-to-daemon through a dropping parent, C1.6.4 pinned revision 2026-07-28 stateless no session header, C1.6.5 one-sentence descriptions ≤200 chars); internal/tools on official go-sdk v1.8.0, 3 tools only; 3/3 sabotages caught (0 uncaught) |
