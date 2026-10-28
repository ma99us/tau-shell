package usage

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ma99us/tau-shell/internal/harness"
	"github.com/ma99us/tau-shell/internal/session"
)

// RecordsOwnCost reports whether a harness logs its own USD cost per call, so
// the ledger stores that cost instead of pricing tokens from a table.
func RecordsOwnCost(tool session.Harness) bool {
	switch tool {
	case session.CopilotCLI, session.GSD, session.OpenCode:
		return true
	default:
		return false
	}
}

// ParseGSDLine reads usage from one GSD session JSONL line. GSD records an
// estimated USD cost with every assistant message.
func ParseGSDLine(line []byte) (Record, bool) {
	if !bytes.Contains(line, []byte(`"usage"`)) || !bytes.Contains(line, []byte(`"assistant"`)) {
		return Record{}, false
	}
	var entry struct {
		Timestamp string `json:"timestamp"`
		Message   struct {
			Role  string `json:"role"`
			Model string `json:"model"`
			Usage *struct {
				Input      int64 `json:"input"`
				Output     int64 `json:"output"`
				CacheRead  int64 `json:"cacheRead"`
				CacheWrite int64 `json:"cacheWrite"`
				Cost       *struct {
					Total float64 `json:"total"`
				} `json:"cost"`
			} `json:"usage"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &entry) != nil || entry.Message.Role != "assistant" || entry.Message.Usage == nil {
		return Record{}, false
	}
	usage := entry.Message.Usage
	record := Record{
		Time:         parseTime(entry.Timestamp),
		Model:        entry.Message.Model,
		Input:        usage.Input,
		Output:       usage.Output,
		CacheRead:    usage.CacheRead,
		CacheWrite5m: usage.CacheWrite,
	}
	if usage.Cost != nil && usage.Cost.Total > 0 {
		record.USD = usage.Cost.Total
		record.Priced = true
	}
	if record.Model == "" || record.Input+record.Output+record.CacheRead+record.CacheWrite5m == 0 {
		return Record{}, false
	}
	return record, true
}

// codexParser reads usage from Codex rollout lines. Token counts arrive in
// event_msg lines without a model, so the parser tracks the model announced by
// the latest turn_context line.
type codexParser struct{ model string }

func newCodexParser(seedModel string) *codexParser { return &codexParser{model: seedModel} }

func (parser *codexParser) parse(line []byte) (Record, bool) {
	switch {
	case bytes.Contains(line, []byte(`"turn_context"`)):
		var entry struct {
			Type    string `json:"type"`
			Payload struct {
				Model string `json:"model"`
			} `json:"payload"`
		}
		if json.Unmarshal(line, &entry) == nil && entry.Type == "turn_context" && entry.Payload.Model != "" {
			parser.model = entry.Payload.Model
		}
		return Record{}, false
	case bytes.Contains(line, []byte(`"token_count"`)):
		var entry struct {
			Timestamp string `json:"timestamp"`
			Payload   struct {
				Type string `json:"type"`
				Info *struct {
					Total struct {
						Total int64 `json:"total_tokens"`
					} `json:"total_token_usage"`
					Last struct {
						Input     int64 `json:"input_tokens"`
						Cached    int64 `json:"cached_input_tokens"`
						Output    int64 `json:"output_tokens"`
						Reasoning int64 `json:"reasoning_output_tokens"`
					} `json:"last_token_usage"`
				} `json:"info"`
			} `json:"payload"`
		}
		if json.Unmarshal(line, &entry) != nil || entry.Payload.Type != "token_count" || entry.Payload.Info == nil || parser.model == "" {
			return Record{}, false
		}
		last := entry.Payload.Info.Last
		// input_tokens includes the cached tokens, and the cumulative total
		// identifies repeated reports of the same call.
		record := Record{
			MessageID: fmt.Sprintf("total:%d", entry.Payload.Info.Total.Total),
			Time:      parseTime(entry.Timestamp),
			Model:     parser.model,
			Input:     max(last.Input-last.Cached, 0),
			Output:    last.Output,
			CacheRead: last.Cached,
		}
		if record.Input+record.Output+record.CacheRead == 0 {
			return Record{}, false
		}
		return record, true
	}
	return Record{}, false
}

// codexModelBefore finds the model of the last turn_context line in the first
// offset bytes of a rollout, so a later read resumes with the right model.
func codexModelBefore(path string, offset int64) string {
	if offset <= 0 {
		return ""
	}
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	reader := bufio.NewReaderSize(io.LimitReader(file, offset), 256<<10)
	parser := newCodexParser("")
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			parser.parse(bytes.TrimSpace(line))
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return parser.model
			}
			return parser.model
		}
	}
}

func harnessLogFiles(tool session.Harness, harnessSessionID string) []string {
	var path string
	switch tool {
	case session.GSD:
		path = harness.GSDSessionPath(harnessSessionID)
	case session.Codex:
		path = harness.CodexRolloutFile(harnessSessionID)
	default:
		return nil
	}
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	return []string{path}
}

// openCodeSource is the ledger key for an OpenCode conversation, which lives
// in a database rather than a log file.
func openCodeSource(harnessSessionID string) string {
	return "opencode://" + strings.TrimSpace(harnessSessionID)
}

// IngestOpenCode replaces the stored usage of one OpenCode conversation when
// its message version changed. OpenCode edits messages in place, so the rows
// are recomputed from all finished assistant messages.
func (ledger *Ledger) IngestOpenCode(sessionID, harnessSessionID string) error {
	messages, version, err := harness.OpenCodeMessages(harnessSessionID)
	if err != nil {
		return err
	}
	source := openCodeSource(harnessSessionID)
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	var stored int64
	known := true
	err = ledger.db.QueryRow(`SELECT offset FROM usage_files WHERE path = ?`, source).Scan(&stored)
	if errors.Is(err, sql.ErrNoRows) {
		known = false
	} else if err != nil {
		return fmt.Errorf("read usage file state: %w", err)
	}
	if known && stored == version {
		return nil
	}
	deltas := map[string]*delta{}
	days := map[string]string{}
	models := map[string]string{}
	for _, message := range messages {
		day := message.Created.Local().Format(dayLayout)
		key := message.Model + "\x00" + day
		current := deltas[key]
		if current == nil {
			current = &delta{}
			deltas[key] = current
			days[key], models[key] = day, message.Model
		}
		current.input += message.Input
		current.output += message.Output + message.Reasoning
		current.cacheRead += message.CacheRead
		current.write5m += message.CacheWrite
		current.records++
		current.usd += message.USD
	}
	return ledger.commit(source, sessionID, session.OpenCode, harnessSessionID, version, "", true, deltas, days, models)
}
