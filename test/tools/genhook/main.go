// genhook writes the documented hook fixtures of test/vectors/hooks/documented/:
// one payload per hook event Court consumes, plus one subagent PostToolUse,
// built by internal/adapter/claude.Payload (the documented shape at the
// pinned version). These are DOCUMENTED stand-ins — never presented as the
// recorded fixtures the supervisor commits under recorded/ from a real
// Claude Code 2.1.325 session.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"court/internal/adapter/claude"
)

var base = claude.PayloadParams{
	SessionID:      "a1b2c3d4-0000-4000-8000-000000000001",
	PromptID:       "p1b2c3d4-0000-4000-8000-0000000000p1",
	TranscriptPath: "/state/claude/projects/-w/transcript-abc.jsonl",
	Cwd:            "/data/court/repo/worlds/w1",
	ScratchpadDir:  "/data/court/repo/worlds/w1/.claude/scratchpad",
	PermissionMode: "default",
	EffortLevel:    "medium",
}

func main() {
	out := "test/vectors/hooks/documented"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		die(err)
	}
	write := func(name, event string, p claude.PayloadParams) {
		raw, err := json.MarshalIndent(claude.Payload(event, p), "", "  ")
		if err != nil {
			die(err)
		}
		if err := os.WriteFile(filepath.Join(out, name+".json"),
			append(raw, '\n'), 0o644); err != nil {
			die(err)
		}
		fmt.Println("wrote", name)
	}

	p := base
	p.ToolName = "Bash"
	p.ToolInput = map[string]any{
		"command":     "go test ./internal/...",
		"description": "run the unit tests",
		"timeout":     120000,
	}
	p.ToolUseID = "toolu_01ABCdefGHIjklMNOpqr01"
	write("PreToolUse-bash", "PreToolUse", p)

	p.ToolResponse = map[string]any{
		"stdout":   "ok  court/internal/ledger 0.42s\n",
		"stderr":   "",
		"exitCode": 0,
	}
	p.DurationMs = 430
	write("PostToolUse-bash", "PostToolUse", p)

	p = base
	p.ToolName = "Edit"
	p.ToolInput = map[string]any{
		"file_path":  "/data/court/repo/worlds/w1/internal/ledger/store.go",
		"old_string": "var SyncLevel = \"NORMAL\"",
		"new_string": "var SyncLevel = \"FULL\"",
	}
	p.ToolUseID = "toolu_02ABCdefGHIjklMNOpqr02"
	write("PreToolUse-edit", "PreToolUse", p)

	p.ToolResponse = map[string]any{
		"filePath":   "/data/court/repo/worlds/w1/internal/ledger/store.go",
		"oldString":  "var SyncLevel = \"NORMAL\"",
		"newString":  "var SyncLevel = \"FULL\"",
		"structured": nil,
	}
	p.DurationMs = 8
	write("PostToolUse-edit", "PostToolUse", p)

	p = base
	p.ToolName = "Bash"
	p.ToolInput = map[string]any{"command": "make check-M1.5"}
	p.ToolUseID = "toolu_03ABCdefGHIjklMNOpqr03"
	p.Error = "tool ran too long"
	p.IsInterrupt = true
	p.DurationMs = 240001
	write("PostToolUseFailure-bash", "PostToolUseFailure", p)

	// Subagent PostToolUse: same session_id, plus agent_id/agent_type.
	p = base
	p.ToolName = "Grep"
	p.ToolInput = map[string]any{"pattern": "func Append", "path": "/data/court/repo/worlds/w1/internal"}
	p.ToolUseID = "toolu_04ABCdefGHIjklMNOpqr04"
	p.AgentID = "a6f2e91"
	p.AgentType = "Explore"
	p.ToolResponse = map[string]any{"matches": 2}
	p.DurationMs = 11
	write("PostToolUse-subagent", "PostToolUse", p)

	p = base
	p.AgentID = "a6f2e91"
	p.AgentType = "Explore"
	write("SubagentStart", "SubagentStart", p)

	p = base
	p.AgentID = "a6f2e91"
	p.AgentType = "Explore"
	p.StopHookActive = false
	p.AgentTranscript = "/state/claude/projects/-w/subagents/agent-a6f2e91.jsonl"
	p.LastAssistant = "Found the append path; no surprises."
	write("SubagentStop", "SubagentStop", p)

	p = base
	p.StopHookActive = false
	p.LastAssistant = "Done with the chunk."
	write("Stop", "Stop", p)

	p = base
	p.Prompt = "Implement the hook endpoint"
	p.SessionTitle = "Court M1.5"
	write("UserPromptSubmit", "UserPromptSubmit", p)
}

func die(err error) { fmt.Fprintln(os.Stderr, "genhook:", err); os.Exit(1) }
