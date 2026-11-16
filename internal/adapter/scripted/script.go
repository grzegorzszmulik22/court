// Package scripted implements the scripted agent of spec/02 §4.2: a
// deterministic adapter that reads a script.yaml (spec/04 §3), applies
// edits, runs commands, calls the agent tools, makes commits and posts
// hook payloads to the daemon in exactly the shape Claude Code sends
// (the documented shapes of internal/adapter/claude).
//
// Every observable action is a hook post (or an MCP call wrapped in one);
// the package is the only place hermetic checks need to simulate an
// agent session.
package scripted

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Script is the parsed script.yaml (spec/04 §3).
type Script struct {
	Agents []*AgentSpec `yaml:"agents"`
}

// AgentSpec is one scripted agent: identity, parent and ordered steps.
type AgentSpec struct {
	ID     string  `yaml:"id"`
	Parent string  `yaml:"parent"`
	Steps  []*Step `yaml:"steps"`
}

// Step is one script action: exactly one of the fields is set.
type Step struct {
	// Edit applies a unified-diff patch to Path inside the world (or writes
	// the file when Write is set). It posts an Edit tool call.
	Edit *EditStep `yaml:"edit"`
	// Tool posts a tool call with the given name and input; a Bash call
	// really runs its command, other tools observe only.
	Tool *ToolStep `yaml:"tool"`
	// Commit stages everything and commits in the world; the generated
	// post-commit hook reports it to the gate.
	Commit *CommitStep `yaml:"commit"`
	// Declare calls record_decision over MCP, wrapped in its hook pair.
	Declare *DeclareStep `yaml:"declare"`
	// Alarm calls the alarm tool over MCP.
	Alarm *AlarmStep `yaml:"alarm"`
	// ClaimDone calls claim_done over MCP.
	ClaimDone *ClaimStep `yaml:"claim_done"`
	// NoDecisions calls no_decisions over MCP.
	NoDecisions *EmptyStep `yaml:"no_decisions"`
	// Backbrief calls backbrief over MCP (the tool arrives in M2).
	Backbrief *BackbriefStep `yaml:"backbrief"`
	// Hook posts one raw hook payload (used to simulate subagent posts and
	// non-tool events).
	Hook *HookStep `yaml:"hook"`
	// Sleep pauses the agent.
	Sleep *SleepStep `yaml:"sleep"`
}

// Kind names the populated field of a Step.
func (s *Step) Kind() string {
	switch {
	case s.Edit != nil:
		return "edit"
	case s.Tool != nil:
		return "tool"
	case s.Commit != nil:
		return "commit"
	case s.Declare != nil:
		return "declare"
	case s.Alarm != nil:
		return "alarm"
	case s.ClaimDone != nil:
		return "claim_done"
	case s.NoDecisions != nil:
		return "no_decisions"
	case s.Backbrief != nil:
		return "backbrief"
	case s.Hook != nil:
		return "hook"
	case s.Sleep != nil:
		return "sleep"
	}
	return ""
}

// EditStep is `edit: {path, patch}` (or `write` for a full file).
type EditStep struct {
	Path  string `yaml:"path"`
	Patch string `yaml:"patch"`
	Write string `yaml:"write"`
}

// ToolStep is `tool: {name, ...}`; every member besides name and the
// control keys below is passed through as tool_input verbatim.
type ToolStep struct {
	Name  string         `yaml:"name"`
	Input map[string]any `yaml:"-"`
	// Fail posts PostToolUseFailure instead of PostToolUse.
	Fail bool `yaml:"fail"`
	// AgentID posts the hook pair inside a subagent.
	AgentID   string `yaml:"agent_id"`
	AgentType string `yaml:"agent_type"`
}

// CommitStep is `commit: {message}`.
type CommitStep struct {
	Message string `yaml:"message"`
}

// DeclareStep is `declare: {question, category, paths, chosen, options?}`.
type DeclareStep struct {
	Question string           `yaml:"question"`
	Category string           `yaml:"category"`
	Paths    []string         `yaml:"paths"`
	Options  []map[string]any `yaml:"options"`
	Chosen   string           `yaml:"chosen"`
	Petition bool             `yaml:"petition"`
}

// AlarmStep is `alarm: {text, paths}`.
type AlarmStep struct {
	Text  string   `yaml:"text"`
	Paths []string `yaml:"paths"`
}

// ClaimStep is `claim_done: {task?, checks}`.
type ClaimStep struct {
	Task   string   `yaml:"task"`
	Checks []string `yaml:"checks"`
}

// EmptyStep is a no-argument step (`no_decisions: {}`).
type EmptyStep struct{}

// BackbriefStep is `backbrief: {...}` (M2 tool; parsed now so scripts are
// stable across milestones).
type BackbriefStep struct {
	Goal     string           `yaml:"goal"`
	Plan     map[string]any   `yaml:"plan"`
	Expected []map[string]any `yaml:"expected"`
}

// HookStep is `hook: {event, ...}`: one raw hook payload for events the
// typed steps do not cover (SubagentStart, Stop, UserPromptSubmit) or for
// posting inside a subagent (agent_id).
type HookStep struct {
	Event     string         `yaml:"event"`
	ToolName  string         `yaml:"tool_name"`
	ToolInput map[string]any `yaml:"tool_input"`
	ToolUseID string         `yaml:"tool_use_id"`
	AgentID   string         `yaml:"agent_id"`
	AgentType string         `yaml:"agent_type"`
	Extra     map[string]any `yaml:"extra"`
}

// SleepStep is `sleep: {ms}`.
type SleepStep struct {
	Ms int `yaml:"ms"`
}

// stepNames enumerates every step key for the unknown-key check.
var stepNames = map[string]bool{
	"edit": true, "tool": true, "commit": true, "declare": true,
	"alarm": true, "claim_done": true, "no_decisions": true,
	"backbrief": true, "hook": true, "sleep": true,
}

// UnmarshalYAML decodes a Step: a single-key mapping whose key names the
// step kind. Unknown step kinds are an error, not a skip.
func (s *Step) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode || len(n.Content)%2 != 0 {
		return fmt.Errorf("script: a step is a single-key mapping")
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key := n.Content[i].Value
		val := n.Content[i+1]
		if !stepNames[key] {
			return fmt.Errorf("script: unknown step %q", key)
		}
		var err error
		switch key {
		case "edit":
			err = val.Decode(&s.Edit)
			if err == nil && s.Edit != nil && s.Edit.Patch == "" && s.Edit.Write == "" {
				err = fmt.Errorf("script: edit step needs patch or write")
			}
		case "tool":
			err = decodeToolStep(val, s)
		case "commit":
			err = val.Decode(&s.Commit)
		case "declare":
			err = val.Decode(&s.Declare)
		case "alarm":
			err = val.Decode(&s.Alarm)
		case "claim_done":
			err = val.Decode(&s.ClaimDone)
		case "no_decisions":
			s.NoDecisions = &EmptyStep{}
		case "backbrief":
			err = val.Decode(&s.Backbrief)
		case "hook":
			err = val.Decode(&s.Hook)
		case "sleep":
			err = val.Decode(&s.Sleep)
		}
		if err != nil {
			return fmt.Errorf("script: bad %s step: %w", key, err)
		}
	}
	if s.Kind() == "" {
		return fmt.Errorf("script: empty step")
	}
	return nil
}

// decodeToolStep splits a tool step into name, control keys, and the
// passthrough tool_input.
func decodeToolStep(val *yaml.Node, s *Step) error {
	if val.Kind != yaml.MappingNode {
		return fmt.Errorf("tool step wants a mapping")
	}
	t := &ToolStep{Input: map[string]any{}}
	for i := 0; i+1 < len(val.Content); i += 2 {
		k := val.Content[i].Value
		v := val.Content[i+1]
		switch k {
		case "name":
			if err := v.Decode(&t.Name); err != nil {
				return err
			}
		case "fail":
			if err := v.Decode(&t.Fail); err != nil {
				return err
			}
		case "agent_id":
			if err := v.Decode(&t.AgentID); err != nil {
				return err
			}
		case "agent_type":
			if err := v.Decode(&t.AgentType); err != nil {
				return err
			}
		default:
			var x any
			if err := v.Decode(&x); err != nil {
				return err
			}
			t.Input[k] = x
		}
	}
	if t.Name == "" {
		return fmt.Errorf("tool step needs a name")
	}
	s.Tool = t
	return nil
}

// Parse reads a script.yaml. Steps keep file order.
func Parse(path string) (*Script, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseBytes(raw)
}

// ParseBytes parses script text.
func ParseBytes(raw []byte) (*Script, error) {
	var sc Script
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&sc); err != nil {
		return nil, fmt.Errorf("script: %w", err)
	}
	if len(sc.Agents) == 0 {
		return nil, fmt.Errorf("script: no agents")
	}
	ids := map[string]bool{}
	for _, a := range sc.Agents {
		if a.ID == "" {
			return nil, fmt.Errorf("script: agent with no id")
		}
		if ids[a.ID] {
			return nil, fmt.Errorf("script: duplicate agent id %q", a.ID)
		}
		ids[a.ID] = true
	}
	return &sc, nil
}
