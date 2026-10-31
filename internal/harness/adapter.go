package harness

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ma99us/tau-shell/internal/session"
)

type Adapter interface {
	ID() session.Harness
	FindExecutable() (string, error)
	StartArgs(harnessSessionID string) []string
	ResumeArgs(harnessSessionID string) []string
	// Argv returns the full argument list for one launch: the user's
	// launch_args plus TauShell's identity arguments, in the order the
	// harness's own parser needs.
	Argv(launchArgs []string, resuming bool, harnessSessionID string) []string
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
	// DiscoversSessionID reports whether the harness picks its own conversation
	// ID, which TauShell can only learn after launch via DiscoverSessionID.
	DiscoversSessionID() bool
	// DiscoverSessionID looks for the conversation a launch in workingDir at or
	// after since created, ignoring IDs in taken. It returns "" while none
	// exists yet and ErrAmbiguousSession when several candidates remain.
	DiscoverSessionID(workingDir string, since time.Time, taken map[string]bool) (string, error)
}

// ErrAmbiguousSession means several unclaimed harness conversations match a
// launch, so none can be safely attributed to it.
var ErrAmbiguousSession = errors.New("several harness conversations match this launch")

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
	startArgs    func(harnessSessionID string) []string
	resumeArgs   func(harnessSessionID string) []string
	// resumeFirst places the resume arguments before launch_args, for
	// harnesses whose resume is a subcommand.
	resumeFirst bool
	discover    func(workingDir string, since time.Time, taken map[string]bool) (string, error)
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
	case session.GSD:
		return cliAdapter{id: id, executable: "gsd", resumeCheck: checkGSDResume,
			startArgs: gsdSessionArgs, resumeArgs: gsdSessionArgs,
			gracefulExit: []ExitStep{
				{Input: "\x1b", Pause: 400 * time.Millisecond},
				{Input: "/quit", Pause: 300 * time.Millisecond},
				{Input: "\r"},
			}}, nil
	case session.Codex:
		return cliAdapter{id: id, executable: "codex", defaultArgs: []string{"--dangerously-bypass-approvals-and-sandbox"}, resumeCheck: checkCodexResume,
			resumeArgs: func(id string) []string { return []string{"resume", id} }, resumeFirst: true,
			discover: discoverCodexSession,
			gracefulExit: []ExitStep{
				{Input: "\x1b", Pause: 400 * time.Millisecond},
				{Input: "/exit", Pause: 300 * time.Millisecond},
				{Input: "\r"},
			}}, nil
	case session.OpenCode:
		return cliAdapter{id: id, executable: "opencode", defaultArgs: []string{"--auto"}, resumeCheck: checkOpenCodeResume,
			resumeArgs: func(id string) []string { return []string{"--session", id} },
			discover:   discoverOpenCodeSession,
			gracefulExit: []ExitStep{
				{Input: "\x1b", Pause: 300 * time.Millisecond},
				{Input: "\x1b", Pause: 400 * time.Millisecond},
				{Input: "/exit", Pause: 300 * time.Millisecond},
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
	if adapter.startArgs != nil {
		return adapter.startArgs(harnessSessionID)
	}
	if adapter.discover != nil {
		return nil
	}
	return []string{"--session-id", harnessSessionID}
}

func (adapter cliAdapter) ResumeArgs(harnessSessionID string) []string {
	if adapter.resumeArgs != nil {
		return adapter.resumeArgs(harnessSessionID)
	}
	return []string{"--resume", harnessSessionID}
}

func (adapter cliAdapter) Argv(launchArgs []string, resuming bool, harnessSessionID string) []string {
	args := make([]string, 0, len(launchArgs)+3)
	if resuming {
		managed := adapter.ResumeArgs(harnessSessionID)
		if adapter.resumeFirst {
			return append(append(args, managed...), launchArgs...)
		}
		return append(append(args, launchArgs...), managed...)
	}
	return append(append(args, launchArgs...), adapter.StartArgs(harnessSessionID)...)
}

func (adapter cliAdapter) DiscoversSessionID() bool {
	return adapter.discover != nil
}

func (adapter cliAdapter) DiscoverSessionID(workingDir string, since time.Time, taken map[string]bool) (string, error) {
	if adapter.discover == nil {
		return "", nil
	}
	return adapter.discover(workingDir, since, taken)
}

func (adapter cliAdapter) GracefulExit() []ExitStep {
	return append([]ExitStep(nil), adapter.gracefulExit...)
}

func (adapter cliAdapter) DefaultLaunchArgs() []string {
	return append([]string(nil), adapter.defaultArgs...)
}

// PowerShellInvocation builds the -Command script. Session environment values
// are deliberately not part of it: they travel in the child environment block
// so they never appear on a process command line.
func PowerShellInvocation(executable string, args []string, utf8 bool) string {
	lines := make([]string, 0, 6)
	if utf8 {
		lines = append(lines,
			"$utf8 = [System.Text.UTF8Encoding]::new($false)",
			"[Console]::InputEncoding = $utf8",
			"[Console]::OutputEncoding = $utf8",
			"$OutputEncoding = $utf8",
			"chcp 65001 > $null",
		)
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
