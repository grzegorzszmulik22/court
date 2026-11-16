#!/usr/bin/env python3
"""frozen_verify_sep_test.py — test for the Windows separator fix in
tools/frozen_verify.py (CR-001 lane note: the only tools/ file this lane may
edit, and the fix must carry a test).

On Windows, glob.glob/os.path.relpath produce '\\'-separated paths while the
manifest lists '/'-separated paths: every frozen file would report as
unlisted. The fix normalises separators before comparing; this test proves
the normalisation and exercises a full verify end to end.
"""
import hashlib
import importlib.util
import os
import sys
import tempfile

tool = os.path.join(os.path.dirname(os.path.abspath(__file__)),
                    "..", "..", "tools", "frozen_verify.py")
spec = importlib.util.spec_from_file_location("frozen_verify", tool)
fv = importlib.util.module_from_spec(spec)
spec.loader.exec_module(fv)

# The separator normalisation itself: a Windows-style relpath result must
# compare equal to the manifest's '/'-separated entry, on any host.
assert fv.normpath("spec\\milestones\\M1-x.md") == "spec/milestones/M1-x.md"
assert fv.normpath("spec/milestones/M1-x.md") == "spec/milestones/M1-x.md"
assert fv.normpath("plain.md") == "plain.md"

# End to end: a synthetic manifest + tree verifies clean; an unlisted file
# matching a frozen pattern is still caught through the same code path.
with tempfile.TemporaryDirectory() as tmp:
    names = ["spec/00-a.md", "spec/milestones/M1-x.md"]
    for name in names:
        p = os.path.join(tmp, *name.split("/"))
        os.makedirs(os.path.dirname(p), exist_ok=True)
        with open(p, "w") as f:
            f.write(name)
    manifest = os.path.join(tmp, "spec", "FROZEN.sha256")
    with open(manifest, "w") as f:
        f.write("# frozen: spec/0*.md spec/milestones/*.md\n")
        for name in names:
            digest = hashlib.sha256(name.encode()).hexdigest()
            f.write(f"{digest}  {name}\n")

    argv, sys.argv = sys.argv, ["frozen_verify.py", manifest, tmp]
    try:
        if fv.main() != 0:
            print("FAIL: clean tree did not verify", file=sys.stderr)
            sys.exit(1)
    finally:
        sys.argv = argv

    # An unlisted file matching a frozen pattern must still be reported.
    extra = os.path.join(tmp, "spec", "01-b.md")
    with open(extra, "w") as f:
        f.write("extra")
    sys.argv = ["frozen_verify.py", manifest, tmp]
    if fv.main() != 1:
        print("FAIL: unlisted frozen-pattern file not reported", file=sys.stderr)
        sys.exit(1)

print("frozen_verify separator test: ok")
