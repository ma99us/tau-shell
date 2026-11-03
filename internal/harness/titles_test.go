package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConversationTitlesFromHarnessLogs(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "projects", "C--demo")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	log := `{"type":"ai-title","aiTitle":"First title","sessionId":"abc"}` + "\n" +
		`{"type":"user","message":"ignored"}` + "\n" +
		`{"type":"ai-title","aiTitle":"Fix the\nlogin bug","sessionId":"abc"}` + "\n"
	if err := os.WriteFile(filepath.Join(project, "abc.jsonl"), []byte(log), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := claudeTitleIn(filepath.Join(root, "projects"), "abc"); got != "Fix the login bug" {
		t.Errorf("claude title = %q", got)
	}
	if got := claudeTitleIn(filepath.Join(root, "projects"), "missing"); got != "" {
		t.Errorf("missing claude log title = %q", got)
	}

	state := filepath.Join(root, "state", "xyz")
	if err := os.MkdirAll(state, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "workspace.yaml"), []byte("id: xyz\nname: "+strings.Repeat("long ", 40)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := copilotTitleIn(filepath.Join(root, "state"), "xyz")
	if !strings.HasSuffix(got, "...") || len([]rune(got)) != maxTitleRunes+3 {
		t.Errorf("copilot title was not bounded: %q", got)
	}
}
