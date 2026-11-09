package harness

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"

	"github.com/ma99us/tau-shell/internal/harness/manifest"
	"github.com/ma99us/tau-shell/internal/session"
)

const maxTitleRunes = 120

// ConversationTitle returns the title the harness itself recorded for a
// conversation, or "" when the harness keeps none or its layout is not
// recognised. Only the title is read: no conversation content is returned.
func ConversationTitle(tool session.Harness, harnessSessionID string) string {
	if harnessSessionID == "" {
		return ""
	}
	item, ok := manifest.Get(string(tool))
	if !ok {
		return ""
	}
	switch item.Title {
	case "claude-ai-title":
		root := os.Getenv("CLAUDE_CONFIG_DIR")
		if root == "" {
			home, err := HomeDir()
			if err != nil {
				return ""
			}
			root = filepath.Join(home, ".claude")
		}
		return claudeTitleIn(filepath.Join(root, "projects"), harnessSessionID)
	case "copilot-workspace":
		home, err := HomeDir()
		if err != nil {
			return ""
		}
		return copilotTitleIn(filepath.Join(home, ".copilot", "session-state"), harnessSessionID)
	default:
		return ""
	}
}

func cleanTitle(title string) string {
	title = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, title)
	title = strings.Join(strings.Fields(title), " ")
	if runes := []rune(title); len(runes) > maxTitleRunes {
		title = string(runes[:maxTitleRunes]) + "..."
	}
	return title
}

func claudeTitleIn(projectsRoot, harnessSessionID string) string {
	matches, err := filepath.Glob(filepath.Join(projectsRoot, "*", harnessSessionID+".jsonl"))
	if err != nil || len(matches) == 0 {
		return ""
	}
	return cleanTitle(lastModel(matches[0], claudeLineTitle))
}

// claudeLineTitle reads Claude Code's {"type":"ai-title","aiTitle":...} lines.
func claudeLineTitle(line []byte) string {
	if !bytes.HasPrefix(line, []byte(`{"type":"ai-title"`)) {
		return ""
	}
	var entry struct {
		AITitle string `json:"aiTitle"`
	}
	if json.Unmarshal(line, &entry) != nil {
		return ""
	}
	return entry.AITitle
}

func copilotTitleIn(root, harnessSessionID string) string {
	data, err := os.ReadFile(filepath.Join(root, harnessSessionID, "workspace.yaml"))
	if err != nil || len(data) > 64<<10 {
		return ""
	}
	var workspace struct {
		Name string `yaml:"name"`
	}
	if yaml.Unmarshal(data, &workspace) != nil {
		return ""
	}
	return cleanTitle(workspace.Name)
}
