package harness

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ma99us/tau-shell/internal/harness/manifest"
	"github.com/ma99us/tau-shell/internal/session"
)

const activityTailBytes = 128 << 10

// copilotTurnGap is how long after a turn ends before it counts as idle: a
// multi-step reply ends one turn and starts the next within moments.
const copilotTurnGap = 2 * time.Second

// Activity values reported from a harness's own log.
const (
	ActivityWorking = "working"
	ActivityIdle    = "idle"
)

// CurrentActivity reports whether the harness is working or idle according to
// its own session log, or "" when the log gives no answer (unsupported harness,
// no conversation yet, unreadable file). It is a hint for the window, never an
// error.
func CurrentActivity(tool session.Harness, harnessSessionID string, now time.Time) string {
	if harnessSessionID == "" {
		return ""
	}
	item, ok := manifest.Get(string(tool))
	if !ok || item.Activity != "copilot-events" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return copilotActivity(filepath.Join(home, ".copilot", "session-state", harnessSessionID, "events.jsonl"), now)
}

func copilotActivity(path string, now time.Time) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return ""
	}
	start := info.Size() - activityTailBytes
	if start < 0 {
		start = 0
	}
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return ""
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return ""
	}
	return copilotActivityFromLines(bytes.Split(data, []byte("\n")), now)
}

// copilotActivityFromLines walks the lifecycle events: a user message, a turn
// start or a tool call means work is under way; a turn end with nothing after
// it for copilotTurnGap means the reply is finished.
func copilotActivityFromLines(lines [][]byte, now time.Time) string {
	state, lastEvent := "", time.Time{}
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte(`{"type":"`)) {
			continue
		}
		var entry struct {
			Type      string `json:"type"`
			Timestamp string `json:"timestamp"`
		}
		if json.Unmarshal(line, &entry) != nil {
			continue
		}
		var next string
		switch entry.Type {
		case "user.message", "assistant.turn_start", "tool.execution_start", "tool.execution_complete":
			next = ActivityWorking
		case "assistant.turn_end":
			next = ActivityIdle
		default:
			continue
		}
		stamp, err := time.Parse(time.RFC3339Nano, entry.Timestamp)
		if err != nil {
			continue
		}
		state, lastEvent = next, stamp
	}
	if state == ActivityIdle && now.Sub(lastEvent) < copilotTurnGap {
		return ActivityWorking
	}
	return state
}
