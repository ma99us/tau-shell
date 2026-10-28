//go:build windows

package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

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

// RunWingetInConsole runs winget in a new visible console window so the user
// can read its output, package agreements, and any installer prompts.
func RunWingetInConsole(ctx context.Context, winget string, args []string) error {
	command := exec.CommandContext(ctx, winget, args...)
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_CONSOLE}
	return command.Run()
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
