# DT4 test-strength — line rules (the five honest-ci checks)

- `dt4-<lang>-deleted_test` — a test file or test function was removed.
- `dt4-<lang>-skip_or_focus` — a skip/only/expected-failure marker was added.
- `dt4-<lang>-fewer_assertions` — assertion count in a surviving test fell.
- `dt4-<lang>-changed_expectation` — expected literal changed while the call did not.
- `dt4-<lang>-loosened_threshold` — numeric bound moved in the permissive direction.

Near misses that must NOT fire: assertion-moved (assertion relocated to
another test), threshold-tightened (bound moved the strict way),
assertion-added, expectation changes where the call under test changed too.
