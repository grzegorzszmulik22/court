package claude

// C1.7.2 outward deny list: every outward program and every publish pair
// is denied wherever it can appear as a command — nested shell -c,
// command substitutions and backticks, subshells, groups, transparent
// wrappers, absolute and relative paths, and nested combinations.

import (
	"fmt"
	"testing"
)

// outwardEntries enumerates the deny list as runnable command fragments.
func outwardEntries() map[string]string {
	entries := map[string]string{}
	for prog := range outwardProgs {
		entries[prog] = prog + " http://example.invalid/x"
	}
	for prog := range cloudProgs {
		entries[prog] = prog + " s3 ls"
	}
	for _, pr := range publishRules {
		cmd := pr.prog + " " + pr.sub
		if pr.extra != "" {
			cmd += " " + pr.extra
		}
		entries[cmd] = cmd
	}
	entries["git push"] = "git push origin main"
	entries["git send-pack"] = "git send-pack origin main"
	return entries
}

// positions wraps a command fragment in every position the deny list
// must see through.
func positions(x string) map[string]string {
	return map[string]string{
		"bare":           x,
		"after-semi":     "echo ok; " + x,
		"after-pipe":     "echo ok | " + x,
		"after-and":      "true && " + x,
		"in-condition":   "if " + x + "; then echo y; fi",
		"subshell":       "( " + x + " )",
		"group":          "{ " + x + "; }",
		"dollar-paren":   "echo $(" + x + ")",
		"backtick":       "echo `" + x + "`",
		"assign-sub":     "v=$(" + x + ")",
		"bash -c":        "bash -c '" + x + "'",
		"sh -c":          "sh -c '" + x + "'",
		"zsh -c":         `zsh -c "` + x + `"`,
		"dash -c":        "dash -c '" + x + "'",
		"nested -c":      `bash -c "sh -c '` + x + `'"`,
		"timeout":        "timeout 5 " + x,
		"timeout-flag":   "timeout -s 9 5 " + x,
		"nohup":          "nohup " + x,
		"xargs":          "echo f | xargs " + x,
		"xargs-opt":      "xargs -I {} " + x,
		"exec":           "exec " + x,
		"time":           "time " + x,
		"stdbuf":         "stdbuf -o0 " + x,
		"setsid":         "setsid " + x,
		"sudo":           "sudo " + x,
		"sudo-opt":       "sudo -u root -E " + x,
		"env":            "env " + x,
		"env-assign":     "env FOO=bar " + x,
		"nice":           "nice -n 10 " + x,
		"command":        "command " + x,
		"abs-path":       "", // filled per entry below
		"rel-path":       "", // filled per entry below
		"eval":           "eval '" + x + "'",
		"find-exec":      "find . -exec " + x + ` \; -print`,
		"env -S":         "env -S '" + x + "'",
		"in-backtick-dq": `echo "` + "`" + x + "`" + `"`,
		"nested-mix":     `bash -c "sh -c 'timeout 3 ` + x + `'"`,
		"proc-sub":       "diff <(echo a) <(" + x + ")",
		"double-quoted":  `sh -c "` + x + `"`,
	}
}

func denied(t *testing.T, line string) bool {
	t.Helper()
	p := Policy{}
	dec, _ := p.DecideHook("Bash", map[string]any{"command": line}, "/world")
	return dec == "deny"
}

func TestOutwardDeny_EveryEntryEveryPosition(t *testing.T) {
	for name, frag := range outwardEntries() {
		pos := positions(frag)
		pos["abs-path"] = "/usr/bin/" + frag
		pos["rel-path"] = "./" + frag
		for pname, line := range pos {
			if !denied(t, line) {
				t.Errorf("%-18s in position %-15s not denied: %q", name, pname, line)
			}
		}
	}
}

// TestOutwardDeny_Allowed shapes that must NOT be denied: denied words
// as data (arguments, quoted strings, comments), non-outward commands,
// and git/package commands that are not pushes or publishes.
func TestOutwardDeny_Allowed(t *testing.T) {
	for _, line := range []string{
		"echo curl",
		"echo 'curl https://example.com'",
		`printf "%s" "git push"`,
		"grep curl notes.txt",
		"git pull --rebase",
		"git pushd .",       // not push
		"git -c x=y status", // config mention, not a push
		"npm install",
		"npm run build",
		"docker build .",
		"pip install requests",
		"cargo build --release",
		"# a comment about curl",
		"echo done # git push is denied",
		"echo 'a; curl x'", // quoted; the ; is data
		"echo '$(curl x)'", // single-quoted: substitution is data
		`true || echo "curl"`,
		"ls -la",
		"go test ./...",
		"python3 -m pytest",   // interpreter content is the stated residual
		"timeout 5 echo curl", // curl is echo's argument, not a command
		"bash -c 'echo curl'",
		"eval 'echo curl'",
		"command -v curl", // lookup, not execution — conservative deny
	} {
		// "command -v curl" is over-denied by design (a command word after
		// a wrapper is treated as the command); the rest must abstain.
		if line == "command -v curl" {
			if !denied(t, line) {
				t.Errorf("expected conservative deny: %q", line)
			}
			continue
		}
		if denied(t, line) {
			t.Errorf("over-denied: %q", line)
		}
	}
}

// TestOutwardDeny_Nested: every entry survives a few combinations the
// checker nests deeper than the flat positions above.
func TestOutwardDeny_Nested(t *testing.T) {
	for name, frag := range outwardEntries() {
		for _, line := range []string{
			`bash -c 'sh -c "timeout 2 ` + frag + `"'`,
			`( echo x && ( ` + frag + ` ) )`,
			`echo $(sh -c '` + frag + `')`,
			`{ ( ` + frag + ` ); }`,
			fmt.Sprintf(`x=1 bash -c 'v=$(echo); %s'`, frag),
			`xargs -0 -n 4 ` + frag,
			`nohup timeout 8 ` + frag,
			`if ! ` + frag + `; then echo failed; fi`,
		} {
			if !denied(t, line) {
				t.Errorf("%-18s nested not denied: %q", name, line)
			}
		}
	}
}
