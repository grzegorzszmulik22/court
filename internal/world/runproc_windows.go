//go:build windows

package world

// Run's time limit must be a wall-clock bound (red-team B3-07): a command
// that launches a detached descendant inheriting its output must not keep
// Run waiting, and the descendant must die. Two mechanisms:
//
//   - stdout/stderr go to temp files, not io.Pipe, so os/exec has no copy
//     goroutines to wait on; cmd.Wait returns when the direct child exits.
//   - the child is created suspended, assigned to a job object with
//     JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE, then resumed. Every descendant,
//     including one `cmd /c start /b` detaches, stays in the job; closing
//     the job (or TerminateJobObject on timeout) kills the whole tree.

import (
	"fmt"
	"os/exec"
	"strconv"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// procAttr creates the child suspended so it can be placed in a job before
// it can spawn anything.
func procAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED}
}

type procTree struct {
	cmd   *exec.Cmd
	job   windows.Handle
	token string
}

var ntResumeProcess = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtResumeProcess")

func startProc(cmd *exec.Cmd) (*procTree, error) {
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	pt := &procTree{cmd: cmd}
	h, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_SUSPEND_RESUME,
		false, uint32(cmd.Process.Pid))
	if err != nil {
		// The child is suspended; without a process handle it cannot be
		// resumed, so fail the run instead of hanging it.
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("OpenProcess: %w", err)
	}
	defer windows.CloseHandle(h)
	if job, err := windows.CreateJobObject(nil, nil); err == nil {
		var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
		info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		if _, err := windows.SetInformationJobObject(job,
			windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err == nil {
			if err := windows.AssignProcessToJobObject(job, h); err == nil {
				pt.job = job
			} else {
				windows.CloseHandle(job)
			}
		} else {
			windows.CloseHandle(job)
		}
	}
	if status, _, _ := ntResumeProcess.Call(uintptr(h)); status != 0 {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("NtResumeProcess: status %#x", status)
	}
	return pt, nil
}

// kill ends the whole job. Without a job (creation failed) it falls back to
// taskkill's tree walk and the direct kill.
func (p *procTree) kill() {
	if p.job != 0 {
		_ = windows.TerminateJobObject(p.job, 1)
	}
	_ = p.cmd.Process.Kill()
	_ = exec.Command("taskkill", "/T", "/F", "/PID",
		strconv.Itoa(p.cmd.Process.Pid)).Run()
}

// close releases the job. KILL_ON_JOB_CLOSE kills any descendant the
// command left behind, even when the command itself exited successfully.
func (p *procTree) close() {
	if p.job != 0 {
		windows.CloseHandle(p.job)
		p.job = 0
	}
}
