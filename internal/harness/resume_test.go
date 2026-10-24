package harness

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopilotResumeRequiresRecordedConversation(t *testing.T) {
	root := t.TempDir()
	empty := filepath.Join(root, "empty-id")
	used := filepath.Join(root, "used-id")
	for _, dir := range []string{empty, used} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(used, "events.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := checkCopilotResumeIn(root, "empty-id"); err == nil {
		t.Error("session without events.jsonl must not be resumable")
	}
	if err := checkCopilotResumeIn(root, "used-id"); err != nil {
		t.Errorf("session with events.jsonl must be resumable: %v", err)
	}
	if err := checkCopilotResumeIn(filepath.Join(root, "missing-store"), "any"); err != nil {
		t.Errorf("absent session store must fail open: %v", err)
	}
}

func TestClaudeResumeIsNotGated(t *testing.T) {
	adapter, err := For("claude-code")
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.CheckResume("anything"); err != nil {
		t.Fatal(err)
	}
}
