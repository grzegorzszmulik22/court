# Recorded Claude Code hook fixtures (spec `02` §4.2)

Recorded once by the supervisor on 2026-11-16 from a real session of **Claude Code 2.1.325** (the version pinned in `tools/versions.env`; npm `@anthropic-ai/claude-code@2.1.325`, integrity `sha512-OUxCb0sx734zJh9zGTP9oF01gZ44DCGFaw7hgD4byepjRRGyvHzgUOwC7dlCgVIljV6CiEVDjq+anY3GUvKFQw==`), on Windows 11, model `claude-sonnet-5-5`, signed in with a claude.ai subscription. Nothing here is written by hand.

## How

- `world/` was a throwaway git repository holding `calc.py` and `test_calc.py`.
- The session was started in it with the launch command of `02` §4.1:
  `claude -p --output-format stream-json --verbose --input-format stream-json --session-id <session-id> --settings settings.json --mcp-config mcp.json --strict-mcp-config --permission-mode default --model sonnet < input.jsonl`
- `settings.json` registers HTTP hooks for the seven events Court consumes, posting to `http://127.0.0.1:47613/hook/fixturetoken01`, with the `allowedHttpHookUrls` entry they need.
- `recorder.py` received the posts. It answered as Court will: `PreToolUse` gets `allow`, except a Bash command containing `curl`, which gets Court's `deny`.
- `input.jsonl` is the one user message: read a missing file, edit `calc.py`, run the test, try `curl`, start a subagent that reads the file and runs the test, then reply.

## What is here

- `hooks/NNN-<event>[-sub].json`: one file per POST, in arrival order. Each holds the method, path, headers, `raw_body` (the bytes received, as text), `body` (parsed), and the recorder's reply. `-sub` means the post carried `agent_id` (it came from inside the subagent).

| Files | Exchange |
|---|---|
| 002 | `UserPromptSubmit` |
| 003–004 | `Read` of a missing file: `PreToolUse`, then `PostToolUseFailure` |
| 005–008 | `Read` then `Edit` of `calc.py`: `PreToolUse` and `PostToolUse` each |
| 009–010 | `Bash` `python test_calc.py`: `PreToolUse`, `PostToolUse` |
| 011 | `Bash` `curl …`: `PreToolUse` answered `deny`; no `PostToolUse` follows |
| 012, 019 | `Agent` tool call: `PreToolUse`, and its `PostToolUse` after the subagent ends |
| 013–018 | Inside the subagent: `SubagentStart`, `Read` and `Bash` pairs with `agent_id`/`agent_type`, `SubagentStop` |
| 020 | `Stop` |

- `stream.jsonl`: the session's stream-json output, for the adapter's stand-in executable (C1.7.3).
- `session-id`, `settings.json`, `mcp.json`, `input.jsonl`, `recorder.py`, `redact.py`: the inputs and tools, so the recording can be repeated at a new pinned version.

## Redactions (by `redact.py`, nothing else changed)

- The recording user's Windows username in paths is replaced by `fixture` everywhere, including inside `raw_body`.
- In `stream.jsonl`, the lists that enumerate the recording user's own installed setup are emptied, keeping their keys: `init.slash_commands`, `init.terminal_slash_commands`, `init.agents`, `init.skills`, `init.plugins`, `init.memory_paths`, `commands_changed.commands`.

Paths are Windows paths with backslashes, as Claude Code sends them on Windows. Field names do not depend on the operating system.
