package harness

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeAged(t *testing.T, path, content string, modified time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeProjectFolderMatchesClaudeNaming(t *testing.T) {
	if got := claudeProjectFolder(`C:\Altura\Projects\GSD\tau-shell`); got != "C--Altura-Projects-GSD-tau-shell" {
		t.Fatalf("folder = %q", got)
	}
}

func TestLatestClaudeSessionFindsNewConversationAfterClear(t *testing.T) {
	root := t.TempDir()
	dir := `C:\work\app`
	folder := filepath.Join(root, claudeProjectFolder(dir))
	launched := time.Now().Add(-time.Hour)
	writeAged(t, filepath.Join(folder, "current.jsonl"), "{}\n", launched.Add(time.Minute))
	writeAged(t, filepath.Join(folder, "old.jsonl"), "{}\n", launched.Add(-time.Hour))
	writeAged(t, filepath.Join(folder, "cleared.jsonl"), "{}\n", launched.Add(30*time.Minute))
	writeAged(t, filepath.Join(root, "other-folder", "foreign.jsonl"), "{}\n", launched.Add(40*time.Minute))
	taken := map[string]bool{"current": true}

	got, err := latestClaudeSessionIn(root, dir, launched, taken)
	if err != nil || got != "cleared" {
		t.Fatalf("got %q, %v; want cleared", got, err)
	}
	if got, err := latestClaudeSessionIn(root, dir, launched, map[string]bool{"current": true, "cleared": true}); err != nil || got != "" {
		t.Fatalf("nothing new should give empty, got %q, %v", got, err)
	}
}

func TestLatestClaudeSessionReportsCompetingCandidates(t *testing.T) {
	root := t.TempDir()
	dir := `C:\work\app`
	folder := filepath.Join(root, claudeProjectFolder(dir))
	launched := time.Now().Add(-time.Hour)
	writeAged(t, filepath.Join(folder, "a.jsonl"), "{}\n", launched.Add(10*time.Minute))
	writeAged(t, filepath.Join(folder, "b.jsonl"), "{}\n", launched.Add(10*time.Minute+time.Second))
	if _, err := latestClaudeSessionIn(root, dir, launched, nil); err != ErrAmbiguousSession {
		t.Fatalf("err = %v, want ErrAmbiguousSession", err)
	}
}

func TestLatestCopilotSessionMatchesFolderAndSkipsTaken(t *testing.T) {
	root := t.TempDir()
	dir := `C:\work\app`
	launched := time.Now().Add(-time.Hour)
	fresh := launched.Add(20 * time.Minute)
	writeAged(t, filepath.Join(root, "new-id", "workspace.yaml"), "id: new-id\ncwd: c:/work/APP\n", fresh)
	writeAged(t, filepath.Join(root, "new-id", "events.jsonl"), "{}\n", fresh)
	writeAged(t, filepath.Join(root, "elsewhere", "workspace.yaml"), "id: elsewhere\ncwd: C:\\other\n", fresh)
	writeAged(t, filepath.Join(root, "elsewhere", "events.jsonl"), "{}\n", fresh)
	writeAged(t, filepath.Join(root, "mine", "workspace.yaml"), "id: mine\ncwd: C:\\work\\app\n", fresh)
	writeAged(t, filepath.Join(root, "mine", "events.jsonl"), "{}\n", fresh)

	got, err := latestCopilotSessionIn(root, dir, launched, map[string]bool{"mine": true})
	if err != nil || got != "new-id" {
		t.Fatalf("got %q, %v; want new-id", got, err)
	}
}
