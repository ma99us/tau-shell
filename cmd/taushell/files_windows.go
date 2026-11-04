//go:build windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ma99us/tau-shell/internal/diagnostics"
	"github.com/ma99us/tau-shell/internal/projectfiles"
	"github.com/ma99us/tau-shell/internal/session"
)

func (application *App) projectRoot(sessionID string) (string, error) {
	config, err := session.FindConfig(application.configDirectory, sessionID)
	if err != nil {
		return "", err
	}
	return config.WorkingDir, nil
}

// ListProjectDir lists one folder of the session's working directory for the file drawer.
// rel is relative to that directory; anything that resolves outside it is refused.
func (application *App) ListProjectDir(sessionID, rel string, showAll bool) (projectfiles.Listing, error) {
	root, err := application.projectRoot(sessionID)
	if err != nil {
		return projectfiles.Listing{}, err
	}
	listing, err := projectfiles.List(root, rel, showAll)
	if err != nil {
		diagnostics.Error("list project folder failed", err, "session_id", sessionID)
	}
	return listing, err
}

// ReadProjectFile returns the text of one file (bounded) for the drawer's viewer.
func (application *App) ReadProjectFile(sessionID, rel string) (projectfiles.File, error) {
	root, err := application.projectRoot(sessionID)
	if err != nil {
		return projectfiles.File{}, err
	}
	file, err := projectfiles.Read(root, rel)
	if err != nil {
		diagnostics.Error("read project file failed", err, "session_id", sessionID)
	}
	return file, err
}

// RevealProjectFile shows one project file or folder in Explorer.
func (application *App) RevealProjectFile(sessionID, rel string) error {
	root, err := application.projectRoot(sessionID)
	if err != nil {
		return err
	}
	path, err := projectfiles.Resolve(root, rel)
	if err != nil {
		return err
	}
	return exec.Command("explorer.exe", "/select,"+path).Start()
}

// RevealProjectRoot opens the session's project folder in Explorer.
func (application *App) RevealProjectRoot(sessionID string) error {
	root, err := application.projectRoot(sessionID)
	if err != nil {
		return err
	}
	return exec.Command("explorer.exe", filepath.Clean(root)).Start()
}

// executableExtensions are never launched through "open", so a file that runs code is not started by one click.
var executableExtensions = map[string]bool{".exe": true, ".com": true, ".bat": true, ".cmd": true, ".ps1": true, ".psm1": true, ".vbs": true, ".vbe": true, ".jse": true, ".wsf": true, ".msi": true, ".scr": true, ".lnk": true, ".reg": true, ".hta": true, ".cpl": true, ".dll": true}

// OpenProjectFile opens one project file in the user's default application.
func (application *App) OpenProjectFile(sessionID, rel string) error {
	root, err := application.projectRoot(sessionID)
	if err != nil {
		return err
	}
	path, err := projectfiles.Resolve(root, rel)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("only files can be opened")
	}
	if executableExtensions[strings.ToLower(filepath.Ext(path))] {
		return errors.New("programs and scripts are not opened from here; use Open in File Explorer")
	}
	return exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", path).Start()
}

// ProjectFullPath returns the absolute path of a project file, confined to the working directory.
func (application *App) ProjectFullPath(sessionID, rel string) (string, error) {
	root, err := application.projectRoot(sessionID)
	if err != nil {
		return "", err
	}
	return projectfiles.Resolve(root, rel)
}
