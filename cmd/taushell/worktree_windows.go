//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ma99us/tau-shell/internal/diagnostics"
	"github.com/ma99us/tau-shell/internal/presets"
	"github.com/ma99us/tau-shell/internal/session"
	"github.com/ma99us/tau-shell/internal/sessionhost"
	"github.com/ma99us/tau-shell/internal/worktree"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// WorktreeOptions is what the dialogs send to create a session in a new worktree.
type WorktreeOptions struct {
	Name   string `json:"name"`
	Branch string `json:"branch"`
	Base   string `json:"base"`
}

// WorktreePlan previews a new worktree before anything is created.
type WorktreePlan struct {
	Name     string   `json:"name"`
	Branch   string   `json:"branch"`
	Path     string   `json:"path"`
	Repo     string   `json:"repo"`
	Base     string   `json:"base"`
	Branches []string `json:"branches"`
	// Dirty is true when the main checkout has uncommitted changes, which are not carried over.
	Dirty bool `json:"dirty"`
}

// GitStatusView is the git status the top bar shows; Worktree is set when the
// folder is a linked worktree.
type GitStatusView struct {
	presets.RepoStatus
	Worktree *worktree.Info `json:"worktree,omitempty"`
}

// WorktreeDeletePlan tells the delete dialog what removing a session's worktree would do.
type WorktreeDeletePlan struct {
	// Managed is true only for a worktree TauShell created for this session.
	Managed  bool   `json:"managed"`
	Name     string `json:"name"`
	Branch   string `json:"branch"`
	Path     string `json:"path"`
	Dirty    bool   `json:"dirty"`
	Unpushed int    `json:"unpushed"`
	Exists   bool   `json:"exists"`
	// SharedBy names another session that uses the same folder, which blocks removal.
	SharedBy string `json:"sharedBy,omitempty"`
}

type worktreeLookup struct {
	info    worktree.Info
	ok      bool
	fetched time.Time
}

var (
	worktreeCacheMu sync.Mutex
	worktreeCache   = map[string]worktreeLookup{}
)

// cachedInspect avoids running git every time the top bar polls status.
func cachedInspect(ctx context.Context, dir string) (worktree.Info, bool) {
	key := dir
	if distro, ok := presets.WSLDistro(ctx); ok {
		key = distro + ":" + dir
	}
	worktreeCacheMu.Lock()
	entry, found := worktreeCache[key]
	worktreeCacheMu.Unlock()
	if found && time.Since(entry.fetched) < time.Minute {
		return entry.info, entry.ok
	}
	info, ok := worktree.Inspect(ctx, dir)
	worktreeCacheMu.Lock()
	worktreeCache[key] = worktreeLookup{info: info, ok: ok, fetched: time.Now()}
	worktreeCacheMu.Unlock()
	return info, ok
}

func worktreeContext(runtimeName, distro string) context.Context {
	if runtimeName == "wsl" {
		return presets.WithWSL(context.Background(), distro)
	}
	return context.Background()
}

// PlanWorktree previews the worktree a new session would get.
func (application *App) PlanWorktree(runtimeName, distro, dir, sessionName string) (plan WorktreePlan, err error) {
	defer func() {
		if err != nil {
			diagnostics.Error("plan worktree failed", err)
		}
	}()
	ctx := worktreeContext(runtimeName, distro)
	if strings.TrimSpace(dir) == "" {
		return WorktreePlan{}, errors.New("choose a project folder first")
	}
	name := worktree.Slug(sessionName)
	if name == "" {
		name = "worktree"
	}
	repo, err := worktree.Root(ctx, dir)
	if err != nil {
		return WorktreePlan{}, err
	}
	path, err := worktree.PlannedPath(ctx, dir, name)
	if err != nil {
		return WorktreePlan{}, err
	}
	branches, _ := worktree.Branches(ctx, repo)
	plan = WorktreePlan{Name: name, Branch: worktree.DefaultBranch(name), Path: path, Repo: repo, Branches: branches}
	if len(branches) > 0 {
		plan.Base = branches[0]
	}
	if status, ok := presets.GitStatus(ctx, repo); ok {
		plan.Dirty = status.Dirty
	}
	return plan, nil
}

// ListWorktrees lists the worktrees of the repository at dir, so a session can adopt an existing one.
func (application *App) ListWorktrees(runtimeName, distro, dir string) ([]worktree.Entry, error) {
	entries, err := worktree.List(worktreeContext(runtimeName, distro), dir)
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// CreateSessionInWorktree creates a new worktree on a new branch and a session working in it.
func (application *App) CreateSessionInWorktree(runtimeName, distro, name, toolName, workingDir string, launchArgs []string, options WorktreeOptions) (snapshot sessionhost.Snapshot, err error) {
	defer func() {
		if err != nil {
			diagnostics.Error("create session in worktree failed", err, "tool", toolName)
			return
		}
		diagnostics.Info("session created in worktree", "session_id", snapshot.ID, "worktree", snapshot.Worktree)
	}()
	config, err := application.newConfigIn(runtimeName, distro, name, toolName, workingDir, launchArgs)
	if err != nil {
		return sessionhost.Snapshot{}, err
	}
	cleanup, err := application.attachWorktree(&config, workingDir, options)
	if err != nil {
		return sessionhost.Snapshot{}, err
	}
	snapshot, err = application.saveNewConfig(config)
	if err != nil {
		cleanup()
		return sessionhost.Snapshot{}, err
	}
	return snapshot, nil
}

// CloneSessionInWorktree clones a session into a new worktree of the same repository.
func (application *App) CloneSessionInWorktree(id, toolName string, launchArgs []string, options WorktreeOptions) (sessionhost.Snapshot, error) {
	return application.duplicateSession(id, toolName, launchArgs, &options)
}

// attachWorktree creates the worktree for config and points config at it. The
// returned function removes the new worktree again if saving the session fails.
func (application *App) attachWorktree(config *session.Config, sourceDir string, options WorktreeOptions) (func(), error) {
	ctx := gitContext(*config)
	if strings.TrimSpace(options.Name) == "" {
		options.Name = worktree.Slug(config.Name)
	}
	if strings.TrimSpace(options.Branch) == "" {
		options.Branch = worktree.DefaultBranch(options.Name)
	}
	result, err := worktree.Create(ctx, worktree.Spec{Dir: sourceDir, Name: options.Name, Branch: options.Branch, Base: options.Base})
	if err != nil {
		return nil, err
	}
	for _, warning := range result.Warnings {
		diagnostics.Info("worktree warning", "worktree", options.Name, "warning", warning)
	}
	config.WorkingDir = result.Path
	config.Worktree = &session.WorktreeConfig{Name: options.Name, Branch: options.Branch, Base: options.Base, Repo: result.MainRepo}
	return func() {
		if removeErr := worktree.Remove(ctx, result.Path, options.Branch, true, true); removeErr != nil {
			diagnostics.Error("worktree rollback failed", removeErr, "worktree", options.Name)
		}
	}, nil
}

// GetWorktreeDeletePlan describes what deleting the session would do to its worktree.
func (application *App) GetWorktreeDeletePlan(id string) (WorktreeDeletePlan, error) {
	config, err := session.FindConfig(application.configDirectory, id)
	if err != nil {
		return WorktreeDeletePlan{}, err
	}
	if config.Worktree == nil {
		return WorktreeDeletePlan{}, nil
	}
	plan := WorktreeDeletePlan{Managed: true, Name: config.Worktree.Name, Branch: config.Worktree.Branch, Path: config.WorkingDir}
	if others, listErr := session.ListConfigs(application.configDirectory); listErr == nil {
		for _, other := range others {
			if other.ID != id && sameFolder(other.WorkingDir, config.WorkingDir) {
				plan.SharedBy = other.Name
				break
			}
		}
	}
	state := worktree.Preflight(gitContext(config), config.WorkingDir)
	plan.Exists, plan.Dirty, plan.Unpushed = state.Exists, state.Dirty, state.Unpushed
	return plan, nil
}

func sameFolder(a, b string) bool {
	return strings.EqualFold(strings.TrimRight(strings.ReplaceAll(a, "\\", "/"), "/"), strings.TrimRight(strings.ReplaceAll(b, "\\", "/"), "/"))
}

// DeleteSessionWithWorktree deletes a session and, as asked, removes the
// worktree TauShell created for it and its branch. Work that is uncommitted or
// unpushed blocks removal unless force is set.
func (application *App) DeleteSessionWithWorktree(id string, removeFolder, deleteBranch, force bool) (err error) {
	defer func() {
		if err != nil {
			diagnostics.Error("delete session with worktree failed", err, "session_id", id)
		}
	}()
	config, err := session.FindConfig(application.configDirectory, id)
	if err != nil {
		return err
	}
	if config.Worktree == nil || (!removeFolder && !deleteBranch) {
		return application.DeleteSession(id)
	}
	if deleteBranch && !removeFolder {
		return errors.New("the branch can only be deleted together with the worktree folder")
	}
	plan, err := application.GetWorktreeDeletePlan(id)
	if err != nil {
		return err
	}
	if plan.SharedBy != "" {
		return fmt.Errorf("session %q uses the same folder; the worktree was not removed", plan.SharedBy)
	}
	ctx := gitContext(config)
	if plan.Exists {
		if info, ok := worktree.Inspect(ctx, config.WorkingDir); !ok || info.Name != config.Worktree.Name {
			return errors.New("the session folder is no longer the worktree TauShell created; nothing was removed")
		}
		if !force && (plan.Dirty || (deleteBranch && plan.Unpushed > 0)) {
			return errors.New("the worktree has uncommitted or unpushed work; review it or force the removal")
		}
	}
	if err := application.DeleteSession(id); err != nil {
		return err
	}
	if !plan.Exists {
		return nil
	}
	if err := worktree.Remove(ctx, config.WorkingDir, config.Worktree.Branch, deleteBranch, force); err != nil {
		return fmt.Errorf("the session was deleted, but %w", err)
	}
	return nil
}

// OpenPullRequest opens the GitHub page that compares the session's branch
// with its base. The address is built here from the repository's remote, never
// taken from the caller.
func (application *App) OpenPullRequest(sessionID string) (err error) {
	defer func() {
		if err != nil {
			diagnostics.Error("open pull request failed", err, "session_id", sessionID)
		}
	}()
	config, err := session.FindConfig(application.configDirectory, sessionID)
	if err != nil {
		return err
	}
	ctx := gitContext(config)
	target, err := compareURL(ctx, config.WorkingDir)
	if err != nil {
		return err
	}
	if application.openURL != nil {
		application.openURL(application.ctx, target)
		return nil
	}
	runtime.BrowserOpenURL(application.ctx, target)
	return nil
}

func compareURL(ctx context.Context, dir string) (string, error) {
	branch := presets.GitBranch(ctx, dir)
	if branch == "" || branch == "detached" {
		return "", errors.New("check out a branch first")
	}
	remote, err := presets.RunGit(ctx, dir, nil, "remote", "get-url", "origin")
	if err != nil {
		return "", errors.New("the repository has no origin remote")
	}
	owner, repo, ok := githubRepo(strings.TrimSpace(remote))
	if !ok {
		return "", errors.New("open PR supports github.com remotes only")
	}
	return "https://github.com/" + owner + "/" + repo + "/compare/" + url.PathEscape(branch) + "?expand=1", nil
}

// githubRepo extracts owner and repo from an https or ssh github.com remote.
func githubRepo(remote string) (owner, repo string, ok bool) {
	var path string
	switch {
	case strings.HasPrefix(remote, "git@github.com:"):
		path = strings.TrimPrefix(remote, "git@github.com:")
	case strings.HasPrefix(remote, "ssh://git@github.com/"):
		path = strings.TrimPrefix(remote, "ssh://git@github.com/")
	case strings.HasPrefix(remote, "https://github.com/"):
		path = strings.TrimPrefix(remote, "https://github.com/")
	default:
		return "", "", false
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
