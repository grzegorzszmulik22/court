# M1.4 supervisor run on Windows (CR-001), lane commit 9a0798b

Host: Windows 11 Pro 10.0.26200, go1.27.4 windows/amd64 (GOTOOLCHAIN=local), Git for Windows 2.55, LongPathsEnabled=1.

- `go vet ./...`: ok
- `go test ./... -count=1`: ok (internal/world 103s)
- Skips in internal/world: TestC1_4_1_DiscardReportsError only (POSIX directory permissions; the code path is shared and is defended on Linux).
- Supervisor sabotages (unseen by the lane), each alone:
  - windows-kill-parent-only: caught (C1.4.6, descendants survived 30s)
  - windows-longpaths-not-set: caught (C1.4.7, commit of >260 path failed)
  - root-limit-not-enforced-on-open: caught (C1.4.7, Open accepted a >200 root)
- Clean tree re-run: PASS
