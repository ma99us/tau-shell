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

func RunNpm(ctx context.Context, args ...string) error {
	return runNpm(ctx, false, args...)
}

func RunNpmInConsole(ctx context.Context, args []string) error {
	return runNpm(ctx, true, args...)
}

func runNpm(ctx context.Context, visible bool, args ...string) error {
	node, err := exec.LookPath("node.exe")
	if err != nil {
		return fmt.Errorf("Node.js executable not found: %w", err)
	}
	npm, err := exec.LookPath("npm.cmd")
	if err != nil {
		return fmt.Errorf("npm.cmd not found: %w", err)
	}
	cli, err := npmCLIPath(node, npm)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, node, append([]string{cli}, args...)...)
	if visible {
		command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_CONSOLE}
	} else {
		hideWindow(command)
	}
	if err := command.Run(); err != nil {
		return fmt.Errorf("run npm: %w", err)
	}
	return nil
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
