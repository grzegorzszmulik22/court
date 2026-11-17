package scripted

// Red-team reproduction: an Edit step's path is joined to the world and
// written without any containment check. The policy decision comes from the
// daemon's PreToolUse answer; the default policy abstains for any path
// outside the data directory, so `..` leaves the world.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func rtRoot(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(file)))), ".rt", "scratch", "scripted")
}

func TestRT_EditEscapesWorld(t *testing.T) {
	base := rtRoot(t)
	os.RemoveAll(base)
	worktree := filepath.Join(base, "data", "worlds", "w1")
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c := &Client{
		HookURL: srv.URL, MCPURL: srv.URL, Worktree: worktree,
		SessionID: "s1", Agent: "a1",
	}
	step := &Step{Edit: &EditStep{
		Path:  filepath.Join("..", "..", "..", "outside", "escape.txt"),
		Write: "CANARY-ESCAPED\n",
	}}
	if err := c.RunStep(step); err != nil {
		t.Fatalf("step: %v", err)
	}
	victim := filepath.Join(outside, "escape.txt")
	data, err := os.ReadFile(victim)
	if err != nil {
		t.Skipf("path did not land outside (join semantics): %v", err)
	}
	if !strings.Contains(string(data), "CANARY-ESCAPED") {
		t.Fatalf("unexpected content %q", data)
	}
	t.Fatalf("scripted Edit wrote outside the world: %s", victim)
}
