//go:build windows

package main

import (
	"github.com/ma99us/tau-shell/internal/session"
	"github.com/ma99us/tau-shell/internal/workspaces"
)

// GetWorkspaces maps every session ID to its automatic workspace.
func (application *App) GetWorkspaces() (map[string]workspaces.Group, error) {
	configs, err := session.ListConfigs(application.configDirectory)
	if err != nil {
		return nil, err
	}
	result := make(map[string]workspaces.Group, len(configs))
	for _, config := range configs {
		result[config.ID] = workspaces.Resolve(config.WorkingDir)
	}
	return result, nil
}
