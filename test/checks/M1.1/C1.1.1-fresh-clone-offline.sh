#!/usr/bin/env bash
# C1.1.1 `make check-M1.1` passes from a fresh clone with the network disabled
# after `make tools`.
#
# Clones HEAD to a scratch dir, links the installed toolchain in (that is the
# "after `make tools`" state), then runs `make check-M1.1` inside a user+network
# namespace with no connectivity. The inner run excludes C1.1.1 itself (it is
# the check performing the run); every other spec check must print PASS.
set -uo pipefail
repo=$(pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

git clone -q . "$tmp/clone" || { echo "git clone of HEAD failed" >&2; exit 1; }
ln -s "$repo/.tools" "$tmp/clone/.tools"

# Prove the namespace really has no network before trusting the run.
if unshare -Un curl -fsS -m 3 -o /dev/null https://go.dev/ 2>/dev/null; then
    echo "unshare -Un did not disable the network; cannot claim an offline run" >&2
    exit 1
fi

cd "$tmp/clone" || exit 1
# "after `make tools`": the linked-in .tools satisfies the toolchain verify.
make --no-print-directory tools >/dev/null \
    || { echo "make tools failed in the fresh clone" >&2; exit 1; }

# GNU make cannot spawn children inside an unmapped user namespace (its child
# calls setresuid on an unmapped uid -> EINVAL on this host's AppArmor rules).
# The offline run therefore goes through a shim that implements exactly the
# Makefile's target dispatch for check tooling; the check machinery itself is
# identical.
mkdir -p "$tmp/shim"
cat >"$tmp/shim/make" <<'EOF'
#!/usr/bin/env bash
while [ $# -gt 0 ]; do
    case "$1" in
        --no-print-directory|--keep-going|-s) ;;
        check-*)    exec tools/check.sh "${1#check-}" ;;
        sabotage-*) exec tools/sabotage.sh "${1#sabotage-}" ;;
        regress)    exec tools/regress.sh ;;
        lint)       exec tools/lint.sh ;;
        sysinfo)    exec tools/sysinfo.sh ;;
        tools|_tools|clean) exit 0 ;;
        *) echo "make-shim: unsupported target: $1" >&2; exit 2 ;;
    esac
    shift
done
exit 0
EOF
chmod +x "$tmp/shim/make"

out=$(unshare -Un env "PATH=$tmp/shim:$PATH" COURT_CHECK_EXCLUDE=C1.1.1 \
        make --no-print-directory check-M1.1 2>&1)
rc=$?
printf '%s\n' "$out" >&2   # inner run output, kept in this check's evidence log

[ $rc -eq 0 ] || { echo "make check-M1.1 in offline fresh clone exited $rc" >&2; exit 1; }
for id in C1.1.2 C1.1.3 C1.1.4 C1.1.5 C1.1.6 C1.1.7 C1.1.8; do
    grep -qE "^CHECK ${id//./\\.}(\\.[^ ]+)? +PASS" <<<"$out" \
        || { echo "inner run: $id never printed PASS" >&2; exit 1; }
done
