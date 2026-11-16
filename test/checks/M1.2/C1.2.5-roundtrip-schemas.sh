#!/usr/bin/env bash
# C1.2.5 Round trip for every event type; every Go body type has a schema and
# every schema a Go type. Also asserts strict decoding: unknown fields and
# duplicate keys are rejected at envelope and body level.
set -uo pipefail
go test ./internal/ledger/ -run 'TestBodyRoundTrip|TestSchemaBijection|TestStrictDecode|TestEnvelopeStrict' -count=1 \
    || { echo "event-type round trip / schema bijection / strict decode failed" >&2; exit 1; }
