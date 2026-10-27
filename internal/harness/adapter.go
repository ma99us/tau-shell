package harness

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ma99us/tau-shell/internal/session"
)

type Adapter interface {
	ID() session.Harness
	FindExecutable() (string, error)
	StartArgs(harnessSessionID string) []string
	ResumeArgs(harnessSessionID string) []string
	// CheckResume reports why the harness cannot resume harnessSessionID, or
	// nil when it can or when that cannot be determined.
	CheckResume(harnessSessionID string) error
	// DefaultLaunchArgs returns the recommended editable launch_args preset
	// for a new session with this harness. These are user-editable template
	// defaults, not TauShell-managed identity arguments.
	DefaultLaunchArgs() []string
	// GracefulExit returns the ordered input steps that make the harness quit
	// on its own so it can flush its conversation state, or nil when only a
	// plain interrupt is known.
	GracefulExit() []ExitStep
}

// ExitStep is one terminal input write followed by a pause.
type ExitStep struct {
	Input string
	Pause time.Duration
}

type cliAdapter struct {
	id           session.Harness
	executable   string
	defaultArgs  []string
	resumeCheck  func(harnessSessionID string) error
	gracefulExit []ExitStep
}

func For(id session.Harness) (Adapter, error) {
	switch id {
	case session.ClaudeCode:
		return cliAdapter{id: id, executable: "claude", defaultArgs: []string{"--dangerously-skip-permissions"}, resumeCheck: checkClaudeResume, gracefulExit: []ExitStep{
			{Input: "\x03", Pause: 400 * time.Millisecond},
			{Input: "/exit", Pause: 400 * time.Millisecond},
			{Input: "\r"},
		}}, nil
	case session.CopilotCLI:
		return cliAdapter{id: id, executable: "copilot", defaultArgs: []string{"--yolo"}, resumeCheck: checkCopilotResume, gracefulExit: []ExitStep{
			{Input: "\x03", Pause: 400 * time.Millisecond},
			{Input: "/exit", Pause: 200 * time.Millisecond},
			{Input: "\r"},
		}}, nil
	default:
		return nil, fmt.Errorf("unsupported harness %q", id)
	}
}

// checkCopilotResume relies on Copilot CLI's own session store: a session that
// never had a conversation turn has no events.jsonl and cannot be resumed.
// This is an undocumented internal layout, so it fails open when the store
// itself is absent.
func checkCopilotResume(harnessSessionID string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	root := filepath.Join(home, ".copilot", "session-state")
	return checkCopilotResumeIn(root, harnessSessionID)
}

func checkCopilotResumeIn(root, harnessSessionID string) error {
	if _, err := os.Stat(root); err != nil {
		return nil
	}
	if _, err := os.Stat(filepath.Join(root, harnessSessionID, "events.jsonl")); errors.Is(err, fs.ErrNotExist) {
		return errors.New("GitHub Copilot CLI has no recorded conversation for this session yet; use Start fresh")
	}
	return nil
}

// checkClaudeResume relies on Claude Code's transcript store: a session that
// never got a first prompt has no <id>.jsonl under any project folder, and
// `claude --resume` then exits with "No conversation found". This is an
// undocumented internal layout, so it fails open when the store is absent.
func checkClaudeResume(harnessSessionID string) error {
	root := os.Getenv("CLAUDE_CONFIG_DIR")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		root = filepath.Join(home, ".claude")
	}
	return checkClaudeResumeIn(filepath.Join(root, "projects"), harnessSessionID)
}

func checkClaudeResumeIn(projectsRoot, harnessSessionID string) error {
	if _, err := os.Stat(projectsRoot); err != nil {
		return nil
	}
	matches, err := filepath.Glob(filepath.Join(projectsRoot, "*", harnessSessionID+".jsonl"))
	if err != nil || len(matches) > 0 {
		return nil
	}
	return errors.New("Claude Code has no recorded conversation for this session yet; use Start fresh")
}

func (adapter cliAdapter) CheckResume(harnessSessionID string) error {
	if adapter.resumeCheck == nil {
		return nil
	}
	return adapter.resumeCheck(harnessSessionID)
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

func (adapter cliAdapter) GracefulExit() []ExitStep {
	return append([]ExitStep(nil), adapter.gracefulExit...)
}

func (adapter cliAdapter) DefaultLaunchArgs() []string {
	return append([]string(nil), adapter.defaultArgs...)
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
