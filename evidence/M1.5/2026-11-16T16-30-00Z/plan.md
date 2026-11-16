# M1.5 plan — scripted agent and the hook endpoint

Files: `internal/daemon` (server, /hook/<token>, /gate, admin, dedupe, refusals log),
`internal/adapter` (interface), `internal/adapter/scripted` (script.yaml runner emitting
Claude-shaped hook posts), `internal/adapter/claude` (documented hook shapes, field table,
deny policy used by the hook endpoint), `cmd/court` (`daemon`, `gate-post`), `tools/hook-fields.txt`,
`test/vectors/hooks/documented/` (generated, clearly NOT recorded) + `recorded/` layout,
`test/checks/M1.5/`, `test/sabotage/M1.5/`, `docs/M1.5.md`.

Checks: C1.5.1 (500 mixed steps -> exact tool.event+world.commit order), C1.5.2 (unknown token
refused+recorded), C1.5.3 (no file contents / command output stored, over every fixture),
C1.5.4 (subagent payload -> child agent), C1.5.5 (loopback only), C1.5.6 (fixture drift ->
exit 78 'recorded hook fixtures not on main yet' until test/vectors/hooks/recorded/ exists).

First test: C1.5.1's scripted run — fails now (no daemon/adapter packages).
