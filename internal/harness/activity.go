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
	if !ok {
		return ""
	}
	switch item.Activity {
	case "copilot-events":
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		lines, ok := readActivityTail(filepath.Join(home, ".copilot", "session-state", harnessSessionID, "events.jsonl"))
		if !ok {
			return ""
		}
		return copilotActivityFromLines(lines, now)
	case "codex-rollout":
		path := CodexRolloutFile(harnessSessionID)
		if path == "" {
			return ""
		}
		lines, ok := readActivityTail(path)
		if !ok {
			return ""
		}
		return codexActivityFromLines(lines, now)
	case "gsd-jsonl":
		lines, ok := readActivityTail(GSDSessionPath(harnessSessionID))
		if !ok {
			return ""
		}
		return gsdActivityFromLines(lines, now)
	default:
		return ""
	}
}

func readActivityTail(path string) ([][]byte, bool) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, false
	}
	start := info.Size() - activityTailBytes
	if start < 0 {
		start = 0
	}
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return nil, false
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, false
	}
	return bytes.Split(data, []byte("\n")), true
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

func codexActivityFromLines(lines [][]byte, now time.Time) string {
	state, lastEvent := "", time.Time{}
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var entry struct {
			Type      string `json:"type"`
			Timestamp string `json:"timestamp"`
			Payload   struct {
				Type string `json:"type"`
			} `json:"payload"`
		}
		if json.Unmarshal(line, &entry) != nil || entry.Type != "event_msg" {
			continue
		}
		var next string
		switch entry.Payload.Type {
		case "task_started":
			next = ActivityWorking
		case "task_complete":
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

func gsdActivityFromLines(lines [][]byte, now time.Time) string {
	state, lastEvent := "", time.Time{}
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var entry struct {
			Type      string `json:"type"`
			Timestamp string `json:"timestamp"`
			Message   struct {
				Role       string `json:"role"`
				StopReason string `json:"stopReason"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &entry) != nil || entry.Type != "message" {
			continue
		}
		var next string
		switch entry.Message.Role {
		case "user", "tool":
			next = ActivityWorking
		case "assistant":
			if entry.Message.StopReason == "toolUse" {
				next = ActivityWorking
			} else if entry.Message.StopReason == "stop" || entry.Message.StopReason == "length" ||
				entry.Message.StopReason == "error" || entry.Message.StopReason == "aborted" {
				next = ActivityIdle
			}
		default:
			continue
		}
		if next == "" {
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
