# 10 — `/gate` records a commit in a world that never had it

**Severity.** Medium (ledger attribution / completeness). The gate endpoint
takes a `world_path` and a commit id, looks the world up by path, and
records every commit in `base..commit` under that world. It never checks
that the commit is on that world's branch. An agent can post another
world's commit id under its own path; `court verify` accepts the resulting
event (it is a normal, well-formed `world.commit`).

**Property, as stated.**

- `spec/01-decision-model.md` §11.2: `world.commit` body is "world,
  commit id, parent ids, changed paths…"; the ledger is the record of what
  agents did.
- `spec/02-architecture.md` §2: "The token in the URL path is how the
  daemon attributes a hook event or a tool call to an agent."
- `internal/daemon/gate.go` says: "The commit is attributed to the session
  whose world it happened in (worlds never move, so the registered path is
  authoritative)."
- M1 Expected results: "Three scripted agents work in three worlds at once,
  and every tool event and commit is in the ledger in order."

**What I did.** Two agents, `w1` and `w2`, each with its own worktree on
its own branch from the same base. In w2's worktree I created a commit
object with `git commit-tree` (parent = the shared base). That creates no
branch and fires no post-commit hook, so the commit is not recorded
anywhere. I then POSTed its id to `POST /gate` with `world_path` = w1's
worktree and no authentication:

```json
{"world_path":"<...>/worlds/w1","commit":"<commit-tree id>","parents":["<base>"]}
```

**What happened.** The daemon ran `git rev-list base..<commit id>` inside
w1's worktree (the object exists in the shared store) and appended a
`world.commit` event whose world is `"w1"` and whose commit is the
fabricated id. `court ledger verify` accepts it: the event is chain-valid
and schema-valid. The test fails on that event
(`TestRT_GateAcceptsForeignCommit`):

```
gate recorded b061f1a5… as world.commit of world "w1" (seq 5);
that world never had it
```

**Reachability.** The endpoint is loopback-only but has no token; the
agent knows its own world path (its working directory) and the daemon's
origin (the MCP URL in its generated configuration is on the same host and
port). The policy's outward list denies `curl`/`wget`/`ssh`, not arbitrary
HTTP from `powershell`, `python -c`, or an MCP client; the policy's own
docs list these as invisible (`internal/adapter/claude/policy.go` comment
on what the scanner cannot see). A real Claude session with Bash can
therefore post. (If the agent tool's permission mode blocks all shell use,
the attack is unavailable; the endpoint itself still trusts unauthenticated
input.)

**What should have happened.** The gate should resolve the commit against
the session's world only: verify the commit is reachable from that world's
`HEAD`/branch (e.g. `git merge-base --is-ancestor` plus a check that the
id is that world's tip or in its first-parent chain), and/or authenticate
the hook post with a per-session secret. A `world.commit` must only record
a commit made in that world.

**How sure.** High that the endpoint accepts the post and records the
event; reproduced twice. The shell-reachability claim is an inference from
the policy (no HTTP client test was used against a real Claude session).

**Reproduction.**

```
go test ./internal/adapter/scripted -run 'TestRT_GateAcceptsForeignCommit' -count=1 -v
```
