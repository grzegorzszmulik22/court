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
// but outside worldDir. The comparison uses the filesystem's own path
// identity (canonicalPath): on Windows that covers case variants, the
// \?\ verbatim prefix, trailing dots, 8.3 names, junctions and
// symlinks; on Unix it resolves symlinks. A cleaned string prefix is not
// an identity test (red-team B3-08).
func (p Policy) insideDataDir(path, worldDir string) bool {
	if p.DataDir == "" {
		return false
	}
	cand, err := canonicalPath(path)
	if err != nil {
		cand = filepath.Clean(path)
	}
	dd, err := canonicalPath(p.DataDir)
	if err != nil {
		dd = filepath.Clean(p.DataDir)
	}
	if worldDir != "" {
		if wd, err := canonicalPath(worldDir); err == nil &&
			(pathsEqualFold(cand, wd) || pathHasPrefixFold(cand, wd)) {
			return false
		}
	}
	return pathsEqualFold(cand, dd) || pathHasPrefixFold(cand, dd)
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

// ---------------------------------------------------------------------------
// Outward command scanning
//
// outwardCommand answers whether a shell command line runs an outward
// program or a publish command. It is a conservative scanner over the
// shell text: it finds command positions (start of a segment, after a
// separator, after a transparent wrapper), follows a shell's -c argument,
// and descends into command substitutions ($(), ``), process
// substitutions (<(), >()), subshells ( ... ) and groups { ...; } —
// recursively, so `bash -c "sh -c 'git push'"` is seen as deeply as it is
// written.
//
// What it cannot see is stated as a limit in docs/M1.7.md: command text
// fed to an interpreter or produced at run time (python -c, sh reading
// stdin or a file, make/ninja recipes) is invisible to a scan of the
// command line itself.
// ---------------------------------------------------------------------------

// shells are programs whose -c (or /c) option is a nested command line.
var shells = map[string]bool{
	"sh": true, "bash": true, "dash": true, "ash": true, "zsh": true,
	"ksh": true, "mksh": true, "pdksh": true, "yash": true, "fish": true,
	"csh": true, "tcsh": true, "rc": true, "es": true, "pwsh": true,
	"powershell": true, "cmd": true,
}

// wrappers are transparent prefixes: the word after them (and their
// options) is still a command position. The value is the number of bare
// positional arguments the wrapper consumes before the command.
var wrappers = map[string]int{
	"sudo": 0, "doas": 0, "command": 0, "env": 0, "nice": 0,
	"nohup": 0, "xargs": 0, "exec": 0, "time": 0, "stdbuf": 0,
	"setsid": 0, "builtin": 0, "noglob": 0, "catchsegv": 0, "arch": 0,
	"busybox": 0, "parallel": 0, "watch": 0, "strace": 0, "ltrace": 0,
	"valgrind": 0, "unshare": 0, "nsenter": 0, "ionice": 0, "numactl": 0,
	"timeout": 1, "chroot": 1, "taskset": 1,
}

// optArgs lists, per wrapper, the options that consume the following word
// as a separate value (-s KILL, -I {}, --user root). Attached forms
// (-sKILL, --user=root) are single words and need no entry.
var optArgs = map[string]map[string]bool{
	"timeout":  {"-s": true, "-k": true, "--signal": true, "--kill-after": true},
	"nice":     {"-n": true, "--adjustment": true},
	"ionice":   {"-c": true, "-n": true, "-p": true, "-P": true, "-t": true, "-u": true, "--class": true, "--classdata": true, "--pid": true, "--pgid": true, "--uid": true},
	"taskset":  {"-c": true, "--cpu-list": true, "-p": true},
	"sudo":     {"-u": true, "-g": true, "-h": true, "-p": true, "-C": true, "-T": true, "-t": true, "-r": true, "-D": true, "-R": true, "-A": true, "--user": true, "--group": true, "--host": true, "--prompt": true, "--close-from": true, "--command-timeout": true, "--type": true, "--role": true, "--chdir": true, "--chroot": true, "--askpass": true},
	"doas":     {"-u": true, "-C": true},
	"env":      {"-u": true, "-C": true, "--unset": true, "--chdir": true},
	"xargs":    {"-I": true, "-n": true, "-L": true, "-P": true, "-d": true, "-s": true, "-a": true, "-E": true, "-e": true, "-R": true, "--replace": true, "--max-args": true, "--max-lines": true, "--max-procs": true, "--delimiter": true, "--max-chars": true, "--arg-file": true, "--eof": true, "--process-slot-var": true},
	"stdbuf":   {"-o": true, "-e": true, "-i": true, "--output": true, "--error": true, "--input": true},
	"exec":     {"-a": true, "--argv0": true},
	"time":     {"-o": true, "-f": true, "--output": true, "--format": true},
	"chroot":   {"--userspec": true, "--groups": true},
	"watch":    {"-n": true, "--interval": true},
	"strace":   {"-o": true, "-e": true, "-p": true, "-P": true, "-u": true, "-s": true, "--output": true, "--trace": true, "--pid": true, "--user": true},
	"ltrace":   {"-o": true, "-e": true, "-p": true, "-u": true, "--output": true, "--pid": true, "--user": true},
	"valgrind": {"--tool": true, "--log-file": true},
	"unshare":  {"-r": true, "-w": true, "-S": true, "-G": true, "--root": true, "--wd": true, "--user": true, "--map-user": true, "--map-group": true, "--map-users": true, "--map-groups": true},
	"nsenter":  {"-t": true, "-S": true, "-G": true, "--target": true, "--setuid": true, "--setgid": true, "--root": true, "--wd": true},
	"parallel": {"-j": true, "-a": true, "--jobs": true, "--timeout": true, "--retries": true, "--joblog": true, "--results": true, "--workdir": true, "--arg-file": true, "--sshlogin": true, "--env": true, "--nice": true, "--load": true, "--delay": true},
	"numactl":  {"-C": true, "-N": true, "-m": true, "-p": true, "--physcpubind": true, "--cpunodebind": true, "--membind": true, "--preferred": true},
}

// keywords keep a command position open: they are shell grammar, not
// programs (`if curl x`, `for f in *; do curl $f; done`).
var keywords = map[string]bool{
	"if": true, "then": true, "else": true, "elif": true, "fi": true,
	"while": true, "until": true, "do": true, "done": true,
	"for": true, "in": true, "case": true, "esac": true,
	"select": true, "function": true, "coproc": true, "!": true,
}

// publishPairs is redeclared over the table above: prog + sub (and an
// optional second word for dotnet nuget push) anywhere in the arguments.
type publishRule struct{ prog, sub, extra string }

var publishRules = []publishRule{
	{"npm", "publish", ""}, {"yarn", "publish", ""}, {"pnpm", "publish", ""},
	{"bun", "publish", ""}, {"twine", "upload", ""}, {"poetry", "publish", ""},
	{"cargo", "publish", ""}, {"gem", "push", ""}, {"dotnet", "nuget", "push"},
	{"docker", "push", ""}, {"helm", "push", ""}, {"pip", "upload", ""},
}

// outwardCommand scans a shell command line for an outward program or a
// publish pair in any command position, including nested ones.
func (p Policy) outwardCommand(cmd string) string {
	return p.scanShell(cmd, 0)
}

// scanShell lexes s into segments and nested regions and scans both;
// depth bounds the -c / substitution / eval recursion.
func (p Policy) scanShell(s string, depth int) string {
	if s == "" || depth > 8 {
		return ""
	}
	segs, nested := lexLine(s)
	for _, n := range nested {
		if r := p.scanShell(n, depth+1); r != "" {
			return r
		}
	}
	for _, words := range segs {
		if r := p.scanSeg(words, depth); r != "" {
			return r
		}
	}
	return ""
}

// opaque reports whether a word contains a substitution, making its value
// unknowable (the substitution's own text was already scanned). Opaque
// words can only be arguments, never identified programs.
func opaque(w string) bool { return strings.IndexByte(w, 0) >= 0 }

// scanSeg finds the command position of one segment and checks it.
func (p Policy) scanSeg(words []string, depth int) string {
	lastWrapper := ""
	bare := 0 // positional args the current wrapper still consumes
	i := 0
	for i < len(words) {
		w := words[i]
		if w == "" || opaque(w) {
			i++
			continue
		}
		if bare > 0 && !envAssign(w) && !strings.HasPrefix(w, "-") {
			bare--
			i++
			continue
		}
		if envAssign(w) {
			i++
			continue
		}
		if strings.HasPrefix(w, "-") {
			if lastWrapper == "env" && (w == "-S" || w == "--split-string") {
				// env -S splits its value as a command line.
				if i+1 < len(words) && !opaque(words[i+1]) {
					if r := p.scanShell(words[i+1], depth+1); r != "" {
						return r
					}
				}
			}
			if optArgs[lastWrapper][w] {
				i += 2
			} else {
				i++
			}
			continue
		}
		if arity, ok := wrappers[base(w)]; ok {
			lastWrapper = base(w)
			bare += arity
			i++
			continue
		}
		if keywords[w] {
			i++
			continue
		}
		return p.checkCommand(words[i:], depth)
	}
	return ""
}

// checkCommand checks words[0] as a program against the outward lists and
// the recursion points it opens (shell -c, eval, find -exec).
func (p Policy) checkCommand(words []string, depth int) string {
	prog := base(words[0])
	args := words[1:]
	if reason, ok := outwardProgs[prog]; ok {
		return reason + " is an outward action and is denied"
	}
	if reason, ok := cloudProgs[prog]; ok {
		return reason + " is an outward action and is denied"
	}
	if prog == "git" {
		for _, a := range args {
			if a == "push" || a == "send-pack" {
				return "git " + a + " is an outward action and is denied"
			}
		}
	}
	for _, pr := range publishRules {
		if prog != pr.prog {
			continue
		}
		if hasWord(args, pr.sub) && (pr.extra == "" || hasWord(args, pr.extra)) {
			sub := pr.sub
			if pr.extra != "" {
				sub += " " + pr.extra
			}
			return prog + " " + sub + " is a package publish and is denied"
		}
	}
	if shells[prog] {
		if r := p.scanShellArg(prog, args, depth); r != "" {
			return r
		}
	}
	if prog == "eval" {
		// eval reparses its arguments joined as a command line.
		var live []string
		for _, a := range args {
			if !opaque(a) {
				live = append(live, a)
			}
		}
		if r := p.scanShell(strings.Join(live, " "), depth+1); r != "" {
			return r
		}
	}
	if prog == "find" {
		// -exec/-execdir/-ok/-okdir run the following words as a command
		// up to a `;` or `+` argument.
		for j, a := range args {
			if a != "-exec" && a != "-execdir" && a != "-ok" && a != "-okdir" {
				continue
			}
			k := j + 1
			var sub []string
			for k < len(args) && args[k] != ";" && args[k] != "+" {
				sub = append(sub, args[k])
				k++
			}
			if len(sub) > 0 && !opaque(sub[0]) {
				if r := p.checkCommand(sub, depth); r != "" {
					return r
				}
			}
		}
	}
	return ""
}

// scanShellArg finds a -c (or cmd's /c) option among a shell's arguments
// and scans its value as a command line.
func (p Policy) scanShellArg(prog string, args []string, depth int) string {
	for j, a := range args {
		if opaque(a) {
			continue
		}
		var val string
		switch {
		case a == "-c" || a == "-Command" || a == "--command":
			if j+1 < len(args) {
				val = args[j+1]
			}
		case (prog == "cmd") && (a == "/c" || a == "/C" || a == "/k" || a == "/K"):
			if j+1 < len(args) {
				val = args[j+1]
			}
		case strings.HasPrefix(a, "-c") && len(a) > 2:
			// getopt clustering: -cX is -c with X attached.
			val = a[2:]
		case strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--"):
			// clustered short options ending in c: -xc takes the next word.
			if i := strings.IndexByte(a, 'c'); i > 0 {
				if i+1 < len(a) {
					val = a[i+1:]
				} else if j+1 < len(args) {
					val = args[j+1]
				}
			}
		}
		if val != "" && !opaque(val) {
			if r := p.scanShell(val, depth+1); r != "" {
				return r
			}
		}
	}
	return ""
}

// hasWord reports whether w occurs in args.
func hasWord(args []string, w string) bool {
	for _, a := range args {
		if a == w {
			return true
		}
	}
	return false
}

// lexLine splits s into command segments and extracts the nested command
// regions inside it ($() and “ anywhere, <() >()). Parentheses and
// braces are separators themselves, so subshell and group bodies land in
// the segment stream without another recursion level. A word containing
// a substitution keeps a \x00 placeholder so it can never be mistaken
// for an identified program.
func lexLine(s string) (segs [][]string, nested []string) {
	var (
		cur   []string
		w     strings.Builder
		dirty bool
	)
	put := func(b byte) { w.WriteByte(b); dirty = true }
	word := func() {
		if dirty {
			cur = append(cur, w.String())
			w.Reset()
			dirty = false
		}
	}
	seg := func() {
		word()
		if len(cur) > 0 {
			segs = append(segs, cur)
			cur = nil
		}
	}
	i := 0
	for i < len(s) {
		switch c := s[i]; {
		case c == ' ' || c == '\t':
			word()
			i++
		case c == ';' || c == '\n' || c == '\r' || c == '|' || c == '&' ||
			c == '(' || c == ')' || c == '{' || c == '}':
			seg()
			i++
		case c == '<' || c == '>':
			if i+1 < len(s) && s[i+1] == '(' {
				inner, j := extractParen(s, i+1)
				nested = append(nested, inner)
				put(0)
				i = j
			} else {
				// A redirection operator ends the word; the target is
				// still part of the same segment.
				word()
				i++
			}
		case c == '\\':
			if i+1 < len(s) {
				put(s[i+1])
				i += 2
			} else {
				i++
			}
		case c == '\'':
			j := strings.IndexByte(s[i+1:], '\'')
			if j < 0 {
				w.WriteString(s[i+1:])
				dirty = true
				i = len(s)
			} else {
				w.WriteString(s[i+1 : i+1+j])
				dirty = true
				i += j + 2
			}
		case c == '"':
			j := i + 1
			for j < len(s) && s[j] != '"' {
				switch s[j] {
				case '\\':
					if j+1 < len(s) {
						put(s[j+1])
						j += 2
					} else {
						j++
					}
				case '`':
					inner, k := extractBacktick(s, j)
					nested = append(nested, inner)
					put(0)
					j = k
				case '$':
					if j+1 < len(s) && s[j+1] == '(' {
						inner, k := extractParen(s, j+1)
						nested = append(nested, inner)
						put(0)
						j = k
					} else {
						put('$')
						j++
					}
				default:
					put(s[j])
					j++
				}
			}
			if j < len(s) {
				j++ // the closing quote
			}
			i = j
		case c == '`':
			inner, j := extractBacktick(s, i)
			nested = append(nested, inner)
			put(0)
			i = j
		case c == '$':
			if i+1 < len(s) && s[i+1] == '(' {
				inner, j := extractParen(s, i+1)
				nested = append(nested, inner)
				put(0)
				i = j
			} else {
				put('$')
				i++
			}
		case c == '#':
			if !dirty {
				// '#' at a word boundary comments out the rest of the line.
				for i < len(s) && s[i] != '\n' {
					i++
				}
			} else {
				put(c)
				i++
			}
		default:
			put(c)
			i++
		}
	}
	seg()
	return segs, nested
}

// extractParen returns the content between s[i]=='(' and its matching
// ')'. Parens inside quotes and backticks do not count.
func extractParen(s string, i int) (string, int) {
	depth := 0
	for j := i; j < len(s); j++ {
		switch s[j] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return s[i+1 : j], j + 1
			}
		case '\'':
			if k := strings.IndexByte(s[j+1:], '\''); k >= 0 {
				j += k + 1 // lands on the closing quote
			} else {
				j = len(s) - 1
			}
		case '"':
			j++
			for j < len(s) && s[j] != '"' {
				switch s[j] {
				case '\\':
					j++
				case '`':
					// a backquote region can hold parens of its own
					k := j + 1
					for k < len(s) && s[k] != '`' {
						if s[k] == '\\' {
							k++
						}
						k++
					}
					j = k
				}
				j++
			}
		case '`':
			for j+1 < len(s) {
				j++
				if s[j] == '\\' {
					j++
					continue
				}
				if s[j] == '`' {
					break
				}
			}
		case '\\':
			j++
		}
	}
	return s[i+1:], len(s) // unterminated: the rest is the region
}

// extractBacktick returns the content between s[i]=='`' and the next
// unescaped backtick.
func extractBacktick(s string, i int) (string, int) {
	for j := i + 1; j < len(s); j++ {
		if s[j] == '\\' {
			j++
			continue
		}
		if s[j] == '`' {
			return s[i+1 : j], j + 1
		}
	}
	return s[i+1:], len(s)
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

func base(name string) string {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	return strings.TrimSuffix(name, ".exe")
}
