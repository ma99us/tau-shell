package harness

import (
	"strings"
	"testing"
	"time"
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
