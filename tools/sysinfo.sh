#!/usr/bin/env bash
# sysinfo.sh — machine description for measurement reports (05 section 12).
set -uo pipefail
cd "$(dirname "$0")/.." 2>/dev/null || true
echo "uname: $(uname -srmo 2>/dev/null || uname -a)"
echo "os: $(. /etc/os-release 2>/dev/null && echo "$PRETTY_NAME")"
echo "arch: $(uname -m)"
echo "cpus: $(nproc 2>/dev/null)"
echo "mem: $(awk '/MemTotal/{printf "%d MiB", $2/1024}' /proc/meminfo 2>/dev/null)"
echo "git: $(git --version 2>/dev/null | awk '{print $3}')"
echo "go: $(go version 2>/dev/null | awk '{print $3}')"
echo "python3: $(python3 --version 2>/dev/null | awk '{print $2}')"
echo "commit: $(git rev-parse HEAD 2>/dev/null || echo unknown)"
