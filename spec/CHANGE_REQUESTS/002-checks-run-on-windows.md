# CR-002: Court's own tooling and checks run on Windows

**Status:** ACCEPTED by the supervisor on 2026-11-17, under the human's CR-001 decision ("it should run on Windows"). No check, threshold or denied item changes.
**Raised by:** supervisor, 2026-11-17, when a Windows PC became a build host for Court lanes
**Affects:** `tools/`, `test/checks/`, `test/sabotage/` (portability only); every M1 chunk's checks

## What was found

CR-001 made Court's code build and pass its Go tests on Windows, but the tooling around it still assumed Linux. In Git Bash on Windows 11:

- `make tools` downloaded the Linux Go tarball and took the Microsoft Store `python3` alias for a real Python. Fixed by the supervisor (`tools/install_tools.sh`, `tools/versions.env`, `Makefile`): the Windows zip is sha256-verified like the tarballs, and the first Python that really runs is used.
- With that fixed, `make lint` and `make check-M1.2` pass. Three checks still fail for portability reasons, not product bugs:
  - C1.1.1: a step runs `go` without GOROOT ("'go' binary is trimmed and GOROOT is not set").
  - C1.1.4: the static-link test expects an ELF binary and reads `file` output.
  - C1.3.1 and C1.3.2: the Go and Python verifiers disagree on Windows (suspected: line endings or path separators in compared output).

## Decision

Every M1 check, its sabotages, `make lint` and `make regress` must pass on Windows (Git Bash, the pinned tools from `make tools`) as well as on Linux, with the same meaning:

- **C1.1.4 on Windows.** "Statically linked" means the binary was built with `CGO_ENABLED=0` and imports only Windows system DLLs (no C runtime or other third-party DLLs). The check states which form it tested.
- **Line endings and separators** are normalised where output is compared, never by weakening what is compared.
- A check that cannot have a Windows meaning is listed in `tools/env-skips.txt` with its reason. None is expected.

## Effect on properties and numbers

None weakened. Checks gain a second platform, and that platform is the supervisor's PC, where Windows behaviour is already re-verified by hand.
