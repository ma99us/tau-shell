//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ma99us/tau-shell/internal/diagnostics"
	"github.com/ma99us/tau-shell/internal/presets"
	"github.com/ma99us/tau-shell/internal/projectfiles"
	"github.com/ma99us/tau-shell/internal/session"
)

// GitPathInfo is what the file drawer needs to offer git actions for one file or folder.
type GitPathInfo struct {
	Repo     bool `json:"repo"`
	Tracked  bool `json:"tracked"`
	Staged   bool `json:"staged"`
	Unstaged bool `json:"unstaged"`
	Ignored  bool `json:"ignored"`
}

// literal keeps git from reading a file name as a glob.
func literal(rel string) string { return ":(literal)" + filepath.ToSlash(rel) }

// GetGitPathInfo reports the git state of one project file or folder. Repo is false outside a repository.
func (application *App) GetGitPathInfo(sessionID, rel string) (GitPathInfo, error) {
	config, err := session.FindConfig(application.configDirectory, sessionID)
	if err != nil {
		return GitPathInfo{}, err
	}
	if _, err := application.resolveProjectPath(sessionID, rel); err != nil {
		return GitPathInfo{}, err
	}
	ctx := gitContext(config)
	dir := config.WorkingDir
	if _, err := presets.RunGit(ctx, dir, nil, "rev-parse", "--show-prefix"); err != nil {
		return GitPathInfo{}, nil
	}
	info := GitPathInfo{Repo: true}
	if out, err := presets.RunGit(ctx, dir, nil, "ls-files", "-z", "--", literal(rel)); err == nil {
		info.Tracked = strings.TrimSpace(out) != ""
	}
	if out, err := presets.RunGit(ctx, dir, nil, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--", literal(rel)); err == nil {
		fields := strings.Split(out, "\x00")
		for index := 0; index < len(fields); index++ {
			entry := fields[index]
			if len(entry) < 4 {
				continue
			}
			x, y := entry[0], entry[1]
			if x == 'R' || x == 'C' {
				index++
			}
			if x == '?' || x == '!' {
				info.Unstaged = info.Unstaged || x == '?'
				continue
			}
			info.Staged = info.Staged || x != ' '
			info.Unstaged = info.Unstaged || y != ' '
		}
	}
	_, err = presets.RunGit(ctx, dir, nil, "check-ignore", "-q", "--", filepath.ToSlash(rel))
	info.Ignored = err == nil
	return info, nil
}

func (application *App) resolveProjectPath(sessionID, rel string) (string, error) {
	root, err := application.projectRoot(sessionID)
	if err != nil {
		return "", err
	}
	if rel == "" {
		return "", errors.New("no path given")
	}
	return projectfiles.Resolve(root, rel)
}

// GitPathAction changes the index or .gitignore for one file or folder. action is
// stage, unstage, untrack (git rm --cached: the file stays on disk, and the removal
// reaches the remote on the next commit and push) or ignore. It returns a short summary.
func (application *App) GitPathAction(sessionID, rel, action string) (summary string, err error) {
	defer func() {
		if err != nil {
			diagnostics.Error("git path action failed", err, "session_id", sessionID, "action", action)
		}
	}()
	config, err := session.FindConfig(application.configDirectory, sessionID)
	if err != nil {
		return "", err
	}
	full, err := application.resolveProjectPath(sessionID, rel)
	if err != nil {
		return "", err
	}
	ctx := gitContext(config)
	dir := config.WorkingDir
	run := func(args ...string) error {
		_, err := presets.RunGit(ctx, dir, []string{"GIT_TERMINAL_PROMPT=0"}, args...)
		return err
	}
	name := filepath.ToSlash(rel)
	switch action {
	case "stage":
		if err := run("add", "--all", "--", literal(rel)); err != nil {
			return "", err
		}
		return "Staged " + name + ".", nil
	case "unstage":
		if err := run("reset", "-q", "--", literal(rel)); err != nil {
			return "", err
		}
		return "Unstaged " + name + ".", nil
	case "untrack":
		if err := run("rm", "--cached", "-r", "-q", "--ignore-unmatch", "--", literal(rel)); err != nil {
			return "", err
		}
		return "Stopped tracking " + name + "; the file stays on disk. Sync changes to remove it from the remote.", nil
	case "ignore":
		root, err := application.projectRoot(sessionID)
		if err != nil {
			return "", err
		}
		prefix, err := presets.RunGit(ctx, dir, nil, "rev-parse", "--show-prefix")
		if err != nil {
			return "", err
		}
		return addToGitignore(root, strings.Trim(strings.TrimSpace(prefix), "/"), full, name)
	}
	return "", fmt.Errorf("unknown git action %q", action)
}

// addToGitignore appends the path to the nearest existing .gitignore at or above
// the path's folder, up to the repository root; with none, it creates one at the
// repository root. prefix is the working directory's path inside the repository.
func addToGitignore(workRoot, prefix, full, name string) (string, error) {
	top := workRoot
	if prefix != "" {
		for range strings.Split(prefix, "/") {
			top = filepath.Dir(top)
		}
	}
	info, err := os.Stat(full)
	if err != nil {
		return "", err
	}
	folder := full
	if !info.IsDir() {
		folder = filepath.Dir(full)
	}
	target := ""
	for current := folder; ; current = filepath.Dir(current) {
		candidate := filepath.Join(current, ".gitignore")
		if stat, err := os.Stat(candidate); err == nil && stat.Mode().IsRegular() {
			target = candidate
			break
		}
		if strings.EqualFold(filepath.Clean(current), filepath.Clean(top)) || filepath.Dir(current) == current {
			break
		}
	}
	if target == "" {
		target = filepath.Join(top, ".gitignore")
	}
	relative, err := filepath.Rel(filepath.Dir(target), full)
	if err != nil || strings.HasPrefix(relative, "..") {
		return "", errors.New("the path is outside the repository")
	}
	entry := "/" + ignorePattern(filepath.ToSlash(relative))
	if info.IsDir() {
		entry += "/"
	}
	data, err := os.ReadFile(target)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	text := string(data)
	newline := "\n"
	if strings.Contains(text, "\r\n") {
		newline = "\r\n"
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimRight(line, "\r") == entry {
			return fmt.Sprintf("%s is already in %s.", name, ignoreLabel(top, target)), nil
		}
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += newline
	}
	if err := os.WriteFile(target, []byte(text+entry+newline), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("Added %s to %s. Files git already tracks stay tracked; use Stop tracking for those.", entry, ignoreLabel(top, target)), nil
}

func ignoreLabel(top, target string) string {
	if relative, err := filepath.Rel(top, target); err == nil {
		return filepath.ToSlash(relative)
	}
	return ".gitignore"
}

// ignorePattern escapes the characters .gitignore treats specially.
func ignorePattern(path string) string {
	var out strings.Builder
	for index, char := range path {
		if strings.ContainsRune(`*?[\`, char) || (index == 0 && (char == '#' || char == '!')) {
			out.WriteByte('\\')
		}
		out.WriteRune(char)
	}
	return strings.TrimRight(out.String(), " ")
}
