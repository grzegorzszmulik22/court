package claude

import (
	"path/filepath"
	"strings"
)

// Policy is Court's PreToolUse outward-action deny policy (spec/02 §4.1):
// web fetch and search tools, shell commands matching the outward list,
// any MCP tool other than Court's, and anything inside the data directory
// that is not the session's own world (spec/02 §2 ledger protection;
// worlds live under <data-dir>/worlds, so the world itself is exempt).
// The daemon evaluates it on every PreToolUse hook post; a "deny"
// decision is recorded with its reason.
//
// The policy never returns "allow": anything it does not deny falls back
// to the tool's own permission mode (auto-approving anything is denied).
type Policy struct {
	DataDir string // absolute Court data directory
}

// DenyReasons enumerates the outward list (spec/02 §4.1).
var (
	// webTools are the tool names that fetch or search the web.
	webTools = map[string]string{
		"WebFetch":  "web fetch is an outward action",
		"WebSearch": "web search is an outward action",
	}
	// outwardProgs are command names whose execution is an outward action.
	outwardProgs = map[string]string{
		"curl": "network write", "wget": "network write",
		"ssh": "remote login", "scp": "remote copy", "sftp": "remote copy",
		"rsync": "remote copy", "ftp": "remote copy", "telnet": "remote login",
		"nc": "network write", "ncat": "network write", "netcat": "network write",
		"socat": "network write",
	}
	// cloudProgs are cloud command-line tools.
	cloudProgs = map[string]string{
		"aws": "cloud CLI", "gcloud": "cloud CLI", "gsutil": "cloud CLI",
		"az": "cloud CLI", "oci": "cloud CLI", "doctl": "cloud CLI",
		"aliyun": "cloud CLI", "hcloud": "cloud CLI", "ibmcloud": "cloud CLI",
	}
	// publishPairs are (prog, subcommand) package-publish commands.
	publishPairs = [][2]string{
		{"npm", "publish"}, {"yarn", "publish"}, {"pnpm", "publish"},
		{"bun", "publish"}, {"twine", "upload"}, {"poetry", "publish"},
		{"cargo", "publish"}, {"gem", "push"}, {"dotnet", "nuget"},
		{"docker", "push"}, {"helm", "push"}, {"pip", "upload"},
	}
)

// fileTools are tool names carrying a path argument the data-directory
// rule applies to.
var fileTools = map[string]bool{
	"Read": true, "Write": true, "Edit": true, "MultiEdit": true,
	"NotebookEdit": true, "Glob": true, "Grep": true, "LS": true,
}

// pathKeys are the tool_input members that hold file paths.
var pathKeys = []string{"file_path", "path", "notebook_path"}

// DecideHook evaluates one PreToolUse payload and returns ("", "") to
// abstain or (decision, reason) when the call is denied. worldDir is the
// session's world (the exempt subtree of the data directory).
func (p Policy) DecideHook(toolName string, toolInput map[string]any,
	worldDir string) (string, string) {
	if reason, ok := webTools[toolName]; ok {
		return "deny", reason + " and outward actions are denied"
	}
	if strings.HasPrefix(toolName, "mcp__") &&
		!strings.HasPrefix(toolName, "mcp__court__") {
		return "deny", "MCP tools other than Court's are denied"
	}
	if fileTools[toolName] {
		for _, k := range pathKeys {
			if v, ok := toolInput[k].(string); ok && p.insideDataDir(v, worldDir) {
				return "deny", "path is inside the Court data directory"
			}
		}
	}
	if toolName == "Bash" {
		cmd, _ := toolInput["command"].(string)
		if p.DataDir != "" && p.commandNamesDataDir(cmd, worldDir) {
			return "deny", "command names the Court data directory"
		}
		if reason := p.outwardCommand(cmd); reason != "" {
			return "deny", reason
		}
	}
	return "", ""
}

// insideDataDir reports whether path resolves inside the data directory
// but outside worldDir. Cleaning resolves `..`, so a path like
// <world>/../ledger.db still lands inside the denied region.
func (p Policy) insideDataDir(path, worldDir string) bool {
	if p.DataDir == "" {
		return false
	}
	cand := filepath.Clean(path)
	if inside(cand, worldDir) {
		return false
	}
	return inside(cand, p.DataDir)
}

// commandNamesDataDir reports whether a shell command reaches into the
// data directory outside the world. For every occurrence of the data dir
// prefix, the path token it starts is normalized (so `..` cannot escape)
// and must land inside the world to be allowed.
func (p Policy) commandNamesDataDir(cmd, worldDir string) bool {
	dd := filepath.Clean(p.DataDir)
	for off := 0; ; {
		i := strings.Index(cmd[off:], dd)
		if i < 0 {
			return false
		}
		end := off + i + len(dd)
		for end < len(cmd) && !strings.ContainsRune(" \t'\";&|<>", rune(cmd[end])) {
			end++
		}
		cand := filepath.Clean(cmd[off+i : end])
		if inside(cand, dd) && !inside(cand, worldDir) {
			return true
		}
		off = end
	}
}

// inside reports whether cand is dir or inside dir.
func inside(cand, dir string) bool {
	if dir == "" {
		return false
	}
	d := filepath.Clean(dir)
	return cand == d || strings.HasPrefix(cand, d+string(filepath.Separator))
}

// outwardCommand splits a shell command into pipeline segments and checks
// each segment's program against the outward list.
func (p Policy) outwardCommand(cmd string) string {
	for _, seg := range splitSegments(cmd) {
		words := fields(seg)
		// Skip env assignments, wrappers and modifiers to reach the program.
		i := 0
		for i < len(words) {
			w := words[i]
			if envAssign(w) {
				i++
				continue
			}
			if w == "sudo" || w == "command" || w == "env" || w == "nice" {
				i++
				continue
			}
			break
		}
		if i >= len(words) {
			continue
		}
		prog := base(words[i])
		rest := words[i+1:]
		if reason, ok := outwardProgs[prog]; ok {
			return reason + " is an outward action and is denied"
		}
		if reason, ok := cloudProgs[prog]; ok {
			return reason + " is an outward action and is denied"
		}
		if prog == "git" {
			for _, w := range rest {
				if w == "push" || w == "send-pack" {
					return "git " + w + " is an outward action and is denied"
				}
			}
		}
		for _, pair := range publishPairs {
			if prog == pair[0] && len(rest) > 0 && rest[0] == pair[1] {
				return prog + " " + pair[1] + " is a package publish and is denied"
			}
		}
		if prog == "dotnet" && len(rest) > 1 && rest[0] == "nuget" && rest[1] == "push" {
			return "dotnet nuget push is a package publish and is denied"
		}
	}
	return ""
}

// envAssign reports whether w looks like NAME=value.
func envAssign(w string) bool {
	eq := strings.IndexByte(w, '=')
	if eq <= 0 {
		return false
	}
	for _, r := range w[:eq] {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
			return false
		}
	}
	return true
}

// splitSegments splits cmd on shell command separators (|, ;, &&, ||,
// newlines). Quotes are not interpreted — a match inside quotes still
// denies, which errs on the safe side for an outward list.
func splitSegments(cmd string) []string {
	var segs []string
	var b strings.Builder
	flush := func() {
		if s := strings.TrimSpace(b.String()); s != "" {
			segs = append(segs, s)
		}
		b.Reset()
	}
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		switch c {
		case ';', '\n', '\r':
			flush()
		case '|', '&':
			flush()
			if i+1 < len(cmd) && cmd[i+1] == c {
				i++
			}
		case '>':
			// redirection target is part of the segment
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	flush()
	return segs
}

func fields(s string) []string { return strings.Fields(s) }

func base(name string) string {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	return strings.TrimSuffix(name, ".exe")
}
