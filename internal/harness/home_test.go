package harness

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ma99us/tau-shell/internal/session"
)

// A temp directory standing in for another runtime's home (for example a WSL
// distro) must be honored by the state readers.
func TestStateReadersFollowHomeDir(t *testing.T) {
	alternate := t.TempDir()
	original := HomeDir
	HomeDir = func() (string, error) { return alternate, nil }
	t.Cleanup(func() { HomeDir = original })
	t.Setenv("CLAUDE_CONFIG_DIR", "")

	if err := os.MkdirAll(filepath.Join(alternate, ".copilot", "session-state"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := checkCopilotResume("", "missing-id"); err == nil {
		t.Fatal("copilot resume check ignored the alternate home")
	}
	if err := os.MkdirAll(filepath.Join(alternate, ".claude", "projects", "p"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := checkClaudeResume("", "missing-id"); err == nil {
		t.Fatal("claude resume check ignored the alternate home")
	}
	transcript := filepath.Join(alternate, ".claude", "projects", "p", "known-id.jsonl")
	if err := os.WriteFile(transcript, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := checkClaudeResume("", "known-id"); err != nil {
		t.Fatalf("known claude session rejected: %v", err)
	}
}

// A WSL session passes its own home: it must be read instead of the Windows
// profile, and the Windows-only CLAUDE_CONFIG_DIR must not redirect it.
func TestExplicitHomeIsolatesFromWindowsProfile(t *testing.T) {
	windows, distro := t.TempDir(), t.TempDir()
	original := HomeDir
	HomeDir = func() (string, error) { return windows, nil }
	t.Cleanup(func() { HomeDir = original })
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(windows, "elsewhere"))

	project := filepath.Join(distro, ".claude", "projects", "p")
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	line := `{"type":"assistant","message":{"model":"claude-test"}}` + "\n"
	if err := os.WriteFile(filepath.Join(project, "known.jsonl"), []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := checkClaudeResume(distro, "known"); err != nil {
		t.Fatalf("explicit home not read: %v", err)
	}
	if got := CurrentModelIn(distro, session.ClaudeCode, "known"); got != "claude-test" {
		t.Fatalf("model = %q", got)
	}
	if got := CurrentModel(session.ClaudeCode, "known"); got != "" {
		t.Fatalf("default home leaked the distro transcript: %q", got)
	}
	for _, tool := range []session.Harness{session.GSD, session.Codex, session.OpenCode} {
		if got := CurrentModelIn(distro, tool, "known"); got != "" {
			t.Fatalf("%s read Windows state for a WSL home: %q", tool, got)
		}
	}
}
