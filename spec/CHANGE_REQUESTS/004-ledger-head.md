# CR-004: The ledger's newest event is covered by its published head, not by the chain

**Status:** ACCEPTED by the supervisor on 2026-11-17.
**Raised by:** supervisor, 2026-11-17, from red-team finding B3-01 (branch `redteam/b3-m1`, `findings/01-ledger-tail-unsealed/`)
**Affects:** `01` §11.1; M1.2, M1.3 (`court ledger verify`, `court replay`)

## What was found

`01` §11.1 says the chain "detects corruption and out-of-band edits". A hash chain protects an event only through the `prev` of the event after it. So the newest event can be edited and re-hashed, deleted, or followed by a forged event (for example a `decision.ruled` with actor `human`), and both verifiers and `court replay` still accept the ledger. The red team showed all three. Editing the newest event is not "rewriting the whole file", which is the only case §11.1 excludes.

## Decision

1. **The claim is narrowed to what a chain can give.** The chain detects any change to an event that has a successor, and any gap, reorder or duplicate. The newest event, and anything appended after the last point a reader saw, is protected only by the **head**: the pair (`seq`, `hash`) of the newest event.
2. **The head is published.**
   - `court ledger verify` prints the head on success, in a stable format: `head seq=<n> hash=<hex>`.
   - `court replay` prints the head too.
   - Every report and queue listing Court shows the human ends with the head.
3. **Verifying against a head someone kept.** `court ledger verify --head <seq>:<hash>` fails unless the ledger contains exactly that event at that `seq`. A ledger cut back below that `seq` also fails. Events appended after the head are reported as appended since that head.
4. **No claim against forged tail events until rulings are signed.** A forged `decision.ruled` or other human event appended to the tail is not detectable by the chain. `01` §11.1 already says signatures arrive with human rulings (later); until then the documentation says so plainly beside the head.
5. **Single-byte claim stands.** M1's expected result ("any single-byte change to a stored event is found by two independent verifiers") is kept for events that have a successor. The verifiers check that the stored bytes are the canonical bytes (red-team finding B3-11), not only that re-canonicalised content hashes correctly.

## Effect on properties and numbers

No check or threshold is weakened. The newest-event case moves from an unstated gap to a stated limit with a tool (`--head`) that closes it for any reader who kept a head. New checks belong to M1.2/M1.3. Each needs a fixture that fails without it:
- a tail edit, a tail deletion and a forged tail event, each detected with `--head`;
- verify and replay print the head;
- non-canonical stored bytes are rejected.
