package scripted

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// Hook payloads are built from the recorded posts in
// test/vectors/hooks/recorded/hooks/ — the authority of C1.5.6 — never
// from a hand-written field list. Each recorded post is a template: the
// adapter loads it, changes values for the keys it carries, and sends it.
// Nothing is ever added or removed, so every posted payload has exactly
// the key set and JSON types of the recorded post for that event and
// context (main agent or subagent).
//
// A payload kind is (event, context, tool). The corpus records the same
// tool only for some posts; for a tool the corpus does not record, the
// first recorded post of that event and context is the template (its
// tool_name, tool_input and tool_response keep their recorded shapes —
// tool_response is per-tool, so it is never synthesized). An event and
// context the corpus does not record at all (say a SubagentStart in the
// main agent) is an error: posting a shape that was never recorded is
// exactly what this exists to prevent.

// PayloadOpts carries the variable values written into a recorded
// template. Empty values keep the recorded field's own value — a payload
// never gains or loses a key this way.
type PayloadOpts struct {
	SessionID      string
	PromptID       string
	TranscriptPath string
	Cwd            string
	ScratchpadDir  string
	PermissionMode string
	EffortLevel    string
	AgentID        string // non-empty selects the subagent context
	AgentType      string
	ToolName       string
	ToolInput      map[string]any
	ToolUseID      string
	// Set overrides the value of any other field the recorded template
	// carries (prompt, error, duration_ms, tool_response, ...). Naming a
	// field the recorded shape does not have is an error — that is how a
	// script is stopped from inventing keys.
	Set map[string]any
}

type payloadTemplate struct {
	body map[string]any
	file string
}

// templateIndex indexes the recorded corpus by kind.
type templateIndex struct {
	exact map[string]payloadTemplate // "event|ctx|tool"
	first map[string]payloadTemplate // "event|ctx" -> first file
}

func (t *templateIndex) pick(event string, sub bool, tool string) (payloadTemplate, error) {
	ctx := "main"
	if sub {
		ctx = "sub"
	}
	if tool != "" {
		if pt, ok := t.exact[event+"|"+ctx+"|"+tool]; ok {
			return pt, nil
		}
	}
	if pt, ok := t.first[event+"|"+ctx]; ok {
		return pt, nil
	}
	return payloadTemplate{}, fmt.Errorf("no recorded %s hook post for context %s", event, ctx)
}

// recordedTemplates loads the corpus once per process.
var recordedTemplates = sync.OnceValues(func() (*templateIndex, error) {
	dir, err := recordedHooksDir()
	if err != nil {
		return nil, err
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read recorded hook templates: %w", err)
	}
	idx := &templateIndex{
		exact: map[string]payloadTemplate{},
		first: map[string]payloadTemplate{},
	}
	var files []string
	for _, e := range ents {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	for _, name := range files {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		var wrap map[string]any
		if err := json.Unmarshal(raw, &wrap); err != nil {
			return nil, fmt.Errorf("recorded hook post %s: %w", name, err)
		}
		body, _ := wrap["body"].(map[string]any)
		if body == nil {
			body = wrap // tolerate a bare payload
		}
		event, _ := body["hook_event_name"].(string)
		if event == "" {
			return nil, fmt.Errorf("recorded hook post %s has no hook_event_name", name)
		}
		_, sub := body["agent_id"]
		ctx := "main"
		if sub {
			ctx = "sub"
		}
		tool, _ := body["tool_name"].(string)
		pt := payloadTemplate{body: body, file: name}
		if _, ok := idx.first[event+"|"+ctx]; !ok {
			idx.first[event+"|"+ctx] = pt
		}
		if tool != "" {
			key := event + "|" + ctx + "|" + tool
			if _, ok := idx.exact[key]; !ok {
				idx.exact[key] = pt
			}
		}
	}
	return idx, nil
})

// recordedHooksDir locates test/vectors/hooks/recorded/hooks: anchored at
// this source file first, then by walking up from the working directory.
// The directory is read-only input — the adapter never writes to it.
func recordedHooksDir() (string, error) {
	tail := filepath.Join("test", "vectors", "hooks", "recorded", "hooks")
	if _, file, _, ok := runtime.Caller(0); ok {
		cand := filepath.Join(filepath.Dir(file), "..", "..", "..", tail)
		if st, err := os.Stat(cand); err == nil && st.IsDir() {
			return cand, nil
		}
	}
	dir, err := os.Getwd()
	if err == nil {
		for {
			cand := filepath.Join(dir, tail)
			if st, err := os.Stat(cand); err == nil && st.IsDir() {
				return cand, nil
			}
			up := filepath.Dir(dir)
			if up == dir {
				break
			}
			dir = up
		}
	}
	return "", fmt.Errorf("recorded hook templates not found (%s)", tail)
}

// BuildPayload renders one hook post from the recorded template of the
// given event and context: the same key set and JSON value types at every
// depth, with values altered only.
func BuildPayload(event string, o PayloadOpts) (map[string]any, error) {
	idx, err := recordedTemplates()
	if err != nil {
		return nil, err
	}
	pt, err := idx.pick(event, o.AgentID != "", o.ToolName)
	if err != nil {
		return nil, err
	}
	// Deep-copy the template through JSON so values can never alias the
	// cached corpus.
	raw, err := json.Marshal(pt.body)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	if got, _ := m["hook_event_name"].(string); got != event {
		return nil, fmt.Errorf("recorded template %s: hook_event_name %q != %q",
			pt.file, got, event)
	}
	setStr := func(key, v string) {
		if v != "" {
			if _, ok := m[key]; ok {
				m[key] = v
			}
		}
	}
	setStr("session_id", o.SessionID)
	setStr("transcript_path", o.TranscriptPath)
	setStr("cwd", o.Cwd)
	setStr("scratchpad_dir", o.ScratchpadDir)
	setStr("prompt_id", o.PromptID)
	setStr("permission_mode", o.PermissionMode)
	setStr("agent_id", o.AgentID)
	setStr("agent_type", o.AgentType)
	setStr("tool_name", o.ToolName)
	setStr("tool_use_id", o.ToolUseID)
	if o.ToolInput != nil {
		if _, ok := m["tool_input"]; ok {
			m["tool_input"] = o.ToolInput
		}
	}
	if o.EffortLevel != "" {
		// effort is an object in the recorded shapes; keep its structure
		// and change the level only.
		if eff, ok := m["effort"].(map[string]any); ok {
			cp := map[string]any{}
			for k, v := range eff {
				cp[k] = v
			}
			cp["level"] = o.EffortLevel
			m["effort"] = cp
		}
	}
	for k, v := range o.Set {
		rec, ok := m[k]
		if !ok {
			return nil, fmt.Errorf("hook %s: %q is not a field of the recorded shape (%s)",
				event, k, pt.file)
		}
		// A value may change, never its JSON type: the posted payload
		// keeps the recorded shape at every depth.
		if reflect.TypeOf(rec) != reflect.TypeOf(v) {
			return nil, fmt.Errorf("hook %s: %q is %T in the recorded shape, not %T (%s)",
				event, k, rec, v, pt.file)
		}
		m[k] = v
	}
	return m, nil
}
