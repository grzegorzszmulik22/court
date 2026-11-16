package scripted_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"court/internal/adapter/scripted"
	"court/internal/daemon"
	"court/internal/ledger"
	"court/internal/tools"
)

// eventsOfType returns ledger events of one type.
func eventsOfType(t *testing.T, h *scripted.Harness, typ string) []*ledger.Event {
	t.Helper()
	var out []*ledger.Event
	evs, err := h.Events()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range evs {
		if e.Type == typ {
			out = append(out, e)
		}
	}
	return out
}

// TestC1_6_1: each tool validates its input; a bad category is rejected
// with the list of valid ones.
func TestC1_6_1(t *testing.T) {
	h := newHarness(t)
	c, err := h.AddAgent(&scripted.AgentSpec{ID: "v1", Parent: "human"})
	if err != nil {
		t.Fatal(err)
	}
	// Unknown token is refused outright (sabotage: endpoint without token).
	resp, err := http.Post(h.Daemon.URL()+"/mcp/nope", "application/json",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode == 200 {
		t.Fatal("unknown token got an MCP endpoint")
	}
	if _, err := c.CallTool("record_decision", map[string]any{
		"question": "q", "category": "bogus", "paths": []string{"a.go"},
		"options": []any{map[string]any{"id": "x", "summary": "s"}},
		"chosen":  "x",
	}); err == nil || !strings.Contains(err.Error(), "valid categories") {
		t.Fatalf("bad category not rejected with the list: %v", err)
	}
	if _, err := c.CallTool("record_decision", map[string]any{
		"question": "q", "category": "interface", "paths": []string{"a.go"},
		"options": []any{map[string]any{"id": "x", "summary": "s"}},
		"chosen":  "nope",
	}); err == nil {
		t.Fatal("chosen outside options accepted")
	}
	if _, err := c.CallTool("alarm", map[string]any{"paths": []string{"a.go"}}); err == nil {
		t.Fatal("empty alarm text accepted")
	}
	if _, err := c.CallTool("claim_done", map[string]any{"checks": []string{"c1"}}); err == nil {
		t.Fatal("empty task accepted")
	}
	// A well-formed call passes.
	if _, err := c.CallTool("record_decision", map[string]any{
		"question": "which logger?", "category": "dependency", "paths": []string{"a.go"},
		"options": []any{
			map[string]any{"id": "slog", "summary": "stdlib"},
			map[string]any{"id": "zap", "summary": "external"},
		},
		"chosen": "slog", "petition": false,
	}); err != nil {
		t.Fatalf("valid record_decision rejected: %v", err)
	}
	ds := eventsOfType(t, h, "decision.declared")
	if len(ds) != 1 || ds[0].Actor != "agent:v1" {
		t.Fatalf("decision.declared = %v", ds)
	}
}

// TestC1_6_2: 1,000 interleaved calls from 8 scripted agents; each call
// attributes to the token's own agent, never another's.
func TestC1_6_2(t *testing.T) {
	h := newHarness(t)
	var cs []*scripted.Client
	for i := 0; i < 8; i++ {
		c, err := h.AddAgent(&scripted.AgentSpec{ID: fmt.Sprintf("a%d", i), Parent: "human"})
		if err != nil {
			t.Fatal(err)
		}
		cs = append(cs, c)
	}
	want := map[string]string{} // alarm text -> agent
	for round := 0; round < 125; round++ {
		for _, c := range cs {
			text := fmt.Sprintf("alarm-%s-%d", c.Agent, round)
			want[text] = "agent:" + c.Agent
			if _, err := c.CallTool("alarm", map[string]any{
				"text": text, "paths": []string{"f.go"},
			}); err != nil {
				t.Fatalf("%s call %d: %v", c.Agent, round, err)
			}
		}
	}
	alarms := eventsOfType(t, h, "alarm.raised")
	if len(alarms) != 1000 {
		t.Fatalf("alarm.raised count = %d, want 1000", len(alarms))
	}
	for _, e := range alarms {
		text := strOf(bodyField(t, e, "text"))
		if want[text] == "" {
			t.Fatalf("unscripted alarm text %q", text)
		}
		if e.Actor != want[text] {
			t.Fatalf("alarm %q attributed to %s, want %s", text, e.Actor, want[text])
		}
	}
}

// TestC1_6_3: a child's alarm reaches the ledger even when the parent's
// script drops every message it receives — the call goes agent→daemon.
func TestC1_6_3(t *testing.T) {
	h := newHarness(t)
	c, err := h.AddAgent(&scripted.AgentSpec{ID: "lead", Parent: "human"})
	if err != nil {
		t.Fatal(err)
	}
	// The child's PreToolUse pairs the next MCP call to the child actor;
	// the parent's script then does nothing at all (drops everything).
	if _, denied, err := c.PreToolUse("mcp__court__alarm",
		map[string]any{"text": "child alarm", "paths": []string{"x.go"}},
		"kid-01", "general-purpose"); err != nil || denied {
		t.Fatalf("child PreToolUse: denied=%v err=%v", denied, err)
	}
	if _, err := c.CallTool("alarm", map[string]any{
		"text": "child alarm", "paths": []string{"x.go"},
	}); err != nil {
		t.Fatal(err)
	}
	alarms := eventsOfType(t, h, "alarm.raised")
	if len(alarms) != 1 {
		t.Fatalf("alarm.raised count = %d", len(alarms))
	}
	if alarms[0].Actor != "agent:lead:kid-01" {
		t.Fatalf("child alarm attributed to %s, want agent:lead:kid-01", alarms[0].Actor)
	}
}

// TestC1_6_4: the endpoint works for a client speaking the pinned
// protocol revision without any session header.
func TestC1_6_4(t *testing.T) {
	h := newHarness(t)
	c, err := h.AddAgent(&scripted.AgentSpec{ID: "p1", Parent: "human"})
	if err != nil {
		t.Fatal(err)
	}
	c.ProtocolVersion = tools.ProtocolVersion // explicit pin; no session header is ever sent
	if _, err := c.CallTool("claim_done", map[string]any{
		"task": "t1", "checks": []string{"C1.6.4"},
	}); err != nil {
		t.Fatalf("pinned-revision call failed: %v", err)
	}
	claims := eventsOfType(t, h, "claim.done")
	if len(claims) != 1 || strOf(bodyField(t, claims[0], "task")) != "t1" {
		t.Fatalf("claim.done = %v", claims)
	}
}

// postRawRPC posts one tools/call JSON-RPC request body verbatim (a
// controlled request id, for retry testing) and returns the status code.
func postRawRPC(t *testing.T, url string, raw []byte) int {
	t.Helper()
	req, err := http.NewRequest("POST", url, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode
}

// TestMCP_RequestIdentity: a retry is recognised by the request's own
// identity — the session plus the JSON-RPC request id — never by content.
// Two identical alarms or decisions from one agent are two events; a
// reposted request id is one event; a repost after a daemon restart
// stays one (C1.8.3's guarantee holds on request identity).
func TestMCP_RequestIdentity(t *testing.T) {
	h := newHarness(t)
	c, err := h.AddAgent(&scripted.AgentSpec{ID: "i1", Parent: "human"})
	if err != nil {
		t.Fatal(err)
	}
	args := map[string]any{"text": "the same alarm twice", "paths": []string{"a.go"}}
	// Two distinct requests with identical arguments: two events.
	for i := 0; i < 2; i++ {
		if _, err := c.CallTool("alarm", args); err != nil {
			t.Fatalf("alarm %d: %v", i, err)
		}
	}
	if n := len(eventsOfType(t, h, "alarm.raised")); n != 2 {
		t.Fatalf("identical alarms recorded %d events, want 2", n)
	}
	// Two identical decisions: two events.
	dargs := map[string]any{
		"question": "same decision?", "category": "interface", "paths": []string{"a.go"},
		"options": []any{map[string]any{"id": "a", "summary": "s"}},
		"chosen":  "a",
	}
	for i := 0; i < 2; i++ {
		if _, err := c.CallTool("record_decision", dargs); err != nil {
			t.Fatalf("decision %d: %v", i, err)
		}
	}
	if n := len(eventsOfType(t, h, "decision.declared")); n != 2 {
		t.Fatalf("identical decisions recorded %d events, want 2", n)
	}
	// One request reposted: the same JSON-RPC id is one event.
	raw, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 424242, "method": "tools/call",
		"params": map[string]any{
			"name": "alarm",
			"arguments": map[string]any{
				"text": "retried request", "paths": []string{"b.go"},
			},
		},
	})
	for i := 0; i < 3; i++ {
		if st := postRawRPC(t, c.MCPURL, raw); st != 200 {
			t.Fatalf("repost %d: status %d", i, st)
		}
	}
	if n := len(eventsOfType(t, h, "alarm.raised")); n != 3 {
		t.Fatalf("retried request recorded %d total alarms, want 3", n)
	}
	// After a daemon restart over the same ledger, the same repost is
	// still deduplicated — the identity is durable, not in memory.
	oldURL := h.Daemon.URL()
	h.Daemon.Close()
	d2, err := daemon.New(h.Store, daemon.Options{
		DataDir: h.DataDir, CourtBin: h.CourtBin})
	if err != nil {
		t.Fatal(err)
	}
	if err := d2.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d2.Close)
	url := strings.Replace(c.MCPURL, oldURL, d2.URL(), 1)
	if st := postRawRPC(t, url, raw); st != 200 {
		t.Fatalf("post-restart repost: status %d", st)
	}
	if n := len(eventsOfType(t, h, "alarm.raised")); n != 3 {
		t.Fatalf("repost after restart recorded %d total alarms, want 3", n)
	}
}

// TestC1_6_5: every tool description is one sentence of at most 200
// characters.
func TestC1_6_5(t *testing.T) {
	h := newHarness(t)
	c, err := h.AddAgent(&scripted.AgentSpec{ID: "d1", Parent: "human"})
	if err != nil {
		t.Fatal(err)
	}
	list, err := c.ListTools()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("tools/list = %d tools, want 3", len(list))
	}
	for _, tool := range list {
		desc, _ := tool["description"].(string)
		if len(desc) > 200 {
			t.Fatalf("%s description %d chars > 200", tool["name"], len(desc))
		}
		if !strings.HasSuffix(desc, ".") || strings.Contains(desc[:len(desc)-1], ".") {
			t.Fatalf("%s description is not one sentence: %q", tool["name"], desc)
		}
	}
}
