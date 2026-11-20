# DT7 failure-handling — line rules

- `dt7-<lang>-swallowed` — an added error handler has an empty body or
  only pass/continue/constant return.
- `dt7-<lang>-broadened` — an exception clause is broadened to catch all.
- `dt7-<lang>-special-case` — a literal from a test file's inputs appears
  in a non-test conditional.
