// Package workspaces groups sessions automatically by project: the git
// repository root containing the working folder, or the folder itself.
package workspaces

import (
	"os"
	"path/filepath"
	"strings"
)

// MissingKey is the group of sessions whose working folder no longer exists.
const MissingKey = "missing-folder"

// Group identifies one workspace. Key is stable and case-insensitive; Name is
// the display label.
type Group struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Path string `json:"path"`
	Repo bool   `json:"repo"`
}

// Resolve returns the workspace for a working folder. It only looks for a
// `.git` entry on disk and never runs git. A `.git` file (a linked worktree
// or submodule) makes that folder its own workspace.
func Resolve(workingDir string) Group {
	dir := filepath.Clean(workingDir)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return Group{Key: MissingKey, Name: "Missing folders", Path: dir}
	}
	for current := dir; ; {
		if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
			return group(current, true)
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return group(dir, false)
}

func group(root string, repo bool) Group {
	name := filepath.Base(root)
	if name == "." || name == string(filepath.Separator) || strings.HasSuffix(name, ":") || strings.HasSuffix(name, `:\`) {
		name = strings.TrimRight(filepath.VolumeName(root), ":")
	}
	return Group{Key: strings.ToLower(root), Name: name, Path: root, Repo: repo}
}
