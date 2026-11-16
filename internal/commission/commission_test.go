package commission_test

// The M2.1 checks (spec/milestones/M2-commission-and-backbrief.md). Each
// test is driven by a thin wrapper under test/checks/M2.1/ that builds the
// court binary and exports COURT_BIN.

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"court/internal/adapter/scripted"
	"court/internal/ledger"
)

// fixture is one commissioned repository plus its data directory.
type fixture struct {
	t    *testing.T
	repo string
	data string
	bin  string
}

// newFixture builds a fresh git repository with the given files.
func newFixture(t *testing.T, files map[string]string) *fixture {
	t.Helper()
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := scripted.InitRepo(repo, files); err != nil {
		t.Fatal(err)
	}
	bin := os.Getenv("COURT_BIN")
	if bin == "" {
		t.Fatal("COURT_BIN is not set (check scripts build the binary)")
	}
	f := &fixture{t: t, repo: repo, data: filepath.Join(root, "data"), bin: bin}
	t.Cleanup(f.shutdown)
	return f
}

// shutdown stops the fixture's daemon, when one is running.
func (f *fixture) shutdown() {
	raw, err := os.ReadFile(filepath.Join(f.data, "daemon.json"))
	if err != nil {
		return
	}
	var df struct {
		URL   string `json:"url"`
		Token string `json:"admin_token"`
	}
	if json.Unmarshal(raw, &df) != nil || df.URL == "" {
		return
	}
	req, _ := http.NewRequest("POST", df.URL+"/admin/shutdown", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+df.Token)
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err == nil {
		resp.Body.Close()
	}
}

// writeScript renders a one-agent script.yaml for the orchestrator.
func (f *fixture) writeScript(steps string) string {
	f.t.Helper()
	p := filepath.Join(filepath.Dir(f.data), "script.yaml")
	content := "agents:\n  - id: orchestrator\n    parent: human\n    steps:\n" + steps
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
	return p
}

// commission runs `court commission` with the given args inside the repo.
// stdin is /dev/null when in is nil.
func (f *fixture) commission(in *os.File, args ...string) (string, error) {
	f.t.Helper()
	argv := append([]string{"commission"}, args...)
	cmd := exec.Command(f.bin, argv...)
	cmd.Dir = f.repo
	var out strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &out
	if in != nil {
		cmd.Stdin = in
	} else {
		cmd.Stdin = nil
	}
	err := cmd.Run()
	return out.String(), err
}

// events exports the fixture ledger through `court ledger export` (the
// daemon keeps the db open; export is the public read surface).
func (f *fixture) events() ([]*ledger.Event, error) {
	cmd := exec.Command(f.bin, "ledger", "export", filepath.Join(f.data, "ledger.db"))
	raw, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var out []*ledger.Event
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		ev, err := ledger.DecodeEnvelope([]byte(line))
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, nil
}

// waitFor polls the ledger until pred matches an event or the deadline
// passes; it returns the matching event.
func (f *fixture) waitFor(pred func(*ledger.Event) bool, d time.Duration) *ledger.Event {
	f.t.Helper()
	deadline := time.Now().Add(d)
	for {
		evs, err := f.events()
		if err == nil {
			for _, e := range evs {
				if pred(e) {
					return e
				}
			}
		}
		if time.Now().After(deadline) {
			f.t.Fatalf("timed out after %s waiting for an event", d)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// bodyStr reads one string member of an event body.
func bodyStr(e *ledger.Event, key string) string {
	for _, m := range e.Body.Obj {
		if m.Key == key {
			return m.Val.Str
		}
	}
	return ""
}

// bodyArr reads one string-array member of an event body.
func bodyArr(e *ledger.Event, key string) []string {
	for _, m := range e.Body.Obj {
		if m.Key == key {
			var out []string
			for _, it := range m.Val.Arr {
				out = append(out, it.Str)
			}
			return out
		}
	}
	return nil
}

// bodyNum reads one number member of an event body.
func bodyNum(e *ledger.Event, key string) int64 {
	for _, m := range e.Body.Obj {
		if m.Key == key {
			n, err := strconv.ParseInt(m.Val.Num, 10, 64)
			if err == nil {
				return n
			}
		}
	}
	return -1
}

// bodyObj reads one object member of an event body.
func bodyObj(e *ledger.Event, key string) *ledger.Value {
	for _, m := range e.Body.Obj {
		if m.Key == key {
			return m.Val
		}
	}
	return nil
}

func findEvent(evs []*ledger.Event, typ string) *ledger.Event {
	for _, e := range evs {
		if e.Type == typ {
			return e
		}
	}
	return nil
}

// TestC2_1_1: in a fresh fixture repository, `court commission "x"` with
// no commission-shaping flags records a commission, starts the daemon,
// creates a world and starts the scripted orchestrator.
func TestC2_1_1(t *testing.T) {
	f := newFixture(t, map[string]string{
		"go.mod":    "module fixture\n\ngo 1.27\n",
		"main.go":   "package main\n\nfunc main() {}\n",
		"README.md": "# fixture\n",
	})
	script := f.writeScript(`      - tool: {name: LS, path: "."}
      - tool: {name: Read, file_path: "README.md"}
`)
	out, err := f.commission(nil, "add a metrics endpoint",
		"--data-dir", f.data, "--script", script)
	if err != nil {
		t.Fatalf("court commission failed: %v\n%s", err, out)
	}
	for _, want := range []string{
		"commission c", "opened", "add a metrics endpoint",
		"done-check: go test ./...", "territory:",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output lacks %q:\n%s", want, out)
		}
	}
	// The daemon answers /admin/info (it is running, started on demand).
	f.waitFor(func(e *ledger.Event) bool {
		return e.Type == "commission.opened" && e.Actor == "human"
	}, 30*time.Second)
	raw, err := os.ReadFile(filepath.Join(f.data, "daemon.json"))
	if err != nil {
		t.Fatalf("daemon.json missing: %v", err)
	}
	var df struct {
		URL   string `json:"url"`
		Token string `json:"admin_token"`
		PID   int    `json:"pid"`
	}
	if err := json.Unmarshal(raw, &df); err != nil || df.URL == "" || df.PID == 0 {
		t.Fatalf("daemon.json malformed: %s %v", raw, err)
	}
	req, _ := http.NewRequest("GET", df.URL+"/admin/info", nil)
	req.Header.Set("Authorization", "Bearer "+df.Token)
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("daemon not answering /admin/info: %v", err)
	}
	resp.Body.Close()

	evs, err := f.events()
	if err != nil {
		t.Fatal(err)
	}
	co := findEvent(evs, "commission.opened")
	if co == nil {
		t.Fatal("no commission.opened")
	}
	if bodyStr(co, "goal") != "add a metrics endpoint" {
		t.Fatalf("goal = %q", bodyStr(co, "goal"))
	}
	if got := bodyArr(co, "done_checks"); len(got) != 1 || got[0] != "go test ./..." {
		t.Fatalf("done_checks = %v", got)
	}
	if got := bodyArr(co, "territory"); len(got) != 1 || got[0] != "**" {
		t.Fatalf("territory = %v, want [**]", got)
	}
	wc := findEvent(evs, "world.created")
	if wc == nil || !strings.HasPrefix(bodyStr(wc, "world"), "orchestrator-") {
		t.Fatalf("world.created missing or wrong world: %v", wc)
	}
	worldDir := filepath.Join(f.data, "worlds", bodyStr(wc, "world"))
	if st, err := os.Stat(worldDir); err != nil || !st.IsDir() {
		t.Fatalf("orchestrator world not on disk: %v", err)
	}
	as := findEvent(evs, "agent.started")
	if as == nil || bodyStr(as, "agent") != "orchestrator" {
		t.Fatalf("agent.started missing: %v", as)
	}
	if bodyStr(as, "tool") != "scripted" {
		t.Fatalf("orchestrator tool = %q, want scripted", bodyStr(as, "tool"))
	}
	if bodyStr(as, "world") != bodyStr(wc, "world") {
		t.Fatalf("agent world %q != world.created %q",
			bodyStr(as, "world"), bodyStr(wc, "world"))
	}
	// The scripted orchestrator is running: its first tool.event lands.
	f.waitFor(func(e *ledger.Event) bool {
		return e.Type == "tool.event" && e.Actor == "agent:orchestrator"
	}, 30*time.Second)
}

// TestC2_1_3: with no detectable test command the commission starts with
// an empty done-check list and says done-claims cannot be checked.
func TestC2_1_3(t *testing.T) {
	f := newFixture(t, map[string]string{
		"README.md": "# fixture\n",
		"notes.txt": "no build files at all\n",
	})
	script := f.writeScript(`      - tool: {name: LS, path: "."}
`)
	out, err := f.commission(nil, "tidy the readme",
		"--data-dir", f.data, "--script", script)
	if err != nil {
		t.Fatalf("court commission failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "done-claims cannot be checked") {
		t.Fatalf("output lacks the cannot-be-checked line:\n%s", out)
	}
	f.waitFor(func(e *ledger.Event) bool { return e.Type == "commission.opened" },
		30*time.Second)
	evs, _ := f.events()
	co := findEvent(evs, "commission.opened")
	if co == nil || len(bodyArr(co, "done_checks")) != 0 {
		t.Fatalf("done_checks = %v, want empty", bodyArr(co, "done_checks"))
	}
	// The commission still starts work.
	f.waitFor(func(e *ledger.Event) bool {
		return e.Type == "tool.event" && e.Actor == "agent:orchestrator"
	}, 30*time.Second)
}

// TestC2_1_5: every default used is listed in commission.opened.defaults,
// and a supplied flag removes its name.
func TestC2_1_5(t *testing.T) {
	f := newFixture(t, map[string]string{
		"go.mod": "module fixture\n", "a.txt": "x\n",
	})
	script := f.writeScript(`      - tool: {name: LS, path: "."}
`)
	if _, err := f.commission(nil, "first goal",
		"--data-dir", f.data, "--script", script); err != nil {
		t.Fatal(err)
	}
	f.waitFor(func(e *ledger.Event) bool { return e.Type == "commission.opened" },
		30*time.Second)
	evs, _ := f.events()
	co := findEvent(evs, "commission.opened")
	def := map[string]bool{}
	for _, d := range bodyArr(co, "defaults") {
		def[d] = true
	}
	for _, want := range []string{
		"done-checks", "territory", "tokens", "minutes", "speculation", "attention",
	} {
		if !def[want] {
			t.Fatalf("defaults lacks %q: %v", want, bodyArr(co, "defaults"))
		}
	}
	// Explicit flags: every supplied name leaves the defaults list;
	// "speculation" has no flag and stays a default.
	f2 := newFixture(t, map[string]string{"a.txt": "x\n"})
	script2 := f2.writeScript(`      - tool: {name: LS, path: "."}
`)
	if _, err := f2.commission(nil, "second goal",
		"--data-dir", f2.data, "--script", script2,
		"--done", "make test", "--territory", "src/**",
		"--tokens", "5", "--minutes", "2", "--attention", "10"); err != nil {
		t.Fatal(err)
	}
	f2.waitFor(func(e *ledger.Event) bool { return e.Type == "commission.opened" },
		30*time.Second)
	evs, _ = f2.events()
	co = findEvent(evs, "commission.opened")
	def = map[string]bool{}
	for _, d := range bodyArr(co, "defaults") {
		def[d] = true
	}
	if len(bodyArr(co, "defaults")) != 1 || !def["speculation"] {
		t.Fatalf("defaults = %v, want [speculation]", bodyArr(co, "defaults"))
	}
	if got := bodyArr(co, "done_checks"); len(got) != 1 || got[0] != "make test" {
		t.Fatalf("done_checks = %v", got)
	}
	if got := bodyArr(co, "territory"); len(got) != 1 || got[0] != "src/**" {
		t.Fatalf("territory = %v", got)
	}
	bud := bodyObj(co, "budgets")
	get := func(k string) int64 {
		for _, m := range bud.Obj {
			if m.Key == k {
				n, err := strconv.ParseInt(m.Val.Num, 10, 64)
				if err == nil {
					return n
				}
			}
		}
		return -1
	}
	if get("tokens") != 5 || get("minutes") != 2 || get("speculation") != 2000 {
		t.Fatalf("budgets wrong: tokens=%d minutes=%d speculation=%d",
			get("tokens"), get("minutes"), get("speculation"))
	}
	if bodyNum(co, "attention") != 10 {
		t.Fatalf("attention = %d, want 10", bodyNum(co, "attention"))
	}
}

// TestC2_1_2: MM2.3 (commission to first agent step <= 3s) over >=5 runs
// and MM2.4 (at most one keystroke; a non-terminal stdin needs zero).
// Raw samples land in $COURT_CHECK_DIR/measurements.json.
func TestC2_1_2(t *testing.T) {
	const runs = 5
	var samples []float64
	for i := 0; i < runs; i++ {
		f := newFixture(t, map[string]string{
			"go.mod": "module fixture\n", "x.txt": "x\n",
		})
		script := f.writeScript(`      - tool: {name: LS, path: "."}
`)
		t0 := time.Now()
		out, err := f.commission(nil, fmt.Sprintf("measure run %d", i),
			"--data-dir", f.data, "--script", script)
		if err != nil {
			t.Fatalf("run %d: commission failed: %v\n%s", i, err, out)
		}
		f.waitFor(func(e *ledger.Event) bool {
			return e.Type == "tool.event" && e.Actor == "agent:orchestrator"
		}, 30*time.Second)
		samples = append(samples, time.Since(t0).Seconds())
		f.shutdown()
	}
	sorted := append([]float64(nil), samples...)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j] < sorted[i] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	median := sorted[len(sorted)/2]
	var mean, sd float64
	for _, s := range samples {
		mean += s
	}
	mean /= float64(len(samples))
	for _, s := range samples {
		sd += (s - mean) * (s - mean)
	}
	cv := 0.0
	if mean > 0 {
		cv = math.Sqrt(sd/float64(len(samples))) / mean
	}
	if median > 3.0 {
		t.Fatalf("MM2.3 median %.3fs exceeds 3s (samples %v)", median, samples)
	}
	dir := os.Getenv("COURT_CHECK_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	rec := map[string]any{
		"measurement":       "MM2.3 commission-to-first-agent-step / MM2.4 keystrokes",
		"command":           `court commission <goal> --data-dir D --script s.yaml`,
		"machine":           os.Getenv("COURT_SYSINFO"),
		"runs":              runs,
		"samples_s":         samples,
		"median_s":          median,
		"cv":                cv,
		"target_s":          3.0,
		"keystrokes":        0,
		"keystrokes_target": 1,
	}
	raw, _ := json.MarshalIndent(rec, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "measurements.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}
