# M3.1 plan — vectors and scenarios first

1. `bench/docketbench/validate` (Go): scenario rules of `04` §2 (twins, registry schema + `source` cites + keywords, necessity vs final.txt + `git diff --stat`, repo tests run, template cap, planted-not-only-change, script step vocabulary incl. `spawn`), vector counts/shape for `test/vectors/detect` and `test/vectors/reconcile` coverage.
2. `test/vectors/detect/DT<n>/<lang>/{pos,neg}-NNN.json` + `RULES.md` per detector: 12 detectors × {go,python,typescript} × ≥20 pos/≥20 neg; every `03` §2 near miss as negatives; DT4 ≥10 neg each for `changed_expectation`, `fewer_assertions` honest-ci FP shapes.
3. `test/vectors/reconcile/NNN.json` + README: ≥60 hand-derived traces covering every required §7 case, with reasoning comments.
4. `bench/docketbench/scenarios/{A,C,D}`: A 60+60, C 20+20, D 20+20; ≥4 scenarios per detector category in A; each `04` §5 variant in ≥8; twin reciprocity; ≤5 dirs per template; planted item never the only change.
5. Checks `test/checks/M3.1/C3.1.{1..6}-*.sh` + bad fixtures under `test/fixtures/docketbench/`; sabotages `test/sabotage/M3.1/{validator-ignores-missing-twin,validator-skips-repo-tests,validator-accepts-empty-registry-in-planted}.patch`.
6. `make check-M3.1`, `make sabotage-M3.1`, `make regress`, `make lint`; freeze: one commit updating `spec/FROZEN.sha256` with every new file; id to `evidence/M3.1/freeze.txt`; `docs/M3.1.md`; progress row; push.
