# Build host

Named by the human on 15 Nov 2026 ("the Devin sessions on the VPS"), written down by the supervisor.

| Host | Role for Court | Account | Limits |
|---|---|---|---|
| `oci-free-a1` | **Primary.** Hub repository `~/git/court.git`; all Court lanes run here | `opc`: no sudo | Ubuntu 24.04, ARM64, 2 cores, 11 GB. Shared with the Countersign build and its kind cluster |
| `agent-vpc` | Not used for Court until the supervisor says so | `csbuild` | Shared with another application; cluster-free work only |

Rules on `oci-free-a1`:

- Everything Court builds lives under the lane directory `~/src/ct-<lane>`. Never read or write `~/src/cs-*`, `~/src/countersign`, `~/git/countersign.git` or `~/reviews`.
- Never use Docker, kind or kubectl. Never stop, inspect or reconfigure a process you did not start.
- No sudo, no global installs. The toolchain goes into `./.tools/` through `make tools`.
- The host is ARM64: every pinned tool must have a `linux/arm64` build.
- Two cores are shared with other work. A check that needs more than 10 minutes of CPU is a design problem; say so in a change request.
- Tests bind only to `127.0.0.1` on ports the operating system assigns.
