//go:build windows

package daemon

import (
	"strconv"

	"golang.org/x/sys/windows"
)

// processStartID identifies this process by its creation time. A pid alone
// is not an identity: the Windows host is shared and recycles pids, so a
// recorded daemon pid can later name an unrelated process.
func processStartID() string {
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(windows.CurrentProcess(), &creation, &exit, &kernel, &user); err != nil {
		return ""
	}
	return strconv.FormatInt(creation.Nanoseconds(), 10)
}
