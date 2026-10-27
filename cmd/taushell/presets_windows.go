//go:build windows

package main

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/ma99us/tau-shell/internal/diagnostics"
	"github.com/ma99us/tau-shell/internal/presets"
	"github.com/ma99us/tau-shell/internal/session"
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
	return presets.Resolve(context.Background(), presets.Merge(global, config.Presets), presets.Context{
		Tool:       string(config.Tool),
		Session:    config.Name,
		WorkingDir: config.WorkingDir,
	}), nil
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
	if err := presets.EnsureExample(path); err != nil {
		return fmt.Errorf("prepare %s: %w", path, err)
	}
	if application.openFile != nil {
		return application.openFile(path)
	}
	// "start" with an empty title opens the file with its associated editor.
	command := exec.Command("cmd", "/c", "start", "", path)
	if err := command.Start(); err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	go command.Wait()
	diagnostics.Info("opened prompt presets file")
	return nil
}
