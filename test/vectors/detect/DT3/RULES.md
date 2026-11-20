# DT3 interface — line rules

- `dt3-go-exported-removed` — exported Go identifier removed outside
  internal/ and _test.go.
- `dt3-go-exported-changed` — exported Go signature changed.
- `dt3-python-toplevel-removed` — top-level public def/class removed.
- `dt3-python-toplevel-changed` — top-level public def signature changed.
- `dt3-ts-export-removed`, `dt3-ts-export-changed` — exported TS symbol
  removed or its parameter list changed.
- `dt3-contract-openapi`, `dt3-contract-graphql`, `dt3-contract-proto` —
  a contract file changed. `added` marks a new contract element (a path,
  a field), `removed` a dropped one, `changed` any other edit including
  comments. `added`/`removed`/`changed` on an ordinary source file are
  not hits: spec/03 §2 fires on removals and signature changes only.
