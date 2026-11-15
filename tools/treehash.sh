#!/usr/bin/env bash
# treehash.sh — one digest over every tracked plus untracked-nonignored file in
# the working tree, except evidence/ (check runs write there) and .tools/
# (a symlink or machine-local toolchain). Used by C1.1.3 to prove that
# `make sabotage-<chunk>` never modifies the working tree.
set -euo pipefail
cd "$(dirname "$0")/.."
git ls-files -co --exclude-standard -z \
    | grep -zv -e '^evidence/' -e '^\.lane/' \
    | sort -z \
    | xargs -0 sha256sum \
    | sha256sum \
    | cut -d' ' -f1
