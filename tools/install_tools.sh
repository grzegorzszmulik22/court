#!/usr/bin/env bash
# install_tools.sh [--if-needed]
# Installs the pinned toolchain of tools/versions.env into ./.tools/ without root.
# Idempotent: an already-correct install is verified and left alone.
#
#   --if-needed   quiet, quick verification pass; installs only what is missing
set -euo pipefail
cd "$(dirname "$0")/.."

TOOLS=$PWD/.tools
BIN=$TOOLS/bin
mkdir -p "$BIN" "$TOOLS/dl"

# Resolve system binaries only from outside .tools. The Makefile puts a
# .tools/bin on PATH (possibly a symlinked one, e.g. inside a scratch copy or a
# fresh clone), so drop every PATH entry that *resolves* to the real
# .tools/bin; otherwise `command -v` finds our own wrappers and they self-exec.
BIN_REAL=$(readlink -f "$TOOLS")/bin
SYS_PATH=$(printf '%s' "$PATH" | tr ':' '\n' | while read -r d; do
    [ -z "$d" ] && continue
    c=$(readlink -f "$d" 2>/dev/null || echo "$d")
    [ "$c" = "$BIN_REAL" ] || printf '%s:' "$d"
done | sed 's/:$//')

# shellcheck disable=SC1091
. tools/versions.env

QUIET=0
[ "${1:-}" = "--if-needed" ] && QUIET=1
say() { [ "$QUIET" -eq 1 ] || echo "tools: $*"; }
die() { echo "tools: ERROR $*" >&2; exit 1; }

ver_ge() { # ver_ge <have> <floor>: true when have >= floor
    [ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | head -1)" = "$2" ]
}

# --- Go toolchain -----------------------------------------------------------
arch=$(uname -m)
case "$arch" in
    aarch64|arm64) goarch=arm64; want_sha=$GO_SHA256_LINUX_ARM64 ;;
    x86_64|amd64)  goarch=amd64; want_sha=$GO_SHA256_LINUX_AMD64 ;;
    *) die "unsupported arch $arch" ;;
esac
tarball="go${GO_VERSION}.linux-${goarch}.tar.gz"

need_go=1
if [ -x "$TOOLS/go/bin/go" ] && [ "$("$TOOLS/go/bin/go" version 2>/dev/null | awk '{print $3}')" = "go${GO_VERSION}" ]; then
    need_go=0
    say "go ${GO_VERSION} already installed"
fi
if [ "$need_go" -eq 1 ]; then
    say "downloading $tarball"
    curl -fsSL -o "$TOOLS/dl/$tarball" "https://dl.google.com/go/${tarball}" \
        || die "download of $tarball failed (network needed for make tools)"
    echo "$want_sha  $TOOLS/dl/$tarball" | sha256sum -c - >/dev/null \
        || die "sha256 mismatch for $tarball"
    rm -rf "$TOOLS/go"
    tar -C "$TOOLS" -xzf "$TOOLS/dl/$tarball"
    rm -f "$TOOLS/dl/$tarball"
    say "installed go ${GO_VERSION} (linux/${goarch}, sha256 verified)"
fi
ln -sf ../go/bin/go "$BIN/go"
ln -sf ../go/bin/gofmt "$BIN/gofmt"

# --- git: verified system binary --------------------------------------------
git_bin=$(PATH="$SYS_PATH" command -v git || true)
[ -n "$git_bin" ] || die "no git on PATH and no arm64 binary distribution exists"
[ "$(readlink -f "$git_bin")" = "$BIN_REAL/git" ] && die "resolved git is inside .tools"
got=$("$git_bin" --version | awk '{print $3}')
ver_ge "$got" "$GIT_VERSION" || die "git $got < pinned $GIT_VERSION"
if [ ! -f "$BIN/git" ] || ! grep -qF "exec $git_bin" "$BIN/git" 2>/dev/null; then
    printf '#!/bin/sh\nexec %s "$@"\n' "$git_bin" >"$BIN/git"
    chmod +x "$BIN/git"
fi
say "git $got (system, >= pinned $GIT_VERSION)"

# --- python3: verified system binary ----------------------------------------
py_bin=$(PATH="$SYS_PATH" command -v python3 || true)
[ -n "$py_bin" ] || die "no python3 on PATH"
[ "$(readlink -f "$py_bin")" = "$BIN_REAL/python3" ] && die "resolved python3 is inside .tools"
"$py_bin" -c "import sys; sys.exit(0 if sys.version_info[:2] >= tuple(int(x) for x in '$PYTHON_VERSION'.split('.')) else 1)" \
    || die "python3 < pinned $PYTHON_VERSION"
if [ ! -f "$BIN/python3" ] || ! grep -qF "exec $py_bin" "$BIN/python3" 2>/dev/null; then
    printf '#!/bin/sh\nexec %s "$@"\n' "$py_bin" >"$BIN/python3"
    chmod +x "$BIN/python3"
fi
say "python3 $("$py_bin" --version | awk '{print $2}') (system, >= pinned $PYTHON_VERSION)"

# --- Go module cache ---------------------------------------------------------
# Warm the pinned module cache so `go build` never needs the network.
say "warming module cache"
GOMODCACHE="$TOOLS/gomod" GOPROXY=https://proxy.golang.org,direct \
    "$BIN/go" mod download || die "go mod download failed"
say "module cache warmed"

cat >"$TOOLS/.resolved" <<EOF
go $("$BIN/go" version | awk '{print $3}') $("$BIN/go" env GOOS GOARCH | tr '\n' '/' | sed 's:/$::')
git $("$BIN/git" --version | awk '{print $3}') $git_bin
python3 $("$BIN/python3" --version | awk '{print $2}') $py_bin
sqlite-driver $SQLITE_DRIVER_VERSION (go.mod)
mcp-go-sdk $MCP_GO_SDK_VERSION (go.mod)
claude-code $CLAUDE_CODE_VERSION (recorded from public docs; not installed)
EOF
say "toolchain ready in .tools/ (.tools/.resolved)"
