package scripted

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Run executes an agent's steps in order against the client's daemon. Each
// step counts as one observable action for the completeness comparison of
// C1.5.1/C1.8.1: the check knows what the script asked for and reconciles
// that against the ledger itself.
func (c *Client) Run(steps []*Step) error {
	for i, st := range steps {
		if err := c.RunStep(st); err != nil {
			return fmt.Errorf("step %d (%s): %w", i, st.Kind(), err)
		}
	}
	return nil
}

// RunStep executes one step.
func (c *Client) RunStep(st *Step) error {
	switch {
	case st.Edit != nil:
		return c.doEdit(st.Edit)
	case st.Tool != nil:
		return c.doTool(st.Tool)
	case st.Commit != nil:
		return c.doCommit(st.Commit)
	case st.Declare != nil:
		return c.doMCPTool("record_decision", declareArgs(st.Declare), "", "")
	case st.Alarm != nil:
		return c.doMCPTool("alarm", map[string]any{
			"text": st.Alarm.Text, "paths": orStrings(st.Alarm.Paths)}, "", "")
	case st.ClaimDone != nil:
		return c.doMCPTool("claim_done", map[string]any{
			"task": st.ClaimDone.Task, "checks": orStrings(st.ClaimDone.Checks)}, "", "")
	case st.NoDecisions != nil:
		return c.doMCPTool("no_decisions", map[string]any{}, "", "")
	case st.Backbrief != nil:
		return c.doMCPTool("backbrief", map[string]any{
			"goal": st.Backbrief.Goal, "plan": orMap(st.Backbrief.Plan),
			"expected_decisions": orList(st.Backbrief.Expected),
			"will_not_touch":     []string{}}, "", "")
	case st.Hook != nil:
		return c.doHook(st.Hook)
	case st.Sleep != nil:
		time.Sleep(time.Duration(st.Sleep.Ms) * time.Millisecond)
		return nil
	}
	return fmt.Errorf("empty step")
}

// doEdit applies a patch (or full write) wrapped in an Edit tool call.
// The tool_input mirrors Claude Code's Edit shape: file_path plus the
// before/after text, so the recorded digest covers the real content while
// the selected fields keep the path only.
func (c *Client) doEdit(e *EditStep) error {
	abs := filepath.Join(c.Worktree, e.Path)
	before, _ := os.ReadFile(abs)
	var after []byte
	var err error
	if e.Write != "" {
		after = []byte(e.Write)
	} else {
		after, err = applyPatch(before, e.Patch)
		if err != nil {
			return fmt.Errorf("edit %s: %w", e.Path, err)
		}
	}
	input := map[string]any{
		"file_path":  abs,
		"old_string": string(before),
		"new_string": string(after),
	}
	tuid, denied, err := c.PreToolUse("Edit", input, "", "")
	if err != nil {
		return err
	}
	if denied {
		c.logf("edit %s denied", e.Path)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(abs, after, 0o644); err != nil {
		return err
	}
	return c.PostToolUse(tuid, "Edit", input, false, "", "")
}

// doTool posts one tool call; a Bash call also runs its command.
func (c *Client) doTool(t *ToolStep) error {
	tuid, denied, err := c.PreToolUse(t.Name, t.Input, t.AgentID, t.AgentType)
	if err != nil {
		return err
	}
	if denied {
		c.logf("tool %s denied", t.Name)
		return nil
	}
	failed := t.Fail
	if t.Name == "Bash" {
		cmd, _ := t.Input["command"].(string)
		if cmd == "" {
			return fmt.Errorf("Bash tool step without command")
		}
		code, err := c.runBash(cmd)
		if err != nil {
			return err
		}
		if code != 0 {
			// A non-zero exit is still a completed tool call;
			// `fail` selects PostToolUseFailure instead.
			c.logf("bash exited %d", code)
		}
	}
	return c.PostToolUse(tuid, t.Name, t.Input, failed, t.AgentID, t.AgentType)
}

// doCommit stages everything and commits; the generated post-commit hook
// reports the commit to the gate, which is the only path world.commit
// takes into the ledger (spec/02 §2).
func (c *Client) doCommit(m *CommitStep) error {
	if err := c.git("add", "-A"); err != nil {
		return err
	}
	msg := m.Message
	if msg == "" {
		msg = "scripted commit"
	}
	if err := c.git("-c", "user.email=agent@court.local",
		"-c", "user.name="+c.Agent,
		"commit", "-qm", msg, "--allow-empty"); err != nil {
		return err
	}
	c.logf("commit %q", msg)
	return nil
}

// doMCPTool posts the mcp__court__<name> hook pair a Claude Code session
// produces around an MCP call, then makes the call itself (spec/02 §5:
// attribution pairs the call with the PreToolUse that carried it).
func (c *Client) doMCPTool(name string, args map[string]any, agentID, agentType string) error {
	toolName := "mcp__court__" + name
	tuid, denied, err := c.PreToolUse(toolName, args, agentID, agentType)
	if err != nil {
		return err
	}
	if denied {
		return fmt.Errorf("%s was denied", toolName)
	}
	_, err = c.CallTool(name, args)
	if err != nil {
		// The call failed; the Post still fires with a failure outcome
		// like a real tool error.
		_ = c.PostToolUse(tuid, toolName, args, true, agentID, agentType)
		return err
	}
	return c.PostToolUse(tuid, toolName, args, false, agentID, agentType)
}

// doHook posts one raw hook event in its recorded shape. Extra entries
// override values of fields the recorded template carries; a key that is
// not in the recorded shape is an error, never an invented field.
func (c *Client) doHook(h *HookStep) error {
	return c.HookEvent(h.Event, PayloadOpts{
		ToolName: h.ToolName, ToolInput: h.ToolInput, ToolUseID: h.ToolUseID,
		AgentID: h.AgentID, AgentType: h.AgentType,
		Set: h.Extra,
	})
}

func orStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func orMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func orList(l []map[string]any) []map[string]any {
	if l == nil {
		return []map[string]any{}
	}
	return l
}

func declareArgs(d *DeclareStep) map[string]any {
	opts := d.Options
	if len(opts) == 0 && d.Chosen != "" {
		opts = []map[string]any{{"id": d.Chosen, "summary": "chosen option"}}
	}
	return map[string]any{
		"question": d.Question, "category": d.Category,
		"paths": orStrings(d.Paths), "options": opts,
		"chosen": d.Chosen, "petition": d.Petition,
	}
}
