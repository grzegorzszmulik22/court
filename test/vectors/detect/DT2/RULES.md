# DT2 schema — line rules

- `dt2-go-new-migration`, `dt2-python-new-migration`, `dt2-typescript-new-migration`
  — a file added under a recognised migration directory.
- `dt2-<lang>-changed-migration` — a file changed under a recognised
  migration directory (any change, comments included).
- `dt2-<lang>-ddl` — a .sql hunk adding CREATE/ALTER/DROP of a table, column or index.
- `dt2-<lang>-format` — a .proto/.avsc/*.schema.json/schema.prisma file changes
  (any change, comments included).
