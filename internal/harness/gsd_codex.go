package harness

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// samePath compares two directory paths the way Windows does: ignoring case,
// slash direction and a trailing separator.
func samePath(left, right string) bool {
	normalize := func(path string) string {
		path = filepath.ToSlash(filepath.Clean(path))
		return strings.ToLower(strings.TrimRight(path, "/"))
	}
	return left != "" && right != "" && normalize(left) == normalize(right)
}

func userHome() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

// ---- GSD (gsd-pi) ----

// GSDSessionPath is the session file TauShell asks GSD to create and later
// reopen for one TauShell-chosen conversation ID. GSD 1.12.0 added --session.
func GSDSessionPath(harnessSessionID string) string {
	root := os.Getenv("GSD_HOME")
	if root == "" {
		root = filepath.Join(userHome(), ".gsd")
	}
	return filepath.Join(root, "sessions", "tau", harnessSessionID+".jsonl")
}

func gsdSessionArgs(harnessSessionID string) []string {
	path := GSDSessionPath(harnessSessionID)
	// Best effort: GSD creates the folder itself when it can.
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	return []string{"--session", path}
}

// checkGSDResume relies on GSD writing its session file only after the first
// assistant reply, so a missing file means nothing to resume.
func checkGSDResume(harnessSessionID string) error {
	if _, err := os.Stat(GSDSessionPath(harnessSessionID)); errors.Is(err, os.ErrNotExist) {
		return errors.New("GSD has no recorded conversation for this session yet; use Start fresh")
	}
	return nil
}

func gsdLineModel(line []byte) string {
	if !bytes.Contains(line, []byte(`"model`)) {
		return ""
	}
	var entry struct {
		Type    string `json:"type"`
		ModelID string `json:"modelId"`
		Message struct {
			Role  string `json:"role"`
			Model string `json:"model"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &entry) != nil {
		return ""
	}
	switch entry.Type {
	case "model_change":
		return entry.ModelID
	case "message":
		if entry.Message.Role == "assistant" {
			return entry.Message.Model
		}
	}
	return ""
}

// ---- Codex ----

func codexSessionsRoot() string {
	root := os.Getenv("CODEX_HOME")
	if root == "" {
		root = filepath.Join(userHome(), ".codex")
	}
	return filepath.Join(root, "sessions")
}

// codexRolloutFile finds the rollout log of one Codex conversation.
func codexRolloutFile(root, harnessSessionID string) string {
	matches, err := filepath.Glob(filepath.Join(root, "*", "*", "*", "rollout-*-"+harnessSessionID+".jsonl"))
	if err != nil || len(matches) == 0 {
		return ""
	}
	return matches[0]
}

// CodexRolloutFile returns the rollout log for a Codex conversation, or "".
func CodexRolloutFile(harnessSessionID string) string {
	return codexRolloutFile(codexSessionsRoot(), harnessSessionID)
}

// checkCodexResume treats a rollout holding only its session_meta line as a
// session without a conversation. The layout is undocumented, so it fails
// open when the store itself is absent.
func checkCodexResume(harnessSessionID string) error {
	return checkCodexResumeIn(codexSessionsRoot(), harnessSessionID)
}

func checkCodexResumeIn(root, harnessSessionID string) error {
	if _, err := os.Stat(root); err != nil {
		return nil
	}
	path := codexRolloutFile(root, harnessSessionID)
	if path == "" {
		return errors.New("Codex has no recorded conversation for this session; use Start fresh")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if bytes.Count(bytes.TrimSpace(data), []byte("\n")) < 1 {
		return errors.New("Codex has no recorded conversation for this session yet; use Start fresh")
	}
	return nil
}

type codexMeta struct {
	Type    string `json:"type"`
	Payload struct {
		ID     string `json:"id"`
		CWD    string `json:"cwd"`
		Source any    `json:"source"`
	} `json:"payload"`
}

func readFirstLine(path string) []byte {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	buffer := make([]byte, 64<<10)
	count, _ := file.Read(buffer)
	buffer = buffer[:count]
	if end := bytes.IndexByte(buffer, '\n'); end >= 0 {
		buffer = buffer[:end]
	}
	return bytes.TrimSpace(buffer)
}

func discoverCodexSession(workingDir string, since time.Time, taken map[string]bool) (string, error) {
	return discoverCodexSessionIn(codexSessionsRoot(), workingDir, since, taken)
}

func discoverCodexSessionIn(root, workingDir string, since time.Time, taken map[string]bool) (string, error) {
	files, _ := filepath.Glob(filepath.Join(root, "*", "*", "*", "rollout-*.jsonl"))
	var found []string
	for _, path := range files {
		info, err := os.Stat(path)
		if err != nil || info.ModTime().Before(since.Add(-2*time.Second)) {
			continue
		}
		var meta codexMeta
		if json.Unmarshal(readFirstLine(path), &meta) != nil || meta.Type != "session_meta" {
			continue
		}
		if meta.Payload.ID == "" || taken[meta.Payload.ID] || !samePath(meta.Payload.CWD, workingDir) {
			continue
		}
		// Sub-agent and review conversations carry an object source.
		if _, nested := meta.Payload.Source.(map[string]any); nested {
			continue
		}
		found = append(found, meta.Payload.ID)
	}
	switch len(found) {
	case 0:
		return "", nil
	case 1:
		return found[0], nil
	default:
		return "", ErrAmbiguousSession
	}
}

func codexLineModel(line []byte) string {
	if !bytes.Contains(line, []byte(`"turn_context"`)) {
		return ""
	}
	var entry struct {
		Type    string `json:"type"`
		Payload struct {
			Model string `json:"model"`
		} `json:"payload"`
	}
	if json.Unmarshal(line, &entry) != nil || entry.Type != "turn_context" {
		return ""
	}
	return entry.Payload.Model
}
