# 04 — Discard leaves the world's git configuration and hook files behind; Digest cannot see either

**Severity.** High for H3 and for persistence: a repository option and an
executable hook installed from inside a world survive `Discard`, and the
H3 digest still reports the repository restored.

**Property, as stated.**

- `spec/02-architecture.md` §3: `Discard() error // restores the repository
  bit for bit`; "Court therefore never changes repository configuration".
- `spec/01-decision-model.md` §8: "A world is treated as reversible only
  because M1.3 proves, on every check run, that discarding a world restores
  the repository bit for bit."
- `spec/milestones/M1-ledger-and-worlds.md`, Expected results: "Discarding
  a world leaves the repository exactly as it was." C1.4.4: "`git config
  --list` for the repository is byte-identical before a commission and
  after it."

**What I did.** Created a repository, forked a world, and ran two ordinary
commands inside it through `World.Run` (the path agents use):

```
git config --local court.canary CANARY-CONFIG
git rev-parse --git-common-dir        # then wrote <common>/hooks/pre-commit
```

Both are allowed by the adapter policy: neither is an outward program and
neither command names the Court data directory. Then `Discard()`.

**What happened.**

- `.git/config` still contains `court.canary=CANARY-CONFIG`; before/after
  `git config --local --list` differ.
- `.git/hooks/pre-commit` still exists with the canary content.
- `Discard()` returned nil.
- `root.Digest()` before `Fork` and after `Discard` is **equal**:
  `Digest` hashes refs, object ids reachable from refs and the worktree
  list only (`internal/world/git.go:522`).

The test output shows the config diff and the surviving hook
(`TestRT_DiscardLeavesConfigAndHooks`).

**Why it matters.** `spec/02` §3 says refs, objects and configuration are
shared by worktrees. `Discard` removes its own worktree and branch, and
nothing else. Configuration and hooks are therefore outside the H3
measure; a world can change future repository behaviour (for example
`core.autocrlf`, `core.hooksPath`, or an installed hook that runs on the
human's next commit) and no digest, `court replay` or `court verify` will
record it. C1.4.4 passes only because its test drives benign Court
commands, never an agent command.

A submodule variant: `git -c protocol.file.allow=always submodule add <local
repo> sub` inside a world leaves `submodule.sub.url` and
`submodule.sub.active` in the shared config after `Discard`. It is the same
residue as above (recorded in `TRIED.md`); if the agent also copies the
generated `.gitmodules` into the main worktree, the next `world.Open`
refuses the repository as submodule-bearing.

**What should have happened.** Either the world must not be able to leave
shared configuration/hooks changed (deny or restore them at discard), or
H3's digest must cover them, and `Discard` must fail rather than claim a
restore it did not make. The spec's "bit for bit" claim is currently
false for any world whose agent ran git.

**How sure.** High. Deterministic; reproduced with the failing test several
times.

**Reproduction.**

```
go test ./internal/world -run 'TestRT_DiscardLeavesConfigAndHooks' -count=1 -v
```

Expected (current code): FAIL with the surviving hook and the
`court.canary` config line. See also finding 05 (refs) and 06 (main
worktree files), which are the same shared-state gap with different
residue.
