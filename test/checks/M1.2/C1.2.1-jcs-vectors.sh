#!/usr/bin/env bash
# C1.2.1 Canonical bytes match the RFC 8785 test vectors (upstream excerpt saved).
#
# Vectors: the json-canonicalization testdata input/output/outhex corpus, the
# RFC 8785 Appendix B IEEE-754 number table, the section 3.2 sorting test, and
# a sha256-covered deterministic stream of 10000 ES6 number serializations
# (testdata/README.md records the expected sha256 of lines "hex,expected").
set -uo pipefail
go test ./internal/ledger/ -run 'TestJCS' -count=1 \
    || { echo "RFC 8785 vectors failed" >&2; exit 1; }
