package usage

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ma99us/tau-shell/internal/session"
)

const largePromptTokens = 100_000

// Record is one model call's usage as logged by a harness.
type Record struct {
	MessageID    string
	Time         time.Time
	Model        string
	Input        int64
	Output       int64
	CacheRead    int64
	CacheWrite5m int64
	CacheWrite1h int64
	// USD and Priced are set only for harnesses that log their own cost.
	USD    float64
	Priced bool
}

// ParseClaudeLine reads usage from one Claude Code transcript line. Streamed
// chunks of one reply share a message ID and the last chunk is the full count,
// so callers must replace, not add, records with the same ID.
func ParseClaudeLine(line []byte) (Record, bool) {
	if !bytes.Contains(line, []byte(`"usage"`)) || !bytes.Contains(line, []byte(`"assistant"`)) {
		return Record{}, false
	}
	var entry struct {
		Type      string `json:"type"`
		Timestamp string `json:"timestamp"`
		Message   struct {
			ID    string `json:"id"`
			Model string `json:"model"`
			Usage *struct {
				Input         int64 `json:"input_tokens"`
				Output        int64 `json:"output_tokens"`
				CacheRead     int64 `json:"cache_read_input_tokens"`
				CacheCreation int64 `json:"cache_creation_input_tokens"`
				Creation      *struct {
					Write5m int64 `json:"ephemeral_5m_input_tokens"`
					Write1h int64 `json:"ephemeral_1h_input_tokens"`
				} `json:"cache_creation"`
			} `json:"usage"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &entry) != nil || entry.Type != "assistant" || entry.Message.Usage == nil {
		return Record{}, false
	}
	usage := entry.Message.Usage
	model := entry.Message.Model
	if model == "" || strings.HasPrefix(model, "<") {
		return Record{}, false
	}
	record := Record{
		MessageID: entry.Message.ID,
		Time:      parseTime(entry.Timestamp),
		Model:     model,
		Input:     usage.Input,
		Output:    usage.Output,
		CacheRead: usage.CacheRead,
	}
	record.CacheWrite5m = usage.CacheCreation
	if usage.Creation != nil && usage.Creation.Write5m+usage.Creation.Write1h > 0 {
		record.CacheWrite5m = usage.Creation.Write5m
		record.CacheWrite1h = usage.Creation.Write1h
	}
	if record.Input+record.Output+record.CacheRead+record.CacheWrite5m+record.CacheWrite1h == 0 {
		return Record{}, false
	}
	if key, ok := NormalizeModel(model); ok && key == "haiku-5.5" &&
		record.Input+record.CacheRead+record.CacheWrite5m+record.CacheWrite1h > largePromptTokens {
		record.Model += largePromptSuffix
	}
	return record, true
}

// ParseCopilotLine reads usage from one Copilot CLI events.jsonl line. The
// cost comes from the log (nanoAIU; 1 AI credit = USD 0.01), not a price table.
func ParseCopilotLine(line []byte) (Record, bool) {
	if !bytes.HasPrefix(line, []byte(`{"type":"session.usage_record"`)) {
		return Record{}, false
	}
	var entry struct {
		Timestamp string `json:"timestamp"`
		Data      struct {
			Usage struct {
				Model        string `json:"model"`
				InputTokens  int64  `json:"inputTokens"`
				OutputTokens int64  `json:"outputTokens"`
				CacheRead    int64  `json:"cacheReadTokens"`
				CacheWrite   int64  `json:"cacheWriteTokens"`
				IsByok       bool   `json:"isByok"`
				CopilotUsage *struct {
					TotalNanoAiu float64 `json:"totalNanoAiu"`
					TokenDetails []struct {
						TokenType  string `json:"tokenType"`
						TokenCount int64  `json:"tokenCount"`
					} `json:"tokenDetails"`
				} `json:"copilotUsage"`
			} `json:"usage"`
		} `json:"data"`
	}
	if json.Unmarshal(line, &entry) != nil {
		return Record{}, false
	}
	usage := entry.Data.Usage
	record := Record{Time: parseTime(entry.Timestamp), Model: usage.Model}
	if usage.CopilotUsage != nil && len(usage.CopilotUsage.TokenDetails) > 0 {
		for _, detail := range usage.CopilotUsage.TokenDetails {
			switch detail.TokenType {
			case "input":
				record.Input += detail.TokenCount
			case "output":
				record.Output += detail.TokenCount
			case "cache_read":
				record.CacheRead += detail.TokenCount
			case "cache_write":
				record.CacheWrite5m += detail.TokenCount
			}
		}
	} else {
		record.Output = usage.OutputTokens
		record.CacheRead = usage.CacheRead
		record.CacheWrite5m = usage.CacheWrite
		if input := usage.InputTokens - usage.CacheRead - usage.CacheWrite; input > 0 {
			record.Input = input
		}
	}
	if usage.CopilotUsage != nil && usage.CopilotUsage.TotalNanoAiu > 0 && !usage.IsByok {
		record.USD = usage.CopilotUsage.TotalNanoAiu / 1e11
		record.Priced = true
	}
	if record.Input+record.Output+record.CacheRead+record.CacheWrite5m == 0 {
		return Record{}, false
	}
	return record, true
}

func parseTime(value string) time.Time {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed
	}
	return time.Now()
}

func claudeRoot() string {
	if root := os.Getenv("CLAUDE_CONFIG_DIR"); root != "" {
		return root
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude")
}

// LogFiles lists the log files holding usage for one harness conversation:
// the main transcript plus Claude subagent transcripts, or Copilot's events
// log. Files that do not exist are omitted.
func LogFiles(tool session.Harness, harnessSessionID string) []string {
	if harnessSessionID == "" {
		return nil
	}
	switch tool {
	case session.ClaudeCode:
		root := claudeRoot()
		if root == "" {
			return nil
		}
		projects := filepath.Join(root, "projects")
		main, _ := filepath.Glob(filepath.Join(projects, "*", harnessSessionID+".jsonl"))
		sub, _ := filepath.Glob(filepath.Join(projects, "*", harnessSessionID, "subagents", "*.jsonl"))
		return append(main, sub...)
	case session.CopilotCLI:
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		path := filepath.Join(home, ".copilot", "session-state", harnessSessionID, "events.jsonl")
		if _, err := os.Stat(path); err != nil {
			return nil
		}
		return []string{path}
	default:
		return nil
	}
}

// LineParser returns the record parser for a harness.
func LineParser(tool session.Harness) func([]byte) (Record, bool) {
	switch tool {
	case session.ClaudeCode:
		return ParseClaudeLine
	case session.CopilotCLI:
		return ParseCopilotLine
	default:
		return nil
	}
}
