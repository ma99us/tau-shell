package harness

import (
	"os"
	"path/filepath"
	"testing"
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
	if err := checkCopilotResume("missing-id"); err == nil {
		t.Fatal("copilot resume check ignored the alternate home")
	}
	if err := os.MkdirAll(filepath.Join(alternate, ".claude", "projects", "p"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := checkClaudeResume("missing-id"); err == nil {
		t.Fatal("claude resume check ignored the alternate home")
	}
	transcript := filepath.Join(alternate, ".claude", "projects", "p", "known-id.jsonl")
	if err := os.WriteFile(transcript, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := checkClaudeResume("known-id"); err != nil {
		t.Fatalf("known claude session rejected: %v", err)
	}
}
