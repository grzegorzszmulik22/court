# DT8 outbound — line rules

- `dt8-<lang>-host-literal` — added line contains a URL/host literal whose
  host appears nowhere in the base tree (subkind `code`).
- `dt8-<lang>-tool-event` — a tool event contacted a host outside the
  commission's allowed hosts (subkind `agent`). Presence in the base
  tree does not matter to this rule — only `allowed_hosts` does.

Near misses that must NOT fire: comment-hostname (host inside a comment),
host-already-present, allowed hosts. Localhost and private IPs are hosts
like any other: they fire `code` when a line adds them to the tree and
`agent` when a tool event contacts them outside `allowed_hosts`.
