// Package adapter defines the agent-tool adapter contract of
// spec/02-architecture.md §4. An adapter starts an agent session inside a
// world, sends follow-up input, and stops it. Tool events never travel
// through the adapter: they reach the daemon's hook endpoint directly, so
// every adapter is observed the same way.
package adapter

// Session carries what Court hands to one agent session.
type Session struct {
	Agent      string // agent id
	Parent     string // parent agent id, or "human"
	Commission string
	Token      string // session token; the path identity on hook and MCP URLs
	World      string // world id
	Writ       string // writ id
	HookURL    string // daemon hook endpoint for this session
	MCPURL     string // daemon MCP endpoint for this session
	DataDir    string // Court data directory (generated files live here)
	Tool       string // agent tool name, recorded on agent.started
	Version    string // tool version, recorded on agent.started
}

// Handle is a running agent session returned by Start. It is opaque to
// Court: each adapter keeps the process handles it needs inside.
type Handle interface {
	// Done reports whether the session's process has exited.
	Done() bool
}

// Adapter is the contract of spec/02 §4.
type Adapter interface {
	// Start begins a session in w with the given prompt. s carries the
	// token, writ and the endpoint URLs the tool configuration needs.
	Start(w World, prompt string, s Session) (Handle, error)
	// Send delivers a follow-up message to a running session.
	Send(h Handle, message string) error
	// Stop ends the session cleanly.
	Stop(h Handle) error
}

// World is the slice of internal/world.World an adapter needs.
type World interface {
	Path() string
}
