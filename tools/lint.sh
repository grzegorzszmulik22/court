#!/usr/bin/env bash
# lint.sh — make lint. gofmt, go vet, bash -n on every tracked shell script,
# and the spec/progress/*.md row linter (spec/05-agent-loop.md section 13).
set -uo pipefail
cd "$(dirname "$0")/.."

fail=0

echo "lint: gofmt"
unfmt=$(gofmt -l cmd internal test 2>/dev/null | grep -v '^$' || true)
if [ -n "$unfmt" ]; then
    echo "$unfmt" | sed 's/^/lint: not gofmt-clean: /'
    fail=1
fi

echo "lint: go vet"
go vet ./... || fail=1

echo "lint: bash -n"
while IFS= read -r f; do
    bash -n "$f" || { echo "lint: bash -n failed: $f"; fail=1; }
done < <(git ls-files '*.sh' 'tools/*.sh' 'test/**/*.sh' | sort -u | grep '\.sh$' || true)

echo "lint: progress rows"
if compgen -G "spec/progress/*.md" >/dev/null; then
    python3 tools/lint_progress.py spec/progress/*.md || fail=1
fi

exit $fail
