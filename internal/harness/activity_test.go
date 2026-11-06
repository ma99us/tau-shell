package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ma99us/tau-shell/internal/session"
)

func TestCopilotActivityFromLines(t *testing.T) {
	line := func(kind, at string) string {
		return `{"type":"` + kind + `","data":{},"timestamp":"` + at + `"}`
	}
	now := time.Date(2026, 11, 3, 22, 0, 10, 0, time.UTC)
	cases := []struct {
		name  string
		lines []string
		want  string
	}{
		{"empty", nil, ""},
		{"turn running", []string{line("user.message", "2026-11-03T22:00:00Z"), line("assistant.turn_start", "2026-11-03T22:00:01Z")}, ActivityWorking},
		{"between turns", []string{line("assistant.turn_start", "2026-11-03T22:00:01Z"), line("assistant.turn_end", "2026-11-03T22:00:09.5Z")}, ActivityWorking},
		{"finished", []string{line("assistant.turn_start", "2026-11-03T22:00:01Z"), line("assistant.turn_end", "2026-11-03T22:00:05Z")}, ActivityIdle},
		{"ignores other events", []string{line("assistant.turn_end", "2026-11-03T22:00:05Z"), line("session.usage_record", "2026-11-03T22:00:09Z")}, ActivityIdle},
	}
	for _, test := range cases {
		var lines [][]byte
		for _, item := range test.lines {
			lines = append(lines, []byte(item))
		}
		lines = append([][]byte{[]byte(strings.Repeat("x", 5))}, lines...)
		if got := copilotActivityFromLines(lines, now); got != test.want {
			t.Errorf("%s: got %q want %q", test.name, got, test.want)
		}
	}
}

func TestCodexActivityFromLines(t *testing.T) {
	line := func(kind, at string) string {
		return `{"type":"event_msg","payload":{"type":"` + kind + `"},"timestamp":"` + at + `"}`
	}
	now := time.Date(2026, 11, 3, 22, 0, 10, 0, time.UTC)
	cases := []struct {
		name  string
		lines []string
		want  string
	}{
		{"empty", nil, ""},
		{"task running", []string{line("task_started", "2026-11-03T22:00:01Z")}, ActivityWorking},
		{"task just completed", []string{line("task_started", "2026-11-03T22:00:01Z"), line("task_complete", "2026-11-03T22:00:09Z")}, ActivityWorking},
		{"task finished", []string{line("task_started", "2026-11-03T22:00:01Z"), line("task_complete", "2026-11-03T22:00:05Z")}, ActivityIdle},
		{"ignores other events", []string{line("task_complete", "2026-11-03T22:00:05Z"), line("item_completed", "2026-11-03T22:00:09Z")}, ActivityIdle},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			lines := make([][]byte, 0, len(test.lines)+1)
			lines = append(lines, []byte(strings.Repeat("x", 5)))
			for _, item := range test.lines {
				lines = append(lines, []byte(item))
			}
			if got := codexActivityFromLines(lines, now); got != test.want {
				t.Errorf("activity = %q, want %q", got, test.want)
			}
		})
	}
}

func TestGSDActivityFromLines(t *testing.T) {
	line := func(role, stopReason, at string) string {
		return `{"type":"message","timestamp":"` + at + `","message":{"role":"` + role + `","stopReason":"` + stopReason + `"}}`
	}
	now := time.Date(2026, 11, 5, 22, 0, 10, 0, time.UTC)
	cases := []struct {
		name  string
		lines []string
		want  string
	}{
		{"empty", nil, ""},
		{"user message is working", []string{line("user", "", "2026-11-05T22:00:01Z")}, ActivityWorking},
		{"tool call is working", []string{line("assistant", "toolUse", "2026-11-05T22:00:01Z")}, ActivityWorking},
		{"tool reply is working", []string{line("tool", "", "2026-11-05T22:00:01Z")}, ActivityWorking},
		{"just completed is working", []string{line("user", "", "2026-11-05T22:00:01Z"), line("assistant", "stop", "2026-11-05T22:00:09Z")}, ActivityWorking},
		{"finished is idle", []string{line("user", "", "2026-11-05T22:00:01Z"), line("assistant", "stop", "2026-11-05T22:00:05Z")}, ActivityIdle},
		{"interrupted is idle", []string{line("user", "", "2026-11-05T22:00:01Z"), line("assistant", "aborted", "2026-11-05T22:00:05Z")}, ActivityIdle},
		{"ignores metadata", []string{line("assistant", "stop", "2026-11-05T22:00:05Z"), `{"type":"thinking_level_change","timestamp":"2026-11-05T22:00:09Z"}`}, ActivityIdle},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			lines := make([][]byte, 0, len(test.lines)+1)
			lines = append(lines, []byte(strings.Repeat("x", 5)))
			for _, item := range test.lines {
				lines = append(lines, []byte(item))
			}
			if got := gsdActivityFromLines(lines, now); got != test.want {
				t.Errorf("activity = %q, want %q", got, test.want)
			}
		})
	}
}

func TestCurrentCodexActivityReadsSessionRollout(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CODEX_HOME", root)
	now := time.Date(2026, 11, 3, 22, 0, 10, 0, time.UTC)
	path := filepath.Join(root, "sessions", "2026", "11", "03", "rollout-test-fixture.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(`{"type":"event_msg","payload":{"type":"task_started"},"timestamp":"2026-11-03T22:00:01Z"}` + "\n")
	if got := CurrentActivity(session.Codex, "fixture", now); got != ActivityWorking {
		t.Fatalf("working activity = %q, want %q", got, ActivityWorking)
	}
	write(`{"type":"event_msg","payload":{"type":"task_complete"},"timestamp":"2026-11-03T22:00:05Z"}` + "\n")
	if got := CurrentActivity(session.Codex, "fixture", now); got != ActivityIdle {
		t.Fatalf("idle activity = %q, want %q", got, ActivityIdle)
	}
}
