//go:build windows

package runtime

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// RunInConsole runs wsl.exe with args in its own visible console so the user
// sees progress and Windows can show its administrator prompt. On failure the
// console stays open until a key is pressed so the message can be read. The
// arguments come from InstallArgs, which only yields safe tokens.
func RunInConsole(ctx context.Context, args []string) error {
	body, err := consoleCommand(args)
	if err != nil {
		return err
	}
	line := `cmd.exe /d /c "` + body + `"`
	command := exec.CommandContext(ctx, "cmd.exe")
	command.SysProcAttr = &syscall.SysProcAttr{CmdLine: line, CreationFlags: windows.CREATE_NEW_CONSOLE}
	if err := command.Run(); err != nil {
		return fmt.Errorf("wsl.exe %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

func consoleCommand(args []string) (string, error) {
	for _, arg := range args {
		if strings.ContainsAny(arg, " \"&|<>^%()") {
			return "", fmt.Errorf("unsafe wsl.exe argument %q", arg)
		}
	}
	return "wsl.exe " + strings.Join(args, " ") + ` || (echo. & echo wsl.exe did not complete. & pause & exit /b 1)`, nil
}

// ErrElevationDeclined means the user dismissed the Windows administrator prompt.
var ErrElevationDeclined = errors.New("the Windows administrator prompt was declined")

type shellExecuteInfo struct {
	size       uint32
	mask       uint32
	hwnd       windows.Handle
	verb       *uint16
	file       *uint16
	parameters *uint16
	directory  *uint16
	show       int32
	instApp    windows.Handle
	idList     uintptr
	class      *uint16
	keyClass   windows.Handle
	hotKey     uint32
	icon       windows.Handle
	process    windows.Handle
}

const seeMaskNoCloseProcess = 0x00000040

var procShellExecuteEx = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")

// RunElevatedInConsole is RunInConsole through the Windows administrator
// prompt (UAC), for actions such as installing WSL that need it. It waits for
// the elevated console to finish. The caller must have confirmed the action
// with the user first; TauShell itself never runs elevated.
func RunElevatedInConsole(ctx context.Context, args []string) error {
	body, err := consoleCommand(args)
	if err != nil {
		return err
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString("cmd.exe")
	params, _ := windows.UTF16PtrFromString(`/d /c "` + body + `"`)
	info := shellExecuteInfo{mask: seeMaskNoCloseProcess, verb: verb, file: file, parameters: params, show: windows.SW_SHOWNORMAL}
	info.size = uint32(unsafe.Sizeof(info))
	if r, _, callErr := procShellExecuteEx.Call(uintptr(unsafe.Pointer(&info))); r == 0 {
		if errors.Is(callErr, windows.ERROR_CANCELLED) {
			return ErrElevationDeclined
		}
		return fmt.Errorf("start elevated wsl.exe: %w", callErr)
	}
	defer windows.CloseHandle(info.process)
	done := make(chan error, 1)
	go func() {
		if _, err := windows.WaitForSingleObject(info.process, windows.INFINITE); err != nil {
			done <- err
			return
		}
		var code uint32
		if err := windows.GetExitCodeProcess(info.process, &code); err != nil {
			done <- err
			return
		}
		if code != 0 {
			done <- fmt.Errorf("wsl.exe %s: exit status %d", strings.Join(args, " "), code)
			return
		}
		done <- nil
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
