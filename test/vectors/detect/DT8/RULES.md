# DT8 outbound — line rules

- `dt8-<lang>-host-literal` — added line contains a URL/host literal whose
  host appears nowhere in the base tree (subkind `code`).
- `dt8-<lang>-tool-event` — a tool event contacted a host outside the
  commission's allowed hosts (subkind `agent`).

Near misses that must NOT fire: comment-hostname (host inside a comment
or doc), host-already-present, allowed hosts, localhost/private.
