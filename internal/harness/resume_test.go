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

func TestClaudeResumeRequiresRecordedConversation(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "C--some-project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "used-id.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := checkClaudeResumeIn(root, "empty-id"); err == nil {
		t.Error("session without a transcript must not be resumable")
	}
	if err := checkClaudeResumeIn(root, "used-id"); err != nil {
		t.Errorf("session with a transcript must be resumable: %v", err)
	}
	if err := checkClaudeResumeIn(filepath.Join(root, "missing-store"), "any"); err != nil {
		t.Errorf("absent transcript store must fail open: %v", err)
	}
}

func TestClaudeGracefulExitQuitsWithSlashExit(t *testing.T) {
	adapter, err := For("claude-code")
	if err != nil {
		t.Fatal(err)
	}
	steps := adapter.GracefulExit()
	if len(steps) != 3 || steps[0].Input != "\x03" || steps[1].Input != "/exit" || steps[2].Input != "\r" {
		t.Fatalf("graceful exit steps = %+v", steps)
	}
}
