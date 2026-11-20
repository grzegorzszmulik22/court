# DT10 deletion — line rules

- `dt10-<lang>-sql` — .sql hunk or string literal adds DROP/TRUNCATE or
  DELETE FROM without WHERE.
- `dt10-<lang>-datafile` — a tracked data file is deleted (.csv/.parquet/
  .sqlite/.db or under data/, fixtures/).
- `dt10-<lang>-destructive-cmd` — a tool event ran rm -r*, git clean,
  git reset --hard, git push --force.
