//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ma99us/tau-shell/internal/diagnostics"
	"github.com/ma99us/tau-shell/internal/harness/manifest"
	"github.com/ma99us/tau-shell/internal/presets"
	"github.com/ma99us/tau-shell/internal/session"
	"github.com/ma99us/tau-shell/internal/usage"
	"golang.org/x/sys/windows"
)

func (application *App) presetsPath() string {
	return filepath.Join(filepath.Dir(application.configDirectory), presets.FileName)
}

// GetPresets returns the global and session prompt presets for one session,
// filtered for its harness and with variables expanded.
func (application *App) GetPresets(sessionID string) (resolved []presets.Resolved, err error) {
	defer func() {
		if err != nil {
			diagnostics.Error("load prompt presets failed", err, "session_id", sessionID)
		}
	}()
	config, err := session.FindConfig(application.configDirectory, sessionID)
	if err != nil {
		return nil, err
	}
	global, err := presets.Load(application.presetsPath())
	if err != nil {
		return nil, err
	}
	var source []presets.Preset
	var available map[string]bool
	if application.skills != nil {
		source = append([]presets.Preset(nil), application.skills.Presets()...)
		for index := range source {
			source[index].Remote = true
		}
		available = application.skills.Available(string(config.Tool))
	}
	layered := presets.Merge(presets.Merge(source, global), config.Presets)
	return presets.Resolve(context.Background(), layered, presets.Context{
		Tool:       string(config.Tool),
		Session:    config.Name,
		WorkingDir: config.WorkingDir,
		Skills:     available,
	}), nil
}

// GetGitBranch returns the session working directory's current git branch,
// or "" when it is not a git repository.
func (application *App) GetGitBranch(sessionID string) (string, error) {
	config, err := session.FindConfig(application.configDirectory, sessionID)
	if err != nil {
		return "", err
	}
	return presets.GitBranch(context.Background(), config.WorkingDir), nil
}

// GetGitStatus returns the session repository's branch, dirty flag and
// ahead/behind counts, or nil when the folder is not a repository.
func (application *App) GetGitStatus(sessionID string) (*presets.RepoStatus, error) {
	config, err := session.FindConfig(application.configDirectory, sessionID)
	if err != nil {
		return nil, err
	}
	status, ok := presets.GitStatus(context.Background(), config.WorkingDir)
	if !ok {
		return nil, nil
	}
	return &status, nil
}

// SyncGit commits all pending changes, rebases onto upstream and pushes.
func (application *App) SyncGit(sessionID string) (err error) {
	defer func() {
		if err != nil {
			diagnostics.Error("git sync failed", err, "session_id", sessionID)
		}
	}()
	config, err := session.FindConfig(application.configDirectory, sessionID)
	if err != nil {
		return err
	}
	return presets.GitSync(context.Background(), config.WorkingDir)
}

// GetSessionModel returns the model the harness last recorded for the
// session, or "" when it is unknown.
func (application *App) GetSessionModel(sessionID string) (string, error) {
	return application.manager.HarnessModel(context.Background(), sessionID)
}

// GetHarnessActivity returns "working" or "idle" from the harness's own log,
// or "" when it offers no such signal.
func (application *App) GetHarnessActivity(sessionID string) (string, error) {
	return application.manager.HarnessActivity(context.Background(), sessionID)
}

// GetSessionUsage returns the session's all-time token and cost totals.
func (application *App) GetSessionUsage(sessionID string) (usage.SessionUsage, error) {
	return application.manager.SessionUsage(context.Background(), sessionID)
}

// GetUsageSummary returns per-session totals plus monthly totals.
func (application *App) GetUsageSummary() (usage.Summary, error) {
	return application.manager.UsageSummary(context.Background())
}

// OpenPresetsFile creates the commented example file if it is missing and
// opens it in the default editor.
func (application *App) OpenPresetsFile() (err error) {
	defer func() {
		if err != nil {
			diagnostics.Error("open prompt presets file failed", err)
		}
	}()
	path := application.presetsPath()
	return application.openEditableFile(path, presets.EnsureExample)
}

func (application *App) openEditableFile(path string, ensure func(string) error) error {
	if err := ensure(path); err != nil {
		return fmt.Errorf("prepare %s: %w", path, err)
	}
	if application.openFile != nil {
		return application.openFile(path)
	}
	if err := shellOpen(path); err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	diagnostics.Info("opened editable file", "file", filepath.Base(path))
	return nil
}

// shellOpen asks the shell to open the file with its associated editor,
// without spawning a console window. Files with no association (common for
// .json) fall back to Notepad.
func shellOpen(path string) error {
	target, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	verb, _ := windows.UTF16PtrFromString("open")
	if err := windows.ShellExecute(0, verb, target, nil, nil, windows.SW_SHOWNORMAL); err == nil {
		return nil
	}
	command := exec.Command("notepad.exe", path)
	if err := command.Start(); err != nil {
		return err
	}
	go command.Wait()
	return nil
}

// toolConfigPath returns the tool's user-level config file and whether it may
// be created empty when missing.
func toolConfigPath(tool string) (path string, create bool, err error) {
	item, ok := manifest.Get(tool)
	if !ok || item.ConfigFile == nil {
		return "", false, fmt.Errorf("no config file known for tool %q", tool)
	}
	var first string
	for _, template := range item.ConfigFile.Paths {
		candidate, expandErr := manifest.ExpandPath(template, "")
		if expandErr != nil {
			return "", false, expandErr
		}
		if first == "" {
			first = candidate
		}
		if item.ConfigFile.MissingHint != "" {
			if _, statErr := os.Stat(candidate); statErr == nil {
				return candidate, false, nil
			}
		}
	}
	if item.ConfigFile.MissingHint != "" {
		return "", false, errors.New(item.ConfigFile.MissingHint)
	}
	return first, item.ConfigFile.Create, nil
}

// OpenToolConfig opens a tool's user-level config file in the default editor.
func (application *App) OpenToolConfig(toolID string) (err error) {
	defer func() {
		if err != nil {
			diagnostics.Error("open tool config failed", err, "tool", toolID)
		}
	}()
	path, create, err := toolConfigPath(toolID)
	if err != nil {
		return err
	}
	return application.openEditableFile(path, func(path string) error {
		if _, statErr := os.Stat(path); statErr == nil || !create {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		content := []byte("{}\n")
		return os.WriteFile(path, content, 0o600)
	})
}
