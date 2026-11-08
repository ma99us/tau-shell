//go:build windows

package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func hideWindow(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
}

// RunWingetHidden runs winget with no console window and returns its output.
func RunWingetHidden(ctx context.Context, winget string, args []string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, winget, args...)
	hideWindow(command)
	output, err := command.CombinedOutput()
	return string(output), err
}

const wingetConsoleCommand = "--taushell-internal-winget-console"

// IsWingetConsoleCommand identifies the private helper invocation used to
// create one visible console before starting WinGet.
func IsWingetConsoleCommand(args []string) bool {
	return len(args) > 0 && args[0] == wingetConsoleCommand
}

// RunWingetInConsole starts a separate TauShell helper. The helper owns the
// visible console, prints a status line, and then runs WinGet in that console.
func RunWingetInConsole(ctx context.Context, winget string, args []string) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate TauShell for WinGet console: %w", err)
	}
	helperArgs := append([]string{wingetConsoleCommand, winget}, args...)
	command := exec.CommandContext(ctx, executable, helperArgs...)
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_CONSOLE}
	return command.Run()
}

// RunWingetConsoleCommand prints the progress line before launching WinGet.
// WinGet shares this one console for output, agreements, and prompts.
func RunWingetConsoleCommand(args []string) int {
	if len(args) < 2 || !strings.EqualFold(filepath.Base(args[0]), "winget.exe") {
		return 2
	}
	output, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0)
	if err != nil {
		return 1
	}
	defer output.Close()
	input, err := os.OpenFile("CONIN$", os.O_RDONLY, 0)
	if err != nil {
		_, _ = fmt.Fprintf(output, "Could not open WinGet console input: %v\n", err)
		return 1
	}
	defer input.Close()
	_, _ = fmt.Fprintln(output, wingetProgressMessage(args[1:]))

	command := exec.Command(args[0], args[1:]...)
	command.Stdin = input
	command.Stdout = output
	command.Stderr = output
	if err := command.Start(); err != nil {
		_, _ = fmt.Fprintf(output, "Could not start WinGet: %v\n", err)
		return 1
	}

	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		_, _ = fmt.Fprintf(output, "Could not supervise WinGet: %v\n", err)
		return 1
	}
	defer windows.CloseHandle(job)
	jobInfo := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&jobInfo)), uint32(unsafe.Sizeof(jobInfo))); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		_, _ = fmt.Fprintf(output, "Could not supervise WinGet: %v\n", err)
		return 1
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(command.Process.Pid))
	if err == nil {
		err = windows.AssignProcessToJobObject(job, process)
		_ = windows.CloseHandle(process)
	}
	if err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		_, _ = fmt.Fprintf(output, "Could not supervise WinGet: %v\n", err)
		return 1
	}
	if err := command.Wait(); err != nil {
		_, _ = fmt.Fprintf(output, "WinGet did not complete successfully: %v\n", err)
		return 1
	}
	return 0
}

func RunNpm(ctx context.Context, args ...string) error {
	return runNpm(ctx, false, args...)
}

// RunNpmOutput runs npm without a visible window and returns its combined output.
func RunNpmOutput(ctx context.Context, args ...string) (string, error) {
	output, err := runNpmCommand(ctx, false, args...)
	return string(output), err
}

func RunNpmInConsole(ctx context.Context, args []string) error {
	return runNpm(ctx, true, args...)
}

func runNpm(ctx context.Context, visible bool, args ...string) error {
	_, err := runNpmCommand(ctx, visible, args...)
	return err
}

func runNpmCommand(ctx context.Context, visible bool, args ...string) ([]byte, error) {
	node, err := exec.LookPath("node.exe")
	if err != nil {
		return nil, fmt.Errorf("Node.js executable not found: %w", err)
	}
	npm, err := exec.LookPath("npm.cmd")
	if err != nil {
		return nil, fmt.Errorf("npm.cmd not found: %w", err)
	}
	cli, err := npmCLIPath(node, npm)
	if err != nil {
		return nil, err
	}
	command := exec.CommandContext(ctx, node, append([]string{cli}, args...)...)
	if visible {
		command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_CONSOLE}
		if err := command.Run(); err != nil {
			return nil, fmt.Errorf("run npm: %w", err)
		}
		return nil, nil
	}
	hideWindow(command)
	output, err := command.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("run npm: %w", err)
	}
	return output, nil
}

func npmCLIPath(node, npm string) (string, error) {
	candidates := []string{
		filepath.Join(filepath.Dir(npm), "node_modules", "npm", "bin", "npm-cli.js"),
		filepath.Join(filepath.Dir(node), "node_modules", "npm", "bin", "npm-cli.js"),
	}
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		key := strings.ToLower(filepath.Clean(candidate))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("npm CLI JavaScript entry point not found beside %s or %s", npm, node)
}

// RefreshProcessPath appends machine and user PATH entries from the registry
// that the current process does not yet have, so newly installed tools can be
// found without restarting TauShell.
func RefreshProcessPath() error {
	var additions []string
	var failures []error
	for _, location := range []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`},
		{registry.CURRENT_USER, `Environment`},
	} {
		key, err := registry.OpenKey(location.root, location.path, registry.QUERY_VALUE)
		if err != nil {
			failures = append(failures, fmt.Errorf("open %s: %w", location.path, err))
			continue
		}
		value, _, err := key.GetStringValue("Path")
		key.Close()
		if err != nil {
			if !errors.Is(err, registry.ErrNotExist) {
				failures = append(failures, fmt.Errorf("read %s Path: %w", location.path, err))
			}
			continue
		}
		if expanded, err := registry.ExpandString(value); err == nil {
			value = expanded
		}
		additions = append(additions, value)
	}
	if err := os.Setenv("PATH", MergePathEntries(os.Getenv("PATH"), additions...)); err != nil {
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}
