#!/usr/bin/env bash
# C1.1.4 The SQLite driver builds with CGO_ENABLED=0 and the resulting binary
# is statically linked.
set -uo pipefail
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

CGO_ENABLED=0 go build -o "$tmp/court" ./cmd/court \
    || { echo "CGO_ENABLED=0 build failed" >&2; exit 1; }

if command -v file >/dev/null; then
    file "$tmp/court" | grep -q "statically linked" \
        || { echo "binary is not statically linked: $(file "$tmp/court")" >&2; exit 1; }
else
    ldd "$tmp/court" 2>&1 | grep -q "not a dynamic executable" \
        || { echo "binary is not statically linked: $(ldd "$tmp/court" 2>&1)" >&2; exit 1; }
fi

out=$("$tmp/court" version) || { echo "static binary failed to run" >&2; exit 1; }
grep -q "sqlite" <<<"$out" \
    || { echo "sqlite driver not usable in the binary: $out" >&2; exit 1; }
