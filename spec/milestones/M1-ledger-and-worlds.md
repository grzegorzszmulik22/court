# M1. Ledger and worlds

**Goal.** Record everything agents do, and confine what they do. A hash-chained ledger with a second, independent verifier; isolated worlds that can be discarded exactly; a scripted agent for hermetic checks; the hook endpoint; the first agent tools; and the Claude Code adapter.

**Properties advanced.** P1 (complete record), P2 (deterministic logic), P3 (isolation). Headlines H1 to H4.

**Normative references.** `01` §2, §11; `02` §2 to §6, §8.

**Expected results.**
- Three scripted agents work in three worlds at once, and every tool event and commit is in the ledger in order.
- Any single-byte change to a stored event is found by two independent verifiers.
- Discarding a world leaves the repository exactly as it was.
- A real Claude Code session can be started in a world and observed, with outward actions denied.

**Measurements [T].**

| ID | Metric | Target |
|---|---|---|
| MM1.1 | Scripted tool events and commits missing from the ledger | 0 of ≥ 10,000 (H1) |
| MM1.2 | Replay reproduces the stored state; mutants detected; Go and Python verifiers agree | 100% of ≥ 200 ledgers; 100% of ≥ 1,000; 100% (H2) |
| MM1.3 | Repository digest restored after discard | 100% of ≥ 200 randomised sessions (H3) |
| MM1.4 | Added latency per tool event (hook round trip, loopback) | p99 ≤ 50 ms (H4) |
| MM1.5 | Ledger append throughput, single writer | ≥ 2,000 events/s |

**Milestone-wide denied.** Signatures, Merkle trees, encryption (`00` D11). Any web interface. A second world backend. Containers. Adapters other than Claude Code and the scripted agent. Storing file contents or command output in the ledger (`01` §11.3). A plug-in system.

---

## M1.1 Repository, toolchain, check tooling

**Build.** Go module; `tools/versions.env` pinning Go, git, the SQLite driver, the MCP Go SDK, Python, and the Claude Code version whose hook shapes are recorded; `make tools` installing into `./.tools/` without root; `make check-<chunk>`, `make sabotage-<chunk>`, `make regress`, `make lint`, `make clean`, `make sysinfo`; automatic capture of every run into `evidence/<chunk>/<UTC timestamp>/`. `tools/host.md` is written by the human first.

Output conventions, relied on by the supervisor's verifier:

- `make check-<chunk>` prints exactly one line per check, `CHECK <id> PASS`, `CHECK <id> FAIL <reason>`, `CHECK <id> BLOCKED <reason>` or `CHECK <id> SKIP <reason>`, where `<id>` is the check ID from the spec (`C1.2.3`). It exits non-zero unless every check passed.
- `make sabotage-<chunk>` applies each patch under `test/sabotage/<chunk>/` to a scratch copy, runs the chunk's checks, and ends with one line `<n> uncaught`. It exits non-zero when `n` is not 0 and names each uncaught patch.
- Each run writes `result.json` in its evidence folder: command line, exit code, duration, git commit.
- `make lint` includes a linter for the rows of `spec/progress/*.md` (`05` §13).

**Checks.**
- C1.1.6 A dummy chunk with a passing suite and a real sabotage: `check` prints its `CHECK … PASS` lines and exits 0; `sabotage` prints `0 uncaught`.
- C1.1.7 A dummy chunk whose suite ignores the sabotage: `sabotage` exits non-zero, prints `1 uncaught` and names the patch.
- C1.1.8 The evidence folder of a run contains the command line, exit code, duration, git commit and full output.
- C1.1.1 `make check-M1.1` passes from a fresh clone with the network disabled after `make tools`.
- C1.1.2 A check that exits non-zero leaves its output in the evidence folder and fails the target.
- C1.1.3 `make sabotage-M1.1` applies a patch to a scratch copy, never to the working tree (the tree's hash is unchanged afterwards).
- C1.1.4 The SQLite driver builds with `CGO_ENABLED=0` and the resulting binary is statically linked.
- C1.1.5 `FROZEN.sha256` verification fails when a listed file changes.

**Sabotage.** `evidence-capture-drops-stderr`; `frozen-check-skips-missing-file`; `sabotage-runner-always-green`.

**Denied.** A continuous-integration service. Docker. Global tool installs. Retrying a flaky check: a check that passes only on retry is a failing check.

## M1.2 Ledger

**Build.** `internal/ledger`: the envelope and canonical bytes of `01` §11.1, strict decoding (unknown fields and duplicate keys rejected), the SQLite store, single-writer append with the chain, typed bodies for every event of `01` §11.2 that M1 emits, and JSON Schemas in `api/schema/`.

**Checks.**
- C1.2.1 Canonical bytes match the RFC 8785 test vectors (upstream excerpt saved).
- C1.2.2 A chain of 10,000 events: `seq` gap-free, every `prev` verifies, event 0 has the zero `prev`.
- C1.2.3 Two processes appending at once: one succeeds per `seq`, none is lost, the chain holds.
- C1.2.4 A crash injected between write and commit leaves a ledger that verifies, at each of 200 injection points.
- C1.2.5 Round trip for every event type; every Go body type has a schema and every schema a Go type.
- C1.2.6 MM1.5.

**Sabotage.** `prev-hashes-body-only`; `seq-reused-after-failed-append`; `decoder-accepts-unknown-field`.

**Denied.** Event deletion, compaction or update. Optional fields "for later".

## M1.3 Reference verifier, replay, mutations

**Build.** `ref/python/refledger`, written from `01` §11 without reading the Go code: reads a ledger file, verifies the chain and every body against its schema. `court replay`: rebuilds derived state from events and compares it with stored state. A mutation tool producing seeded single-change mutants: byte flip, event removed, two events swapped, `seq` edited, `prev` edited, body field edited, event duplicated, truncation.

**Checks.**
- C1.3.1 MM1.2 on ≥ 200 generated ledgers and ≥ 1,000 mutants, both verifiers reporting the same error class for each mutant.
- C1.3.2 Golden ledgers in `test/vectors/ledger/` verify in both; frozen at M1 exit.
- C1.3.3 Replay of a ledger produced by M1.8's run yields identical derived tables (row-for-row comparison).
- C1.3.4 The Python verifier runs with only the standard library.

**Sabotage.** `verifier-skips-last-event`; `replay-reads-derived-tables`; `mutant-truncation-accepted`.

**Denied.** Sharing code or generated files between the two verifiers other than the JSON Schemas.

## M1.4 Worlds

**Build.** `internal/world`: the interface and git worktree backend of `02` §3, including per-world git settings passed by environment, garbage collection off during a commission, the submodule refusal, and the digest used for H3.

**Checks.**
- C1.4.1 MM1.3: random sequences of fork, edit, commit, run and discard across up to 8 worlds.
- C1.4.2 Two worlds cannot hold the same branch; the error is reported, not retried.
- C1.4.3 Eight worlds committing concurrently for 60 seconds: no lost commit, no ref-lock failure surfaced to an agent (retries are internal and counted).
- C1.4.4 `git config --list` for the repository is byte-identical before a commission and after it.
- C1.4.5 A repository with a submodule is refused with one sentence and no world is created.
- C1.4.6 `Run` enforces its time limit and reports a timeout distinctly from a non-zero exit.
- C1.4.7 On Windows: a world path longer than 260 characters works. `SKIPPED-ENV` where no Windows host exists.

**Sabotage.** `discard-leaves-branch`; `gc-left-enabled`; `config-written-to-repo`; `run-timeout-reported-as-exit-1`.

**Denied.** Copying the repository instead of using worktrees. Merging worlds (that arrives with rulings).

## M1.5 Scripted agent and the hook endpoint

**Build.** The daemon with `/hook/<token>`; the mapping from hook payloads to `tool.event`; `internal/adapter/scripted` per `02` §4.2; `test/vectors/hooks/`: payload fixtures recorded from a real session at the pinned version, one per hook event Court consumes, including one from inside a subagent.

**Checks.**
- C1.5.1 A script of 500 mixed steps yields exactly the expected `tool.event` and `world.commit` events, in order.
- C1.5.2 A payload with an unknown token is refused and recorded as refused; nothing is attributed.
- C1.5.3 The stored `tool.event` contains no file contents and no command output for any fixture (asserted over every fixture).
- C1.5.4 A subagent payload is recorded under a child agent id whose parent is the session's agent.
- C1.5.5 The endpoint binds to loopback only; a connection from another interface is refused.
- C1.5.6 Fixture drift: a test compares the field names of each fixture with a field list in `tools/hook-fields.txt` and fails on a difference.

**Sabotage.** `hook-attributes-by-session-id-not-token`; `tool-event-stores-full-input`; `subagent-recorded-as-parent`.

**Denied.** Parsing agent prose. Recording model reasoning or message text.

## M1.6 Agent tools

**Build.** `internal/tools`: the MCP endpoint `/mcp/<token>` on the official Go SDK, with `record_decision`, `claim_done` (record only; the check run arrives in M3.6) and `alarm`. Tool descriptions per `02` §5. Pairing of a tool call with its `PreToolUse` event for subagent attribution.

**Checks.**
- C1.6.1 Each tool validates its input against its schema; a bad `category` is rejected with the list of valid ones.
- C1.6.2 A call on one token is never attributed to another agent (1,000 interleaved calls from 8 scripted agents).
- C1.6.3 An `alarm` from a child agent is in the ledger even when the parent's script drops every message it receives.
- C1.6.4 The endpoint works with a client speaking the pinned protocol revision without a session header.
- C1.6.5 Tool descriptions are each one sentence of at most 200 characters.

**Sabotage.** `tools-share-one-endpoint-without-token`; `alarm-routed-through-parent`; `category-not-validated`.

**Denied.** Tools beyond those of `02` §5. Prompts, resources or sampling features of the protocol.

## M1.7 Claude Code adapter

**Build.** `internal/adapter/claude` per `02` §4.1: settings and MCP configuration generated per session, process start in the world, the outward-action deny list, follow-up input, stop.

**Checks (hermetic).**
- C1.7.1 Generated settings and MCP configuration validate against fixtures of the documented shapes.
- C1.7.2 The deny decision is returned for every entry of the outward list and for a file path inside the data directory, in the documented response shape; and not for an ordinary edit or `go test`.
- C1.7.3 The adapter starts a stand-in executable (a test double printing recorded stream lines) with the world as its working directory and the documented flags, asserted from the double's own log.

**Checks (live, not part of done).**
- L1.7.1 `make live-claude-smoke`: a real session in a world edits one file, runs a test, calls `record_decision`, and is stopped. The ledger shows each step; an attempted `curl` is denied. Token count recorded.

**Sabotage.** `outward-list-not-applied`; `settings-written-into-repo`; `datadir-path-allowed`.

**Denied.** Using the vendor's SDK packages (they have no Go version, and the command line is enough). Auto-approving anything outside the world.

## M1.8 Completeness and overhead

**Build.** A load run: 8 scripted agents, 10,000 or more tool events and 500 or more commits in total, with random interleaving; the latency harness for MM1.4.

**Checks.**
- C1.8.1 MM1.1: every scripted step has exactly one ledger event; the comparison is against the scripts, not against Court's own counters.
- C1.8.2 MM1.4, measured as a paired difference: hook round trip against a no-op endpoint.
- C1.8.3 Killing the daemon mid-run and restarting it loses no acknowledged event; scripted agents whose hook post failed retry, and no step is recorded twice.
- C1.8.4 The exit review for M1 answers the two questions of `00` §5.

**Sabotage.** `ack-before-durable-write`; `retry-creates-duplicate-event`.

**Denied.** Batching that acknowledges before the write is durable.
