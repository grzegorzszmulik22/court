package scripted_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"court/internal/adapter/scripted"
	"court/internal/daemon"
)

// runConcurrent runs every agent's steps in parallel (the load run).
func runConcurrent(t *testing.T, h *scripted.Harness, sc *scripted.Script) {
	t.Helper()
	for _, a := range sc.Agents {
		if _, err := h.AddAgent(a); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	errs := make(chan error, len(sc.Agents))
	for _, a := range sc.Agents {
		wg.Add(1)
		go func(a *scripted.AgentSpec) {
			defer wg.Done()
			c, _ := h.Client(a.ID)
			if err := c.Run(a.Steps); err != nil {
				errs <- fmt.Errorf("%s: %w", a.ID, err)
			}
		}(a)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

// TestC1_8_1 (MM1.1): 8 scripted agents, >10,000 tool events and >500
// commits, randomly interleaved; every scripted step has exactly one
// ledger event, compared against the script.
func TestC1_8_1(t *testing.T) {
	h := newHarness(t)
	sc := &scripted.Script{}
	for i := 0; i < 8; i++ {
		a := &scripted.AgentSpec{ID: fmt.Sprintf("load%d", i), Parent: "human"}
		for j := 0; j < 63; j++ {
			// 20 tool calls then an edit+commit pair, repeated 63 times.
			for k := 0; k < 20; k++ {
				a.Steps = append(a.Steps, &scripted.Step{Tool: &scripted.ToolStep{
					Name:  "Read",
					Input: map[string]any{"file_path": fmt.Sprintf("f%02d.txt", (j+k)%64)},
				}})
			}
			a.Steps = append(a.Steps,
				&scripted.Step{Edit: &scripted.EditStep{
					Path:  fmt.Sprintf("f%02d.txt", j%64),
					Write: fmt.Sprintf("agent %d generation %d\n", i, j),
				}},
				&scripted.Step{Commit: &scripted.CommitStep{
					Message: fmt.Sprintf("a%d gen %d", i, j),
				}})
		}
		sc.Agents = append(sc.Agents, a)
	}
	runConcurrent(t, h, sc)

	var wantTool, wantCommit int
	for _, a := range sc.Agents {
		for _, st := range a.Steps {
			switch {
			case st.Commit != nil:
				wantCommit++
			default:
				wantTool++
			}
		}
	}
	var gotTool, gotCommit int
	evs, err := h.Events()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range evs {
		switch e.Type {
		case "tool.event":
			gotTool++
		case "world.commit":
			gotCommit++
		}
	}
	if gotTool != wantTool {
		t.Fatalf("tool.event = %d, script asked for %d", gotTool, wantTool)
	}
	if gotCommit != wantCommit {
		t.Fatalf("world.commit = %d, script asked for %d", gotCommit, wantCommit)
	}
	if gotTool < 10000 || gotCommit < 500 {
		t.Fatalf("load run too small: %d events, %d commits", gotTool, gotCommit)
	}
}

// TestC1_8_2 (MM1.4): paired hook round trip against a no-op endpoint,
// p99 ≤ 50 ms.
func TestC1_8_2(t *testing.T) {
	h := newHarness(t)
	c, err := h.AddAgent(&scripted.AgentSpec{ID: "lat", Parent: "human"})
	if err != nil {
		t.Fatal(err)
	}
	noop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(200)
	}))
	defer noop.Close()
	httpc := &http.Client{Timeout: 10 * time.Second}
	var diffs []time.Duration
	for i := 0; i < 150; i++ {
		payload, err := c.PayloadFor("PreToolUse", fmt.Sprintf("toolu_lat_%d", i),
			"Read", map[string]any{"file_path": "f.txt"}, "", "")
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(payload)
		t0 := time.Now()
		resp, err := httpc.Post(c.HookURL, "application/json", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		hookRT := time.Since(t0)
		t0 = time.Now()
		resp, err = httpc.Post(noop.URL, "application/json", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		diffs = append(diffs, hookRT-time.Since(t0))
	}
	sort.Slice(diffs, func(i, j int) bool { return diffs[i] < diffs[j] })
	p99 := diffs[int(0.99*float64(len(diffs)))]
	t.Logf("paired p99 = %v (median %v)", p99, diffs[len(diffs)/2])
	if p99 > 50*time.Millisecond {
		t.Fatalf("paired p99 %v > 50ms", p99)
	}
}

// TestC1_8_3: killing the daemon mid-run and restarting loses no
// acknowledged event; failed hook posts are retried by the client; no
// step is recorded twice. Also asserts append-before-ack: every
// successful post is already in the ledger when the response arrives.
func TestC1_8_3(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := scripted.InitRepo(repo, map[string]string{"f.txt": "0\n"}); err != nil {
		t.Fatal(err)
	}
	h, err := scripted.NewHarness(scripted.Options{
		Repo: repo, DataDir: filepath.Join(root, "data"),
		Commission: "c18", AppendDelay: 60 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.Close)
	if err := h.OpenCommission("kill test", nil); err != nil {
		t.Fatal(err)
	}
	c, err := h.AddAgent(&scripted.AgentSpec{ID: "survivor", Parent: "human"})
	if err != nil {
		t.Fatal(err)
	}
	c.Retries = 200 // retry through the restart gap
	// Mid-run kill: the step runner posts through the restart.
	steps := make([]*scripted.Step, 40)
	for i := range steps {
		steps[i] = &scripted.Step{Tool: &scripted.ToolStep{
			Name:  "Read",
			Input: map[string]any{"file_path": fmt.Sprintf("f%02d", i)},
		}}
	}
	var runErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		runErr = c.Run(steps)
	}()
	time.Sleep(300 * time.Millisecond)
	oldURL := h.Daemon.URL()
	h.Daemon.Close()
	d2, err := daemon.New(h.Store, daemon.Options{
		DataDir: h.DataDir, CourtBin: h.CourtBin,
		AppendDelay: 60 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if err := d2.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d2.Close)
	c.HookURL = strings.Replace(c.HookURL, oldURL, d2.URL(), 1)
	c.MCPURL = strings.Replace(c.MCPURL, oldURL, d2.URL(), 1)
	<-done
	if runErr != nil {
		t.Fatalf("run through restart: %v", runErr)
	}
	// 40 tool steps => 80 posts => exactly 40 tool.events, none twice.
	if n := len(toolEvents(t, h)); n != 40 {
		t.Fatalf("tool.event = %d, want 40 (lost acked post or duplicate)", n)
	}
	// Resend a byte-identical payload: dedupe must not record it again.
	p, err := c.PayloadFor("PostToolUse", "toolu_survivor_1", "Read",
		map[string]any{"file_path": "f00"}, "", "")
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
		t.Fatalf("repost after restart: %s", resp.Status)
	}
	if n := len(toolEvents(t, h)); n != 40 {
		t.Fatalf("duplicate after restart recorded: %d", n)
	}
	// Ack implies durable: after each successful post the event is in
	// the ledger — never an acknowledged-but-missing event.
	p2, err := c.PayloadFor("PostToolUse", "toolu_ackcheck", "Read",
		map[string]any{"file_path": "ack"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	raw2, _ := json.Marshal(p2)
	resp, err = http.Post(c.HookURL, "application/json", bytes.NewReader(raw2))
	if err != nil {
		t.Fatal(err)
	}
	// Do returns when the response headers arrive: the ack is on the
	// wire. The event must already be durable — before the body drains,
	// while a premature ack would still be inside the append delay.
	if resp.StatusCode != 200 {
		t.Fatalf("ack check post: %s", resp.Status)
	}
	found := false
	evs, _ := h.Events()
	for _, e := range evs {
		if e.Type == "tool.event" && strOf(bodyField(t, e, "tool_use_id")) == "toolu_ackcheck" {
			found = true
		}
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if !found {
		t.Fatalf("acknowledged post not durable at ack time")
	}
}

// TestC1_8_4: the M1 exit review answers both questions of spec/00 §5.
func TestC1_8_4(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(findRepoRoot(t), "docs", "M1-exit-review.md"))
	if err != nil {
		t.Fatalf("exit review missing: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "approval inbox") ||
		!strings.Contains(text, "ten minutes") {
		t.Fatal("exit review does not answer both north-star questions")
	}
	if !strings.Contains(text, "weakest") {
		t.Fatal("exit review does not name the weakest point")
	}
}
