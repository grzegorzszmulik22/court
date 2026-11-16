# M1 exit review

## Would a technical lead learn something an approval inbox could not show?

Yes. The load run proves Court sees what an agent never asks about:
10,504 tool events and 504 commits across 8 concurrent agents, every one
attributed to the right token (never to a session_id or parent), each
acknowledged only after a durable ledger write, and zero lost or doubled
under a mid-run kill. The weakest point is that child-agent alarms depend
on PreToolUse pairing, so a tool call whose hook never fires attributes
to the session agent — recorded, but coarser than ideal.

## Could a developer who has never heard of Court use this in ten minutes, without a manual or a config file?

Mostly yes: `court daemon` is the only command, the world, hooks,
settings and MCP config are generated per session into the data
directory, and nothing is written to the repository or asked of the
agent. The weakest point is the same one the lane is stopped on — a real
Claude Code session (L1.7.1 smoke and recorded hook fixtures) has not run
on this host, so "ten minutes" is proven for the scripted agent, not yet
for the live one.
