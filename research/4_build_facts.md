# 4. Build facts: integration surfaces (read 2026-11-15)

All facts below were read from the cited pages on 2026-11-15. "Not documented" means the pages I read do not state it. UNCONFIRMED marks anything I could not confirm from an official source. Version numbers quoted are the ones the docs themselves print.

---

## 1. Claude Code

Source: official (code.claude.com/docs; markdown versions at `<page>.md`, index at https://code.claude.com/docs/llms.txt)

### 1a. Hooks — https://code.claude.com/docs/en/hooks.md

**Events (33):** `SessionStart`, `Setup`, `InstructionsLoaded`, `UserPromptSubmit`, `UserPromptExpansion`, `MessageDisplay`, `PreToolUse`, `PermissionRequest`, `PermissionDenied`, `PostToolUse`, `PostToolUseFailure`, `PostToolBatch`, `Notification`, `SubagentStart`, `SubagentStop`, `TaskCreated`, `TaskCompleted`, `Stop`, `StopFailure`, `TeammateIdle`, `ConfigChange`, `CwdChanged`, `DirectoryAdded`, `FileChanged`, `WorktreeCreate`, `WorktreeRemove`, `PreCompact`, `PostCompact`, `PreModelSwitch`, `PostModelSwitch`, `Elicitation`, `ElicitationResult`, `SessionEnd`.

**Handler types (`type`):** `"command"` (JSON on stdin; exit code + stdout), `"http"` (JSON as POST body, `Content-Type: application/json`; response body uses the same JSON output format), `"mcp_tool"` (calls a tool on a configured MCP server; fields `server`, tool), `"prompt"`, `"agent"` (experimental).
- Common handler fields: `type`, `if` (one permission rule, e.g. `"Bash(git *)"`; tool events only), `timeout` (seconds; defaults 600 for command/http/mcp_tool, 30 prompt, 60 agent; 30 on `UserPromptSubmit`; `SessionEnd` shares a 1.5 s budget), `statusMessage`, `once`.
- Command fields: `command`, `args` (exec form, no shell), `async`, `asyncRewake`, `shell` (`"bash"` | `"powershell"`), `onFailure` (`"continue"` | `"block"`, needs v2.1.295+).
- HTTP fields: `url`, `headers`, `allowedEnvVars`, `onFailure`. Settings `allowedHttpHookUrls` and `httpHookAllowedEnvVars` gate HTTP hooks.
- Matching hooks for one event run **in parallel**. Identical handler in several settings files runs once.

**Where configured:** `~/.claude/settings.json`, `.claude/settings.json`, `.claude/settings.local.json`, managed policy settings, plugin `hooks/hooks.json`, skill frontmatter, subagent frontmatter. Shape: `{"hooks": {"<Event>": [{"matcher": "...", "hooks": [{"type": ...}]}]}}`. Hook entries **merge** across levels. `--settings <file-or-inline-JSON>` overrides/adds settings for one session (cli-reference). `--bare` and `--safe-mode` skip hooks. `disableAllHooks` setting exists; `allowManagedHooksOnly` (managed) blocks user/project/plugin hooks.

**Common stdin fields:** `session_id`, `prompt_id` (absent until first user input), `transcript_path`, `cwd`, `scratchpad_dir` (v2.1.257+), `permission_mode` (`default|plan|acceptEdits|auto|dontAsk|bypassPermissions`; not on every event), `effort.level`, `hook_event_name`; plus `agent_id` and `agent_type` when the hook fires inside a subagent / teammate / `--agent` session. Env var `CLAUDE_PROJECT_DIR` is set for hooks.

**Per-event stdin (additional fields):**
- `PreToolUse`: `tool_name`, `tool_input`, `tool_use_id`; MCP tools also `mcp_server` `{name, source}`. File tool paths are always absolute; on Windows with backslashes. Does not fire for `@`-file mentions or `EndConversation`.
- `PostToolUse`: `tool_name`, `tool_input`, `tool_response` (structured tool output), `tool_use_id`, `duration_ms` (optional).
- `PostToolUseFailure`: `tool_name`, `tool_input`, `tool_use_id`, `error`, `is_interrupt`, `duration_ms`. Not fired for validation rejections or permission denials.
- `PostToolBatch`: `tool_calls[]` each `{tool_name, tool_input, tool_use_id, tool_response}` (here `tool_response` is the serialized `tool_result` the model sees). No matcher.
- `UserPromptSubmit`: `prompt`, optional `session_title`. Also fires for scheduled-task turns, background-subagent reports, and messages from another session.
- `SessionStart`: `source` (`startup|resume|clear|compact|fork`), optional `model`, `agent_type`, `session_title`; on resume/fork also `seconds_since_last_response`, `context_tokens`, `prompt_cache_likely_expired`, `estimated_cache_write_usd` (v2.1.251+). Only `command` and `mcp_tool` handler types supported.
- `Stop`: `stop_hook_active`, `last_assistant_message`, `background_tasks[]` (`id,type,status,description,command,agent_type,server,tool,name`), `session_crons[]`. Not fired on user interrupt; API errors fire `StopFailure`.
- `SubagentStart`: `agent_id`, `agent_type` (matcher = agent type).
- `SubagentStop`: `stop_hook_active`, `agent_id`, `agent_type`, `agent_transcript_path` (nested `subagents/agent-<id>.jsonl`), `last_assistant_message`, `background_tasks`, `session_crons`. Also fires for internal agents (prompt suggestions, `/btw`).
- `TaskCreated` / `TaskCompleted`: `task_id`, `task_subject`, optional `task_description`, `teammate_name`, `team_name` (deprecated), `agent_id` (v2.1.290+). No matcher.
- `Notification`: `message`, optional `title`, `notification_type` (`permission_prompt`, `idle_prompt`, `auth_success`, ...).
- `SessionEnd`: `reason` (`clear|resume|logout|prompt_input_exit|other`).
- `WorktreeCreate`: `name`; hook must return the worktree path (stdout last line, or HTTP `hookSpecificOutput.worktreePath`); it **replaces** default git behaviour.

**Blocking / modifying a tool call (PreToolUse):** exit 0 and print
`{"hookSpecificOutput": {"hookEventName": "PreToolUse", "permissionDecision": "allow"|"deny"|"ask"|"defer", "permissionDecisionReason": "...", "updatedInput": {...}, "additionalContext": "..."}}`.
- `updatedInput` replaces the **whole** input object. Precedence across hooks: `deny` > `defer` > `ask` > `allow`.
- Exit code `2` = block; stderr is the reason shown to Claude. Any other non-zero = non-blocking error (unless `onFailure: "block"`).
- `"defer"` (only in `-p` mode, only when the turn has a single tool call): the tool does not run, the turn ends with a result `stop_reason: "tool_deferred"` and `deferred_tool_use {id,name,input}`; host resumes with `claude -p --resume <session-id>`, the hook fires again and can return `allow` + `updatedInput`. No timeout. This is the documented pause-and-resume mechanism.
- Top-level `{"continue": false, "stopReason": "..."}` stops Claude entirely (does not by itself block a PreToolUse call).
- `PermissionRequest` hooks: `hookSpecificOutput.decision.behavior` `allow|deny`, optional `updatedInput`.
- `PostToolUse`: `decision: "block"` + `reason` (adds reason next to result), `updatedToolOutput` (replace what Claude sees), `additionalContext`.
- `Stop` / `SubagentStop`: `decision: "block"` + `reason` keeps the agent working; cap of 8 consecutive continuations (`CLAUDE_CODE_STOP_HOOK_BLOCK_CAP`).
- `UserPromptSubmit`: `decision: "block"` rejects the prompt; cannot rewrite it.
- `TaskCreated`: exit 2 or `decision: "block"` deletes the task. `TaskCompleted`: exit 2 prevents completion.

**Injecting context back to the model:** `hookSpecificOutput.additionalContext` (wrapped as a system reminder at the point the hook fired) on `SessionStart`, `SubagentStart`, `UserPromptSubmit`, `UserPromptExpansion`, `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `PostToolBatch`, `Stop`, `SubagentStop`, `PostModelSwitch`. Plain stdout is also added as context on `UserPromptSubmit`, `UserPromptExpansion`, `SessionStart`, `PostModelSwitch`. Cap 10,000 characters per string (overflow goes to a file with a 2,000-char preview). `SessionStart` can also return `initialUserMessage`, `sessionTitle`, `watchPaths`, `reloadSkills`. Docs advise factual statements, not imperative "system" commands.

### 1b. Headless mode — https://code.claude.com/docs/en/headless.md , https://code.claude.com/docs/en/cli-reference.md , https://code.claude.com/docs/en/permission-modes.md

- Non-interactive: `claude -p "<prompt>"` (`--print`). Exit 0 on success. Rejects `--bg`.
- Output: `--output-format text|json|stream-json`. `stream-json` needs `--verbose`; add `--include-partial-messages` (token deltas as `stream_event`), `--include-hook-events` (`hook_started`/`hook_progress`/`hook_response`), `--forward-subagent-text` (subagent text/thinking with `parent_tool_use_id`), `--replay-user-messages`.
- Stream content: first event `system/init` (model, tools, `mcp_servers[{name,status}]`, `mcp_server_errors`, `plugins`, `capabilities[]`); `assistant` / `user` messages; `system/api_retry`; `permission_denied` system messages; last line is a `result` message (response text, cost, `session_id`, `permission_denials`).
- Input: `--input-format text|stream-json`. With `stream-json` the process stays open and reads user messages from stdin (follow-up input to a running session). The line shape is documented as the SDK type `SDKUserMessage`: `{"type":"user","message":{"role":"user","content":...},"parent_tool_use_id":null, "priority":"now"|"next"|"later", "shouldQuery":false, ...}`.
- Resume: `--continue` / `-c` (most recent in cwd), `--resume <session-id | name | path-to-.jsonl>` / `-r`, `--fork-session`, `--session-id <uuid>` (choose the ID up front), `--no-session-persistence`. Session ID lookup spans all projects on the machine (v2.1.223+). Transcripts: `~/.claude/projects/<project>/<session-id>.jsonl`.
- Stop: SIGTERM → exit code 143, turn left unfinished, `SessionEnd` hooks run. SIGINT (or SDK `interrupt()`) ends the turn cleanly. `CLAUDE_CODE_RESUME_INTERRUPTED_TURN=1` continues an interrupted turn on resume.
- Permission modes (`--permission-mode`): `default` (alias `manual`), `acceptEdits`, `plan`, `auto`, `dontAsk`, `bypassPermissions`; `--dangerously-skip-permissions` = bypass. `claude -p` and Agent SDK runs start in `default` (v2.1.302+: calls needing approval are denied unless an allow rule, hook, `canUseTool`, or `--permission-prompt-tool` approves). `--permission-prompt-tool mcp__<server>__<tool>` delegates prompts to an MCP tool; `--permission-prompts none` (v2.1.259+) denies instead of waiting. `--allowedTools`, `--disallowedTools`, `--tools`.
- Working directory: no `--cwd` flag for a session is listed in the CLI reference (only `claude agents --cwd`); the session uses the process working directory. `--add-dir` adds directories. `--worktree`/`-w <name>` creates `<repo>/.claude/worktrees/<name>` on branch `worktree-<name>`.
- MCP config: `--mcp-config <file-or-JSON ...>` (shape `{"mcpServers": {"name": {"type": "http"|"sse"|"ws"|stdio-default, "url"|"command","args","env","headers"}}}`), `--strict-mcp-config` (ignore all other MCP sources). `-p` waits for servers up to `MCP_TIMEOUT` (30 s default). An entry with `url` but no `type` is skipped.
- Other: `--settings`, `--setting-sources user,project,local`, `--append-system-prompt[-file]`, `--agents <json>`, `--max-turns`, `--max-budget-usd`, `--json-schema`, `--bare`, `--name`.
- Background sessions: `claude --bg`, `claude attach|logs|stop|respawn|rm <id>`, `claude agents --json`.
- Channels (research preview): an MCP stdio server declaring capability `claude/channel` can push `notifications/claude/channel` `{content, meta}` into a running session; needs `--channels` (allowlisted plugins) or `--dangerously-load-development-channels server:<name>`; not available on MCP revision 2026-07-28 connections. https://code.claude.com/docs/en/channels-reference.md

### 1c. Agent SDK — https://code.claude.com/docs/en/agent-sdk/typescript.md , .../python.md , .../hooks.md , .../custom-tools.md , .../subagents.md

- Packages: TypeScript `@anthropic-ai/claude-agent-sdk` (bundles the native Claude Code binary; SDK v0.3.N bundles Claude Code v2.1.N), Python `claude-agent-sdk`. No Go SDK is documented; the SDKs drive the CLI as a subprocess (`spawnClaudeCodeProcess`, `pathToClaudeCodeExecutable`).
- Start: TS `query({prompt: string | AsyncIterable<SDKUserMessage>, options})` returns `Query` (an `AsyncGenerator<SDKMessage>`). Python `query()` or `ClaudeSDKClient` (`connect`, `query`, `receive_messages`, `receive_response`, `interrupt`, `set_permission_mode`, `stop_task`, `disconnect`).
- Key options: `cwd`, `mcpServers`, `strictMcpConfig`, `hooks`, `canUseTool`, `permissionMode`, `allowedTools`, `disallowedTools`, `resume`, `forkSession`, `sessionId`, `continue`, `settingSources`, `agents`, `includePartialMessages`, `includeHookEvents`, `forwardSubagentText`, `agentProgressSummaries`, `maxTurns`, `maxBudgetUsd`, `env`, `systemPrompt`.
- Messages (`SDKMessage` union, selected): `SDKAssistantMessage` (`type:"assistant"`, `message` with `tool_use` blocks, `parent_tool_use_id`, `agent_id`), `SDKUserMessage` (`tool_result` blocks, `tool_use_result` structured output), `SDKResultMessage`, `SDKSystemMessage` (init), `SDKPartialAssistantMessage`, `SDKToolProgressMessage` (`tool_progress`), `SDKHookStartedMessage`/`SDKHookProgressMessage`/`SDKHookResponseMessage`, `SDKTaskStartedMessage` (`task_started`: `task_id`, `run_id`, `tool_use_id`, `task_type` `local_bash|local_agent|mcp_task|remote_agent`, `is_backgrounded`, `spawn_depth`, `parent_task_id`), `SDKTaskProgressMessage`, `SDKTaskUpdatedMessage`, `SDKTaskNotificationMessage` (`status completed|failed|stopped`), `SDKPermissionDeniedMessage`, `SDKAPIRetryMessage`.
- Interrupt / steer: `Query.interrupt()` (streaming input mode only; returns `{still_queued[], cancelled?[]}` on CLIs advertising `interrupt_receipt_v1`), `streamInput(stream)`, `setPermissionMode()`, `setModel()`, `stopTask(taskId)`, `backgroundTasks()`, `setMcpServers()`, `close()`. A user message sent mid-turn with `priority: "now"` interrupts (or backgrounds running work when `origin: {kind:"human"}`); `"next"` (default) is read after current tool calls; `"later"` waits for turn end; `shouldQuery: false` appends context without a model call.
- Permission callback: `canUseTool(toolName, input, {toolUseID, agentID, requestId, ...})` → `{behavior:"allow", updatedInput?}` | `{behavior:"deny", message, interrupt?}`. Called only when the permission flow would prompt; use a `PreToolUse` hook to see every call.
- SDK hook callbacks: `hooks: {PreToolUse: [{matcher, hooks:[callback]}]}`; callback `(input, toolUseID, {signal})` returns the same JSON as shell hooks. Python supports only `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `UserPromptSubmit`, `Stop`, `SubagentStart`, `SubagentStop`, `PreCompact`, `PermissionRequest`, `Notification`; TypeScript supports all. A `PreToolUse` callback that times out blocks the call.
- Custom in-process tools: TS `tool(name, description, zodSchema, handler)` + `createSdkMcpServer({name, version, tools})`, passed in `mcpServers`; Python `@tool` + `create_sdk_mcp_server()`. Tool names appear as `mcp__<server>__<tool>`.

### 1d. Subagent visibility
- Hooks: "Hooks from settings files, managed policy settings, and plugins also run inside subagents. When a subagent calls a tool, tool events such as `PreToolUse` and `PostToolUse` fire the same configured hooks as in the main conversation, and the input carries the `agent_id` and `agent_type`" (hooks.md, Hook locations). `session_id` is the parent's.
- Stream/SDK: subagent `tool_use` and `tool_result` blocks appear by default as `assistant`/`user` messages with `parent_tool_use_id` = the `Agent` tool call ID (main thread = `null`); text/thinking only with `--forward-subagent-text` / `forwardSubagentText`; all nesting depths forwarded. TS messages also carry `agent_id` (= `task_id` of the `task_started` event; SDK v0.3.292+). Docs say match on `agent_id`, not `parent_tool_use_id`, once subagents are resumed.
- The tool is named `Agent` in `tool_use` blocks (`Task` in `system:init` tools list and before v2.1.63).
- Foreground `Agent` call result (`PostToolUse.tool_response`): `status`, `agentId`, `content`, `totalToolUseCount`, `totalDurationMs`, `totalTokens`. Background: `status: "async_launched"`, `agentId`, `outputFile`.

---

## 2. OpenAI Codex CLI

Source: official (developers.openai.com/codex → now served under learn.chatgpt.com/docs; `.md` appended gives markdown)

- **Hooks: yes, enabled by default.** https://developers.openai.com/codex/hooks.md
  - Events: `SessionStart`, `SessionEnd`, `SubagentStart`, `SubagentStop`, `PreToolUse`, `PermissionRequest`, `PostToolUse`, `PreModelRequest`, `PostModelRequest`, `PreCompact`, `PostCompact`, `UserPromptSubmit`, `Stop`, `Interrupt`.
  - Config: `~/.codex/hooks.json`, `~/.codex/config.toml` (`[hooks]`), `<repo>/.codex/hooks.json`, `<repo>/.codex/config.toml`; plugins. Same three-level shape as Claude Code (`event → matcher group → handlers`, `type: "command"`; MCP tool hooks also exist). Non-managed hooks must be reviewed and trusted; project hooks load only in trusted projects. Off switch: `[features] hooks = false`.
  - Stdin common fields: `session_id` ("Subagent hooks use the parent session id"), `transcript_path`, `cwd`, `hook_event_name`, `model`, `turn_id` (turn-scoped), `permission_mode`.
  - `PreToolUse` stdin: `turn_id`, `tool_name` (`Bash`, `apply_patch`, `mcp__server__tool`, function tool names), `tool_use_id`, `tool_input`. Deny: `hookSpecificOutput.permissionDecision: "deny"` + `permissionDecisionReason`, or legacy `{"decision":"block","reason":...}`, or exit 2 + stderr. Rewrite: `permissionDecision: "allow"` + `updatedInput`. `additionalContext` supported. `permissionDecision: "ask"` is parsed but **not supported**.
  - `PostToolUse` stdin adds `tool_response`; `decision: "block"` replaces the tool result with the feedback. `Stop` / `SubagentStop`: `stop_hook_active`, `last_assistant_message`; `decision:"block"` + `reason` continues.
  - Coverage gap stated in docs: hosted tools such as `WebSearch` do not fire `PreToolUse`/`PostToolUse`; "Treat tool hooks as a useful guardrail, not a complete enforcement boundary."
- **Event stream: yes.** `codex exec --json` prints JSONL on stdout: `thread.started` (`thread_id`), `turn.started`, `turn.completed` (`usage`), `turn.failed`, `item.started` / `item.completed` (item types: agent messages, reasoning, command executions, file changes, MCP tool calls, web searches, plan updates; sample `{"type":"item.started","item":{"id":"item_1","type":"command_execution","command":"bash -lc ls","status":"in_progress"}}`), `error`. https://developers.openai.com/codex/noninteractive.md
  - Default sandbox for `codex exec` is read-only; `--sandbox workspace-write|danger-full-access`; `--full-auto` removed; `--approve-for-me`; `--ephemeral`; `--skip-git-repo-check`; `--worktree`; global `--cd <DIR>` (`-C`) sets the working directory.
- **`notify`:** config key running an external program; only event `agent-turn-complete`. https://developers.openai.com/codex/config-advanced.md
- **MCP client: yes.** `[mcp_servers.<name>]` in `config.toml` (or `codex mcp add`); STDIO (`command`, `args`, `env`, `cwd`) and Streamable HTTP (`url`, `bearer_token_env_var`, `http_headers`, `env_http_headers`). `required = true` makes startup fail if the server fails. https://developers.openai.com/codex/mcp.md
- **Resume:** `codex exec resume --last "<prompt>"`, `codex exec resume <SESSION_ID>`, `codex exec fork <SESSION_ID>`; interactive `codex resume`, `codex fork`.
- **Steer a running session: only through the app server.** `codex app-server` (JSON-RPC 2.0 without the `jsonrpc` header; transports `stdio://` default JSONL, `ws://` "experimental and unsupported", `unix://`). Methods: `thread/start`, `thread/resume`, `thread/fork`, `turn/start`, `turn/steer` (`threadId`, `input`, `expectedTurnId`), `turn/interrupt` (`threadId`, `turnId`), `thread/inject_items`. Notifications: `turn/started`, `turn/completed`, `item/started`, `item/completed` (items `commandExecution`, `fileChange`, `mcpToolCall`, `collabToolCall`, ...), `hook/started`, `hook/completed`; approval requests are server→client requests. `dynamicTools` (client-executed tools) are experimental. https://developers.openai.com/codex/app-server.md
- **SDK:** `@openai/codex-sdk` (TS: `new Codex().startThread()`, `thread.run()`, `codex.resumeThread(id)`), Python library also documented. `codex mcp-server` has been **removed**. https://developers.openai.com/codex/sdk.md

---

## 3. Devin CLI (Cognition)

Source: official (docs.devin.ai; index https://docs.devin.ai/llms.txt)

- **Exists:** "Devin CLI, a local command-line coding agent". `devin [OPTIONS] [prompt]`. https://docs.devin.ai/cli/index.md , https://docs.devin.ai/cli/reference/commands.md
- Flags: `--print`/`-p` (non-interactive), `--continue`/`-c`, `--resume <SESSION_ID>`/`-r`, `--permission-mode normal|accept-edits|smart|dangerous|autonomous`, `--sandbox`, `--model`, `--prompt-file`, `--config`, `--export [PATH]` ("Export conversation to a file after each turn (ATIF format)"), `--respect-workspace-trust false` (needed for `-p` in untrusted dirs), `--cloud`. `devin list --format json` lists sessions.
- **Hooks: yes.** https://docs.devin.ai/cli/extensibility/hooks/overview.md , https://docs.devin.ai/cli/extensibility/hooks/lifecycle-hooks.md
  - Events: `PreToolUse`, `PostToolUse`, `PermissionRequest`, `UserPromptSubmit`, `Stop`, `PostCompaction`, `SessionStart`, `SessionEnd` (not fired in cloud sessions).
  - Types: `command`, `prompt`. Fields `matcher` (regex on `tool_name`), `command`, `prompt`, `timeout`.
  - Stdin: `hook_event_name`, `tool_name`, `tool_input`, `session_id`, `prompt_id`; `PostToolUse` adds `tool_response {success, output, error}`; `Stop` has `stop_hook_active`. Env `DEVIN_PROJECT_DIR`.
  - Output: `{"decision": "approve"|"block", "reason": ...}`; `hookSpecificOutput.additionalContext` (for `UserPromptSubmit`, `SessionStart`, `PostToolUse`); `hookSpecificOutput.updatedInput` (PreToolUse; **merged** into arguments, unlike Claude Code's full replace). Exit 0 ok, 2 block, other = logged error.
  - Locations: `.devin/hooks.v1.json` (whole file is the hooks object), `.devin/config.json`, `.devin/config.local.json`, `~/.config/devin/config.json` (`%APPDATA%\devin\config.json`), and Claude Code files (`.claude/settings.json`, `.claude/settings.local.json`, `~/.claude/settings.json`, `~/.claude.json`) when `read_config_from.claude` is enabled (default).
  - Tool names: `read`, `write`, `edit`, `apply_patch`, `grep`, `glob`, `exec`, `get_output`, `write_to_process`, `kill_shell`, `webfetch`, `ask_user_question`, `todo_write`, `exit_plan_mode`, `skill`, `run_subagent`, `read_subagent`, `request_scope`, `mcp__<server>__<tool>`.
- **MCP: yes.** stdio, Streamable HTTP (falls back to SSE on 4xx), SSE. `devin mcp add [-s project|user] <name> <URL | -- command>`; files `.devin/mcp_config.json`, `~/.config/devin/mcp_config.json` (key `mcpServers`, per-server `transport: "http"|"sse"`). https://docs.devin.ai/cli/extensibility/mcp/configuration.md
- **Session API reporting tool events:** `devin acp` runs an Agent Client Protocol server (JSON-RPC over stdin/stdout) for editors. https://docs.devin.ai/cli/reference/commands.md#devin-acp — the ACP message set itself is defined at agentclientprotocol.com, not in the Devin docs.
- Not documented: a streaming JSON event output for `devin -p`; `SubagentStart`/`SubagentStop` events; whether hooks fire for tool calls made inside subagents; `cwd`, `transcript_path` or `tool_use_id` in hook stdin; sending input to a running local session from another process (other than via ACP).

---

## 4. MCP server in Go

Source: official (github.com/modelcontextprotocol/go-sdk README, docs/protocol.md, GitHub releases API)

- Official SDK: `github.com/modelcontextprotocol/go-sdk` ("the official Go software development kit (SDK) for the Model Context Protocol"). Packages `mcp`, `jsonrpc`, `auth`, `oauthex`.
- Version: **v1.8.0**, published 2026-09-14 (latest release; v1.x = stable line since v1.0.0). v1.7.0 (2026-07-28) added protocol revision `2026-07-28`. Supported spec versions for v1.7.0+: `2026-07-28`, `2025-11-25`, `2025-06-18`, `2025-03-26`, `2024-11-05`.
- Transports: stdio (`mcp.StdioTransport` server side, `mcp.CommandTransport` client side); Streamable HTTP (`mcp.NewStreamableHTTPHandler(getServer func(*http.Request) *mcp.Server, *mcp.StreamableHTTPOptions)`, `StreamableServerTransport`, `StreamableClientTransport`); legacy SSE (`SSEHandler`); `InMemoryTransport`; custom via `mcp.Transport`.
- One process, several agent sessions: yes over Streamable HTTP. `StreamableHTTPHandler` "creates a new `StreamableServerTransport` for each new session"; each connection is a `ServerSession` (`ServerSession.ID`, reachable in handlers as `req.Session`). The `getServer` callback receives the `*http.Request`, so the server can be chosen per request (URL path, header).
- Caveat that affects telling sessions apart: revision `2026-07-28` is a **stateless** model (no `initialize` handshake; each request carries protocol version and capabilities in `_meta`; `Mcp-Session-Id` "removed in stateless mode"). The Go SDK accepts `2026-07-28` requests on Streamable HTTP **only** when `StreamableHTTPOptions.Stateless = true`, which "uses a temporary session to handle requests" and cannot make server→client requests. Claude Code's v2 MCP runtime (default on v2.1.232+/v2.1.274+) negotiates `2026-07-28` with HTTP and stdio servers that support it (https://code.claude.com/docs/en/mcp.md, "MCP client runtimes"; `MCP_SDK_GENERATION=v1` or `MCP_PROTOCOL_NEGOTIATION=legacy` forces the old handshake).
  - Consequence (my inference, not a doc statement): identify each agent by something the host controls in its MCP config — a per-agent URL path or a per-agent `headers` value (Claude Code `headers`, Codex `http_headers`, both documented) — not by `Mcp-Session-Id`.
- A stdio server is one process per client (the client spawns it), so stdio cannot share one process between agents.
- Roots, sampling and logging are deprecated as of `2026-07-28` (SEP-2577).

URLs: https://github.com/modelcontextprotocol/go-sdk , https://github.com/modelcontextprotocol/go-sdk/blob/main/docs/protocol.md , https://github.com/modelcontextprotocol/go-sdk/releases

---

## 5. git worktrees

Source: official (git-scm.com/docs/git-worktree, git-config, git-gc; gitforwindows.org for long paths)

- **One branch per worktree:** `git worktree add` "refuses to create a new worktree when `<commit-ish>` is a branch name and is already checked out by another worktree"; `--force` overrides. Use `-b <new-branch>` per worktree, or `-d`/`--detach`.
- **What is shared:** linked worktrees share "everything except per-worktree files such as `HEAD`, `index`, etc." All `refs/` are shared except `refs/bisect`, `refs/worktree`, `refs/rewritten`; pseudo-refs like `HEAD` are per-worktree. So each worktree has its own index (and its own `index.lock`), but object database, branches, tags, stash and `packed-refs` are common.
- **Config is shared** by default; per-worktree config needs `git config extensions.worktreeConfig true` then `git config --worktree` (older Git versions refuse such repos). `core.sparseCheckout` should not be shared.
- **Ref lock contention:** `core.filesRefLockTimeout` default 100 ms retry for a single ref; `core.packedRefsTimeout` default 1000 ms; `core.configLockTimeout` 1000 ms. A lock holder past these fails the other command.
- **gc:** `git gc` warns that running "concurrently with another process" risks deleting an object the other process is using; mitigated by the prune grace period (`gc.pruneExpire`, default 2 weeks) — do not use `--prune=now` with agents running. `gc` calls `git worktree prune --expire 3.months.ago` (`gc.worktreePruneExpire`). `git gc --force` exists to run "even if there may be another git gc instance running".
- **Admin data:** each linked worktree has `$GIT_DIR/worktrees/<id>/`; deleting a worktree directory by hand leaves stale admin files until `git worktree prune`. `git worktree lock [--reason]` prevents pruning/moving/deleting; removing a locked worktree needs `--force` twice; `remove` refuses an unclean worktree without `--force`. `git worktree repair` fixes links after moves. `worktree.useRelativePaths` / `--relative-paths` (sets `extensions.relativeWorktrees`, incompatible with older Git).
- **Submodules:** BUGS section: "Multiple checkout in general is still experimental, and the support for submodules is incomplete. It is NOT recommended to make multiple checkouts of a superproject." Worktrees containing submodules cannot be moved with `git worktree move`; removal needs `--force`.
- **Windows long paths:** not in upstream git-config. Git for Windows: long paths "disabled by default"; enable with `git config core.longpaths true` ("Scripted git commands may still fail"); limit is the Win32 260-character path limit. https://gitforwindows.org/git-cannot-create-a-file-or-directory-with-a-long-path.html . Claude Code's default `<repo>/.claude/worktrees/<name>/` adds path depth.
- **Windows specifics in git docs:** `core.fsmonitor` built-in daemon is available on Windows and macOS (per working directory); `core.protectNTFS` defaults true on Windows.
- **Performance for 3–10 worktrees:** no limit or figure is documented. Each worktree is a full checkout of the working files (objects are not duplicated).

URLs: https://git-scm.com/docs/git-worktree , https://git-scm.com/docs/git-config , https://git-scm.com/docs/git-gc

---

## 6. WebAuthn / passkeys on localhost or a private name

Source: official (W3C WebAuthn Level 3, W3C Secure Contexts, Tailscale KB, publicsuffix.org, FIDO Alliance); third-party where marked

- **Secure context required:** "user agents only expose this API to callers in secure contexts ... only includes those accessed via a secure transport (e.g., TLS) established without errors". `PublicKeyCredential` is `[SecureContext]`. https://www.w3.org/TR/webauthn-3/
- **localhost exception:** Secure Contexts treats `127.0.0.0/8`, `::1/128`, `localhost` and `*.localhost` as potentially trustworthy (https://www.w3.org/TR/secure-contexts/). WebAuthn spec: "Another example of a valid origin is `http://localhost:8000`, due to the origin being localhost." So HTTPS is **not** needed on `http://localhost:<port>`.
- **IP addresses are not allowed as RP ID:** the effective domain must be a valid domain; "Only the domain format of host is allowed here" (not ipv4/ipv6). So `http://127.0.0.1` is a secure context but WebAuthn's RP ID check needs a domain — use `localhost`, not `127.0.0.1`, and not a LAN/Tailscale IP.
- **RP ID rule:** RP ID must equal the origin's effective domain or be a registrable domain suffix of it; port is unrestricted; scheme must be `https` (or `http` for localhost). A credential is bound to its RP ID: a passkey made for `localhost` is unusable on any other name, and vice versa.
- **Private-network name (Tailscale):** any non-localhost name needs HTTPS with a certificate the browser accepts. Tailscale issues Let's Encrypt certificates for `<machine>.<tailnet>.ts.net` via `tailscale cert` (HTTPS must be enabled in the admin console; machine names are published in Certificate Transparency logs; 90-day expiry). https://tailscale.com/kb/1153/enabling-https . `ts.net` is on the Public Suffix List (private section, "Tailscale Inc."), so the registrable domain is `<tailnet>.ts.net`; valid RP IDs for the page are `<machine>.<tailnet>.ts.net` or `<tailnet>.ts.net` (derived from spec + PSL, not stated by Tailscale).
- A bare MagicDNS short name over plain HTTP (`http://machine:port`) is not a secure context → WebAuthn unavailable (follows from the Secure Contexts algorithm).
- **Phone passkey signing for a page served by a laptop — two routes:**
  1. Phone browser opens `https://<machine>.<tailnet>.ts.net` directly (phone on the same tailnet): ordinary platform-authenticator ceremony on the phone, RP ID as above.
  2. Laptop browser opens the page and the user picks "use a phone": FIDO cross-device authentication, CTAP 2.2 **hybrid** transport — QR code, a BLE advertisement for proximity, and an internet tunnel service between the two devices. "CTAP is implemented by authenticators and client platforms, not Relying Parties" (https://fidoalliance.org/passkeys-2/ ; CTAP 2.2 spec §11.5). Needs Bluetooth on both devices and internet access for both; the relying party cannot opt in or out. `AuthenticatorTransport` enum includes `"hybrid"`.
- UNCONFIRMED: whether phone authenticators accept RP ID `localhost` over hybrid transport (no official statement found).

---

## 7. PulseEngine decision inbox ("factory-notice")

Source: official for this project (the author's blog post and repository): https://pulseengine.eu/blog/the-scarce-resource-is-attention/ (dated 2026-11-03; repo file `content/blog/2026-10-30-the-scarce-resource-is-attention.md`), PRs https://github.com/pulseengine/pulseengine.eu/pull/280 , /pull/282 , /pull/284 , issue /issues/283 , files under `claude-tooling/plugins/pulseengine-claude/skills/factory-notice/` (`notice.schema.json`, `attention.schema.json`, `SKILL.md`, `notice.py`) and `hooks/factory-notice.sh`.

- **#280** (plugin 0.43.0): PR admission + batching. Before opening a PR the `ci-status` skill checks the CI queue; docs changes batch at ≥1 queued run or when the session already has an open PR in the repo; fix/feature hold at ≥5; security and red-main fixes always open; one PR per repo per session for trivial changes. Instruction only, "no code enforces it".
- **#282** (plugin 0.44.0): the inbox. **#284** (0.45.0): attention budget. **#283**: blog-ideas issue (source of the post).
- **Notice format** (`notice.schema.json`, notice `schema_version` 2, answer `schema_version` 1):
  - Required: `schema_version`, `id`, `kind`, `revision` (int ≥1), `created_at`, `agent`, `title`.
  - `kind`: `heartbeat | shipped | blocked | needs-decision | session-summary | info`.
  - Lineage: `agent`, `session`, `parent`, `repo`, `flow`, `links[]`. Also `body`, `tags[]`, `opened_at`, `status` (`open | answered | superseded | withdrawn | queued`; `queued` is view-only, never stored).
  - Decision fields: `question`, `options[]` (min 2; each `{id, label, description, pros[], cons[]}`), `recommended` (must name an offered option), `recommended_why`, `reversible` (boolean), `blocks` (string: what waits on it), `answer`.
  - Session summary fields: `done[]`, `in_flight[]`, `blocked[]`, `needs_you[]` (items `{text, links[]}`), `auto`.
  - **Reversibility** = boolean `reversible`. **Urgency is not a field**: it is a tag — a decision tagged `security` or `red-main` (the `always_open_kinds` list in `attention.schema.json`) bypasses the cap and windows.
- **Answer format:** `{schema_version: 1, notice_id, revision, option_id, note?, answered_at, via: "warte"|"local"|"chat"}`.
- **How an agent posts:** `python3 notice.py post --kind needs-decision --json decision.json` (also `--tag security`, `--revise <id>`, `--withdraw <id>`). Written to a user-level local store `${PULSEENGINE_NOTICE_DIR:-~/.local/state/pulseengine/notices}/<agent>/{notices,outbox,answers,consumed}/` and, if reachable, to the "warte" server as agent data `notice.v1` (needs `Authorization: Bearer`). The store is deliberately outside the repo/worktree.
- **How the owner answers:** `notice.py list`, `notice.py answer <notice-id> <option-id> --note "..."`, `notice.py review` (interactive walk, refuses non-tty), or through warte, or in chat (recorded with `--via chat`). `notice.py digest` writes one `DIGEST.md`; `notice.py window-tick` (launchd/systemd every 10 min) posts one macOS notification containing only counts.
- **How the answer gets back:** pull only. The agent runs `notice.py poll --flow <flow>` "on each wake"; an accepted answer prints `ACCEPTED` with the option id. A `SessionStart` hook lists waiting answers read-only (consumes nothing); `SessionEnd` hook writes an automatic `session-summary` and syncs the outbox. "An agent picks up an answer when it next wakes and polls; nothing wakes it."
- **Answer acceptance rules** (checked against the agent's local copy): notice id was posted by this agent; `option_id` is one it offered; `revision` equals the current revision; each (notice, revision) consumed once (record created with `O_EXCL`); `note` is printed quoted as data, never instructions.
- **Agent rules (instructions in SKILL.md):** post the fork and do not take it; continue independent work; never auto-apply the recommendation on timeout; irreversible actions always wait; re-post with a new revision when facts change.
- **Attention budget** (`attention.schema.json`): `max_asking_flows` (default 3), `windows[]` (defaults 08:30–09:00 and 17:00–17:30), `available[]`, `focus[]`, `off[]`, `timezone`, `always_open_kinds[]`. Flow = (agent, flow), ordered by oldest `opened_at`.
- **What the author says does not work / is not true yet:**
  - "All of it is days old and nothing about it has been measured yet"; as of 28 Oct "one agent had posted one notice, a `session-summary`, so no decision has gone through the budget yet"; no baseline exists.
  - The agent rules "are instructions in the skill; no code enforces them". "Nothing stops a session from asking in the chat instead"; chat questions do not appear in the load line.
  - Writes to the deployed warte return 401 and no route/CLI to issue a token was found, so notices stay in the local outbox; `GET /api/attention` and `/api/admission` do not exist yet (404).
  - Nothing wakes an agent when an answer arrives.
  - An `answer.v1` row in warte "is not proof the owner wrote it" (warte does not compare the token's agent to the path's agent; reads are public).
  - Admission falls open: "when no source answers the agent opens anyway".
  - Stated success test: oldest open decision under 48 hours at each weekly reading; otherwise "the budget has moved the bottleneck". Cap of 3 is a starting value taken from a self-reported study of tools, not sessions; n=1.
  - Anti-patterns listed: waiting in a loop for an answer; tagging a routine fork `security` to jump the queue; taking the recommended option because nobody answered; one notice per tool call.

---

## 8. Cumulative-change / anti-splitting policies

Source: mixed — official/primary for each named rule; the mapping to coding agents is my reading. No off-the-shelf CI tool that sums diffs across separate PRs over a time window was found (UNCONFIRMED that none exists).

Finance and law (rule looks at the aggregate, not the single item):
- **CTR aggregation, 31 CFR § 1010.313(b)** (official): multiple currency transactions "shall be treated as a single transaction" if by or on behalf of one person and totalling more than $10,000 "during any one business day". https://www.ecfr.gov/current/title-31/subtitle-B/chapter-X/part-1010/subpart-C/section-1010.313
- **Structuring offence, 31 CFR § 1010.314 and definition § 1010.100(xx)** (official): prohibits structuring "for the purpose of evading" reporting; transactions "need not exceed the $10,000 reporting threshold at any single financial institution on any single day". Intent to evade is itself the violation. https://www.ecfr.gov/current/title-31/subtitle-B/chapter-X/part-1010
- **AML transaction monitoring practice** (third-party): cumulative cash over rolling 1-day/7-day/30-day windows; density of amounts just under the threshold; linking actors by shared identifiers. https://www.gfn.network/risk-taxonomy/structuring-smurfing
- **PSD2 SCA low-value exemption, Commission Delegated Regulation (EU) 2018/389 Art. 16** (official): no strong authentication if amount ≤ EUR 30 **and** (cumulative amount since the last strong authentication ≤ EUR 100 **or** ≤ five consecutive transactions since then). A counter that resets only on a real human check. https://eur-lex.europa.eu/eli/reg_del/2018/389/2023-07-25/eng (Article 16 text: https://www.legislation.gov.uk/eur/2018/389/article/16)
- **Split-purchase ban, FAR 13.003(c)(2)** (official): "Do not break down requirements aggregating more than the simplified acquisition threshold ... or the micro-purchase threshold into several purchases that are less than the applicable threshold merely to ... avoid any requirement that applies to purchases exceeding the micro-purchase threshold." https://www.acquisition.gov/far/13.003

Software operations:
- **Error budget policy (Google SRE Workbook)** (official): "If the service has exceeded its error budget for the preceding four-week window, we will halt all changes and releases" except top-priority issues and security fixes. Rolling four-week window; budget shared by all changes. https://sre.google/workbook/error-budget-policy/
- **Kubernetes PodDisruptionBudget** (official): each eviction request through the Eviction API is checked against the current aggregate state (`minAvailable` / `maxUnavailable`), so many separate small evictions cannot together exceed the budget. https://kubernetes.io/docs/concepts/workloads/pods/disruptions/
- **Performance "death by a thousand cuts"** (third-party): per-change thresholds miss small regressions that accumulate; remedy is comparison against a fixed baseline and change-point detection over the series. https://williewheeler.com/posts/latency-regression-detection-is-a-change-point-problem/

Code review:
- **GitHub branch protection** (official): "Dismiss stale pull request approvals when new commits are pushed" and "Require approval of the most recent reviewable push" — approval is tied to the reviewed content; later pushes void it. Works within one PR only. https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/about-protected-branches
- **Google / Chromium Large-Scale Change (LSC) process** (official for Chromium; for Google I read a third-party summary of the book chapter, not the book): the whole change is described and authorised as a unit **before** it is sharded (Google tool: Rosie) into many small CLs; Chromium: "You can use a single LSC document to describe your entire series of changes". Approval attaches to the aggregate. https://chromium.googlesource.com/chromium/src/+/HEAD/docs/process/lsc/large_scale_changes.md ; Software Engineering at Google, ch. 22.
- **PR size limits** (third-party examples): per-PR line/file caps measured "across its complete diff from the merge base"; these explicitly tell authors to split, i.e. they encourage what a cumulative rule must catch. e.g. https://github.com/realkarych/kora/issues/278

AI agents:
- **Ctrl-Z: Controlling AI Agents via Resampling** (arXiv 2504.10374, primary research): multi-step agent setting where an attack is spread over several individually innocuous steps; best protocol "combines resampling with analysis of previous steps", cutting attack success from 58% to 7% at a 5% usefulness cost. https://arxiv.org/abs/2504.10374
- **Claude Code auto mode** (official): classifier input is "a portion of the transcript plus the pending action" (user messages and prior non-read-only tool calls; tool results stripped), so earlier actions are in view; cumulative denial counters "3 times in a row or 20 times total" pause auto mode; subagent work is reviewed again as a whole when it finishes. https://code.claude.com/docs/en/permission-modes.md
- **PulseEngine #280** (see Q7): "one PR per repo per session for trivial changes" plus queue-based admission — a per-session aggregate rule, instruction-only.

Common mechanisms seen across these: (1) sum over a window keyed by actor, not per item; (2) counter resets only on a genuine human check (PSD2); (3) approve the aggregate before splitting (LSC); (4) compare against a fixed baseline, not the previous step; (5) treat evasion intent / just-under-threshold clustering as a signal in itself (structuring).

---

## UNCONFIRMED / not documented — summary

1. Claude Code: the raw stdin/stdout control protocol for `--input-format stream-json` (`control_request` / `control_response`, `interrupt` with `cancel_queued`) is referenced but has no standalone specification in the pages read; only the SDK types document it.
2. Claude Code: no `--cwd` flag for a session in the CLI reference (process working directory, SDK `cwd`).
3. Codex: `agent_id`/`agent_type` on `PreToolUse`/`PostToolUse` stdin for subagent tool calls is not in the field tables (only "Subagent hooks use the parent session id"); full list of `item.type` strings for `codex exec --json` is given in prose only; no way to steer a running `codex exec` (steer exists only as app-server `turn/steer`); app-server WebSocket transport is "experimental and unsupported".
4. Devin CLI: no documented JSON event stream for `-p`; no subagent hook events; hooks for subagent tool calls not documented; ACP message details not in Devin docs.
5. MCP Go SDK: minimum Go version not checked; per-agent identification by URL path/header is my inference from the `getServer(*http.Request)` signature and stateless-mode text.
6. git: no documented figures for performance or contention with 3–10 worktrees; `core.longpaths` is Git for Windows only.
7. WebAuthn: phone passkey over hybrid transport for RP ID `localhost` — no official statement; `<tailnet>.ts.net` as RP ID is derived from the PSL entry, not stated by Tailscale.
8. Cumulative-change tools: no existing CI/code-review product found that enforces a cross-PR rolling-window diff budget; absence not proven.
