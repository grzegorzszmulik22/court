# 06 — A world can write into the repository's main worktree; Discard leaves it and Digest does not see it

**Severity.** Medium-high for isolation and H3: the main repository's
checked-out files are changed outside the world, `Discard` reports success,
and the H3 digest is unchanged.

**Property, as stated.**

- `spec/02-architecture.md` §3: `Discard` restores the repository bit for
  bit; `Run(cmd, limits)` executes in the world.
- `spec/01-decision-model.md` §8 and M1 expected result: "Discarding a
  world leaves the repository exactly as it was."
- P3 (isolation): the world is the only place an agent works.

**What I did.** Through `World.Run` (the agent's shell path) ran:

```
cmd /c echo CANARY-MAIN > <repo>\canary-main.txt
```

The world's working directory is the worktree, but a shell may use any
absolute path. Then `Discard()`.

**What happened.** The file remains in the main worktree; `Discard`
returns nil; `root.Digest()` before and after is equal. `Digest` hashes
`for-each-ref`, `rev-list --objects --all` and `git worktree list`, not
the contents of the main worktree (or of any worktree).

**What should have happened.** Either `World.Run` must confine the process
to the world (it does not; it is an arbitrary process launch, and the
adapter policy only denies outward actions and the data directory), or the
H3 digest must include the main worktree so the difference is visible and
`Discard` cannot claim a restore. As it stands, "bit for bit" is not
measured for files, only for refs/objects/worktree list.

**How sure.** High; deterministic.

**Reproduction.**

```
go test ./internal/world -run 'TestRT_DiscardLeavesMainWorktreeWrite' -count=1 -v
```

Note: this and findings 04/05 are the same boundary — worlds share a
filesystem, refs and configuration, and only the world's own worktree and
branch are removed. The examples are split because each residue has a
different visibility: config/hooks are invisible to `Digest`; refs change
it; main-worktree files do not.
