package ledger

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Shared test helpers. The check scripts drive these tests:
//   C1.2.2 TestChain10000, C1.2.3 TestConcurrentAppend,
//   C1.2.4 TestCrashRecovery, C1.2.5 round-trip/strict tests,
//   C1.2.6 TestAppendThroughput.

func checkDir(t *testing.T) string {
	// `go test ... -args <dir>`: the scratch dir is the last positional arg.
	if a := os.Args[len(os.Args)-1]; a != "" && a[0] != '-' {
		return a
	}
	return t.TempDir()
}

func toolBody(i int) *ToolEvent {
	return &ToolEvent{
		Tool: "Read", ToolUseID: fmt.Sprintf("tu_%08d", i),
		InputDigest: fmt.Sprintf("%064x", i+1),
		Fields:      ToolFields{Paths: []string{"a.go"}, Command: "cat a.go", Hosts: []string{}},
		Outcome:     "ok",
	}
}

func appendN(s *Store, from, n int) error {
	for i := 0; i < n; i++ {
		if _, err := s.Append("C1", "agent:a1", "tool.event", toolBody(from+i)); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Worker re-exec: the tests spawn the test binary itself with
// COURT_LEDGER_WORKER set, so appends happen in a separate process.
// ---------------------------------------------------------------------------

func workerEnv() []string {
	return append(os.Environ(), "COURT_LEDGER_WORKER=1")
}

func TestMain(m *testing.M) {
	if os.Getenv("COURT_LEDGER_WORKER") != "" {
		os.Exit(workerMain())
	}
	os.Exit(m.Run())
}

// workerMain performs appends in a child process. Env:
//
//	COURT_TEST_DB    ledger path
//	COURT_TEST_N     number of events to append
//	COURT_TEST_FROM  first event index (distinguishes the two processes)
//	COURT_LEDGER_FAULT  optional "point[:seq]" injection
func workerMain() int {
	path := os.Getenv("COURT_TEST_DB")
	var n, from int
	fmt.Sscanf(os.Getenv("COURT_TEST_N"), "%d", &n)
	fmt.Sscanf(os.Getenv("COURT_TEST_FROM"), "%d", &from)
	s, err := Open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "worker open:", err)
		return 2
	}
	defer s.Close()
	for i := 0; i < n; i++ {
		if _, err := s.Append("C1", "agent:a1", "tool.event", toolBody(from+i)); err != nil {
			fmt.Fprintln(os.Stderr, "worker append:", err)
			return 3
		}
	}
	return 0
}

func spawnWorker(t *testing.T, db string, env ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=NONE")
	cmd.Env = append(workerEnv(), "COURT_TEST_DB="+db)
	cmd.Env = append(cmd.Env, env...)
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	return cmd
}

// ---------------------------------------------------------------------------
// C1.2.2: 10,000-event chain verifies; every field feeds the hash.
// ---------------------------------------------------------------------------

func TestChain10000(t *testing.T) {
	dir := checkDir(t)
	db := filepath.Join(dir, "chain10k.sqlite")
	s, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := appendN(s, 0, 10000); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if err := LoadSchemas(""); err != nil {
		t.Fatal(err)
	}
	v := VerifyFile(db)
	if v.Class != ClassOK {
		t.Fatalf("verify: %+v", v)
	}
	if v.Events != 10000 {
		t.Fatalf("verified %d events, want 10000", v.Events)
	}
}

// TestHashCoversEveryField mutates each envelope field in turn and requires
// the recomputed hash to change (the "prev-hashes-body-only" sabotage fails
// here: a hash over the body alone misses seq, prev, at, commission, actor,
// type).
func TestHashCoversEveryField(t *testing.T) {
	body, err := BodyFrom(toolBody(0))
	if err != nil {
		t.Fatal(err)
	}
	base := &Event{Seq: 7, Prev: ZeroPrev, At: "2026-11-15T00:00:00.000Z",
		Commission: "C1", Actor: "agent:a1", Type: "tool.event", Body: body}
	h0, err := base.ComputeHash()
	if err != nil {
		t.Fatal(err)
	}
	muts := []*Event{
		{Seq: 8, Prev: base.Prev, At: base.At, Commission: base.Commission, Actor: base.Actor, Type: base.Type, Body: body},
		{Seq: base.Seq, Prev: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", At: base.At, Commission: base.Commission, Actor: base.Actor, Type: base.Type, Body: body},
		{Seq: base.Seq, Prev: base.Prev, At: "2026-11-15T00:00:01.000Z", Commission: base.Commission, Actor: base.Actor, Type: base.Type, Body: body},
		{Seq: base.Seq, Prev: base.Prev, At: base.At, Commission: "C2", Actor: base.Actor, Type: base.Type, Body: body},
		{Seq: base.Seq, Prev: base.Prev, At: base.At, Commission: base.Commission, Actor: "agent:a2", Type: base.Type, Body: body},
		{Seq: base.Seq, Prev: base.Prev, At: base.At, Commission: base.Commission, Actor: base.Actor, Type: "alarm.raised", Body: body},
	}
	names := []string{"seq", "prev", "at", "commission", "actor", "type"}
	for i, m := range muts {
		h, err := m.ComputeHash()
		if err != nil {
			t.Fatal(err)
		}
		if h == h0 {
			t.Fatalf("hash unchanged after mutating %s", names[i])
		}
	}
	// A body mutation must also move the hash.
	b2, err := BodyFrom(toolBody(1))
	if err != nil {
		t.Fatal(err)
	}
	m := &Event{Seq: base.Seq, Prev: base.Prev, At: base.At,
		Commission: base.Commission, Actor: base.Actor, Type: base.Type, Body: b2}
	h, err := m.ComputeHash()
	if err != nil {
		t.Fatal(err)
	}
	if h == h0 {
		t.Fatal("hash unchanged after mutating body")
	}
}

// ---------------------------------------------------------------------------
// C1.2.3: two processes append at once; seq is unique, chain holds.
// ---------------------------------------------------------------------------

func TestConcurrentAppend(t *testing.T) {
	dir := checkDir(t)
	db := filepath.Join(dir, "concurrent.sqlite")
	s, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	s.Close() // schema initialized; workers reopen.

	const per = 150
	c1 := spawnWorker(t, db, "COURT_TEST_N=150", "COURT_TEST_FROM=0")
	c2 := spawnWorker(t, db, "COURT_TEST_N=150", "COURT_TEST_FROM=100000")
	if err := c1.Wait(); err != nil {
		t.Fatalf("worker 1: %v", err)
	}
	if err := c2.Wait(); err != nil {
		t.Fatalf("worker 2: %v", err)
	}
	if err := LoadSchemas(""); err != nil {
		t.Fatal(err)
	}
	v := VerifyFile(db)
	if v.Class != ClassOK {
		t.Fatalf("verify after concurrent append: %+v", v)
	}
	if v.Events != 2*per {
		t.Fatalf("got %d events, want %d: an append was lost", v.Events, 2*per)
	}
	// Seq must be gap-free (implicit in verify) and each body distinct.
	lines, _ := ReadLedgerFile(db)
	seen := map[string]bool{}
	for _, l := range lines {
		ev, _ := DecodeEnvelope(l)
		id, _ := objGet(ev.Body, "tool_use_id")
		if seen[strOf(id)] {
			t.Fatalf("duplicate tool_use_id %q: an append overwrote another", strOf(id))
		}
		seen[strOf(id)] = true
	}
}

// ---------------------------------------------------------------------------
// C1.2.4: crash between write and commit at 200 injection points.
// ---------------------------------------------------------------------------

func TestCrashRecovery(t *testing.T) {
	dir := checkDir(t)
	db := filepath.Join(dir, "crash.sqlite")
	s, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if err := LoadSchemas(""); err != nil {
		t.Fatal(err)
	}
	const iterations = 200
	for i := 0; i < iterations; i++ {
		point := FaultPoints[i%len(FaultPoints)]
		fault := fmt.Sprintf("%s:%d", point, i)
		cmd := exec.Command(os.Args[0], "-test.run=NONE")
		cmd.Env = append(workerEnv(),
			"COURT_TEST_DB="+db,
			"COURT_TEST_N=1", "COURT_TEST_FROM=0",
			"COURT_LEDGER_FAULT="+fault)
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err == nil {
			t.Fatalf("iteration %d (fault %s): worker did not crash", i, fault)
		}
		// Recover: the worker's single append crashed mid-flight, so the
		// committed head is still i-1 and the chain must verify.
		v := VerifyFile(db)
		if v.Class == ClassEmpty && i == 0 {
			// crash before the first insert: empty ledger is legitimate
		} else if v.Class != ClassOK {
			t.Fatalf("iteration %d (fault %s): verify = %+v", i, fault, v)
		}
		s, err := Open(db)
		if err != nil {
			t.Fatalf("iteration %d: reopen: %v", i, err)
		}
		head, h, err := s.Head()
		if err != nil {
			t.Fatal(err)
		}
		if head != int64(i-1) {
			t.Fatalf("iteration %d (fault %s): head seq %d, want %d (hash %s)",
				i, fault, head, i-1, h)
		}
		// Retry the append that crashed: it must get the next seq, never
		// reusing the crashed attempt's number.
		ev, err := s.Append("C1", "agent:a1", "tool.event", toolBody(i))
		if err != nil {
			t.Fatalf("iteration %d: retry append: %v", i, err)
		}
		if ev.Seq != int64(i) {
			t.Fatalf("iteration %d: retried append got seq %d, want %d", i, ev.Seq, i)
		}
		s.Close()
	}
	v := VerifyFile(db)
	if v.Class != ClassOK {
		t.Fatalf("final verify: %+v", v)
	}
}

// ---------------------------------------------------------------------------
// C1.2.6: MM1.5 single-writer throughput >= 2000 events/s.
// ---------------------------------------------------------------------------

func TestAppendThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip("MM1.5 throughput, C1.2.6: a timed measurement; make check-M1.2 runs it in full, make regress runs go test -short")
	}
	dir := checkDir(t)
	db := filepath.Join(dir, "throughput.sqlite")
	const runs, n = 5, 2000
	var samples []float64
	for r := 0; r < runs; r++ {
		s, err := Open(db)
		if err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		if err := appendN(s, r*n, n); err != nil {
			t.Fatal(err)
		}
		el := time.Since(start)
		s.Close()
		samples = append(samples, float64(n)/el.Seconds())
	}
	med := median(samples)
	cv := cvOf(samples)
	writeMeasurements(t, map[string]any{
		"command":     "append 2000 tool.event rows via Store.Append",
		"measurement": "MM1.5 single-writer ledger append throughput",
		"unit":        "events/s",
		"runs":        runs,
		"samples":     samples,
		"median":      med,
		"cv":          cv,
		"threshold":   2000.0,
	})
	if med < 2000 {
		t.Fatalf("median throughput %.0f events/s < 2000 (samples %v)", med, samples)
	}
}

func median(xs []float64) float64 {
	ys := append([]float64(nil), xs...)
	for i := 0; i < len(ys); i++ {
		for j := i + 1; j < len(ys); j++ {
			if ys[j] < ys[i] {
				ys[i], ys[j] = ys[j], ys[i]
			}
		}
	}
	return ys[len(ys)/2]
}

func cvOf(xs []float64) float64 {
	var m float64
	for _, x := range xs {
		m += x
	}
	m /= float64(len(xs))
	var v float64
	for _, x := range xs {
		v += (x - m) * (x - m)
	}
	v /= float64(len(xs))
	if m == 0 {
		return 0
	}
	return math.Sqrt(v) / m
}

// writeMeasurements appends the measurement record to COURT_MEASUREMENTS
// (JSON array) when the check script asks for it.
func writeMeasurements(t *testing.T, rec map[string]any) {
	out := os.Getenv("COURT_MEASUREMENTS")
	if out == "" {
		return
	}
	rec["machine"] = machineDesc()
	var all []any
	if b, err := os.ReadFile(out); err == nil && len(b) > 0 {
		_ = json.Unmarshal(b, &all)
	}
	all = append(all, rec)
	b, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, b, 0o644); err != nil {
		t.Fatalf("write measurements: %v", err)
	}
}

func machineDesc() map[string]any {
	d := map[string]any{}
	if b, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			k, v, ok := strings.Cut(l, ":")
			if ok && (strings.TrimSpace(k) == "model name" ||
				strings.TrimSpace(k) == "Model") {
				d["cpu"] = strings.TrimSpace(v)
				break
			}
		}
	}
	if u, err := exec.Command("uname", "-smr").Output(); err == nil {
		d["uname"] = strings.TrimSpace(string(u))
	}
	d["go"] = runtime.Version()
	return d
}
