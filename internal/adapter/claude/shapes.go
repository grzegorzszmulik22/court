// Package claude holds the Claude Code adapter's knowledge of the hook
// payload shapes, the field mapping the daemon uses, the outward-action
// deny policy, and the per-session settings/MCP configuration. The shapes
// come from the documented hook format at the pinned version 2.1.325
// (research/4_build_facts.md §1a; tools/versions.env). When recorded
// fixtures land under test/vectors/hooks/recorded/ they replace the
// documented shapes as the check authority (C1.5.6).
package claude

// HookEvents is the set of hook events Court consumes (spec/02 §4.1).
var HookEvents = []string{
	"PreToolUse", "PostToolUse", "PostToolUseFailure",
	"SubagentStart", "SubagentStop", "Stop", "UserPromptSubmit",
}

// CommonFields is the documented stdin field list every hook payload
// carries (research §1a "common stdin fields"). prompt_id is absent until
// the first user input; agent_id and agent_type appear only inside
// subagents and are therefore per-payload additions, not common fields.
var CommonFields = []string{
	"session_id", "prompt_id", "transcript_path", "cwd", "scratchpad_dir",
	"permission_mode", "effort", "hook_event_name",
}

// EventFields maps each consumed hook event to its documented payload
// fields: the common list plus the event-specific fields.
var EventFields = map[string][]string{
	"PreToolUse": append(append([]string{}, CommonFields...),
		"tool_name", "tool_input", "tool_use_id"),
	"PostToolUse": append(append([]string{}, CommonFields...),
		"tool_name", "tool_input", "tool_response", "tool_use_id", "duration_ms"),
	"PostToolUseFailure": append(append([]string{}, CommonFields...),
		"tool_name", "tool_input", "tool_use_id", "error", "is_interrupt", "duration_ms"),
	"SubagentStart": append(append([]string{}, CommonFields...),
		"agent_id", "agent_type"),
	"SubagentStop": append(append([]string{}, CommonFields...),
		"stop_hook_active", "agent_id", "agent_type", "agent_transcript_path",
		"last_assistant_message", "background_tasks", "session_crons"),
	"Stop": append(append([]string{}, CommonFields...),
		"stop_hook_active", "last_assistant_message", "background_tasks", "session_crons"),
	"UserPromptSubmit": append(append([]string{}, CommonFields...),
		"prompt", "session_title"),
}

// SubagentFields are added by the tool when the hook fires inside a
// subagent (research §1a, §1d).
var SubagentFields = []string{"agent_id", "agent_type"}

// PayloadParams carries the per-payload variable content of a hook post.
type PayloadParams struct {
	SessionID       string
	PromptID        string
	TranscriptPath  string
	Cwd             string
	ScratchpadDir   string
	PermissionMode  string
	EffortLevel     string
	AgentID         string // non-empty only inside a subagent
	AgentType       string // set when AgentID is
	ToolName        string
	ToolInput       map[string]any
	ToolUseID       string
	ToolResponse    map[string]any
	Error           string
	IsInterrupt     bool
	DurationMs      int64
	Prompt          string
	SessionTitle    string
	StopHookActive  bool
	LastAssistant   string
	AgentTranscript string
}

// Payload builds one hook payload in the documented shape for the given
// event. It is what the scripted adapter posts (spec/02 §4.2: "in exactly
// the shape Claude Code sends") and what the documented fixtures under
// test/vectors/hooks/documented/ are generated from.
func Payload(event string, p PayloadParams) map[string]any {
	m := map[string]any{
		"session_id":      p.SessionID,
		"prompt_id":       p.PromptID,
		"transcript_path": p.TranscriptPath,
		"cwd":             p.Cwd,
		"scratchpad_dir":  p.ScratchpadDir,
		"permission_mode": p.PermissionMode,
		"effort":          map[string]any{"level": p.EffortLevel},
		"hook_event_name": event,
	}
	switch event {
	case "PreToolUse", "PostToolUse", "PostToolUseFailure":
		m["tool_name"] = p.ToolName
		m["tool_input"] = orEmpty(p.ToolInput)
		m["tool_use_id"] = p.ToolUseID
		if event == "PostToolUse" {
			m["tool_response"] = orEmpty(p.ToolResponse)
			m["duration_ms"] = p.DurationMs
		}
		if event == "PostToolUseFailure" {
			m["error"] = p.Error
			m["is_interrupt"] = p.IsInterrupt
			m["duration_ms"] = p.DurationMs
		}
	case "SubagentStart":
		// agent fields are mandatory below.
	case "SubagentStop":
		m["stop_hook_active"] = p.StopHookActive
		m["agent_transcript_path"] = p.AgentTranscript
		m["last_assistant_message"] = p.LastAssistant
		m["background_tasks"] = []any{}
		m["session_crons"] = []any{}
	case "Stop":
		m["stop_hook_active"] = p.StopHookActive
		m["last_assistant_message"] = p.LastAssistant
		m["background_tasks"] = []any{}
		m["session_crons"] = []any{}
	case "UserPromptSubmit":
		m["prompt"] = p.Prompt
		m["session_title"] = p.SessionTitle
	}
	if p.AgentID != "" {
		m["agent_id"] = p.AgentID
		m["agent_type"] = p.AgentType
	}
	return m
}

func orEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}
