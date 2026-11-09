package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ma99us/tau-shell/internal/diagnostics"
	"github.com/ma99us/tau-shell/internal/session"
	"github.com/ma99us/tau-shell/internal/sessionhost"
)

func sessionIDFromName(name string) string {
	id := strings.Trim(sessionIDPattern.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(id) > 63 {
		id = strings.TrimRight(id[:63], "-")
	}
	return id
}

// SetSessionOrder saves the display order shared by Home, the sidebar and the tabs.
func (application *App) SetSessionOrder(ids []string) (err error) {
	defer func() {
		if err != nil {
			diagnostics.Error("save session order failed", err)
		}
	}()
	return application.manager.SetOrder(context.Background(), ids)
}

// GetSessionLaunchArgs returns a session's configured launch arguments, to
// prefill the clone dialog.
func (application *App) GetSessionLaunchArgs(id string) ([]string, error) {
	config, err := session.FindConfig(application.configDirectory, id)
	if err != nil {
		return nil, err
	}
	return append([]string{}, config.LaunchArgs...), nil
}

// DuplicateSession saves a copy of a session definition under a new unique
// name and ID, placed right after the source. The working folder is always
// inherited; the harness and launch arguments may be overridden (an empty
// toolName or nil launchArgs keeps the source's). Runtime state (resume IDs,
// restore outcomes) is never copied.
func (application *App) DuplicateSession(id, toolName string, launchArgs []string) (snapshot sessionhost.Snapshot, err error) {
	defer func() {
		if err != nil {
			diagnostics.Error("duplicate session failed", err, "session_id", id)
			return
		}
		diagnostics.Info("session duplicated", "session_id", id, "copy_id", snapshot.ID)
	}()
	source, err := session.FindConfig(application.configDirectory, id)
	if err != nil {
		return sessionhost.Snapshot{}, err
	}
	copied := source
	if toolName != "" {
		copied.Tool = session.Harness(toolName)
	}
	copied.LaunchArgs = append([]string(nil), source.LaunchArgs...)
	if launchArgs != nil {
		copied.LaunchArgs = append([]string(nil), launchArgs...)
	}
	if source.Environment != nil {
		copied.Environment = make(map[string]string, len(source.Environment))
		for key, value := range source.Environment {
			copied.Environment[key] = value
		}
	}
	copied.Tags = append([]string(nil), source.Tags...)
	copied.Presets = append(copied.Presets[:0:0], source.Presets...)
	if source.Secrets != nil {
		copied.Secrets = make(map[string]string, len(source.Secrets))
		for key, value := range source.Secrets {
			copied.Secrets[key] = value // the vault name, never the value
		}
	}
	for n := 2; ; n++ {
		copied.Name = fmt.Sprintf("%s (%d)", source.Name, n)
		copied.ID = sessionIDFromName(copied.Name)
		if copied.ID == "" {
			return sessionhost.Snapshot{}, errors.New("session name must include at least one letter or number")
		}
		if _, statErr := os.Stat(filepath.Join(application.configDirectory, copied.ID+".yaml")); errors.Is(statErr, os.ErrNotExist) {
			break
		}
	}
	snapshot, err = application.saveNewConfig(copied)
	if err != nil {
		return sessionhost.Snapshot{}, err
	}
	snapshot.Color = copied.Color
	snapshot.Notes = copied.Notes
	snapshot.Link = copied.Link
	snapshot.LinkLabel = session.LinkLabel(copied.Link)
	snapshot.Tags = copied.Tags
	if err := application.manager.PlaceAfter(context.Background(), snapshot.ID, id); err != nil {
		return sessionhost.Snapshot{}, err
	}
	return snapshot, nil
}
