package harness

import (
	"os"
	"path/filepath"
)

// SessionHome resolves the home a session's readers use. An empty home means
// the default (Windows) profile; a WSL session passes its distro home.
func SessionHome(home string) (string, error) {
	if home != "" {
		return home, nil
	}
	return HomeDir()
}

// ClaudeConfigRoot returns Claude Code's config folder for a session home, or
// "" when it cannot be determined. CLAUDE_CONFIG_DIR is a Windows variable, so
// it only applies to the default home.
func ClaudeConfigRoot(home string) string {
	if home == "" {
		if root := os.Getenv("CLAUDE_CONFIG_DIR"); root != "" {
			return root
		}
	}
	resolved, err := SessionHome(home)
	if err != nil {
		return ""
	}
	return filepath.Join(resolved, ".claude")
}
