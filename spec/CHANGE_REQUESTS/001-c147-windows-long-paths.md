# CR-001: C1.4.7 cannot be met as written; restate it, and fix the Windows build

**Status:** PROPOSED (only a human may mark it ACCEPTED)
**Raised by:** supervisor, 2026-11-16, after running C1.4.7 on a Windows PC
**Affects:** M1.4 (check C1.4.7), and every later chunk that builds `internal/world` on Windows
**Evidence:** `evidence/M1.4/2026-11-16T10-13-32Z-windows-supervisor/` (sysinfo, build output, probe script and output)

## What was found

1. **`internal/world` does not compile on Windows.** `git.go` uses Unix-only process-group calls in `Run` (`SysProcAttr.Setpgid`, `syscall.Kill(-pid, SIGKILL)`). On `windows/amd64` with the pinned Go 1.27.4 the package fails to build, so C1.4.7 could never have passed: it was recorded as SKIPPED-ENV ("no Windows host") while the real state was "does not build on Windows".

2. **C1.4.7 as written is unachievable with git worktrees.** On Windows 11 with Git for Windows 2.55.0 and `LongPathsEnabled=1`, `git worktree add` refuses any worktree whose own root is longer than 260 characters (`fatal: '$GIT_DIR' too big`), even with `core.longpaths=true`. No world design built on git worktrees can make "a world path longer than 260 characters" work.

3. **What does work, and is what matters to agents:** a world with a short root holding files whose full path exceeds 260 characters. With `core.longpaths=true` (which `internal/world` already sets on Windows) a 406-character file path was written, committed, diffed from the repository and discarded with the world folder removed. The same checkout without `core.longpaths` fails, so the setting is what makes it work.

## Proposed change

Replace C1.4.7 with:

> **C1.4.7** On Windows: a world whose root path is at most 200 characters holds, commits, diffs and discards a file whose full path is longer than 260 characters; the same check run with `core.longpaths` disabled fails (proving the setting is what makes it pass). Opening a commission whose data directory would put a world root above 200 characters fails with a one-sentence error naming the limit. `SKIPPED-ENV` only where no Windows host exists; a package that does not build on Windows is a FAIL, never SKIPPED-ENV.

And add to M1.4's build: `Run` gets a Windows implementation of its process-tree timeout (for example a job object, or `taskkill /T /F`), behind build tags, so `internal/world` builds and C1.4.6 holds on Windows.

## Effect on properties and numbers

None weakened: the new wording tests the case agents actually hit (deep paths inside a repository) and adds a negative control. The root-length limit becomes a stated, tested limit rather than an untested promise.

## Alternatives considered

- Keep the wording and leave C1.4.7 SKIPPED-ENV: hides a real Windows build failure.
- Use the `\\?\` prefix for worktree roots: git itself refuses the long root, so a prefix in Court cannot help.
