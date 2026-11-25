package harness

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// LatestSessionFinder is implemented by adapters that can notice a
// conversation the harness started on its own (for example after /clear or
// /new) from the harness's own state. It is best effort: an adapter without a
// reliable signal does not implement it, so only conversations TauShell
// starts are tracked for that harness.
type LatestSessionFinder interface {
	// LatestSessionIn returns the newest conversation in workingDir that was
	// written at or after since and is not in taken. It returns "" when there
	// is none and ErrAmbiguousSession when several recent candidates compete.
	LatestSessionIn(home, workingDir string, since time.Time, taken map[string]bool) (string, error)
}

// latestTie is how close two candidates' last writes may be before neither
// can be attributed to this session (two harness instances in one folder).
const latestTie = 5 * time.Second

func (adapter cliAdapter) LatestSessionIn(home, workingDir string, since time.Time, taken map[string]bool) (string, error) {
	if adapter.DiscoversSessionID() {
		if home != "" {
			return "", nil
		}
		return adapter.DiscoverSessionID(workingDir, since, taken)
	}
	switch adapter.manifest.ResumeCheck {
	case "claude-transcript":
		root := ClaudeConfigRoot(home)
		if root == "" {
			return "", nil
		}
		return latestClaudeSessionIn(filepath.Join(root, "projects"), workingDir, since, taken)
	case "copilot-session-state":
		resolved, err := SessionHome(home)
		if err != nil {
			return "", nil
		}
		return latestCopilotSessionIn(filepath.Join(resolved, ".copilot", "session-state"), workingDir, since, taken)
	}
	return "", nil
}

type latestCandidate struct {
	id       string
	modified time.Time
}

// newestUnique picks the most recently written candidate, or reports an
// ambiguity when the runner-up was written within latestTie of it.
func newestUnique(candidates []latestCandidate) (string, error) {
	var best, second *latestCandidate
	for index := range candidates {
		candidate := &candidates[index]
		switch {
		case best == nil || candidate.modified.After(best.modified):
			best, second = candidate, best
		case second == nil || candidate.modified.After(second.modified):
			second = candidate
		}
	}
	if best == nil {
		return "", nil
	}
	if second != nil && best.modified.Sub(second.modified) < latestTie {
		return "", ErrAmbiguousSession
	}
	return best.id, nil
}

var claudeNonAlnum = regexp.MustCompile(`[^A-Za-z0-9]`)

// claudeProjectFolder is the folder name Claude Code derives from a working
// directory: every character that is not a letter or digit becomes "-".
func claudeProjectFolder(workingDir string) string {
	return claudeNonAlnum.ReplaceAllString(workingDir, "-")
}

func latestClaudeSessionIn(projectsRoot, workingDir string, since time.Time, taken map[string]bool) (string, error) {
	if workingDir == "" {
		return "", nil
	}
	entries, err := os.ReadDir(filepath.Join(projectsRoot, claudeProjectFolder(workingDir)))
	if err != nil {
		return "", nil
	}
	var candidates []latestCandidate
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		id := strings.TrimSuffix(name, ".jsonl")
		if taken[id] {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().Before(since) {
			continue
		}
		candidates = append(candidates, latestCandidate{id, info.ModTime()})
	}
	return newestUnique(candidates)
}

func latestCopilotSessionIn(root, workingDir string, since time.Time, taken map[string]bool) (string, error) {
	if workingDir == "" {
		return "", nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", nil
	}
	var candidates []latestCandidate
	for _, entry := range entries {
		id := entry.Name()
		if !entry.IsDir() || taken[id] {
			continue
		}
		info, err := os.Stat(filepath.Join(root, id, "events.jsonl"))
		if err != nil || info.ModTime().Before(since) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, id, "workspace.yaml"))
		if err != nil || len(data) > 64<<10 {
			continue
		}
		var workspace struct {
			Cwd string `yaml:"cwd"`
		}
		if yaml.Unmarshal(data, &workspace) != nil || !samePath(workspace.Cwd, workingDir) {
			continue
		}
		candidates = append(candidates, latestCandidate{id, info.ModTime()})
	}
	return newestUnique(candidates)
}
