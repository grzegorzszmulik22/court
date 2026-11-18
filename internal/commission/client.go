package commission

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"court/internal/daemon"
	"court/internal/ledger"
)

// Client talks to a running daemon's admin endpoint.
type Client struct {
	Base  string
	Token string
	HTTP  *http.Client
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (c *Client) post(path string, v any, into any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", c.Base+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("%s: %s: %s", path, resp.Status, string(bytes.TrimSpace(body)))
	}
	if into != nil {
		return json.Unmarshal(body, into)
	}
	return nil
}

func (c *Client) get(path string, into any) error {
	req, err := http.NewRequest("GET", c.Base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("%s: %s", path, resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(into)
}

// SessionReply is /admin/session's answer.
type SessionReply struct {
	Token   string `json:"token"`
	HookURL string `json:"hook_url"`
	MCPURL  string `json:"mcp_url"`
	GateURL string `json:"gate_url"`
}

// OpenCommission posts /admin/commission.
func (c *Client) OpenCommission(cm daemon.Commission) error {
	return c.post("/admin/commission", cm, nil)
}

// StartSession posts /admin/session.
func (c *Client) StartSession(sess daemon.Session) (*SessionReply, error) {
	var r SessionReply
	if err := c.post("/admin/session", sess, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// SubmitClerkBackbrief records the blind clerk's list as a
// backbrief.submitted event with actor "clerk" (M2.3).
func (c *Client) SubmitClerkBackbrief(commission string, bb *ledger.BackbriefSubmitted) error {
	return c.post("/admin/clerk", map[string]any{
		"commission": commission, "backbrief": bb}, nil)
}

// Rule records a human ruling on a decision (M2.5).
func (c *Client) Rule(commission, decision, option string) error {
	return c.post("/admin/rule", map[string]any{
		"commission": commission, "decision": decision, "option": option}, nil)
}

// IssueWrit asks the daemon to validate and record a writ for an agent:
// the root writ (parent "human") is checked against the commission's own
// authority, any other against the issuing agent's remaining authority.
func (c *Client) IssueWrit(commission string, w *ledger.WritIssued) (string, error) {
	var r struct {
		Writ string `json:"writ"`
	}
	if err := c.post("/admin/writ", map[string]any{
		"commission": commission, "writ": w}, &r); err != nil {
		return "", err
	}
	return r.Writ, nil
}

// Shutdown asks the daemon to stop.
func (c *Client) Shutdown() { _ = c.post("/admin/shutdown", map[string]any{}, nil) }

// --------------------------------------------------------------------------
// daemon.json discovery and on-demand startup
// --------------------------------------------------------------------------

// daemonFile mirrors the fields the daemon writes into
// <data-dir>/daemon.json at Start.
type daemonFile struct {
	URL        string `json:"url"`
	AdminToken string `json:"admin_token"`
	PID        int    `json:"pid"`
}

func readDaemonFile(dataDir string) (*daemonFile, error) {
	raw, err := os.ReadFile(filepath.Join(dataDir, "daemon.json"))
	if err != nil {
		return nil, err
	}
	var f daemonFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, err
	}
	if f.URL == "" || f.AdminToken == "" {
		return nil, fmt.Errorf("daemon.json is incomplete")
	}
	return &f, nil
}

// EnsureDaemon returns a client for the data directory's daemon, starting
// `court daemon` when none answers. A stale daemon.json (dead pid) is
// replaced by a fresh start.
func EnsureDaemon(courtBin, dataDir, repo string) (*Client, error) {
	if f, err := readDaemonFile(dataDir); err == nil {
		cl := &Client{Base: f.URL, Token: f.AdminToken}
		probe := &http.Client{Timeout: 2 * time.Second}
		old := cl.HTTP
		cl.HTTP = probe
		// Probe more than once before deciding the recorded daemon is dead:
		// on a loaded host one slow /admin/info reply must not start a
		// second daemon on the same data directory and leak the first.
		for attempt := 0; attempt < 5; attempt++ {
			var info map[string]any
			if err := cl.get("/admin/info", &info); err == nil {
				cl.HTTP = old
				return cl, nil
			}
			time.Sleep(50 * time.Millisecond)
		}
		cl.HTTP = old
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	args := []string{"daemon", "--data-dir", dataDir,
		"--ledger", filepath.Join(dataDir, "ledger.db")}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	logf, err := os.OpenFile(filepath.Join(dataDir, "daemon.log"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(courtBin, args...)
	cmd.Stdout, cmd.Stderr = logf, logf
	if err := detach(cmd); err != nil {
		logf.Close()
		return nil, fmt.Errorf("daemon detach: %w", err)
	}
	if err := cmd.Start(); err != nil {
		logf.Close()
		return nil, fmt.Errorf("daemon start: %w", err)
	}
	// The child is reparented; do not wait on it.
	go cmd.Wait()
	logf.Close()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if f, err := readDaemonFile(dataDir); err == nil {
			cl := &Client{Base: f.URL, Token: f.AdminToken}
			var info map[string]any
			if err := cl.get("/admin/info", &info); err == nil {
				return cl, nil
			}
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("daemon did not come up in %s", dataDir)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// Clerk posts the clerk's recorded output (M2.3).
func (c *Client) Clerk(cr daemon.ClerkResult) error {
	return c.post("/admin/clerk", cr, nil)
}

// Events returns the commission's ledger events in order.
func (c *Client) Events(commission string) ([]*ledger.Event, error) {
	var out struct {
		Events []*ledger.Event `json:"events"`
	}
	if err := c.get("/admin/events?commission="+commission, &out); err != nil {
		return nil, err
	}
	return out.Events, nil
}
