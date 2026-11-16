package scripted_test

// C1.5.6, shape property: every payload the scripted adapter can send has
// exactly the key set, and the same JSON value types at every depth, of
// the recorded post for the same event and context in
// test/vectors/hooks/recorded/hooks/. The adapter builds payloads by
// loading the recorded post as the template and changing values only, so
// this test enumerates every payload kind and proves the property holds.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"court/internal/adapter/scripted"
)

// recordedBodies loads every recorded hook post's body, in file order.
func recordedBodies(t *testing.T) []map[string]any {
	t.Helper()
	ents, err := filepath.Glob(filepath.Join(findRepoRoot(t),
		"test/vectors/hooks/recorded/hooks/*.json"))
	if err != nil || len(ents) == 0 {
		t.Skip("recorded hook fixtures not on this branch")
	}
	sort.Strings(ents)
	var bodies []map[string]any
	for _, f := range ents {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var wrap map[string]any
		if err := json.Unmarshal(raw, &wrap); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		body, _ := wrap["body"].(map[string]any)
		if body == nil {
			body = wrap
		}
		bodies = append(bodies, body)
	}
	return bodies
}

// kindOf names the payload kind a recorded post stands for:
// (event, context, tool).
func kindOf(body map[string]any) (event, ctx, tool string) {
	event, _ = body["hook_event_name"].(string)
	ctx = "main"
	if _, ok := body["agent_id"]; ok {
		ctx = "sub"
	}
	tool, _ = body["tool_name"].(string)
	return
}

// shapeIs asserts want and got have the same JSON types at every depth,
// recursing into maps; slices compare element types pairwise over their
// shared prefix (recorded slices are homogeneous). Key sets must be
// identical — a payload may never gain or lose a field.
func shapeIs(t *testing.T, path string, want, got any) {
	t.Helper()
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			t.Fatalf("%s: type %T, want object", path, got)
		}
		var extra, missing []string
		for k := range w {
			if _, ok := g[k]; !ok {
				missing = append(missing, k)
			}
		}
		for k := range g {
			if _, ok := w[k]; !ok {
				extra = append(extra, k)
			}
		}
		if len(missing) > 0 || len(extra) > 0 {
			t.Fatalf("%s: key drift: missing %v extra %v", path, missing, extra)
		}
		for k, wv := range w {
			shapeIs(t, path+"."+k, wv, g[k])
		}
	case []any:
		g, ok := got.([]any)
		if !ok {
			t.Fatalf("%s: type %T, want array", path, got)
		}
		for i := 0; i < len(w) && i < len(g); i++ {
			shapeIs(t, fmt.Sprintf("%s[%d]", path, i), w[i], g[i])
		}
	default:
		if reflect.TypeOf(want) != reflect.TypeOf(got) {
			t.Fatalf("%s: type %T, want %T", path, got, want)
		}
	}
}

// TestC1_5_6_PayloadShapes: for every recorded (event, context, tool)
// kind, the payload the adapter builds has exactly that recorded post's
// key set and value types at every depth; a kind that was never recorded
// cannot be built at all.
func TestC1_5_6_PayloadShapes(t *testing.T) {
	bodies := recordedBodies(t)
	seen := map[string]bool{}
	var firstOf map[string]map[string]any = map[string]map[string]any{}
	for _, body := range bodies {
		event, ctx, tool := kindOf(body)
		// The full variable surface: every value the adapter may change.
		// Fields the recorded post does not carry must stay absent
		// (permission_mode and effort in SubagentStart, effort and
		// session_title in UserPromptSubmit, ...).
		o := scripted.PayloadOpts{
			SessionID: "sess", PromptID: "prompt",
			TranscriptPath: "/t.jsonl", Cwd: "/world",
			ScratchpadDir: "/sp", PermissionMode: "acceptEdits",
			EffortLevel: "high",
			ToolName:    tool, ToolUseID: "toolu_test",
		}
		if ctx == "sub" {
			o.AgentID, o.AgentType = "a1", "general-purpose"
		}
		if ti, ok := body["tool_input"].(map[string]any); ok {
			o.ToolInput = ti // the recorded input: real calls vary only values
		}
		got, err := scripted.BuildPayload(event, o)
		if err != nil {
			t.Fatalf("%s/%s/%s: %v", event, ctx, tool, err)
		}
		shapeIs(t, event+"/"+ctx, body, got)

		key := event + "|" + ctx
		if _, ok := firstOf[key]; !ok {
			firstOf[key] = body
		}
		seen[fmt.Sprintf("%s|%s|%s", event, ctx, tool)] = true
	}

	// An unrecorded tool in a recorded (event, context): the adapter
	// falls back to the first recorded post of that event and context,
	// so the envelope's key set and types still match — tool_input is
	// the call's own input data (asserted an object, nothing more).
	for key, body := range firstOf {
		event, ctx, _ := kindOf(body)
		if _, hasTool := body["tool_name"]; !hasTool {
			continue
		}
		o := scripted.PayloadOpts{
			SessionID: "sess", PromptID: "prompt", Cwd: "/world",
			ToolName: "ZxUnrecordedTool", ToolUseID: "toolu_zx",
			ToolInput: map[string]any{"made_up_field": 1},
		}
		if ctx == "sub" {
			o.AgentID, o.AgentType = "a1", "general-purpose"
		}
		got, err := scripted.BuildPayload(event, o)
		if err != nil {
			t.Fatalf("fallback %s: %v", key, err)
		}
		// tool_input is the call's own input data: the envelope's key set
		// and types must match, the input only has to be an object.
		if _, ok := got["tool_input"].(map[string]any); !ok {
			t.Fatalf("fallback %s: tool_input %T, want object", key, got["tool_input"])
		}
		delete(got, "tool_input")
		want := map[string]any{}
		for k, v := range body {
			want[k] = v
		}
		delete(want, "tool_input")
		shapeIs(t, "fallback/"+key, want, got)
		if tn, _ := got["tool_name"].(string); tn != "ZxUnrecordedTool" {
			t.Fatalf("fallback %s: tool_name %q", key, tn)
		}
	}

	// Kinds never recorded must be refused outright — the adapter cannot
	// invent a shape, and Set cannot invent a field.
	for _, bad := range []struct {
		event string
		o     scripted.PayloadOpts
	}{
		{"SubagentStart", scripted.PayloadOpts{}},                   // main ctx unrecorded
		{"SubagentStop", scripted.PayloadOpts{}},                    // main ctx unrecorded
		{"Stop", scripted.PayloadOpts{AgentID: "a1"}},               // sub ctx unrecorded
		{"UserPromptSubmit", scripted.PayloadOpts{AgentID: "a1"}},   // sub ctx unrecorded
		{"PostToolUseFailure", scripted.PayloadOpts{AgentID: "a1"}}, // sub ctx unrecorded
		{"PreToolUse", scripted.PayloadOpts{Set: map[string]any{
			"session_title": "invented"}}}, // not a field of the shape
		{"PreToolUse", scripted.PayloadOpts{Set: map[string]any{
			"effort": "high"}}}, // wrong type vs recorded object
	} {
		if _, err := scripted.BuildPayload(bad.event, bad.o); err == nil {
			t.Fatalf("unrecorded kind %+v built a payload", bad)
		}
	}
}
