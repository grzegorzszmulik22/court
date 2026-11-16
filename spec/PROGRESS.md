# Progress

Updated by the implementation agent on every loop iteration (`05-agent-loop.md` §1, §13).
Statuses: `TODO`, `IN_PROGRESS`, `DONE`, `BLOCKED`, `BLOCKED-CR-NNN`, `SKIPPED-ENV`, `AWAITING-GATE`.

M0 is human work and is tracked here only so the gate is visible.

| chunk | title | status | attempts | last evidence folder | note |
|---|---|---|---|---|---|
| M0.1 | Intervention log (human) | TODO | | | |
| M0.2 | Decision labels on real agent history (human) | TODO | | | |
| **G0** | **Human gate: do the central bets hold on real work** | TODO | | | |
| M1.1 | Repository, toolchain, check tooling | DONE | 3 | evidence/M1.1/2026-11-15T16-25-07Z | lane foundation; supervisor review 1 sent back SKIP and stale-patch exit paths; review 2 caught 5/5 unseen sabotages. C1.1.1 runs through a make shim because GNU make cannot spawn children in an unprivileged namespace on this host (verified by supervisor) |
| M1.2 | Ledger | TODO | 0 | | |
| M1.3 | Reference verifier, replay, mutations | TODO | 0 | | |
| M1.4 | Worlds | BLOCKED-CR-001 | 3 | evidence/M1.4/2026-11-16T10-13-32Z-windows-supervisor | Merged; C1.4.7 run by the supervisor on a Windows PC (2026-11-16): FAIL, not SKIPPED-ENV. internal/world does not compile on Windows (Setpgid, syscall.Kill in Run), and C1.4.7 as written is unachievable: git refuses worktree roots over 260 chars. Long file paths inside a short-rooted world work with core.longpaths. Restated check and Windows Run fix proposed in spec/CHANGE_REQUESTS/001; awaiting a human decision. Earlier reviews: fail-open deletion in Discard fixed; 3/3 unseen sabotages caught |
| M1.5 | Scripted agent and the hook endpoint | TODO | 0 | | |
| M1.6 | Agent tools | TODO | 0 | | |
| M1.7 | Claude Code adapter | TODO | 0 | | |
| M1.8 | Completeness and overhead | TODO | 0 | | |
| M2.1 | The commission | TODO | 0 | | |
| M2.2 | Backbrief and ordering | TODO | 0 | | |
| M2.3 | The blind clerk | TODO | 0 | | |
| M2.4 | Writs and spawn | TODO | 0 | | |
| M2.5 | The upfront hearing | TODO | 0 | | |
| M3.1 | Vectors and scenarios first | TODO | 0 | | freeze commit recorded here |
| M3.2 | The declaration gate | TODO | 0 | | |
| M3.3 | Detectors DT1 to DT6 | TODO | 0 | | |
| M3.4 | Detectors DT7 to DT12 | TODO | 0 | | |
| M3.5 | Reconciliation: reference, then engine | TODO | 0 | | |
| M3.6 | Claims | TODO | 0 | | |
| M3.7 | Contests | TODO | 0 | | |
| M3.8 | Phrasing and the report | TODO | 0 | | |
| M3.9 | Scoring and measurements | TODO | 0 | | needs M0.2 for MM3.7 |
| **G1** | **Human gate: held-out run; specify phases 3 and 4** | TODO | | | |
