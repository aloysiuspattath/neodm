//go:build windows
// +build windows

package cli

import (
	"os"
	"syscall"
)

var (
	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	procAttachConsole = kernel32.NewProc("AttachConsole")
)

const ATTACH_PARENT_PROCESS = ^uintptr(0) // 0xFFFFFFFF

// AttachConsoleIfNeeded attaches to the invoking console if run from cmd or powershell.
func AttachConsoleIfNeeded() {
	r, _, _ := procAttachConsole.Call(ATTACH_PARENT_PROCESS)
	if r != 0 {
		if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0666); err == nil {
			os.Stdout = f
			os.Stderr = f
		}
		if f, err := os.OpenFile("CONIN$", os.O_RDONLY, 0666); err == nil {
			os.Stdin = f
		}
	}
}
