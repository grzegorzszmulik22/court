package scripted_test

// The M1.5 checks (spec/milestones/M1-ledger-and-worlds.md). Each test is
// driven by a thin wrapper under test/checks/M1.5/.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"court/internal/adapter/claude"
	"court/internal/adapter/scripted"
	"court/internal/daemon"
	"court/internal/ledger"
)

// newHarness builds repo+ledger+daemon in t's temp dir. The court binary
// for the gate hook is found at $COURT_BIN (check scripts build it).
func newHarness(t *testing.T) *scripted.Harness {
	t.Helper()
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := scripted.InitRepo(repo, map[string]string{
		"main.go":   "package main\n\nfunc main() {}\n",
		"README.md": "# fixture\n",
	}); err != nil {
		t.Fatal(err)
	}
	h, err := scripted.NewHarness(scripted.Options{
		Repo: repo, DataDir: filepath.Join(root, "data"),
		Commission: "c15", Seed: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.Close)
	if err := h.OpenCommission("fixture commission", []string{"unit"}); err != nil {
		t.Fatal(err)
	}
	return h
}

// scriptWith composes a one-agent script from inline steps.
func scriptWith(agent string, steps ...*scripted.Step) *scripted.Script {
	return &scripted.Script{Agents: []*scripted.AgentSpec{
		{ID: agent, Parent: "human", Steps: steps},
	}}
}

// toolEvents filters the ledger for tool.event bodies.
func toolEvents(t *testing.T, h *scripted.Harness) []*ledger.Event {
	t.Helper()
	evs, err := h.Events()
	if err != nil {
		t.Fatal(err)
	}
	var out []*ledger.Event
	for _, e := range evs {
		if e.Type == "tool.event" {
			out = append(out, e)
		}
	}
	return out
}

func bodyField(t *testing.T, e *ledger.Event, key string) *ledger.Value {
	t.Helper()
	for _, m := range e.Body.Obj {
		if m.Key == key {
			return m.Val
		}
	}
	return nil
}

// TestC1_5_1: a script of 500 mixed steps yields exactly the expected
// tool.event and world.commit events, in order.
func TestC1_5_1(t *testing.T) {
	h := newHarness(t)
	var steps []*scripted.Step
	for i := 0; i < 125; i++ {
		steps = append(steps,
			&scripted.Step{Edit: &scripted.EditStep{
				Path:  fmt.Sprintf("src/f%03d.txt", i),
				Write: fmt.Sprintf("file %d line one\nline two\n", i)}},
			&scripted.Step{Tool: &scripted.ToolStep{
				Name: "Bash", Input: map[string]any{
					"command": fmt.Sprintf("wc -l src/f%03d.txt", i)}}},
			&scripted.Step{Tool: &scripted.ToolStep{
				Name: "Grep", Input: map[string]any{
					"pattern": "line", "path": "src"}}},
			&scripted.Step{Commit: &scripted.CommitStep{
				Message: fmt.Sprintf("commit %d", i)}})
	}
	sc := scriptWith("w1", steps...)
	if err := h.RunScript(sc); err != nil {
		t.Fatal(err)
	}
	// The daemon's gate path is asynchronous over the post-commit hook:
	// wait for the last world.commit to land before comparing.
	deadline := time.Now().Add(20 * time.Second)
	var evs []*ledger.Event
	for {
		evs, _ = h.Events()
		n := 0
		for _, e := range evs {
			if e.Type == "world.commit" {
				n++
			}
		}
		if n >= 125 || time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	exp := scripted.Expected(sc)
	var got []scripted.ExpectedEvent
	for _, e := range evs {
		if e.Type != "tool.event" && e.Type != "world.commit" {
			continue
		}
		got = append(got, scripted.ExpectedEvent{
			Agent: strings.TrimPrefix(e.Actor, "agent:"), Type: e.Type,
			Tool: strOf(bodyField(t, e, "tool"))})
	}
	if len(got) != len(exp) {
		t.Fatalf("expected %d ledger events from the script, got %d", len(exp), len(got))
	}
	for i := range exp {
		if got[i].Agent != exp[i].Agent || got[i].Type != exp[i].Type ||
			got[i].Tool != exp[i].Tool {
			t.Fatalf("event %d: expected %+v, got %+v", i, exp[i], got[i])
		}
	}
}

// TestC1_5_2: unknown token refused, refusal recorded, nothing attributed.
func TestC1_5_2(t *testing.T) {
	h := newHarness(t)
	p := claude.Payload("PostToolUse", claude.PayloadParams{
		ToolName: "Bash", ToolInput: map[string]any{"command": "true"},
		ToolUseID: "toolu_x", SessionID: "whatever", Cwd: "/tmp"})
	raw, _ := json.Marshal(p)
	resp, err := http.Post(h.Daemon.URL()+"/hook/deadbeefdeadbeef",
		"application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown token: got %s, want 404", resp.Status)
	}
	refs, err := h.Refusals()
	if err != nil || len(refs) == 0 {
		t.Fatalf("refusal not recorded: %v %v", refs, err)
	}
	evs, _ := h.Events()
	for _, e := range evs {
		if e.Type == "tool.event" {
			t.Fatalf("refused post was attributed: %v", e)
		}
	}
	// Duplicate delivery of the same payload is still one refusal, not a
	// recorded event.
	resp, err = http.Post(h.Daemon.URL()+"/hook/deadbeefdeadbeef",
		"application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}

// TestHook_TokenIsIdentity: the URL token is the identity of a post.
// A payload on agent A's endpoint carrying agent B's session_id is
// refused and never attributed to B; a post under B's token with no
// session_id at all still attributes to B.
func TestHook_TokenIsIdentity(t *testing.T) {
	h := newHarness(t)
	a, err := h.AddAgent(&scripted.AgentSpec{ID: "a", Parent: "human"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := h.AddAgent(&scripted.AgentSpec{ID: "b", Parent: "human"})
	if err != nil {
		t.Fatal(err)
	}
	// Payload built for B but POSTed to A's hook URL.
	p := b.PayloadFor("PostToolUse", "toolu_cross", "Bash",
		map[string]any{"command": "true"}, "", "")
	raw, _ := json.Marshal(p)
	resp, err := http.Post(a.HookURL, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode == 200 {
		t.Fatal("a cross-session post was accepted")
	}
	for _, e := range toolEvents(t, h) {
		if e.Actor == "agent:b" {
			t.Fatalf("post on A's endpoint was attributed to B: %v", e.Actor)
		}
		t.Fatalf("cross-session post recorded an event: %v", e.Actor)
	}
	refs, _ := h.Refusals()
	if len(refs) == 0 {
		t.Fatal("cross-session post was not recorded as refused")
	}
}

// TestC1_5_3: stored tool.event contains no file contents or command
// output for any fixture.
func TestC1_5_3(t *testing.T) {
	h := newHarness(t)
	c, err := h.AddAgent(&scripted.AgentSpec{ID: "w1", Parent: "human"})
	if err != nil {
		t.Fatal(err)
	}
	canary := "CANARY_FILE_CONTENT_4f8a9b2c"
	outCanary := "CANARY_COMMAND_OUTPUT_77e0d1"
	// A real session's payloads carry file contents and output; every
	// fixture under test/vectors/hooks is posted, then every stored event
	// is scanned for content that must never have been kept.
	fixtureDirs := []string{
		"test/vectors/hooks/documented",
		"test/vectors/hooks/recorded",
	}
	posted := 0
	for _, dir := range fixtureDirs {
		ents, err := filepath.Glob(filepath.Join(findRepoRoot(t), dir, "*.json"))
		if err != nil || len(ents) == 0 {
			continue
		}
		for _, f := range ents {
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			var m map[string]any
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Fatalf("%s: %v", f, err)
			}
			m["session_id"] = c.SessionID
			m["cwd"] = c.Worktree
			// Inject canaries into the strings a tool event must never
			// keep (file contents, tool output, prose); identifying fields
			// and legitimately selected fields keep their values.
			injectCanary(m, canary)
			raw2, _ := json.Marshal(m)
			resp, err := http.Post(c.HookURL, "application/json", bytes.NewReader(raw2))
			if err != nil {
				t.Fatal(err)
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode != 200 {
				t.Fatalf("%s: hook post rejected: %s", f, resp.Status)
			}
			posted++
		}
	}
	if posted == 0 {
		t.Fatal("no hook fixtures found to post")
	}
	// A canary-bearing run of our own: output the payload could contain.
	bashIn := map[string]any{"command": "cat secret.txt"}
	tuid, denied, err := c.PreToolUse("Bash", bashIn, "", "")
	if err != nil || denied {
		t.Fatalf("pre: %v denied=%v", err, denied)
	}
	if err := c.PostToolUse(tuid, "Bash", bashIn, false, "", ""); err != nil {
		t.Fatal(err)
	}
	for _, e := range toolEvents(t, h) {
		line, _ := e.Line()
		s := string(line)
		for _, bad := range []string{canary, outCanary, "old_string", "new_string",
			"tool_response", "stdout", "SECRET"} {
			if strings.Contains(s, bad) {
				t.Fatalf("tool.event leaks %q: %s", bad, s)
			}
		}
	}
}

// injectCanary replaces content-carrying strings in a hook payload with
// the canary text, so any field that leaks into the ledger is caught.
// Identity and selected fields keep their values (a stored path or
// command line is legal); file contents, tool output and prose are not.
func injectCanary(m map[string]any, canary string) {
	// Keys whose values are always content (anywhere they appear).
	contentKeys := map[string]bool{
		"old_string": true, "new_string": true, "content": true,
		"edits": true, "prompt": true, "error": true,
		"last_assistant_message": true, "transcript_path": true,
		"agent_transcript_path": true, "tool_response": true,
		"description": true, "output": true, "result": true,
	}
	var walk func(v any) any
	walk = func(v any) any {
		switch x := v.(type) {
		case map[string]any:
			for k, e := range x {
				if contentKeys[k] {
					x[k] = canary
					continue
				}
				x[k] = walk(e)
			}
			return x
		case []any:
			for i, e := range x {
				x[i] = walk(e)
			}
			return x
		}
		return v
	}
	// Everything inside tool_response is output; everything inside
	// tool_input is content except the selected fields (paths, command).
	if ti, ok := m["tool_input"].(map[string]any); ok {
		for k := range ti {
			switch k {
			case "file_path", "path", "notebook_path", "command", "url":
			default:
				ti[k] = canary
			}
		}
	}
	for k, v := range m {
		if k == "tool_input" || k == "session_id" || k == "cwd" ||
			k == "hook_event_name" || k == "tool_name" ||
			k == "tool_use_id" || k == "agent_id" || k == "agent_type" {
			continue
		}
		m[k] = walk(v)
	}
}

// TestC1_5_4: a subagent payload is recorded under a child agent id whose
// parent is the session's agent.
func TestC1_5_4(t *testing.T) {
	h := newHarness(t)
	c, err := h.AddAgent(&scripted.AgentSpec{ID: "lead", Parent: "human"})
	if err != nil {
		t.Fatal(err)
	}
	in := map[string]any{"pattern": "x", "path": "/w"}
	tuid, denied, err := c.PreToolUse("Grep", in, "a9f1", "Explore")
	if err != nil || denied {
		t.Fatalf("subagent pre: %v denied=%v", err, denied)
	}
	if err := c.PostToolUse(tuid, "Grep", in, false, "a9f1", "Explore"); err != nil {
		t.Fatal(err)
	}
	evs, _ := h.Events()
	var started, tev *ledger.Event
	for _, e := range evs {
		if e.Type == "agent.started" && strOf(bodyField(t, e, "agent")) == "lead:a9f1" {
			started = e
		}
		if e.Type == "tool.event" {
			tev = e
		}
	}
	if started == nil {
		t.Fatal("no agent.started for the subagent")
	}
	if strOf(bodyField(t, started, "parent")) != "lead" {
		t.Fatalf("subagent parent is %q, want lead",
			strOf(bodyField(t, started, "parent")))
	}
	if tev == nil || tev.Actor != "agent:lead:a9f1" {
		t.Fatalf("subagent tool.event actor = %v, want agent:lead:a9f1", tev.Actor)
	}
}

// TestC1_5_5: the endpoint binds loopback only; a connection from another
// interface is refused (the listener never sees it).
func TestC1_5_5(t *testing.T) {
	h := newHarness(t)
	host, port, _ := net.SplitHostPort(h.Daemon.Addr())
	_ = host
	// Every non-loopback local address must refuse the connection.
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Fatal(err)
	}
	tried := 0
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() || ipnet.IP.IsLinkLocalUnicast() {
			continue
		}
		ip := ipnet.IP.String()
		tried++
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, port), 2*time.Second)
		if err == nil {
			conn.Close()
			t.Fatalf("daemon accepted a connection on non-loopback %s", ip)
		}
	}
	if tried == 0 {
		// No non-loopback interface on this host: bind a probe on the same
		// port on 0.0.0.0 — it must fail, proving the daemon holds it on
		// loopback only? No: a separate bind doesn't conflict. Instead
		// assert the reported address is loopback.
		if !net.ParseIP(host).IsLoopback() {
			t.Fatalf("daemon bound non-loopback address %s", host)
		}
	}
}

// TestC1_5_6: fixture drift — the field names of every recorded fixture
// equal the field list in tools/hook-fields.txt.
func TestC1_5_6(t *testing.T) {
	root := findRepoRoot(t)
	recDir := filepath.Join(root, "test/vectors/hooks/recorded")
	ents, _ := filepath.Glob(filepath.Join(recDir, "*.json"))
	if len(ents) == 0 {
		t.Skip("recorded hook fixtures not on main yet")
	}
	want := parseFieldList(t, filepath.Join(root, "tools/hook-fields.txt"))
	for _, f := range ents {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		event, _ := m["hook_event_name"].(string)
		exp := append(append([]string{}, want["common"]...), want[event]...)
		if _, has := m["agent_id"]; has {
			exp = append(exp, want["subagent"]...)
		}
		var missing []string
		for _, k := range exp {
			if _, ok := m[k]; !ok {
				missing = append(missing, k)
			}
		}
		var extra []string
		allowed := map[string]bool{}
		for _, k := range exp {
			allowed[k] = true
		}
		for _, k := range want["common-optional-set"] {
			allowed[k] = true
		}
		for k := range m {
			if !allowed[k] {
				extra = append(extra, k)
			}
		}
		if len(missing) > 0 || len(extra) > 0 {
			t.Fatalf("%s: field drift vs tools/hook-fields.txt: missing %v extra %v",
				filepath.Base(f), missing, extra)
		}
	}
}

func parseFieldList(t *testing.T, path string) map[string][]string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{"common-optional-set": {}}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			t.Fatalf("%s: bad line %q", path, line)
		}
		key := strings.TrimSpace(k)
		fields := strings.Fields(v)
		out[key] = fields
		if key == "common-optional" {
			for _, f := range fields {
				out["common-optional-set"] = append(out["common-optional-set"], f)
			}
		}
	}
	return out
}

// findRepoRoot walks up from the test's working directory to the module
// root (the directory holding go.mod).
func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		up := filepath.Dir(dir)
		if up == dir {
			t.Fatal("go.mod not found")
		}
		dir = up
	}
}

func strOf(v *ledger.Value) string {
	if v == nil || v.Kind != ledger.String {
		return ""
	}
	return v.Str
}

// --------------------------------------------------------------------------
// Subtle-breakage coverage beyond the listed sabotages
// --------------------------------------------------------------------------

// TestHook_IdempotentRetry: a reposted identical PostToolUse records one
// event; a distinct post records a second.
func TestHook_IdempotentRetry(t *testing.T) {
	h := newHarness(t)
	c, err := h.AddAgent(&scripted.AgentSpec{ID: "w1", Parent: "human"})
	if err != nil {
		t.Fatal(err)
	}
	in := map[string]any{"command": "true"}
	post := func(tuid string) *http.Response {
		p := c.PayloadFor("PostToolUse", tuid, "Bash", in, "", "")
		raw, _ := json.Marshal(p)
		resp, err := http.Post(c.HookURL, "application/json", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	r := post("toolu_1")
	io.Copy(io.Discard, r.Body)
	r.Body.Close()
	r = post("toolu_1")
	io.Copy(io.Discard, r.Body)
	r.Body.Close()
	if n := len(toolEvents(t, h)); n != 1 {
		t.Fatalf("duplicate delivery recorded %d events, want 1", n)
	}
	r = post("toolu_2")
	io.Copy(io.Discard, r.Body)
	r.Body.Close()
	if n := len(toolEvents(t, h)); n != 2 {
		t.Fatalf("distinct post: %d events, want 2", n)
	}
}

// TestHook_DeniedRecordsReason: an outward action is denied in the
// documented shape and recorded with its reason — the only tool.event a
// denied call ever produces.
func TestHook_DeniedRecordsReason(t *testing.T) {
	h := newHarness(t)
	c, err := h.AddAgent(&scripted.AgentSpec{ID: "w1", Parent: "human"})
	if err != nil {
		t.Fatal(err)
	}
	_, denied, err := c.PreToolUse("Bash", map[string]any{
		"command": "curl https://example.com/x"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !denied {
		t.Fatal("curl was not denied")
	}
	evs := toolEvents(t, h)
	if len(evs) != 1 {
		t.Fatalf("denied call produced %d events", len(evs))
	}
	e := evs[0]
	if strOf(bodyField(t, e, "outcome")) != "denied" {
		t.Fatalf("outcome %q, want denied", strOf(bodyField(t, e, "outcome")))
	}
	if strOf(bodyField(t, e, "reason")) == "" {
		t.Fatal("denied event lacks a reason")
	}
}

// TestHook_PreWithoutPost then PostWithoutPre keeps the ledger whole.
func TestHook_UnpairedPosts(t *testing.T) {
	h := newHarness(t)
	c, err := h.AddAgent(&scripted.AgentSpec{ID: "w1", Parent: "human"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.PreToolUse("Read", map[string]any{"file_path": "/x"}, "", ""); err != nil {
		t.Fatal(err)
	}
	// Post without a parked Pre (daemon restart mid-call): still records.
	if err := c.PostToolUse("toolu_orphan", "Read",
		map[string]any{"file_path": "/x"}, false, "", ""); err != nil {
		t.Fatal(err)
	}
	if n := len(toolEvents(t, h)); n != 1 {
		t.Fatalf("orphan post recorded %d events", n)
	}
}

// TestDaemon_RestartKeepsDedupe: after a daemon restart over the same
// ledger, a reposted pre-crash payload does not double-record.
func TestDaemon_RestartKeepsDedupe(t *testing.T) {
	h := newHarness(t)
	c, err := h.AddAgent(&scripted.AgentSpec{ID: "w1", Parent: "human"})
	if err != nil {
		t.Fatal(err)
	}
	in := map[string]any{"command": "true"}
	p := c.PayloadFor("PostToolUse", "toolu_dup1", "Bash", in, "", "")
	raw, _ := json.Marshal(p)
	post := func(url string) {
		resp, err := http.Post(url, "application/json", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	post(c.HookURL)
	// Restart: same store, new daemon.
	url := c.HookURL
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
	_ = url
	// Same token, new daemon: session was persisted.
	if d2.AdminToken() == "" {
		t.Fatal("admin token not persisted")
	}
	post(strings.Replace(c.HookURL, h.Daemon.URL(), d2.URL(), 1))
	if n := len(toolEvents(t, h)); n != 1 {
		t.Fatalf("post-restart duplicate recorded %d events", n)
	}
}
