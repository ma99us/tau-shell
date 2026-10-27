package harness

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ma99us/tau-shell/internal/session"
)

const (
	modelTailBytes = 1 << 20
	modelHeadBytes = 256 << 10
)

// CurrentModel reports the model the harness last recorded for
// harnessSessionID in its own session log, or "" when it is unknown. The log
// layouts are undocumented internals, so every failure yields "" instead of an
// error and nothing is ever guessed.
func CurrentModel(tool session.Harness, harnessSessionID string) string {
	if harnessSessionID == "" {
		return ""
	}
	switch tool {
	case session.ClaudeCode:
		root := os.Getenv("CLAUDE_CONFIG_DIR")
		if root == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return ""
			}
			root = filepath.Join(home, ".claude")
		}
		return claudeModelIn(filepath.Join(root, "projects"), harnessSessionID)
	case session.CopilotCLI:
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return copilotModelIn(filepath.Join(home, ".copilot", "session-state"), harnessSessionID)
	default:
		return ""
	}
}

func claudeModelIn(projectsRoot, harnessSessionID string) string {
	matches, err := filepath.Glob(filepath.Join(projectsRoot, "*", harnessSessionID+".jsonl"))
	if err != nil || len(matches) == 0 {
		return ""
	}
	return lastModel(matches[0], claudeLineModel)
}

func copilotModelIn(root, harnessSessionID string) string {
	return lastModel(filepath.Join(root, harnessSessionID, "events.jsonl"), copilotLineModel)
}

// lastModel returns the last model found by lineModel in the tail of the file,
// falling back to the head (where Copilot records its initial model).
func lastModel(path string, lineModel func([]byte) string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return ""
	}
	start := info.Size() - modelTailBytes
	if start < 0 {
		start = 0
	}
	if model := scanModel(file, start, lineModel); model != "" {
		return model
	}
	if start > 0 {
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return ""
		}
		return scanModel(io.LimitReader(file, modelHeadBytes), 0, lineModel)
	}
	return ""
}

func scanModel(source io.Reader, start int64, lineModel func([]byte) string) string {
	if seeker, ok := source.(io.Seeker); ok {
		if _, err := seeker.Seek(start, io.SeekStart); err != nil {
			return ""
		}
	}
	reader := bufio.NewReaderSize(source, 64<<10)
	if start > 0 {
		// The first line of a mid-file read is usually cut off.
		if _, err := reader.ReadBytes('\n'); err != nil {
			return ""
		}
	}
	model := ""
	for {
		line, err := reader.ReadBytes('\n')
		if found := lineModel(bytes.TrimSpace(line)); found != "" {
			model = found
		}
		if err != nil {
			return model
		}
	}
}

// claudeModelCommand picks up a /model switch, which Claude Code logs as
// "<local-command-stdout>Set model to `Opus 5.5` ..." (a display name, not a
// model ID) before any assistant reply uses the new model.
func claudeModelCommand(line []byte) string {
	const marker = "<local-command-stdout>Set model to `"
	index := bytes.Index(line, []byte(marker))
	if index < 0 {
		return ""
	}
	rest := line[index+len(marker):]
	end := bytes.IndexByte(rest, '`')
	if end <= 0 {
		return ""
	}
	return string(rest[:end])
}

func claudeLineModel(line []byte) string {
	if model := claudeModelCommand(line); model != "" {
		return model
	}
	if !bytes.Contains(line, []byte(`"model`)) {
		return ""
	}
	var entry struct {
		Message struct {
			Model string `json:"model"`
		} `json:"message"`
		Attachment struct {
			Type     string `json:"type"`
			Identity struct {
				ModelID string `json:"modelId"`
			} `json:"identity"`
		} `json:"attachment"`
	}
	if json.Unmarshal(line, &entry) != nil {
		return ""
	}
	model := entry.Message.Model
	if model == "" && entry.Attachment.Type == "model" {
		model = entry.Attachment.Identity.ModelID
	}
	if strings.HasPrefix(model, "<") {
		return ""
	}
	return model
}

func copilotLineModel(line []byte) string {
	if !bytes.HasPrefix(line, []byte(`{"type":"session.`)) {
		return ""
	}
	var entry struct {
		Type string `json:"type"`
		Data struct {
			SelectedModel string `json:"selectedModel"`
			NewModel      string `json:"newModel"`
		} `json:"data"`
	}
	if json.Unmarshal(line, &entry) != nil {
		return ""
	}
	switch entry.Type {
	case "session.start":
		return entry.Data.SelectedModel
	case "session.model_change":
		return entry.Data.NewModel
	}
	return ""
}
