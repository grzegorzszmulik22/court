package daemon

import (
	"net/http"
)

// handleMCP serves POST /mcp/<token>. The real tools arrive in M1.6
// (internal/tools); for now the endpoint validates the token and answers
// protocol-level errors so a hooked agent cannot wedge on it.
func (d *Daemon) handleMCP(w http.ResponseWriter, r *http.Request) {
	if _, ok := d.sessionOf(r); !ok {
		d.refuse(w, r, "mcp", "unknown token", http.StatusNotFound)
		return
	}
	d.refuse(w, r, "mcp", "agent tools are not mounted yet", http.StatusNotImplemented)
}
