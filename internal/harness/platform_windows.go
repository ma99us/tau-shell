//go:build windows

package harness

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func hideWindow(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
}
