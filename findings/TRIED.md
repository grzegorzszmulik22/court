# Attacks tried

One line each. "refused" means the code did what the spec says and no
finding was written. Finding numbers point at `findings/<NN>-…/`.

## Ledger — tampering and canonical bytes

1. Edit the newest JSONL event, recompute its hash: accepted by both verifiers — **01**.
2. Delete the newest JSONL event (28 → 27): accepted by both verifiers and `court replay` — **01**.
3. Append a forged `decision.ruled` (`actor:human`, `D1`/`o1`) chained to the real tail: accepted by both verifiers and replay — **01**.
4. Edit the newest SQLite row and recompute its hash: accepted (`VerifyFile` ok) — **01**.
5. Delete the newest SQLite `ledger` row (event projects to nothing): `court ledger verify` and `court replay` both pass, fewer events — **01**.
6. Edit the newest SQLite row and blank `hash`: Go accepts, refledger reports `envelope` (disagreement) — **02**.
7. `court ledger export` of that database: produces a JSONL both verifiers accept (“laundering”) — **02**.
8. Edit/delete/reorder an event in the middle: refused (`seq`, `prev` or `hash` class) — covered by the mutation suite below.
9. Duplicate a `seq` in a properly created database: refused by the primary key; a hand-built table without one is caught by the position check (`seq`).
10. Create a `seq` gap (delete seq 5): refused, class `seq` in both verifiers.
11. Delete the first event: refused (`seq` at position 0).
12. Reorder the first two events: refused (`seq`/`prev`).
13. Insert an event in the middle with fresh hashes: needs every following `prev` rewritten — treated as the whole-file rewrite `01` §11.1 excludes; not pursued.
14. Duplicate object key (envelope and body): refused (existing rejection vectors and `court ledger mutate`).
15. Unknown envelope field: refused.
16. Unknown body field: refused by the schemas (`additionalProperties: false`).
17. Trailing data after the JSON object: refused.
18. Invalid UTF-8, raw control characters, bad escapes, lone surrogates (4 forms): refused (`court ledger mutate` all 24 kinds).
19. Numbers out of IEEE-754 range (`1e999`): refused (`json` class in both).
20. Underflow (`1e-400`), `-0`, `1e21`, `1e-6`, exponent forms: Go and Python canonicalize identically; no semantic collision found.
21. Key ordering with astral-plane and control characters: Go (UTF-16 code units) and Python (UTF-16BE bytes) agree.
22. RFC 8785 upstream vectors and the 10,000-number ES6 stream: both pass (`go test ./internal/ledger -run TestJCS…`, `refledger --selftest`).
23. One-byte whitespace insertion in a stored line: accepted by both verifiers — **11**.
24. CRLF conversion of a whole ledger: accepted by both verifiers and replay — **11**.
25. Reorder object members / alternate escapes of the same value: accepted (same canonical bytes); not written up separately.
26. A body number whose literal is not representable as a double (`9007199254740993`): both sides canonicalize to the same double; no semantic difference — note, not a finding.
27. Derived-table tamper (`decisions.state` changed in SQLite): `court ledger verify` accepts, `court replay` catches — NOTES.
28. `backbrief` with `plan.lines = -1` from an agent: append accepts, whole ledger becomes unverifiable — **03**.
29. `court ledger append decision.contested` with empty world ids: append accepts, verify rejects — same class as **03** (operator path; recorded there).
30. `court ledger append backbrief.submitted` with `invalid:true` and 2001-character `output`: append accepts, verify rejects — same class as **03** (operator path).
31. A stored `hash` blanked on an otherwise untouched middle row: accepted by Go (recomputed equals original); refledger rejects — **02**.
32. 20 `court ledger append` processes against one database at once: all rows present (20), chain verifies, no loss/duplicate seen.
33. Two processes at once, the C1.2.3 shape, 50 rounds: 100/100 events present, chain verifies, no error.
34. 100 simultaneous `court ledger append` processes: 99 events and one `SQLITE_BUSY` error returned to that caller; no chain damage — note, not the specified two-process case.
35. Crash injected between write and commit (`COURT_LEDGER_FAULT`): not attacked; C1.2.4's own fault points not rerun.
36. `court replay` on a live database while another process appends: not run — NOTES.
37. `court ledger mutate` on a generated ledger, all 24 kinds: 24/24 mutants rejected by Go and by refledger.

## Worlds — isolation, discard, availability

38. `git config --local court.canary …` inside a world: survives Discard, Digest unchanged — **04**.
39. A `pre-commit` hook written into the common `.git/hooks` from a world: survives Discard, Digest unchanged — **04**.
40. `git tag canary-ref` inside a world: survives Discard, Digest changes but Discard returns nil — **05**.
41. Write a file into the main worktree with an absolute path via `World.Run`: survives Discard, Digest unchanged — **06**.
42. Directory junction inside a world pointing outside, then Discard: outside canary intact — refused (git does not follow the reparse point).
43. Untracked files, ignored files, a nested git repository, a read-only file, and a >260-character path in a world: all removed by Discard; digest restored — refused.
44. `git config`/hook changes: see 37/38; the adapter policy does not deny either command.
45. `Run` timeout with an ordinary descendant holding the pipes (the C1.4.6 shape): killed and reported (existing test passes).
46. `Run` timeout with `cmd /c start /b ping …` (detached descendant inheriting stdout): returns after ~5.1 s for a 0.5 s limit — **07**.
47. C1.4.4, C1.4.6 and C1.4.7 on this Windows host: all pass under `go test ./internal/world`.
48. Submodule refusal (C1.4.5): not attacked. Code paths checked; gitlink in a non-HEAD branch is not scanned (NOTES).
49. `git gc --prune=now` from one world against a sibling's staged blob: not run; git's gc is documented to consider index objects, so no path was found to a loss.
50. Renaming/moving a world's own branch or worktree (`git worktree move`): reasoned to make Discard fail with an error (no silent restore); not run.
51. Windows device names (`CON`, `NUL`, `COM1`) as world or edit names: no file created / no escape observed; not pursued.
52. `git submodule add` a local repository from inside a world, then Discard: the worktree and branch are removed but `.git/config` keeps `submodule.sub.url` and `submodule.sub.active` — another form of the configuration residue in **04**.

## Adapters and isolation

53. `Policy.DecideHook` with the data-directory path in a different case: not denied; write reaches the real file — **08**.
54. `\\?\` verbatim spelling of the data-directory path: not denied; write reaches the real file — **08**.
55. A junction inside the world to the data directory: file-tool path not denied; write reaches the real file — **08**.
56. `<data-dir>.\canary.txt` (Windows strips the trailing dot): not denied; write reaches the real file — **08**.
57. 8.3 short name of the data directory: not available on this volume (8.3 generation disabled) — skipped, recorded.
58. Alternate-data-stream path into the data directory (`ledger.db:stream`): matched by the prefix rule and denied (no separate finding).
59. Scripted Edit step with `../../../outside/escape.txt`: written outside the world — **09**.
60. Scripted Edit with an absolute Windows path: `filepath.Join` keeps the world as prefix, so it does not escape directly; `..` does — no separate finding.
61. Scripted Bash step writing outside the world: same as **09** (the shell is not confined); not written up separately.
62. `POST /gate` with a sibling world's commit id and this world's path: recorded as this world's `world.commit`; verify accepts — **10**.
63. `POST /gate` with the world's own base commit: no event (the `base..head` range is empty) — refused.
64. Direct hook post with an `agent_id` containing characters the actor regex rejects: could poison the ledger, but a real tool generates the id; not attempted — NOTES.
65. Reusing an old `tool_use_id` / JSON-RPC id to suppress an event: theory (idempotency keys are request identities); not attempted — NOTES.
66. Endpoint binding to loopback only (C1.5.5): existing check, not rerun.
