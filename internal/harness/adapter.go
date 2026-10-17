package harness

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/ma99us/tau-shell/internal/session"
)

type Adapter interface {
	ID() session.Harness
	FindExecutable() (string, error)
	StartArgs(harnessSessionID string) []string
	ResumeArgs(harnessSessionID string) []string
}

type cliAdapter struct {
	id         session.Harness
	executable string
}

func For(id session.Harness) (Adapter, error) {
	switch id {
	case session.ClaudeCode:
		return cliAdapter{id: id, executable: "claude"}, nil
	case session.CopilotCLI:
		return cliAdapter{id: id, executable: "copilot"}, nil
	default:
		return nil, fmt.Errorf("unsupported harness %q", id)
	}
}

func (adapter cliAdapter) ID() session.Harness {
	return adapter.id
}

func (adapter cliAdapter) FindExecutable() (string, error) {
	path, err := exec.LookPath(adapter.executable)
	if err != nil {
		return "", fmt.Errorf("%s executable not found on PATH: %w", adapter.id, err)
	}
	return path, nil
}

func (adapter cliAdapter) StartArgs(harnessSessionID string) []string {
	return []string{"--session-id", harnessSessionID}
}

func (adapter cliAdapter) ResumeArgs(harnessSessionID string) []string {
	return []string{"--resume", harnessSessionID}
}

func PowerShellInvocation(executable string, args []string, environment map[string]string, utf8 bool) string {
	lines := make([]string, 0, len(environment)+4)
	if utf8 {
		lines = append(lines,
			"$utf8 = [System.Text.UTF8Encoding]::new($false)",
			"[Console]::InputEncoding = $utf8",
			"[Console]::OutputEncoding = $utf8",
			"$OutputEncoding = $utf8",
			"chcp 65001 > $null",
		)
	}
	names := make([]string, 0, len(environment))
	for name := range environment {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		value := environment[name]
		lines = append(lines, "$env:"+name+" = "+QuotePowerShell(value))
	}

	quoted := make([]string, 0, len(args)+1)
	quoted = append(quoted, QuotePowerShell(executable))
	for _, arg := range args {
		quoted = append(quoted, QuotePowerShell(arg))
	}
	lines = append(lines, "& "+strings.Join(quoted, " "))
	return strings.Join(lines, "; ")
}

func QuotePowerShell(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
