// Package world implements isolated agent workspaces ("worlds") on git
// worktrees, per spec/02-architecture.md §3. Version 1 has exactly one
// backend — git worktree — and refuses repositories with submodules
// (upstream marks multi-worktree submodule support incomplete).
//
// Isolation (property P3) is proven by the digest in this package: every ref
// and every object id reachable from a ref is hashed before Fork and after
// Discard, and the two digests must be equal (headline H3 / check C1.4.1).
package world

import "time"

// Commit is a commit id in the repository's object format (full hex).
type Commit string

// Changes is the result of Diff: per-file added/deleted line counts.
type Changes struct {
	Files []FileChange
}

// FileChange records the line counts of one path changed between two
// commits. Binary files report Added and Deleted as -1.
type FileChange struct {
	Path    string
	Added   int
	Deleted int
}

// Limits bound a Run call.
type Limits struct {
	// Timeout is the wall-clock limit of the command and every internal
	// retry. Zero selects the backend default (DefaultRunTimeout).
	Timeout time.Duration
}

// Result reports how a Run ended. A timeout is reported distinctly from a
// non-zero exit: TimedOut is true and ExitCode is -1. A command that ran to
// completion reports TimedOut false and its real exit code.
type Result struct {
	ExitCode int
	TimedOut bool
	Stdout   string
	Stderr   string
	Duration time.Duration
	// Retries counts internal ref-lock retries spent on this call; lock
	// failures are never surfaced to the caller (spec C1.4.3).
	Retries int
}

// World is one agent's isolated copy of the repository (spec/02 §3).
type World interface {
	// Fork creates a new isolated copy of the repository at base on its own
	// branch court/<commission>/<name>, checked out under the data
	// directory.
	Fork(base Commit, name string) (World, error)
	// Diff returns per-file line counts between two commits in this world.
	Diff(from, to Commit) (Changes, error)
	// Run executes cmd with this world as working directory and the
	// per-world git configuration in its environment. A ref-lock failure is
	// retried internally and counted; it is never reported to the caller.
	Run(cmd []string, limits Limits) (Result, error)
	// Commits lists the commits reachable from this world's HEAD,
	// newest first.
	Commits() ([]Commit, error)
	// Discard removes the worktree, deletes its branch, and prunes the
	// worktree's administrative files. It never deletes from the shared
	// object store — objects the world created stay unreachable garbage a
	// later gc collects; they cannot change Digest, so after Discard the
	// digest equals the digest before Fork.
	Discard() error
}
