# Progress: lane `gate` — Court M3.2 (the declaration gate)

Windows 11 host (Git Bash); Linux behaviour is authoritative (CR-002). Row
format of `spec/05-agent-loop.md` §13; evidence folders are committed.

| chunk | status | attempts | last evidence folder | note |
|---|---|---|---|---|
| M3.2 | DONE | 1 | evidence/M3.2/2026-11-17T17-08-18Z | Checks C3.2.1–C3.2.6 PASS twice from `make clean` (second run above); 6/6 sabotages caught, 0 uncaught (evidence/M3.2/2026-11-17T17-09-09Z-sabotage; the three spec-named ones plus gate-window-never-closes, repo-hooks-skipped, sync-not-triggered). internal/gate installs pre-commit and post-commit through core.hooksPath into `<data-dir>/hooks` (outside every world), derives the per-world declaration window from ledger order, stamps `gated` only on the pre-commit parent+tree handshake, and scans world heads on ledger writes so a hooks-bypassed commit is recorded `gated:false` within one second. `no_decisions` (decision.none) added to the six agent tools; `court gate pre` is the hook's query. Repository hooks run after Court's, including a repository-configured core.hooksPath. Upstream fact: `origin/main` lacked lane/windows2's verified Windows fixes (no `script`, daemon log leaks), so `make regress` was red on this host before this chunk; I adopted those portability changes (internal/commission test helpers, daemon shutdown plumbing, TestMain binary naming) verbatim from the VERIFIED lane/windows2 without touching any check threshold. |
