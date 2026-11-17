//go:build windows

package commission_test

// The C2.5.x hearing checks need a real terminal for standard input:
// isatty must see a console, not a pipe. Git Bash has no `script`, so
// Windows uses a pseudo-console (ConPTY, Windows 10 1809+) created here
// with the kernel32 primitives x/sys/windows exposes. The property
// proven is the same as on Linux: the hearing appears only on a
// terminal, asks in the specified order and respects the cap.

import (
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// commissionPTY runs `court commission` with stdin on a pseudo-console
// fed the answer lines in answers (the Windows form of the Linux `script`
// runner).
func (f *fixture) commissionPTY(answers string, args ...string) (string, error) {
	f.t.Helper()
	return runOnPTY(f.repo, f.bin, append([]string{"commission"}, args...), answers)
}

// runOnPTY runs bin with args in dir under a ConPTY, feeding answers to
// its console and returning everything the console rendered.
func runOnPTY(dir, bin string, args []string, answers string) (string, error) {
	// CreatePipe gives synchronous handles, which is what the pseudoconsole
	// channels require (os.Pipe's overlapped handles make conhost's reads
	// fail). The conhost side duplicates them at CreatePseudoConsole.
	var inR, inW, outR, outW windows.Handle
	if err := windows.CreatePipe(&inR, &inW, nil, 0); err != nil {
		return "", fmt.Errorf("input pipe: %w", err)
	}
	if err := windows.CreatePipe(&outR, &outW, nil, 0); err != nil {
		windows.CloseHandle(inR)
		windows.CloseHandle(inW)
		return "", fmt.Errorf("output pipe: %w", err)
	}
	var hpc windows.Handle
	if err := windows.CreatePseudoConsole(
		windows.Coord{X: 120, Y: 30}, inR, outW, 0, &hpc); err != nil {
		for _, h := range []windows.Handle{inR, inW, outR, outW} {
			windows.CloseHandle(h)
		}
		return "", fmt.Errorf("CreatePseudoConsole: %w", err)
	}
	// conhost holds duplicates; ours would only keep the output channel
	// from reaching EOF when the console closes.
	windows.CloseHandle(inR)
	windows.CloseHandle(outW)
	closePC := true
	defer func() {
		if closePC {
			windows.ClosePseudoConsole(hpc)
		}
	}()

	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		windows.CloseHandle(inW)
		windows.CloseHandle(outR)
		return "", fmt.Errorf("attribute list: %w", err)
	}
	defer attrs.Delete()
	// PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE takes the HPCON value itself.
	// Reading the handle as a pointer keeps the value identical while
	// avoiding a uintptr->unsafe.Pointer conversion (which go vet
	// reports as a possible misuse).
	pcValue := *(*unsafe.Pointer)(unsafe.Pointer(&hpc))
	if err := attrs.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
		pcValue, unsafe.Sizeof(hpc)); err != nil {
		windows.CloseHandle(inW)
		windows.CloseHandle(outR)
		return "", fmt.Errorf("pseudoconsole attribute: %w", err)
	}

	argv := append([]string{bin}, args...)
	cmdline, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(argv))
	if err != nil {
		windows.CloseHandle(inW)
		windows.CloseHandle(outR)
		return "", err
	}
	exe, err := windows.UTF16PtrFromString(bin)
	if err != nil {
		windows.CloseHandle(inW)
		windows.CloseHandle(outR)
		return "", err
	}
	dirp, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		windows.CloseHandle(inW)
		windows.CloseHandle(outR)
		return "", err
	}
	si := &windows.StartupInfoEx{
		StartupInfo: windows.StartupInfo{
			Cb: uint32(unsafe.Sizeof(windows.StartupInfoEx{})),
		},
		ProcThreadAttributeList: attrs.List(),
	}
	// Windows otherwise lets the child inherit the host's redirected
	// standard handles even though it is attached to the pseudoconsole
	// (microsoft/terminal#15814): the terminal then appears to work
	// only when the host's stdio is a console. Empty STARTF_USESTDHANDLES
	// slots make the attached console the child's stdio.
	si.Flags |= windows.STARTF_USESTDHANDLES
	si.StdInput, si.StdOutput, si.StdErr = 0, 0, 0
	var pi windows.ProcessInformation
	if err := windows.CreateProcess(exe, cmdline, nil, nil, false,
		windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_UNICODE_ENVIRONMENT,
		nil, dirp, &si.StartupInfo, &pi); err != nil {
		windows.CloseHandle(inW)
		windows.CloseHandle(outR)
		return "", fmt.Errorf("CreateProcess: %w", err)
	}
	windows.CloseHandle(pi.Thread)
	defer windows.CloseHandle(pi.Process)

	outCh := make(chan []byte, 1)
	go func() {
		var all []byte
		buf := make([]byte, 4096)
		for {
			var n uint32
			err := windows.ReadFile(outR, buf, &n, nil)
			if n > 0 {
				all = append(all, buf[:n]...)
			}
			if err != nil {
				break
			}
		}
		outCh <- all
	}()
	go func() {
		// A console submits a line on CR, not LF; the pty on Linux does
		// the translation on its own, ConPTY does not.
		input := strings.ReplaceAll(answers, "\n", "\r")
		var n uint32
		_ = windows.WriteFile(inW, []byte(input), &n, nil)
	}()

	windows.WaitForSingleObject(pi.Process, windows.INFINITE)
	var code uint32
	_ = windows.GetExitCodeProcess(pi.Process, &code)
	// Closing the console closes conhost, which closes the output pipe.
	// The input pipe stays open until now: conhost treats an early EOF
	// on its input as a control event and kills the client.
	closePC = false
	windows.ClosePseudoConsole(hpc)
	windows.CloseHandle(inW)
	out := <-outCh
	windows.CloseHandle(outR)
	if code != 0 {
		return string(out), fmt.Errorf("exit status %d", int32(code))
	}
	return string(out), nil
}

// normalizePTYOutput removes the terminal-control traffic conhost adds
// around a session (cursor, title, mode toggles). The hearing's own text
// carries no escape sequences, so this only drops transport noise; the
// comparison of what the human saw stays byte-for-byte.
func normalizePTYOutput(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != 0x1b || i+1 >= len(s) {
			b.WriteByte(s[i])
			i++
			continue
		}
		switch s[i+1] {
		case '[': // CSI: ESC [ ... final byte 0x40-0x7e
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			if j < len(s) {
				j++
			}
			i = j
		case ']': // OSC: ESC ] ... BEL or ESC backslash
			j := i + 2
			for j < len(s) && s[j] != 0x07 {
				if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
					j++
					break
				}
				j++
			}
			if j < len(s) {
				j++
			}
			i = j
		default:
			i += 2
		}
	}
	return b.String()
}
