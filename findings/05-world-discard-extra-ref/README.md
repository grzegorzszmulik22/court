# 05 — A ref created inside a world survives Discard and changes the H3 digest

**Severity.** High for H3: `Discard` returns nil while the H3 measure
(`Digest`) says the repository was not restored. Unlike finding 04, this
one is visible to the digest — but nothing checks it after discard.

**Property, as stated.**

- `spec/02-architecture.md` §3: `Discard` "restores the repository bit for
  bit"; "H3 is checked by hashing every ref and every object id reachable
  before `Fork` and after `Discard`."
- `spec/milestones/M1-ledger-and-worlds.md` C1.4.1/MM1.3: 100% of
  randomised sessions restore the digest; Expected results: "Discarding a
  world leaves the repository exactly as it was."

**What I did.** In a forked world, ran

```
git tag canary-ref <base>
```

(or `git update-ref refs/any/thing <commit>`), then `Discard()`. Tags and
other refs live in the shared ref store; `Discard` removes only the
world's own `court/<commission>/<name>` branch.

**What happened.**

```
refs not restored by Discard:
before:
refs/heads/main <sha>
after:
refs/heads/main <sha>
refs/tags/canary-ref <sha>
```

`Discard()` returned nil. `root.Digest()` after discard differs from the
digest before `Fork` — so H3 would fail if anyone recomputed it, but the
world API reports success and the check harness's random sessions never
create refs. A variant is an agent that renames its own branch and leaves
a decoy under the expected name: `git branch -D` then deletes the decoy,
leaving the renamed branch.

**What should have happened.** `Discard` should remove refs created in the
world (or return an error and leave the digest check to the caller), and
H3 must be asserted after every discard, not only in the check's generated
sessions. Creating refs is not denied by the adapter policy either (it is
a normal git command).

**How sure.** High. Deterministic failing test; reproduced repeatedly.

**Reproduction.**

```
go test ./internal/world -run 'TestRT_DiscardLeavesExtraRef' -count=1 -v
```
