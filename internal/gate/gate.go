// Package gate implements the declaration gate of spec/03-detection.md §4
// (milestone M3.2): the pre-commit and post-commit git hooks Court installs
// through core.hooksPath into a directory outside every world, the
// declaration window ("has this agent declared since the previous commit in
// this world"), and the gate-pass handshake that lets a commit be recorded
// as gated.
//
// Everything here is a pure function of ledger order and the hooks'
// handshakes; no model takes part (spec/00 §6, D2/D3).
package gate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Message is the exact one-line refusal of spec/03 §4.
const Message = "Declare this commit's decisions with record_decision, or call no_decisions."

// HooksDir is where Court installs its hooks: under the data directory and
// outside every world (<data-dir>/worlds/<name>). The world backend points
// core.hooksPath here (spec/02 §3).
func HooksDir(dataDir string) string { return filepath.Join(dataDir, "hooks") }

// WorldKey scopes gate state by commission and world. One session owns a
// world, but world ids are not required to be unique across commissions.
func WorldKey(commission, world string) string { return commission + "/" + world }

// State is the declaration window per world, derived from ledger order:
// a declaration (decision.declared or decision.none) opens a world's gate;
// the next observed commit in that world closes it. It is safe for
// concurrent use.
type State struct {
	mu     sync.Mutex
	decl   map[string]int64 // world key -> seq of the latest declaration
	commit map[string]int64 // world key -> seq of the latest observed commit
}

// NewState returns an empty gate state.
func NewState() *State {
	return &State{decl: map[string]int64{}, commit: map[string]int64{}}
}

// Declared notes a declaration at ledger seq in the world.
func (s *State) Declared(key string, seq int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if seq > s.decl[key] {
		s.decl[key] = seq
	}
}

// Committed notes an observed commit at ledger seq in the world: any
// declaration made before it is consumed, so the next commit needs a new
// one (spec/03 §4.2, spec/01 §7.4).
func (s *State) Committed(key string, seq int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if seq > s.commit[key] {
		s.commit[key] = seq
	}
}

// Open reports whether the world's gate is open: a declaration was made
// after the last commit observed in that world. A world with no declaration
// (and no commit) is closed.
func (s *State) Open(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.decl[key] > s.commit[key]
}

// Pass is a pre-commit authorization: the pre-commit hook ran for the
// pending commit whose parent is Parent and whose index tree is Tree. A
// post-commit observation is gated only when both match, so a commit that
// bypassed the hook cannot be stamped gated by an unrelated pass.
type Pass struct {
	Parent string `json:"parent"`
	Tree   string `json:"tree"`
}

// InstallOptions configures hook installation.
type InstallOptions struct {
	// DataDir is the Court data directory; hooks go into HooksDir(DataDir).
	DataDir string
	// URL is the daemon's /gate base URL.
	URL string
	// CourtBin is the court binary the hooks exec. Empty writes hooks that
	// post with curl or wget.
	CourtBin string
}

// Install writes pre-commit and post-commit into the hooks directory. Both
// scripts run the repository's own hook of the same name afterwards, so
// hooks already present in the repository still run, after Court's.
func Install(o InstallOptions) error {
	if o.DataDir == "" || o.URL == "" {
		return fmt.Errorf("gate: DataDir and URL are required")
	}
	dir := HooksDir(o.DataDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for name, script := range map[string]string{
		"pre-commit":  preScript(o),
		"post-commit": postScript(o),
	} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			return err
		}
		if err := os.Chmod(path, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// repoHookSnippet finds the repository's own hook of $1 and runs it, after
// Court's, guarding against re-entering Court's hook through a configured
// core.hooksPath. The repository config is read with --file so the per-world
// GIT_CONFIG_* entries that point core.hooksPath at Court's directory do not
// hide the repository's own setting.
const repoHookSnippet = `repo_hook() {
  common=$(git rev-parse --git-common-dir 2>/dev/null) || return 0
  case "$common" in /*|?:*) ;; *) common="$top/$common" ;; esac
  hp=$(git config --file "$common/config" --get core.hooksPath 2>/dev/null) || hp=
  if [ -n "$hp" ]; then
    case "$hp" in /*|?:*) ;; *) hp="$top/$hp" ;; esac
    h="$hp/$1"
  else
    h="$common/hooks/$1"
  fi
  if [ -x "$h" ] && [ "$h" != "$0" ] && [ "${COURT_GATE_DEPTH:-}" != "1" ]; then
    COURT_GATE_DEPTH=1 "$h"
  fi
}
`

// shQuote single-quotes a value for /bin/sh.
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func preScript(o InstallOptions) string {
	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	b.WriteString("# Written by the Court daemon; do not edit.\n")
	b.WriteString("# The declaration gate (spec/03 section 4): a commit needs a\n")
	b.WriteString("# declaration from its agent since the previous commit in this world.\n")
	b.WriteString("top=$(git rev-parse --show-toplevel 2>/dev/null) || exit 0\n")
	b.WriteString("head=$(git rev-parse -q --verify HEAD 2>/dev/null) || head=\n")
	b.WriteString("tree=$(git write-tree 2>/dev/null) || tree=\n")
	if o.CourtBin != "" {
		fmt.Fprintf(&b, "%s gate pre --url %s --world \"$top\" --head \"$head\" --tree \"$tree\" --timeout 10s\n",
			shQuote(o.CourtBin), shQuote(o.URL+"/pre"))
		b.WriteString("rc=$?\n")
		b.WriteString("if [ $rc -eq 3 ]; then\n")
		fmt.Fprintf(&b, "  printf '%%s\\n' %s >&2\n", shQuote(Message))
		b.WriteString("  exit 1\n")
		b.WriteString("fi\n")
		b.WriteString("if [ $rc -ne 0 ]; then\n")
		b.WriteString("  printf '%s\\n' 'Court gate is not reachable; commit blocked.' >&2\n")
		b.WriteString("  exit 1\n")
		b.WriteString("fi\n")
	} else {
		b.WriteString("json=$(printf '{\"world_path\":\"%s\",\"head\":\"%s\",\"tree\":\"%s\"}' \"$(printf '%s' \"$top\" | sed 's/\\\\/\\\\\\\\/g; s/\"/\\\\\"/g')\" \"$head\" \"$tree\")\n")
		b.WriteString("allow=0\n")
		b.WriteString("if command -v curl >/dev/null 2>&1; then\n")
		fmt.Fprintf(&b, "  out=$(printf '%%s' \"$json\" | curl -sS -m 10 -X POST -H 'Content-Type: application/json' --data-binary @- -w '\\n%%{http_code}' %s) || out=\n", shQuote(o.URL+"/pre"))
		b.WriteString("  code=$(printf '%s' \"$out\" | tail -n1)\n")
		b.WriteString("  payload=$(printf '%s' \"$out\" | sed '$d')\n")
		b.WriteString("elif command -v wget >/dev/null 2>&1; then\n")
		b.WriteString("  tmp=/tmp/court-gate-pre.$$\n")
		fmt.Fprintf(&b, "  wget -q -O \"$tmp\" -T 10 --post-data=\"$json\" --header='Content-Type: application/json' %s\n", shQuote(o.URL+"/pre"))
		b.WriteString("  code=$?\n")
		b.WriteString("  payload=$(cat \"$tmp\" 2>/dev/null)\n")
		b.WriteString("  rm -f \"$tmp\"\n")
		b.WriteString("  if [ \"$code\" = 0 ]; then code=200; elif [ \"$code\" = 8 ]; then code=403; else code=000; fi\n")
		b.WriteString("else\n")
		b.WriteString("  code=000\n")
		b.WriteString("  payload=\n")
		b.WriteString("fi\n")
		b.WriteString("case \"$code\" in\n")
		b.WriteString("  200|2*)\n")
		b.WriteString("    case \"$payload\" in\n")
		b.WriteString("      *'\"gate\":\"open\"'*) allow=1 ;;\n")
		b.WriteString("    esac\n")
		b.WriteString("    ;;\n")
		fmt.Fprintf(&b, "  403) printf '%%s\\n' %s >&2; exit 1 ;;\n", shQuote(Message))
		b.WriteString("esac\n")
		b.WriteString("if [ \"$allow\" -ne 1 ]; then\n")
		b.WriteString("  printf '%s\\n' 'Court gate is not reachable; commit blocked.' >&2\n")
		b.WriteString("  exit 1\n")
		b.WriteString("fi\n")
	}
	b.WriteString(repoHookSnippet)
	b.WriteString("repo_hook pre-commit || exit $?\n")
	b.WriteString("exit 0\n")
	return b.String()
}

func postScript(o InstallOptions) string {
	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	b.WriteString("# Written by the Court daemon; do not edit.\n")
	b.WriteString("# Reports the new HEAD commit to the gate endpoint, then runs the\n")
	b.WriteString("# repository's own post-commit hook, if any.\n")
	b.WriteString("top=$(git rev-parse --show-toplevel 2>/dev/null) || exit 0\n")
	b.WriteString("head=$(git rev-parse HEAD 2>/dev/null) || exit 0\n")
	b.WriteString("parents=$(git rev-list --parents -n 1 HEAD 2>/dev/null | cut -d' ' -f2-)\n")
	if o.CourtBin != "" {
		fmt.Fprintf(&b, "%s gate --url %s --world \"$top\" --commit \"$head\" --parents \"$parents\" || true\n",
			shQuote(o.CourtBin), shQuote(o.URL))
	} else {
		b.WriteString("pj='['; first=1\n")
		b.WriteString("for p in $parents; do\n")
		b.WriteString("  if [ $first -eq 1 ]; then first=0; else pj=\"$pj,\"; fi\n")
		b.WriteString("  pj=\"$pj\\\"$p\\\"\"\n")
		b.WriteString("done\n")
		b.WriteString("pj=\"$pj]\"\n")
		b.WriteString("tw=$(printf '%s' \"$top\" | sed 's/\\\\/\\\\\\\\/g; s/\"/\\\\\"/g')\n")
		b.WriteString("body=\"{\\\"world_path\\\":\\\"$tw\\\",\\\"commit\\\":\\\"$head\\\",\\\"parents\\\":$pj}\"\n")
		b.WriteString("if command -v curl >/dev/null 2>&1; then\n")
		fmt.Fprintf(&b, "  curl -s -o /dev/null -m 10 -X POST -H 'Content-Type: application/json' --data-binary \"$body\" %s || true\n", shQuote(o.URL))
		b.WriteString("elif command -v wget >/dev/null 2>&1; then\n")
		fmt.Fprintf(&b, "  wget -q -O /dev/null -T 10 --post-data \"$body\" --header='Content-Type: application/json' %s || true\n", shQuote(o.URL))
		b.WriteString("fi\n")
	}
	b.WriteString(repoHookSnippet)
	b.WriteString("repo_hook post-commit || true\n")
	b.WriteString("exit 0\n")
	return b.String()
}
