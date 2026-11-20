# DT9 authority — line rules

- `dt9-<lang>-touched` — a changed path has a segment matching
  auth/login/session/token/permission/acl/rbac/secret/credential/crypto.
  Segment matching is exact: the whole `/`-separated path segment must
  equal the term (the term may carry the file extension, e.g.
  `auth/session.go`). `test_crypto.py`, `login_runbook.md` and
  `token_count.csv` all fail it the same way — the term is inside a
  longer segment, not a segment of its own; use real segments
  (`ops/login/runbook.md`, `data/token/count.csv`) for positive cases.
- `dt9-<lang>-widened` — an added line widens access (`*` origin, mode
  777, wildcard in a policy file). Widening means broadening what may be
  done or reached, not listing a stronger scope: `scopes: [read, admin]`
  is a permission grant change, not an access-surface widening.
