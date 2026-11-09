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

	"github.com/ma99us/tau-shell/internal/harness/manifest"
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
	// HasSkillDirectoryRoute reports whether this adapter has a verified
	// per-launch route for TauShell's skill directory.
	HasSkillDirectoryRoute() bool
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
	// ExistingSessionIDs snapshots conversations in workingDir before launch,
	// so a recently modified external conversation is not mistaken for this run.
	ExistingSessionIDs(workingDir string) (map[string]bool, error)
	// DiscoverSessionID looks for the conversation a launch in workingDir at or
	// after since created, ignoring IDs in taken. It returns "" while none
	// exists yet and ErrAmbiguousSession when several candidates remain.
	DiscoverSessionID(workingDir string, since time.Time, taken map[string]bool) (string, error)
}

// ErrAmbiguousSession means several unclaimed harness conversations match a
// launch, so none can be safely attributed to it.
var ErrAmbiguousSession = errors.New("several harness conversations match this launch")

// CombineSessionIDs returns IDs already claimed by TauShell or present before
// launch without mutating either input map.
func CombineSessionIDs(claimed, existing map[string]bool) map[string]bool {
	combined := make(map[string]bool, len(claimed)+len(existing))
	for id := range claimed {
		combined[id] = true
	}
	for id := range existing {
		combined[id] = true
	}
	return combined
}

// ExitStep is one terminal input write followed by a pause.
type ExitStep struct {
	Input string
	Pause time.Duration
}

// cliAdapter is the manifest-backed adapter: every per-harness fact comes
// from the manifest, and every private-format reader is chosen by name.
type cliAdapter struct {
	manifest *manifest.Manifest
}

// For returns the adapter for a harness declared by a built-in or user manifest.
func For(id session.Harness) (Adapter, error) {
	item, ok := manifest.Get(string(id))
	if !ok {
		return nil, fmt.Errorf("unsupported harness %q", id)
	}
	return cliAdapter{manifest: item}, nil
}

// resumeCheckers maps manifest resume_check names to their readers.
var resumeCheckers = map[string]func(harnessSessionID string) error{
	"claude-transcript":     checkClaudeResume,
	"copilot-session-state": checkCopilotResume,
	"gsd-session-file":      checkGSDResume,
	"codex-rollout":         checkCodexResume,
	"opencode-db":           checkOpenCodeResume,
}

type discoveryReader struct {
	discover func(workingDir string, since time.Time, taken map[string]bool) (string, error)
	existing func(workingDir string) (map[string]bool, error)
}

// discoveryReaders maps manifest session.discovery names to their readers.
var discoveryReaders = map[string]discoveryReader{
	"codex-rollout": {discoverCodexSession, existingCodexSessionIDs},
	"opencode-db":   {discoverOpenCodeSession, existingOpenCodeSessionIDs},
}

// checkCopilotResume relies on Copilot CLI's own session store: a session that
// never had a conversation turn has no events.jsonl and cannot be resumed.
// This is an undocumented internal layout, so it fails open when the store
// itself is absent.
func checkCopilotResume(harnessSessionID string) error {
	home, err := HomeDir()
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
		home, err := HomeDir()
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
	check, ok := resumeCheckers[adapter.manifest.ResumeCheck]
	if !ok {
		return nil
	}
	return check(harnessSessionID)
}

func (adapter cliAdapter) HasSkillDirectoryRoute() bool {
	return adapter.manifest.Skills.SkillDirOption() != ""
}

func (adapter cliAdapter) ID() session.Harness {
	return session.Harness(adapter.manifest.ID)
}

// ExecutableName returns the bare command name a harness manifest declares,
// for runtimes that resolve it themselves (for example inside WSL).
func ExecutableName(id session.Harness) (string, error) {
	item, ok := manifest.Get(string(id))
	if !ok {
		return "", fmt.Errorf("unknown harness %q", id)
	}
	return item.Executable, nil
}

func (adapter cliAdapter) FindExecutable() (string, error) {
	path, err := exec.LookPath(adapter.manifest.Executable)
	if err != nil {
		return "", fmt.Errorf("%s executable not found on PATH: %w", adapter.manifest.ID, err)
	}
	return path, nil
}

// expandArgs fills {id} and {session_file}. The session file's folder is
// created best effort, because some harnesses do not create it themselves.
func (adapter cliAdapter) expandArgs(args []string, harnessSessionID string) []string {
	if len(args) == 0 {
		return nil
	}
	sessionFile := ""
	if template := adapter.manifest.Session.File; template != "" {
		if path, err := manifest.ExpandPath(template, harnessSessionID); err == nil {
			sessionFile = path
			_ = os.MkdirAll(filepath.Dir(path), 0o700)
		}
	}
	expanded := make([]string, len(args))
	for index, arg := range args {
		expanded[index] = manifest.ExpandArg(arg, harnessSessionID, sessionFile)
	}
	return expanded
}

func (adapter cliAdapter) StartArgs(harnessSessionID string) []string {
	return adapter.expandArgs(adapter.manifest.Session.StartArgs, harnessSessionID)
}

func (adapter cliAdapter) ResumeArgs(harnessSessionID string) []string {
	return adapter.expandArgs(adapter.manifest.Session.ResumeArgs, harnessSessionID)
}

func (adapter cliAdapter) Argv(launchArgs []string, resuming bool, harnessSessionID string) []string {
	args := make([]string, 0, len(launchArgs)+3)
	if resuming {
		managed := adapter.ResumeArgs(harnessSessionID)
		if adapter.manifest.Session.ResumePos == "before" {
			return append(append(args, managed...), launchArgs...)
		}
		return append(append(args, launchArgs...), managed...)
	}
	return append(append(args, launchArgs...), adapter.StartArgs(harnessSessionID)...)
}

func (adapter cliAdapter) DiscoversSessionID() bool {
	return adapter.manifest.Session.ID == "discover"
}

func (adapter cliAdapter) ExistingSessionIDs(workingDir string) (map[string]bool, error) {
	if !adapter.DiscoversSessionID() {
		return nil, nil
	}
	reader, ok := discoveryReaders[adapter.manifest.Session.Discovery]
	if !ok {
		return nil, fmt.Errorf("%s session discovery cannot snapshot existing conversations", adapter.manifest.ID)
	}
	return reader.existing(workingDir)
}

func (adapter cliAdapter) DiscoverSessionID(workingDir string, since time.Time, taken map[string]bool) (string, error) {
	reader, ok := discoveryReaders[adapter.manifest.Session.Discovery]
	if !adapter.DiscoversSessionID() || !ok {
		return "", nil
	}
	return reader.discover(workingDir, since, taken)
}

func (adapter cliAdapter) GracefulExit() []ExitStep {
	var steps []ExitStep
	for _, step := range adapter.manifest.GracefulExit {
		steps = append(steps, ExitStep{Input: step.Input, Pause: step.Pause()})
	}
	return steps
}

func (adapter cliAdapter) DefaultLaunchArgs() []string {
	return append([]string(nil), adapter.manifest.Launch.DefaultArgs...)
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
