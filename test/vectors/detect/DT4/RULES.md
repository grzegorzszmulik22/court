# DT4 test-strength — line rules (the five honest-ci checks)

- `dt4-<lang>-deleted_test` — a test file or test function was removed
  (a rename removes the old name).
- `dt4-<lang>-skip_or_focus` — a skip/only/expected-failure marker was added.
- `dt4-<lang>-fewer_assertions` — assertion count in a surviving test fell
  (an assertion moved into another test counts against the test it left).
- `dt4-<lang>-changed_expectation` — expected literal changed while the
  call did not (equality literals included; only inequality bounds are
  judged by direction).
- `dt4-<lang>-loosened_threshold` — numeric bound moved in the permissive direction.

Near misses that must NOT fire: assertion-moved (assertion relocated
inside the same test function), threshold-tightened (an inequality bound
moved the strict way), assertion-added, expectation changes where the
call under test changed too.
