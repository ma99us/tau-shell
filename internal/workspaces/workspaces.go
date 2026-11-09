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
// or submodule) makes that folder its own workspace, except that a linked
// worktree joins the workspace of its main checkout.
func Resolve(workingDir string) Group {
	dir := filepath.Clean(workingDir)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return Group{Key: MissingKey, Name: "Missing folders", Path: dir}
	}
	for current := dir; ; {
		dotGit := filepath.Join(current, ".git")
		if info, err := os.Stat(dotGit); err == nil {
			if !info.IsDir() {
				if main, ok := worktreeMain(current, dotGit); ok {
					return group(main, true)
				}
			}
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

// worktreeMain returns the main working tree of a linked worktree, read from
// the gitdir pointer in its .git file (<main>/.git/worktrees/<name>). A
// submodule pointer (.git/modules/...) has no such shape and stays its own workspace.
func worktreeMain(folder, dotGit string) (string, bool) {
	data, err := os.ReadFile(dotGit)
	if err != nil {
		return "", false
	}
	target, found := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:")
	if !found {
		return "", false
	}
	target = filepath.FromSlash(strings.TrimSpace(target))
	if !filepath.IsAbs(target) {
		target = filepath.Join(folder, target)
	}
	target = filepath.Clean(target)
	gitDir := filepath.Dir(filepath.Dir(target))
	if !strings.EqualFold(filepath.Base(filepath.Dir(target)), "worktrees") || !strings.EqualFold(filepath.Base(gitDir), ".git") {
		return "", false
	}
	main := filepath.Dir(gitDir)
	if info, err := os.Stat(main); err != nil || !info.IsDir() {
		return "", false
	}
	return main, true
}

func group(root string, repo bool) Group {
	name := filepath.Base(root)
	if name == "." || name == string(filepath.Separator) || strings.HasSuffix(name, ":") || strings.HasSuffix(name, `:\`) {
		name = strings.TrimRight(filepath.VolumeName(root), ":")
	}
	return Group{Key: strings.ToLower(root), Name: name, Path: root, Repo: repo}
}
