//go:build !windows

package daemon

// processStartID is empty away from Windows: the leak barrier there uses
// the pid, and a Linux daemon's /proc identity is not needed by any check.
func processStartID() string { return "" }
