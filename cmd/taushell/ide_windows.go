//go:build windows

package main

import (
	"fmt"
	"os/exec"

	"github.com/ma99us/tau-shell/internal/diagnostics"
	"github.com/ma99us/tau-shell/internal/ide"
	"github.com/ma99us/tau-shell/internal/session"
)

// IDEOption is one IDE the session's project folder can be opened in.
type IDEOption struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Icon    string `json:"icon"`
	Matched bool   `json:"matched"`
}

// ideMatches lists the installed IDEs that suit the session folder. A WSL
// session is never scanned from Windows (that would wake the distro).
func (application *App) ideMatches(sessionID string) (session.Config, []ide.Match, error) {
	config, err := session.FindConfig(application.configDirectory, sessionID)
	if err != nil {
		return config, nil, err
	}
	root := config.WorkingDir
	if config.IsWSL() {
		return config, ide.Candidates(root, ide.DetectInstalled(), false), nil
	}
	return config, ide.Candidates(root, ide.DetectInstalled(), true), nil
}

// ListIDEs returns the installed IDEs that suit the session's project folder,
// best match first, each with its own icon.
func (application *App) ListIDEs(sessionID string) ([]IDEOption, error) {
	_, matches, err := application.ideMatches(sessionID)
	if err != nil {
		return nil, err
	}
	options := make([]IDEOption, 0, len(matches))
	for _, match := range matches {
		options = append(options, IDEOption{ID: match.ID, Name: match.Name, Icon: ide.IconDataURL(match.Exe), Matched: match.Matched})
	}
	return options, nil
}

// OpenInIDE opens the session's project folder in one of the IDEs ListIDEs
// offered. It returns a note when the IDE could not do exactly what was asked.
func (application *App) OpenInIDE(sessionID, ideID string) (note string, err error) {
	defer func() {
		if err != nil {
			diagnostics.Error("open in ide failed", err, "session_id", sessionID, "ide", ideID)
		}
	}()
	config, matches, err := application.ideMatches(sessionID)
	if err != nil {
		return "", err
	}
	for _, match := range matches {
		if match.ID != ideID {
			continue
		}
		distro := ""
		if config.IsWSL() {
			distro = config.WSL.Distro
		}
		command, err := ide.BuildCommand(match.Installed, config.WorkingDir, distro)
		if err != nil {
			return "", err
		}
		process := exec.Command(command.Exe, command.Args...)
		if err := process.Start(); err != nil {
			return "", fmt.Errorf("start %s: %w", match.Name, err)
		}
		go process.Wait()
		diagnostics.Info("opened project in ide", "session_id", sessionID, "ide", ideID)
		return command.Note, nil
	}
	return "", fmt.Errorf("%s is not available for this project", ideID)
}
