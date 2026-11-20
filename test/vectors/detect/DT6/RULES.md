# DT6 guardrail — line rules

- `dt6-<lang>-ci` — a CI workflow loses a step/job or gains
  continue-on-error / `|| true`.
- `dt6-<lang>-lint` — a lint or formatter config disables a rule.
- `dt6-<lang>-threshold` — a coverage/quality threshold falls.
- `dt6-<lang>-strictness` — a compiler strictness flag is turned off.
- `dt6-<lang>-hook` — a git hook or pre-commit entry is removed.
- `dt6-<lang>-suppressed` — an inline lint suppression is added.
