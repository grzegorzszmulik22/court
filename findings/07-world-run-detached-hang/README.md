# 07 — `World.Run` ignores its time limit while a detached descendant holds the output pipes

**Severity.** Medium (availability). `Run` reports `TimedOut=true` but
returns only when the escaped descendant exits. With a long-lived child the
call blocks for as long as the child lives (hours), so any Court code that
runs agent-supplied commands through `Run` can be wedged.

**Property, as stated.**

- `spec/milestones/M1-ledger-and-worlds.md` M1.4: "`Run`'s time limit kills
  the whole process tree on both, through build-tagged implementations
  (Linux: process groups; Windows: a job object or an equivalent that kills
  descendants)." C1.4.6: "`Run` enforces its time limit and reports a
  timeout distinctly from a non-zero exit."
- `spec/02-architecture.md` §3 does not qualify this by process shape.

**What I did.** In a forked world:

```go
gw.Run([]string{"cmd", "/c", "start /b ping -n 6 127.0.0.1"},
       world.Limits{Timeout: 500 * time.Millisecond})
```

`start /b` launches a child that inherits the parent's redirected
stdout/stderr and detaches from the direct wait; the direct `cmd` exits
immediately.

**What happened.**

```
Run returned after 5.0947555s (timedOut=true exit=-1)
```

The child ping runs about five seconds, and `Run` returns then, not at the
500 ms deadline. The same shape with `ping -n 100000` would hold the call
for ~27 hours.

**Why it works.** `runOnce` sets `cmd.Stdout/Stderr` to `capBuf` (not an
`*os.File`), so `os/exec` creates pipes and copy goroutines; `cmd.Wait`
waits for those copies to finish after the direct child exits. On timeout
`killProcTree` runs `taskkill /T /F /PID <pid>` against a process that has
already exited, which cannot find the detached descendant, and
`cmd.Process.Kill()` has nothing left to kill. The read end of the pipe
stays open in the descendant, so `<-done` blocks until it exits.

**What should have happened.** At the deadline, `Run` should kill every
descendant and return promptly with `TimedOut=true`. The Windows build
already has the permitted tool for this in the spec: a job object
(`JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`), or a process-tree snapshot taken
while the direct child is alive. The existing C1.4.6 check passes because
its grandchild is an ordinary descendant (`TestHelperProcess` spawns
without detaching) and because the check allows up to 10 seconds for a
300 ms limit (`internal/world/c14_test.go:838`), so a five-second overrun
would still be green.

**How sure.** High; measured five times, deterministic.

**Reproduction.**

```
go test ./internal/world -run 'TestRT_RunDetachedChildHangs' -count=1 -v
```

The test's `ping` is ours and exits in about five seconds; nothing is left
running after the test returns.
