package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeModelUsesLastRealModel(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "proj", "abc.jsonl"), strings.Join([]string{
		`{"attachment":{"type":"model","identity":{"modelId":"claude-opus-4"}}}`,
		`{"message":{"model":"claude-sonnet-5-5","role":"assistant"}}`,
		`{"message":{"model":"<synthetic>","role":"assistant"}}`,
		`{"message":{"role":"user","content":"<local-command-stdout>Set model to ` + "`Opus 5.5`" + ` and saved as your default</local-command-stdout>"}}`,
		`{"message":{"role":"user"}}`,
		`{"message":{"model":"claude-par`,
	}, "\n"))
	if got := claudeModelIn(root, "abc"); got != "Opus 5.5" {
		t.Fatalf("model = %q", got)
	}
	if got := claudeModelIn(root, "missing"); got != "" {
		t.Fatalf("missing transcript model = %q", got)
	}
}

func TestCopilotModelFollowsModelChanges(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sid", "events.jsonl")
	writeFile(t, path, strings.Join([]string{
		`{"type":"session.start","data":{"selectedModel":"claude-opus-4.6"}}`,
		`{"type":"model.turn_started","data":{"model":"gpt-4o-mini"}}`,
		`{"type":"session.model_change","data":{"newModel":"claude-sonnet-5.5"}}`,
	}, "\n")+"\n")
	if got := copilotModelIn(root, "sid"); got != "claude-sonnet-5.5" {
		t.Fatalf("model = %q", got)
	}
}

func TestCopilotModelFallsBackToStartWhenTailHasNone(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sid", "events.jsonl")
	filler := `{"type":"assistant.message","data":{"content":"` + strings.Repeat("x", 4096) + `"}}` + "\n"
	writeFile(t, path, `{"type":"session.start","data":{"selectedModel":"claude-opus-4.6"}}`+"\n"+strings.Repeat(filler, 400))
	if got := copilotModelIn(root, "sid"); got != "claude-opus-4.6" {
		t.Fatalf("model = %q", got)
	}
}
