//go:build windows

package commission

// A child started through detach must not inherit this process's standard
// handles: Windows copies every inheritable handle when a process is
// created with handle inheritance, and a daemon or agent that outlives its
// test would keep the test's output pipe open. This test makes the parent's
// stdout inheritable, runs the real detach, and requires the inheritance
// bit to be cleared — the condition under which no child can inherit it.

import (
	"os/exec"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procGetHandleInformation = windows.NewLazySystemDLL("kernel32.dll").
	NewProc("GetHandleInformation")

// handleInheritable reports the HANDLE_FLAG_INHERIT bit of h.
func handleInheritable(h windows.Handle) (bool, error) {
	var flags uint32
	r1, _, err := procGetHandleInformation.Call(uintptr(h), uintptr(unsafe.Pointer(&flags)))
	if r1 == 0 {
		return false, err
	}
	return flags&windows.HANDLE_FLAG_INHERIT != 0, nil
}

func TestDetachClearsStandardHandleInheritance(t *testing.T) {
	h := windows.Stdout
	if h == 0 {
		t.Skip("no standard output handle")
	}
	// Leave the handle cleared afterwards: that is detach's normal effect
	// and the safe state for later children.
	defer func() {
		if err := windows.SetHandleInformation(h, windows.HANDLE_FLAG_INHERIT, 0); err != nil {
			t.Errorf("clear stdout inheritance after the test: %v", err)
		}
	}()
	if err := windows.SetHandleInformation(h, windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT); err != nil {
		t.Fatalf("make stdout inheritable: %v", err)
	}
	inheritable, err := handleInheritable(h)
	if err != nil {
		t.Fatalf("read stdout inheritance: %v", err)
	}
	if !inheritable {
		t.Fatal("stdout could not be made inheritable")
	}

	cmd := exec.Command("cmd", "/c", "exit 0")
	if err := detach(cmd); err != nil {
		t.Fatalf("detach: %v", err)
	}
	inheritable, err = handleInheritable(h)
	if err != nil {
		t.Fatalf("read stdout inheritance after detach: %v", err)
	}
	if inheritable {
		t.Fatal("detach left the standard handle inheritable; a child would inherit it")
	}
}
