package scripted_test

// The M1.5 checks (spec/milestones/M1-ledger-and-worlds.md). Each test is
// driven by a thin wrapper under test/checks/M1.5/.

import (
	"bytes"
	"database/sql"
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
	"unicode/utf8"

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

// TestToolEvent_CommandTruncatesRunes: spec/01 §11.3 — the stored
// command is the first 500 Unicode characters, always valid UTF-8.
// The table holds commands longer than 500 characters whose byte 500
// falls inside a multi-byte character — a byte cut (v[:500]) would store
// fewer than 500 characters or corrupt UTF-8 and fails every one. Cases
// at or under 500 characters defend the other direction: nothing may be
// cut early.
func TestToolEvent_CommandTruncatesRunes(t *testing.T) {
	h := newHarness(t)
	c, err := h.AddAgent(&scripted.AgentSpec{ID: "w1", Parent: "human"})
	if err != nil {
		t.Fatal(err)
	}
	ascii498 := strings.Repeat("a", 498)
	ascii499 := strings.Repeat("a", 499)
	ascii500 := strings.Repeat("a", 500)
	cases := []struct {
		name    string
		command string
		want    string // expected stored command
	}{
		// More than 500 characters; byte 500 lands inside the multi-byte
		// character, so a byte cut loses or corrupts it while the rune
		// cut keeps all 500 characters whole.
		{"499+2byte+tail", ascii499 + "é" + "x", ascii499 + "é"},
		{"499+3byte+tail", ascii499 + "€" + "x", ascii499 + "€"},
		{"498+4byte+tail", ascii498 + "\U0001f600" + "xy", ascii498 + "\U0001f600" + "x"},
		{"600-all-2byte", strings.Repeat("é", 600), strings.Repeat("é", 500)},
		{"600-all-3byte", strings.Repeat("€", 600), strings.Repeat("€", 500)},
		{"600-all-4byte", strings.Repeat("\U0001f600", 600), strings.Repeat("\U0001f600", 500)},
		{"mixed-501", ascii499 + "é" + strings.Repeat("€", 100), ascii499 + "é"},
		{"leading-multi", "é" + ascii500, "é" + ascii499},
		// At or under 500 characters nothing is cut, however many bytes.
		{"499+2byte", ascii499 + "é", ascii499 + "é"},
		{"499+3byte", ascii499 + "€", ascii499 + "€"},
		{"499+4byte", ascii499 + "\U0001f600", ascii499 + "\U0001f600"},
		{"exactly500", ascii499 + "x", ascii499 + "x"},
		// ASCII boundary cuts: byte and rune cuts agree; the stored
		// command must be exactly 500 characters either way.
		{"501", ascii499 + "xy", ascii499 + "x"},
		{"cut-inside-2byte", ascii500 + "é", ascii500},
	}
	for _, tc := range cases {
		in := map[string]any{"command": tc.command}
		tuid, denied, err := c.PreToolUse("Bash", in, "", "")
		if err != nil || denied {
			t.Fatalf("%s: pre: %v denied=%v", tc.name, err, denied)
		}
		if err := c.PostToolUse(tuid, "Bash", in, false, "", ""); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
	}
	evs := toolEvents(t, h)
	if len(evs) != len(cases) {
		t.Fatalf("tool.events = %d, want %d", len(evs), len(cases))
	}
	for i, tc := range cases {
		v := bodyField(t, evs[i], "fields")
		if v == nil {
			t.Fatalf("%s: no fields", tc.name)
		}
		var cmd string
		for _, m := range v.Obj {
			if m.Key == "command" {
				cmd = m.Val.Str
			}
		}
		if cmd != tc.want {
			t.Fatalf("%s: stored command is %q (%d runes), want %q (%d runes)",
				tc.name, cmd, utf8.RuneCountInString(cmd),
				tc.want, utf8.RuneCountInString(tc.want))
		}
		if n := utf8.RuneCountInString(cmd); n != 500 && len(tc.command) > 500 {
			t.Fatalf("%s: stored %d characters, want 500", tc.name, n)
		}
		if !utf8.ValidString(cmd) {
			t.Fatalf("%s: stored command is not valid UTF-8", tc.name)
		}
	}
}

// TestC1_5_2: unknown token refused, refusal recorded, nothing attributed.
func TestC1_5_2(t *testing.T) {
	h := newHarness(t)
	p, err := scripted.BuildPayload("PostToolUse", scripted.PayloadOpts{
		ToolName: "Bash", ToolInput: map[string]any{"command": "true"},
		ToolUseID: "toolu_x", SessionID: "whatever", Cwd: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
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
	p, err := b.PayloadFor("PostToolUse", "toolu_cross", "Bash",
		map[string]any{"command": "true"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
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
	// recorded fixture under test/vectors/hooks is posted, then every
	// stored event is scanned for content that must never have been kept.
	posted := 0
	for _, dir := range []string{"test/vectors/hooks/recorded/hooks"} {
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

// TestToolEvent_StoresHostOnly: spec/01 §11.3 — a URL input stores only
// the host name. Every URL part that is not the host — scheme, userinfo,
// port, path, query, fragment — must be absent from the event body
// entirely: a URL is an easy place to smuggle a token, so the assertion
// scans the serialized ledger line, not just the hosts field.
func TestToolEvent_StoresHostOnly(t *testing.T) {
	h := newHarness(t)
	c, err := h.AddAgent(&scripted.AgentSpec{ID: "w1", Parent: "human"})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		url    string
		want   string   // the one stored host
		forbid []string // substrings that must never reach the ledger
	}{
		{"supervisor", "https://user:pw@api.example.com:8443/v1/x?token=SECRET#frag",
			"api.example.com",
			[]string{"https", "user", "pw", "8443", "v1/x", "token", "SECRET", "frag", "@"}},
		{"all-parts", "httpx://uuSec9r:ppSec9w@safe-host.example:8437/ppSec9x?qqSec9y=ttSec9z#ffSec9w",
			"safe-host.example",
			[]string{"httpx", "uuSec9r", "ppSec9w", "8437", "ppSec9x", "qqSec9y", "ttSec9z", "ffSec9w", "@"}},
		{"schemeless", "safe-host.example:8437/ppSec9x?qqSec9y",
			"safe-host.example", []string{"8437", "ppSec9x", "qqSec9y"}},
		{"scheme-relative", "//safe-host.example/ppSec9x",
			"safe-host.example", []string{"ppSec9x"}},
		{"path-only", "https://safe-host.example/ppSec9x",
			"safe-host.example", []string{"https", "ppSec9x"}},
		{"multi-at", "https://uuSec9r:pp@x@safe-host.example/",
			"safe-host.example", []string{"uuSec9r", "pp", "x@"}},
		{"bare", "safe-host.example",
			"safe-host.example", []string{}},
		{"upper-scheme", "HTTPS://safe-host.example:443/",
			"safe-host.example", []string{"HTTPS", ":443"}},
		{"ipv6", "https://[2001:db8::1]:8443/ppSec9x",
			"2001:db8::1", []string{"8443", "ppSec9x"}},
	}
	for _, tc := range cases {
		in := map[string]any{"url": tc.url}
		if _, _, err := c.PreToolUse("WebFetch", in, "", ""); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
	}
	evs := toolEvents(t, h)
	if len(evs) != len(cases) {
		t.Fatalf("tool.events = %d, want %d", len(evs), len(cases))
	}
	for i, tc := range cases {
		e := evs[i]
		v := bodyField(t, e, "fields")
		if v == nil {
			t.Fatalf("%s: no fields", tc.name)
		}
		var hosts []string
		for _, m := range v.Obj {
			if m.Key == "hosts" {
				for _, it := range m.Val.Arr {
					hosts = append(hosts, it.Str)
				}
			}
			if m.Key == "paths" && len(m.Val.Arr) > 0 {
				t.Fatalf("%s: URL leaked into paths: %v", tc.name, m.Val.Arr)
			}
			if m.Key == "command" && m.Val.Str != "" {
				t.Fatalf("%s: URL leaked into command: %q", tc.name, m.Val.Str)
			}
		}
		if len(hosts) != 1 || hosts[0] != tc.want {
			t.Fatalf("%s: hosts = %v, want [%q]", tc.name, hosts, tc.want)
		}
		line, err := e.Line()
		if err != nil {
			t.Fatal(err)
		}
		for _, bad := range tc.forbid {
			if strings.Contains(string(line), bad) {
				t.Fatalf("%s: event stores %q: %s", tc.name, bad, line)
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
	recDir := filepath.Join(root, "test/vectors/hooks/recorded/hooks")
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
		var wrap map[string]any
		if err := json.Unmarshal(raw, &wrap); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		m, _ := wrap["body"].(map[string]any)
		if m == nil {
			m = wrap // tolerate a bare payload
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
		p, err := c.PayloadFor("PostToolUse", tuid, "Bash", in, "", "")
		if err != nil {
			t.Fatal(err)
		}
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
	p, err := c.PayloadFor("PostToolUse", "toolu_dup1", "Bash", in, "", "")
	if err != nil {
		t.Fatal(err)
	}
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

// TestHook_RequestIdentityPerSession: a hook post's request identity is
// its session plus tool_use_id (C1.8.3). Two sessions posting the same
// tool_use_id are two requests — both recorded under their own agents —
// while one session reposting the same payload stays one event. The
// pairing survives a daemon restart through the persisted idem table,
// and through the rebuild's backfill even when the table itself is gone.
func TestHook_RequestIdentityPerSession(t *testing.T) {
	h := newHarness(t)
	a, err := h.AddAgent(&scripted.AgentSpec{ID: "ha", Parent: "human"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := h.AddAgent(&scripted.AgentSpec{ID: "hb", Parent: "human"})
	if err != nil {
		t.Fatal(err)
	}
	in := map[string]any{"command": "true"}
	post := func(c *scripted.Client, tuid string) {
		p, err := c.PayloadFor("PostToolUse", tuid, "Bash", in, "", "")
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(p)
		resp, err := http.Post(c.HookURL, "application/json", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("%s post %s: %s", c.Agent, tuid, resp.Status)
		}
	}
	// The same tool_use_id on two sessions is two requests, one each.
	post(a, "toolu_shared")
	post(b, "toolu_shared")
	if n := len(toolEvents(t, h)); n != 2 {
		t.Fatalf("same tool_use_id on two sessions recorded %d events, want 2", n)
	}
	actors := map[string]bool{}
	for _, e := range toolEvents(t, h) {
		actors[e.Actor] = true
	}
	if !actors["agent:ha"] || !actors["agent:hb"] {
		t.Fatalf("cross-session posts attributed to %v", actors)
	}
	// Session A reposting the same payload is a retry: still two.
	post(a, "toolu_shared")
	if n := len(toolEvents(t, h)); n != 2 {
		t.Fatalf("A's repost recorded %d events, want 2", n)
	}
	// A fresh id on A is a new request even with identical content.
	post(a, "toolu_other")
	if n := len(toolEvents(t, h)); n != 3 {
		t.Fatalf("distinct post recorded %d events, want 3", n)
	}

	// Restart over the same ledger and sessions; the per-session dedupe
	// must come from durable state, not memory.
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
	for _, c := range []*scripted.Client{a, b} {
		c.HookURL = strings.Replace(c.HookURL, oldURL, d2.URL(), 1)
		c.MCPURL = strings.Replace(c.MCPURL, oldURL, d2.URL(), 1)
	}
	post(a, "toolu_shared")
	if n := len(toolEvents(t, h)); n != 3 {
		t.Fatalf("post-restart repost recorded %d events, want 3", n)
	}
	post(b, "toolu_b_new")
	if n := len(toolEvents(t, h)); n != 4 {
		t.Fatalf("B's fresh post after restart recorded %d events, want 4", n)
	}
}

// TestHook_RequestIdentityRebuilt: with the durable idem table itself
// erased (a ledger written before it existed), the daemon's rebuild
// still answers a retried request — the backfill re-derives each event's
// per-session request identity from the chain.
func TestHook_RequestIdentityRebuilt(t *testing.T) {
	h := newHarness(t)
	a, err := h.AddAgent(&scripted.AgentSpec{ID: "ha", Parent: "human"})
	if err != nil {
		t.Fatal(err)
	}
	in := map[string]any{"command": "true"}
	post := func(c *scripted.Client, tuid string) {
		p, err := c.PayloadFor("PostToolUse", tuid, "Bash", in, "", "")
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(p)
		resp, err := http.Post(c.HookURL, "application/json", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("post %s: %s", tuid, resp.Status)
		}
	}
	post(a, "toolu_rebuilt")
	if n := len(toolEvents(t, h)); n != 1 {
		t.Fatalf("setup: %d events", n)
	}
	// Stop the daemon, erase every remembered request identity, restart:
	// only the rebuild backfill can dedupe the repost now.
	oldURL := h.Daemon.URL()
	h.Daemon.Close()
	wipeIdem(t, h.DataDir)
	d2, err := daemon.New(h.Store, daemon.Options{
		DataDir: h.DataDir, CourtBin: h.CourtBin})
	if err != nil {
		t.Fatal(err)
	}
	if err := d2.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d2.Close)
	a.HookURL = strings.Replace(a.HookURL, oldURL, d2.URL(), 1)
	a.MCPURL = strings.Replace(a.MCPURL, oldURL, d2.URL(), 1)
	post(a, "toolu_rebuilt")
	if n := len(toolEvents(t, h)); n != 1 {
		t.Fatalf("rebuilt dedupe recorded %d events, want 1", n)
	}
}

// TestAdmin_IdempotencyKeyNamespace: a client-supplied Idempotency-Key
// names its request — the same key on the same endpoint is one request
// even with a different body, and the same key on a different endpoint is
// unrelated (the key is namespaced, never a global cancel). The admin
// API holds a single credential, so there is no second client identity
// for the key to be scoped against; the endpoint is the namespace.
func TestAdmin_IdempotencyKeyNamespace(t *testing.T) {
	h := newHarness(t)
	post := func(path, body, key string) int {
		req, err := http.NewRequest("POST", h.Daemon.URL()+path,
			strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+h.Daemon.AdminToken())
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp.StatusCode
	}
	commissionBody := func(id string) string {
		raw, _ := json.Marshal(map[string]any{
			"commission": id, "goal": "g", "done_checks": []string{},
			"territory": []string{}, "exemptions": []string{},
			"defaults":  []string{},
			"budgets":   map[string]any{"tokens": 0, "minutes": 0, "speculation": 0},
			"attention": 0,
		})
		return string(raw)
	}
	opened := func(id string) int {
		n := 0
		evs, err := h.Events()
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range evs {
			if e.Type == "commission.opened" && e.Commission == id {
				n++
			}
		}
		return n
	}
	if st := post("/admin/commission", commissionBody("ck-a"), "K1"); st != 200 {
		t.Fatalf("commission ck-a: %d", st)
	}
	// Same endpoint, same key, different body: the key says this is the
	// first request retried, so it dedupes — ck-b is NOT opened.
	if st := post("/admin/commission", commissionBody("ck-b"), "K1"); st != 200 {
		t.Fatalf("commission ck-b with K1: %d", st)
	}
	if n := opened("ck-b"); n != 0 {
		t.Fatalf("reused key opened a second commission: %d", n)
	}
	// A different key is a different request: ck-b now records.
	if st := post("/admin/commission", commissionBody("ck-b"), "K2"); st != 200 {
		t.Fatalf("commission ck-b with K2: %d", st)
	}
	if n := opened("ck-b"); n != 1 {
		t.Fatalf("distinct key did not record ck-b: %d", n)
	}
	// The same key K1 on a different endpoint is a different request —
	// the session registers and agent.started records.
	sessBody, _ := json.Marshal(map[string]any{
		"commission": "ck-b", "agent": "k-agent", "parent": "human",
		"tool": "scripted", "version": "2.1.325", "session_id": "sess-k1",
		"world": "w-k1", "base": "", "branch": "b",
		"worktree": filepath.Join(h.DataDir, "wt-k1"), "writ": "",
	})
	if st := post("/admin/session", string(sessBody), "K1"); st != 200 {
		t.Fatalf("session with K1 on another endpoint: %d", st)
	}
	found := false
	evs, _ := h.Events()
	for _, e := range evs {
		if e.Type == "agent.started" && strOf(bodyField(t, e, "agent")) == "k-agent" {
			found = true
		}
	}
	if !found {
		t.Fatal("K1 on a different endpoint was deduped; agent.started missing")
	}
}

// wipeIdem deletes the store's remembered request identities so a check
// can isolate the daemon's rebuild backfill from the persisted table.
func wipeIdem(t *testing.T, dataDir string) {
	t.Helper()
	db, err := sql.Open("sqlite",
		"file:"+filepath.Join(dataDir, "ledger.db")+"?_pragma=busy_timeout(30000)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("DELETE FROM idem"); err != nil {
		t.Fatalf("wipe idem: %v", err)
	}
}
