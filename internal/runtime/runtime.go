// Package runtime describes where a session's harness process runs. Today the
// only runtime is PowerShell 7 on Windows; the interface is the seam a later
// runtime (for example WSL) plugs into. See docs/wsl/02-design.md.
package runtime

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/ma99us/tau-shell/internal/harness"
)

// Kind names a runtime.
type Kind string

const PowerShell Kind = "powershell"

// LaunchSpec is what a runtime needs to build the process command line.
type LaunchSpec struct {
	Executable string
	Args       []string
	UTF8       bool
}

// Command is an executable and its arguments, ready for the ConPTY host.
type Command struct {
	Executable string
	Args       []string
}

// Runtime builds launch commands and locates the harness state it owns.
type Runtime interface {
	Kind() Kind
	// Available reports whether the runtime can launch, before any state changes.
	Available() error
	// Command resolves the host executable and arguments for a harness launch.
	Command(spec LaunchSpec) (Command, error)
	// HomeDir is the directory, readable from Windows, holding harness state.
	HomeDir() (string, error)
}

// Default returns the runtime used by every session today.
func Default() Runtime {
	return powershellRuntime{lookPath: exec.LookPath, homeDir: os.UserHomeDir}
}

type powershellRuntime struct {
	lookPath func(string) (string, error)
	homeDir  func() (string, error)
}

func (powershellRuntime) Kind() Kind { return PowerShell }

func (runtime powershellRuntime) Available() error {
	_, err := runtime.powershell()
	return err
}

func (runtime powershellRuntime) powershell() (string, error) {
	path, err := runtime.lookPath("pwsh.exe")
	if err != nil {
		return "", fmt.Errorf("PowerShell 7 (pwsh.exe) is required and must be on PATH: %w", err)
	}
	return path, nil
}

func (runtime powershellRuntime) Command(spec LaunchSpec) (Command, error) {
	powershell, err := runtime.powershell()
	if err != nil {
		return Command{}, err
	}
	script := harness.PowerShellInvocation(spec.Executable, spec.Args, spec.UTF8)
	return Command{Executable: powershell, Args: []string{"-NoLogo", "-NoProfile", "-Command", script}}, nil
}

func (runtime powershellRuntime) HomeDir() (string, error) { return runtime.homeDir() }
