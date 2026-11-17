#!/usr/bin/env bash
# C1.1.4 The SQLite driver builds with CGO_ENABLED=0 and the resulting binary
# is statically linked.
set -uo pipefail
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

CGO_ENABLED=0 go build -o "$tmp/court" ./cmd/court \
    || { echo "CGO_ENABLED=0 build failed" >&2; exit 1; }
# `-o <name>` is the exact output on every host, but accept the .exe form too.
if [ -f "$tmp/court" ]; then
    bin=$tmp/court
elif [ -f "$tmp/court.exe" ]; then
    bin=$tmp/court.exe
else
    echo "CGO_ENABLED=0 build produced no binary" >&2; exit 1
fi

case "$(uname -s)" in
MINGW*|MSYS*|CYGWIN*)
    # CR-002: on Windows "statically linked" means the binary was built with
    # CGO_ENABLED=0 and imports only Windows system DLLs (no C runtime or other
    # third-party DLLs). Read the PE import directory: every imported DLL must
    # exist in the Windows system directory (or be an API-set contract).
    echo "static form: Windows PE, CGO_ENABLED=0, imports must be Windows system DLLs" >&2
    python3 - "$bin" <<'PYEOF' || { echo "binary is not statically linked (PE imports)" >&2; exit 1; }
import os, struct, sys
p = sys.argv[1]
b = open(p, 'rb').read()
if b[:2] != b'MZ':
    raise SystemExit('not a PE binary')
pe = struct.unpack_from('<I', b, 0x3c)[0]
if b[pe:pe+4] != b'PE\0\0':
    raise SystemExit('not a PE binary')
nsec = struct.unpack_from('<H', b, pe + 6)[0]
optsz = struct.unpack_from('<H', b, pe + 20)[0]
oh = pe + 24
magic = struct.unpack_from('<H', b, oh)[0]
if magic not in (0x10b, 0x20b):
    raise SystemExit('unknown optional-header magic %#x' % magic)
dd = oh + (112 if magic == 0x20b else 96)
imp_rva, _ = struct.unpack_from('<II', b, dd + 8)
secs = []
for i in range(nsec):
    off = oh + optsz + i * 40
    vsz, vaddr, rsz, raddr = struct.unpack_from('<IIII', b, off + 8)
    secs.append((vaddr, max(vsz, rsz), raddr))
def rva2off(rva):
    for vaddr, size, raddr in secs:
        if vaddr <= rva < vaddr + size:
            return raddr + (rva - vaddr)
    raise SystemExit('RVA %#x is outside every section' % rva)
names = []
if imp_rva:
    o = rva2off(imp_rva)
    while True:
        ilt, ts, fwd, nrva, iat = struct.unpack_from('<IIIII', b, o)
        if not ilt and not nrva:
            break
        no = rva2off(nrva)
        end = b.index(b'\0', no)
        names.append(b[no:end].decode('ascii'))
        o += 20
sysroot = os.environ.get('SystemRoot', r'C:\Windows')
system32 = os.path.join(sysroot, 'System32')
bad = []
for n in names:
    low = n.lower()
    if low.startswith('api-ms-win-') or os.path.isfile(os.path.join(system32, low)):
        continue
    bad.append(n)
print('imports: %s' % (', '.join(names) or 'none'))
if bad:
    print('non-system DLLs imported: %s' % ', '.join(bad))
    sys.exit(1)
PYEOF
    ;;
*)
    if command -v file >/dev/null; then
        file "$bin" | grep -q "statically linked" \
            || { echo "binary is not statically linked: $(file "$bin")" >&2; exit 1; }
    else
        ldd "$bin" 2>&1 | grep -q "not a dynamic executable" \
            || { echo "binary is not statically linked: $(ldd "$bin" 2>&1)" >&2; exit 1; }
    fi
    ;;
esac

out=$("$bin" version) || { echo "static binary failed to run" >&2; exit 1; }
grep -q "sqlite" <<<"$out" \
    || { echo "sqlite driver not usable in the binary: $out" >&2; exit 1; }
