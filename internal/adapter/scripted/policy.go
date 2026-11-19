package scripted

import (
	"path/filepath"

	"court/internal/daemon"
	"court/internal/paths"
)

// WorldPolicy wraps the session's deny policy so a file-writing tool call
// whose path resolves outside the session's world is denied before the
// inner policy sees it. B3-09: an abstaining policy is not permission —
// writes stay inside the world whether or not the inner policy says so.
// The daemon records the denied tool.event with this reason.
type WorldPolicy struct{ Inner daemon.DenyPolicy }

// writeTools carry a path argument that writes or rewrites files.
var writeTools = map[string]bool{
	"Write": true, "Edit": true, "MultiEdit": true, "NotebookEdit": true,
}

// editPathKeys are the tool_input members that hold the target file.
var editPathKeys = []string{"file_path", "path", "notebook_path"}

func (w WorldPolicy) DecideHook(toolName string, toolInput map[string]any,
	worldDir string) (string, string) {
	if writeTools[toolName] && worldDir != "" {
		for _, k := range editPathKeys {
			v, ok := toolInput[k].(string)
			if !ok || v == "" {
				continue
			}
			if inside, _ := paths.WithinDir(worldDir, v); !inside {
				return "deny", "path is outside the world"
			}
		}
	}
	if w.Inner != nil {
		return w.Inner.DecideHook(toolName, toolInput, worldDir)
	}
	return "", ""
}

// editConfineReason is the adapter's own check of an Edit step's path,
// independent of any hook endpoint's answer (B3-09): an absolute step
// path is refused outright; anything else must resolve inside the
// worktree — `..` and symlinks that leave the world fail here.
func editConfineReason(worktree, p string) string {
	if filepath.IsAbs(p) {
		return "absolute path is outside the world"
	}
	if ok, reason := paths.WithinDir(worktree, p); !ok {
		return reason + ": outside the world"
	}
	return ""
}
