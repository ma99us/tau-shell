//go:build windows

package runtime

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// RunInConsole runs wsl.exe with args in its own visible console so the user
// sees progress and Windows can show its administrator prompt. On failure the
// console stays open until a key is pressed so the message can be read. The
// arguments come from InstallArgs, which only yields safe tokens.
func RunInConsole(ctx context.Context, args []string) error {
	for _, arg := range args {
		if strings.ContainsAny(arg, " \"&|<>^%()") {
			return fmt.Errorf("unsafe wsl.exe argument %q", arg)
		}
	}
	line := `cmd.exe /d /c "wsl.exe ` + strings.Join(args, " ") + ` || (echo. & echo wsl.exe did not complete. & pause & exit /b 1)"`
	command := exec.CommandContext(ctx, "cmd.exe")
	command.SysProcAttr = &syscall.SysProcAttr{CmdLine: line, CreationFlags: windows.CREATE_NEW_CONSOLE}
	if err := command.Run(); err != nil {
		return fmt.Errorf("wsl.exe %s: %w", strings.Join(args, " "), err)
	}
	return nil
}
