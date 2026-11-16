package daemon

import (
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"court/internal/tools"
)

// newMCPHandler builds the one Streamable-HTTP handler for all
// /mcp/<token> posts; the per-request server lookup is what binds a call
// to its session. Stateless: a client speaking the pinned protocol
// revision needs no session header (C1.6.4).
func (d *Daemon) newMCPHandler() http.Handler {
	return mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		sess, ok := d.sessionOf(r)
		if !ok {
			return nil
		}
		return d.mcpServer(sess)
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
}

func (d *Daemon) mcpServer(sess *Session) *mcp.Server {
	d.mu.Lock()
	defer d.mu.Unlock()
	if s := d.mcpSrv[sess]; s != nil {
		return s
	}
	s := tools.New(tools.Deps{
		Actor: func(tool string) string { return d.actorForMCP(sess, tool) },
		Emit: func(actor, typ string, body any) error {
			_, _, err := d.emit(sess.Commission, "agent:"+actor, typ, body)
			return err
		},
	})
	d.mcpSrv[sess] = s
	return s
}

// actorForMCP pairs a tool call with its parked PreToolUse: a subagent's
// MCP call arrives on the session's own endpoint, but the hook already
// told us which agent is calling. The alarm path is identical — the call
// goes from agent to daemon directly, so a parent that drops the message
// cannot intercept it (spec/02 §5).
func (d *Daemon) actorForMCP(sess *Session, tool string) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	list := d.pending[sess.Token]
	for i, e := range list {
		if e.tool == tool || strings.HasSuffix(e.tool, "__"+tool) {
			d.pending[sess.Token] = append(list[:i], list[i+1:]...)
			return e.actor
		}
	}
	return sess.Agent
}

// handleMCP serves /mcp/<token>: unknown tokens are refused, everything
// else goes to the SDK's streamable transport.
func (d *Daemon) handleMCP(w http.ResponseWriter, r *http.Request) {
	if _, ok := d.sessionOf(r); !ok {
		d.refuse(w, r, "mcp", "unknown token", http.StatusNotFound)
		return
	}
	d.mcpHTTP.ServeHTTP(w, r)
}
