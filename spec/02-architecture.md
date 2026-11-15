# 02. Architecture

Normative for M1 to M3. External facts were read on 15 Nov 2026 and are in `../research/4_build_facts.md`; each must be re-checked at the pinned version before use (`05` §8).

## 1. Shape

One binary, `court`, written in Go. No server to install, no account, no configuration file.

| Command | Who | Since |
|---|---|---|
| `court commission "<goal>"` | human | M2 |
| `court hold` | human | after G1 |
| `court progress` | human | after G1 |
| `court report [commission]` | human, during M1 to M3 only | M3 |
| `court replay <ledger>` | checks | M1 |
| `court gate`, `court daemon` | internal; never typed by a person | M1, M3 |

`court report` exists because M3 has no docket yet. It is removed when `court hold` arrives; the user surface stays at three commands (`00` §5).

## 2. Processes and data

```
 human ──▶ court commission ──▶ court daemon (one per repository, started on demand, 127.0.0.1 only)
                                   │
            ┌──────────────────────┼─────────────────────────┐
            │ /hook/<token>        │ /mcp/<token>            │ /gate
            │ tool events (HTTP)   │ agent tools (MCP)       │ git hooks
            ▼                      ▼                         ▼
      agent session A        agent session A           world A (git worktree)
      (agent tool process, working directory = world A)
```

- The **daemon** is the only writer of the ledger. It listens on a loopback port chosen at start.
- **Data directory**: outside the repository, in the user's state directory, keyed by the repository's path: the ledger, worlds, git hook scripts, generated agent settings. Nothing is written into the repository except branches named `court/<commission>/<agent>`.
- Each agent session gets a random **token**. The token in the URL path is how the daemon attributes a hook event or a tool call to an agent. (The current MCP revision is stateless and has no session header, so the path is the identity.)

**Ledger protection.** Agents have no tool that writes the ledger. The adapter denies any file-tool call whose path is inside the data directory, and any shell command that names it. An agent with a shell could still reach it; the hash chain shows an edit to a past event, and version 1 claims no more than that (`01` §11.1).

## 3. Worlds

```go
type World interface {
    Fork(base Commit, name string) (World, error) // a new isolated copy on its own branch
    Diff(from, to Commit) (Changes, error)
    Run(cmd []string, limits Limits) (Result, error)
    Commits() ([]Commit, error)
    Discard() error                               // restores the repository bit for bit
}
```

The version 1 backend is `git worktree`:

- `Fork` runs `git worktree add <datadir>/worlds/<id> -b court/<commission>/<agent> <base>`. One branch per worktree is a git rule and is relied on.
- The index and `HEAD` are per worktree; refs, objects and configuration are shared. Court therefore never changes repository configuration: per-world settings (`core.hooksPath`, `gc.auto=0`, `core.longpaths=true` on Windows) are passed in the agent's environment as `GIT_CONFIG_COUNT` entries.
- Automatic garbage collection is off for the duration of a commission, because concurrent collection with several worktrees carries a documented risk.
- `Discard` runs `git worktree remove --force`, deletes the branch, and prunes. H3 is checked by hashing every ref and every object id reachable before `Fork` and after `Discard`.
- Submodules are not supported in version 1 (git documents multi-worktree support for them as incomplete). A repository with submodules is refused with one sentence.

## 4. Adapters

```go
type Adapter interface {
    Start(w World, prompt string, s Session) (Handle, error) // s carries token, writ, hook and tool URLs
    Send(h Handle, message string) error                      // a follow-up message to a running session
    Stop(h Handle) error
}
```

Tool events do not come through the adapter. They arrive at the daemon's `/hook` endpoint, so that every adapter is observed the same way.

### 4.1 Claude Code (the only live adapter in M1 to M3)

Started with the world as the process working directory (the tool has no working-directory flag):

```
claude -p --output-format stream-json --verbose --input-format stream-json \
       --session-id <uuid> --settings <generated.json> \
       --mcp-config <generated.json> --strict-mcp-config --permission-mode default
```

- **Hooks** are generated per session with handler type `http`, posting to `http://127.0.0.1:<port>/hook/<token>`, for `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `SubagentStart`, `SubagentStop`, `Stop`, `UserPromptSubmit`. The generated settings include the `allowedHttpHookUrls` entry those hooks need.
- **Mapping to `tool.event`**: `tool_name`, the selected fields of `tool_input` (`01` §11.3), `tool_use_id`, and the outcome from `PostToolUse` (ok), `PostToolUseFailure` (error) or a deny decision (denied).
- **Subagents** the tool starts by itself share the session's world. Their tool calls fire the same hooks with `agent_id` and `agent_type`; Court records them as child agents of the session, with the parent's writ. Agents that need their own world are started with `spawn` (§5).
- **Outward actions are denied** by Court's `PreToolUse` response (`permissionDecision: "deny"`): web fetch and search tools; shell commands matching the outward list (`git push`, `curl`, `wget`, `ssh`, `scp`, package publish commands, cloud command-line tools); any MCP tool other than Court's. The denial and its reason are recorded. This is what keeps every decision in version 1 reversible (`01` §8).
- **Follow-up input** uses the stream-json input channel. **Stopping** uses SIGINT, which ends the turn cleanly.
- Pausing a tool call for a human (`permissionDecision: "defer"`) exists and is not used before the docket.

### 4.2 The scripted agent

A second adapter, used by every hermetic check. It reads a `script.yaml` (`04` §3), applies edits, runs commands, calls the agent tools, makes commits, and posts hook payloads to the daemon in exactly the shape Claude Code sends. The payload shapes come from fixtures recorded once from a real session at the pinned version (`test/vectors/hooks/`), with a check that fails when the pinned tool's shapes drift.

### 4.3 Other tools

Codex CLI and Devin CLI both document hooks of the same three-level shape, and Devin reads Claude Code hook files by default. Adapters for them are denied until after G1; the hook endpoint must not assume Claude-only field names beyond the mapping table in `internal/adapter/claude`.

## 5. Agent tools

Court serves one MCP endpoint per agent session, `/mcp/<token>`, over Streamable HTTP, using the official Go SDK. Six tools:

| Tool | Input | Effect |
|---|---|---|
| `backbrief` | restated goal, plan (files, estimated lines), expected decisions `[{question, category, paths, intended_default}]`, will-not-touch | `backbrief.submitted`. Until it is recorded, every other tool call of that agent is denied (P6) |
| `spawn` | task, writ `{categories, territory, budgets}` | Checks attenuation; creates a world; starts a session; `agent.started`, `writ.issued` or `writ.refused` |
| `record_decision` | question, category, paths, options, chosen, `petition` | `decision.declared` |
| `no_decisions` | none | Opens the gate for the next commit with an explicit statement of none |
| `claim_done` | task, done-check ids | `claim.done`, then Court's own run and `claim.checked` |
| `alarm` | text, paths | `alarm.raised`, shown in `court report` first. A parent agent cannot intercept it: the call goes from the agent to the daemon directly |

Tool descriptions are one sentence each and state when to call the tool. They are the only instructions Court adds to an agent, besides the commission text: goal, territory, done-checks, and the sentence "Declare each decision you make with record_decision before you commit."

A call from a subagent inside a session arrives on the session's token. Court attributes it to the subagent by pairing it with the `PreToolUse` hook event for the same tool call, which carries `agent_id`.

## 6. Storage

- SQLite in write-ahead mode, through a driver that needs no C compiler, so the binary stays a single static file. The driver choice is verified at M1.1 and recorded.
- One table is the ledger: `seq, prev, at, commission, actor, type, body, hash`. Every other table is derived and can be dropped and rebuilt by replay.
- The canonical bytes of an event are RFC 8785 JSON of the envelope without `hash`.

## 7. Models Court itself calls

In M1 to M3 there is one: the blind clerk's expected-decision list (M2.3). It runs as a one-shot session of an installed agent tool with every tool disabled and only the commission as input. Where the user has a second agent tool from another vendor installed, the clerk uses that one, so that the two expectation lists do not share a model family. Its output is recorded as `backbrief.submitted` with actor `clerk` before it is used (`01` §2). With no model available, the commission proceeds on the orchestrator's list alone and says so.

## 8. Repository layout

```
cmd/court/                 the binary
internal/ledger/           envelope, chain, storage, replay            (M1)
internal/world/            interface, git worktree backend             (M1)
internal/adapter/          interface, claude, scripted                 (M1)
internal/tools/            the six agent tools over MCP                (M1, M2, M3)
internal/commission/       commission format, backbrief, writs         (M2)
internal/gate/             git hooks, declaration gate                 (M3)
internal/detect/           detectors DT1 to DT12, exemptions           (M3)
internal/reconcile/        matching, verdicts, contests                (M3)
internal/claims/           done-claim checking                         (M3)
ref/python/                refledger, refreconcile                     (M1, M3)
bench/docketbench/         scenarios, validate, score                  (M3)
test/vectors/              ledger, hooks, writ, detect, reconcile      (frozen per 05 §7)
test/sabotage/             patch files per chunk
evidence/                  raw outputs of every check run
docs/                      at most one page per chunk
tools/                     versions.env, host.md
```

## 9. Technology choices

| Choice | Reason |
|---|---|
| Go, one static binary | Install is one file; question 2 of `00` §5 |
| Official MCP Go SDK, Streamable HTTP | One process serves all agent sessions |
| Git worktrees | Isolation with no new dependency; discard is exact |
| HTTP hooks, not shell-script hooks | No script to install per platform; the daemon sees every event directly |
| Python reference implementations | A second, independent reading of the two specs that matter most (`01` §11, `01` §7) |
| No web interface before G1 | Nothing to show until detection is proven |

The working name appears only as the binary name, the branch prefix and the tool-server name. Renaming is one substitution over those three.
