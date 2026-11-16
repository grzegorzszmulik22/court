package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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

// rpcIDCtxKey carries the JSON-RPC request id of the current /mcp post into
// the tool handler. The SDK puts the id in the handler context under an
// unexported key, so the daemon captures it from the wire body instead;
// streamable.go explicitly passes req.Context() to the session, which is
// what reaches the handler.
type rpcIDCtxKey struct{}

func rpcIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(rpcIDCtxKey{}).(string)
	return v
}

func (d *Daemon) mcpServer(sess *Session) *mcp.Server {
	d.mu.Lock()
	defer d.mu.Unlock()
	if s := d.mcpSrv[sess]; s != nil {
		return s
	}
	s := tools.New(tools.Deps{
		Actor: func(tool string) string { return d.actorForMCP(sess, tool) },
		Emit: func(ctx context.Context, actor, typ string, body any) error {
			var keys []string
			if id := rpcIDFrom(ctx); id != "" {
				// MCP retry identity: the session plus the JSON-RPC
				// request id (spec/02 §5). The raw id bytes go in the
				// key so a string "5" and a number 5 stay distinct.
				keys = append(keys, "mcp|"+sess.Token+"|"+id)
			}
			_, _, err := d.emit(sess.Commission, "agent:"+actor, typ, body, keys...)
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
// else goes to the SDK's streamable transport. The body is peeked at first
// to attach the JSON-RPC request id to the request context — the retry
// identity of a tool call.
func (d *Daemon) handleMCP(w http.ResponseWriter, r *http.Request) {
	if _, ok := d.sessionOf(r); !ok {
		d.refuse(w, r, "mcp", "unknown token", http.StatusNotFound)
		return
	}
	if r.Body != nil {
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<20))
		if err == nil {
			r.Body = io.NopCloser(bytes.NewReader(raw))
			var env struct {
				ID json.RawMessage `json:"id"`
			}
			if json.Unmarshal(raw, &env) == nil && len(env.ID) > 0 && string(env.ID) != "null" {
				r = r.WithContext(context.WithValue(r.Context(), rpcIDCtxKey{}, string(env.ID)))
			}
		} else {
			// Oversized or unreadable body: reset to what was read and let
			// the SDK produce the protocol error.
			r.Body = io.NopCloser(bytes.NewReader(raw))
		}
	}
	d.mcpHTTP.ServeHTTP(w, r)
}
