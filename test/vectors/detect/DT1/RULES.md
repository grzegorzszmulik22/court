# DT1 dependency — line rules (spec/03 §2.1)

A hit fires when a dependency entry is added, removed, or changes version
in a recognised manifest, or when a manifest changed while its lockfile
did not (`lock-mismatch`).

- `dt1-go-manifest-added` — `require` line added in go.mod (single or block form).
- `dt1-go-manifest-removed` — `require` line removed from go.mod.
- `dt1-go-manifest-version` — a `require`d module's version changes.
- `dt1-go-lock-unchanged` — go.mod changed, go.sum did not.
- `dt1-python-manifest-added` — pin added in requirements*.txt or pyproject dependencies.
- `dt1-python-manifest-removed` — pin removed.
- `dt1-python-manifest-version` — `==` version changed.
- `dt1-python-lock-unchanged` — manifest changed, requirements.lock did not.
- `dt1-typescript-manifest-added` — entry added under dependencies/devDependencies.
- `dt1-typescript-manifest-removed` — entry removed.
- `dt1-typescript-manifest-version` — semver string changed on an existing entry.
- `dt1-typescript-lock-unchanged` — package.json changed, package-lock.json did not.

Near misses that must NOT fire: dependency-reordered (same entries, new
order), comments mentioning packages, lockfile-only regeneration.
