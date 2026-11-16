// Package claude holds the Claude Code adapter's knowledge of the hook
// events Court consumes, the field mapping the daemon uses, the
// outward-action deny policy, and the per-session settings/MCP
// configuration. Hook payload *shapes* are not declared here at all: the
// recorded posts in test/vectors/hooks/recorded/hooks/ are the authority
// (C1.5.6), and the scripted adapter builds every payload by loading the
// recorded post as a template and changing values only.
package claude

// HookEvents is the set of hook events Court consumes (spec/02 §4.1).
var HookEvents = []string{
	"PreToolUse", "PostToolUse", "PostToolUseFailure",
	"SubagentStart", "SubagentStop", "Stop", "UserPromptSubmit",
}
