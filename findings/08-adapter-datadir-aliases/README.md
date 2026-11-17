# 08 — The data-directory deny rule is a case-sensitive string prefix: case, `\\?\` and junctions bypass it

**Severity.** Medium-high: `spec/02` §2 promises the adapter denies any
file-tool path inside the Court data directory. On Windows four spellings
of the same file are not denied, and the file is really written.

**Property, as stated.**

- `spec/02-architecture.md` §2 ("Ledger protection"): "The adapter denies
  any file-tool call whose path is inside the data directory, and any shell
  command that names it."
- M1.7 denied: "Auto-approving anything outside the world."

**What I did.** Called `claude.Policy.DecideHook("Write", …)`, the exact
function the daemon runs on every `PreToolUse`, with paths that name a file
inside `<data-dir>` (a canary beside the ledger):

1. the exact path (control: denied);
2. the same path upper-cased (`strings.ToUpper`);
3. the same path with the `\\?\` verbatim prefix;
4. `<data-dir>.\canary.txt` (Windows strips the trailing dot of a path
   component, so this names the same directory);
5. `<world>/j/canary.txt`, where `<world>/j` is a directory junction made
   with `cmd /c mklink /J <world>/j <data-dir>` (the world itself is the
   exempt subtree, so the lexical path is "inside the world").

Each test first writes through the alias and reads the canary at its real
path, proving the OS resolves them to one file, then asks the policy.

**What happened.**

```
case-varied data-directory path not denied: decision="" reason=""
verbatim data-directory path not denied: decision="" reason=""
trailing-dot data-directory alias not denied: decision="" reason=""
junction path into the data directory not denied: decision="" reason=""
```

Only the exact spelling is denied. `insideDataDir` does
`filepath.Clean` and `strings.HasPrefix` (`internal/adapter/claude/policy.go:96`),
which does not resolve NTFS case, verbatim prefixes, trailing dots, or reparse points.

**Why it matters.** The stated control is the only thing protecting the
ledger from file tools: an agent's `Write`/`Edit` with
`<DATADIR>\LEDGER.DB` (or the verbatim/junction form) is allowed by
Court's policy. If the agent tool's own permission mode auto-approves
edits (a user-level setting; Court does not add an allow rule but does not
control the user's settings either), the ledger can be modified without a
shell. The junction form is a two-step bypass: a shell command that
creates the junction never literally names the data directory if it uses a
relative target, which defeats `commandNamesDataDir` as well.

**8.3 short names.** Tried, but this volume has 8.3 name generation
disabled (`dir /x` shows no short names, FSO `ShortPath` returns the long
path), so the test skips. Recorded in `findings/TRIED.md`; if enabled,
`insideDataDir` would not match the short spelling either.

**What should have happened.** Compare the resolved file identity, not a
cleaned string: `os.Open`/`GetFinalPathNameByHandle`, or resolve each
component and compare case-insensitively, including reparse points and
verbatim prefixes. The deny rule is a security boundary only if it uses
the filesystem's own resolution.

**How sure.** High; deterministic, repeats on every run.

**Reproduction.**

```
go test ./internal/adapter/claude -run 'TestRT_Policy' -count=1 -v
```

Expected current: case, verbatim and junction subtests FAIL; 8.3 SKIP.
