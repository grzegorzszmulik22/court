# 09 — The scripted adapter writes an Edit step outside the world via `..`

**Severity.** Medium (isolation). The adapter has no containment check of
its own; it treats "the policy did not deny" as permission. A path with
`..` that leaves the data directory is allowed and written. The same
policy governs a live session's `PreToolUse`, so Court contributes no
confinement to the world beyond the tool's own permission mode.

**Property, as stated.**

- M1 goal (`spec/milestones/M1-ledger-and-worlds.md`): "Record everything
  agents do, and confine what they do." P3 is isolation.
- `spec/02-architecture.md` §4.2: the scripted agent "applies edits, runs
  commands, calls the agent tools, makes commits" in its world.
- M1.7 denied: "Auto-approving anything outside the world." The scripted
  adapter's rule is exactly "not denied by Court = run it".

**What I did.** Built a `scripted.Client` with `Worktree` set to
`<base>/data/worlds/w1` and a stand-in hook endpoint that returns an
ordinary allow response (`{}`), then ran one Edit step with

```
Path: "../../../outside/escape.txt"
```

The client joins the path to the worktree
(`filepath.Join(c.Worktree, e.Path)`, `internal/adapter/scripted/run.go:61`)
and writes it after `PreToolUse` returns no deny. A real daemon's default
policy also abstains: the resolved path is outside the data directory, and
`insideDataDir` only checks the data directory.

**What happened.**

```
scripted Edit wrote outside the world:
.../\.rt/scratch/scripted/outside/escape.txt
```

The file was created; no `Discard` would remove it (it is outside the
worktree; see findings 04–06). An absolute path is not a direct escape
because Go's `filepath.Join` keeps the base, but `..` is.

**What should have happened.** `doEdit` should reject an absolute path and
verify the cleaned resolved target is under `Worktree` before writing;
"the daemon's policy abstained" is not a permission grant. If the intended
model is that only Claude's permission system confines edits, that should
be stated and the scripted adapter should still refuse to write outside
the directory it calls the agent's world.

**Trust-model caveat (do not overclaim).** Scripts are authored by Court's
own checks today, so a hostile script is not an external attacker. The
finding is that the adapter's isolation is borrowed from a policy that
does not express it: any script bug, or a script built from recorded agent
behaviour, silently writes outside the world, and the isolation checks do
not notice. The supervisor decides whether that is in scope; it is
reported because the brief names the scripted adapter paths and `..` as
targets.

**How sure.** High; deterministic.

**Reproduction.**

```
go test ./internal/adapter/scripted -run 'TestRT_EditEscapesWorld' -count=1 -v
```
